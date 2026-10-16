package cautem

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	controlv1 "github.com/cautem/cautem-proto/gen/go/cautem/control/v1"
	"google.golang.org/grpc"
)

// SandboxMainProcessSession attaches to the sandbox's canonical process.
// Closing the session detaches the client and leaves the process running.
type SandboxMainProcessSession interface {
	io.ReadWriteCloser
	Resize(cols, rows uint32) error
	ExitCode() (int, error)
}

type mainProcessAttachStream interface {
	grpc.BidiStreamingClient[controlv1.AttachMainProcessRequest, controlv1.AttachMainProcessResponse]
}

// AttachControlMainProcess attaches to the existing main process without
// starting another command. A later call reconnects to the same process.
func (c *Client) AttachControlMainProcess(ctx context.Context, workspace, name string, tty bool, cols, rows uint32) (SandboxMainProcessSession, error) {
	if workspace == "" || name == "" {
		return nil, fmt.Errorf("usage: AttachControlMainProcess(workspace, name, tty, cols, rows)")
	}
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	stream, err := controlv1.NewSandboxServiceClient(conn).AttachMainProcess(streamCtx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("attach to canonical process in sandbox %q: %w", name, err)
	}
	start := &controlv1.AttachMainProcessStart{Workspace: workspace, Name: name, Tty: tty, Cols: cols, Rows: rows}
	if err := stream.Send(&controlv1.AttachMainProcessRequest{Payload: &controlv1.AttachMainProcessRequest_Start{Start: start}}); err != nil {
		cancel()
		return nil, fmt.Errorf("start canonical process attach in sandbox %q: %w", name, err)
	}
	session := &controlMainProcessSession{stream: stream, cancel: cancel, ctx: streamCtx, data: make(chan []byte, 32), done: make(chan struct{})}
	go session.readLoop()
	return session, nil
}

type controlMainProcessSession struct {
	stream mainProcessAttachStream
	cancel context.CancelFunc
	ctx    context.Context

	sendMu sync.Mutex
	close  sync.Once

	data chan []byte
	done chan struct{}
	mu   sync.Mutex
	buf  []byte
	err  error
	exit int
	seen bool
}

func (s *controlMainProcessSession) readLoop() {
	defer close(s.done)
	defer close(s.data)
	for {
		response, err := s.stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.setError(err)
			} else {
				s.mu.Lock()
				seen := s.seen
				s.mu.Unlock()
				if !seen {
					s.setError(errors.New("canonical process stream ended without exit status"))
				}
			}
			return
		}
		switch payload := response.GetPayload().(type) {
		case *controlv1.AttachMainProcessResponse_Output:
			if len(payload.Output) == 0 {
				continue
			}
			select {
			case s.data <- append([]byte(nil), payload.Output...):
			case <-s.ctx.Done():
				return
			}
		case *controlv1.AttachMainProcessResponse_ExitCode:
			s.mu.Lock()
			s.exit, s.seen = int(payload.ExitCode), true
			s.mu.Unlock()
			return
		default:
			s.setError(errors.New("canonical process stream returned an empty event"))
			return
		}
	}
}

func (s *controlMainProcessSession) setError(err error) {
	s.mu.Lock()
	if s.err == nil {
		s.err = err
	}
	s.mu.Unlock()
}

func (s *controlMainProcessSession) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(s.buf) == 0 {
		chunk, ok := <-s.data
		if !ok {
			s.mu.Lock()
			err := s.err
			s.mu.Unlock()
			if err != nil {
				return 0, err
			}
			return 0, io.EOF
		}
		s.buf = chunk
	}
	n := copy(p, s.buf)
	s.buf = s.buf[n:]
	return n, nil
}

func (s *controlMainProcessSession) Write(p []byte) (int, error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	select {
	case <-s.done:
		return 0, io.ErrClosedPipe
	default:
	}
	if err := s.stream.Send(&controlv1.AttachMainProcessRequest{Payload: &controlv1.AttachMainProcessRequest_Stdin{Stdin: append([]byte(nil), p...)}}); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *controlMainProcessSession) Resize(cols, rows uint32) error {
	if cols == 0 || rows == 0 {
		return fmt.Errorf("terminal dimensions must be positive")
	}
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	select {
	case <-s.done:
		return io.ErrClosedPipe
	default:
	}
	return s.stream.Send(&controlv1.AttachMainProcessRequest{Payload: &controlv1.AttachMainProcessRequest_Resize{Resize: &controlv1.AttachMainProcessResize{Cols: cols, Rows: rows}}})
}

func (s *controlMainProcessSession) ExitCode() (int, error) {
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return -1, s.err
	}
	if !s.seen {
		return -1, errors.New("canonical process stream ended without exit status")
	}
	return s.exit, nil
}

func (s *controlMainProcessSession) Close() error {
	s.close.Do(func() {
		s.cancel()
		s.sendMu.Lock()
		defer s.sendMu.Unlock()
		_ = s.stream.CloseSend()
	})
	return nil
}
