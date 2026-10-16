package cautem

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	dm "github.com/NVIDIA/OpenShell/sdk/go/proto/datamodelv1"
	pb "github.com/NVIDIA/OpenShell/sdk/go/proto/openshellv1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
)

// ExecInteractiveWithTTY opens the OpenShell bidirectional exec RPC with an
// explicit PTY setting. The upstream Go facade currently hardcodes TTY=true.
func (c *Client) ExecInteractiveWithTTY(ctx context.Context, workspace, name string, argv []string, tty bool, cols, rows uint32, options SandboxExecOptions) (SandboxInteractiveOutput, error) {
	return c.ExecInteractiveWithTTYTimeout(ctx, workspace, name, argv, tty, cols, rows, 0, options)
}

// ExecInteractiveWithTTYTimeout opens interactive exec with an explicit PTY
// setting and optional server-side process deadline.
func (c *Client) ExecInteractiveWithTTYTimeout(ctx context.Context, workspace, name string, argv []string, tty bool, cols, rows uint32, timeout time.Duration, options SandboxExecOptions) (SandboxInteractiveOutput, error) {
	if workspace == "" || name == "" || len(argv) == 0 {
		return nil, fmt.Errorf("usage: ExecInteractiveWithTTY(workspace, name, argv, tty, cols, rows, options)")
	}
	rpc, err := c.compatClient()
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	stream, err := rpc.ExecSandboxInteractive(streamCtx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open interactive exec in sandbox %q: %w", name, err)
	}
	request := &pb.ExecSandboxRequest{
		Sandbox:        name,
		Command:        append([]string(nil), argv...),
		Workdir:        options.WorkDir,
		Environment:    cloneStringMap(options.Env),
		NoLoginShell:   options.NoLoginShell,
		Tty:            tty,
		WorkspaceScope: &dm.WorkspaceSelector{Selection: &dm.WorkspaceSelector_Workspace{Workspace: workspace}},
	}
	if timeout > 0 {
		request.ExecutionTimeout = durationpb.New(timeout)
	}
	if tty {
		request.Cols, request.Rows = cols, rows
	}
	if err := stream.Send(&pb.ExecSandboxInput{Payload: &pb.ExecSandboxInput_Start{Start: request}}); err != nil {
		cancel()
		return nil, fmt.Errorf("start interactive exec in sandbox %q: %w", name, err)
	}
	return newGatewayInteractiveSession(streamCtx, cancel, stream), nil
}

func (c *Client) execStreamingWithTimeout(ctx context.Context, workspace, name string, argv []string, timeout time.Duration, options SandboxExecOptions) (SandboxExecStream, error) {
	if workspace == "" || name == "" || len(argv) == 0 {
		return nil, fmt.Errorf("usage: ExecStreamingWithTimeout(workspace, name, argv, timeout, options)")
	}
	rpc, err := c.compatClient()
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	request := &pb.ExecSandboxRequest{
		Sandbox:        name,
		Command:        append([]string(nil), argv...),
		Workdir:        options.WorkDir,
		Environment:    cloneStringMap(options.Env),
		NoLoginShell:   options.NoLoginShell,
		WorkspaceScope: &dm.WorkspaceSelector{Selection: &dm.WorkspaceSelector_Workspace{Workspace: workspace}},
	}
	if timeout > 0 {
		request.ExecutionTimeout = durationpb.New(timeout)
	}
	stream, err := rpc.ExecSandbox(streamCtx, request)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stream exec in sandbox %q: %w", name, err)
	}
	return &gatewayExecOutputStream{stream: stream, cancel: cancel}, nil
}

type gatewayExecOutputStream struct {
	stream  grpc.ServerStreamingClient[pb.ExecSandboxEvent]
	cancel  context.CancelFunc
	exit    int
	done    bool
	hasExit bool
}

func (s *gatewayExecOutputStream) Next() (*SandboxExecChunk, error) {
	if s.done {
		return nil, io.EOF
	}
	event, err := s.stream.Recv()
	if err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, err
	}
	if output := event.GetStdout(); output != nil {
		return &SandboxExecChunk{Stream: "stdout", Data: append([]byte(nil), output.GetData()...)}, nil
	}
	if output := event.GetStderr(); output != nil {
		return &SandboxExecChunk{Stream: "stderr", Data: append([]byte(nil), output.GetData()...)}, nil
	}
	if exit := event.GetExit(); exit != nil {
		s.exit, s.done, s.hasExit = int(exit.GetExitCode()), true, true
		return nil, io.EOF
	}
	return nil, fmt.Errorf("exec stream returned an empty event")
}

func (s *gatewayExecOutputStream) ExitCode() (int, error) {
	if !s.done {
		for {
			_, err := s.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return -1, err
			}
		}
	}
	if !s.hasExit {
		return -1, fmt.Errorf("exec stream ended without exit event")
	}
	return s.exit, nil
}

func (s *gatewayExecOutputStream) Close() error {
	s.cancel()
	return nil
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

type gatewayInteractiveStream interface {
	grpc.BidiStreamingClient[pb.ExecSandboxInput, pb.ExecSandboxEvent]
}

type gatewayInteractiveSession struct {
	stream gatewayInteractiveStream
	cancel context.CancelFunc

	sendMu      sync.Mutex
	inputClosed bool
	closeOnce   sync.Once
	closeErr    error

	dataCh chan *SandboxExecChunk
	done   chan struct{}
	errMu  sync.Mutex
	err    error
	buf    []byte

	exitCode    int
	hasExitCode bool
}

func newGatewayInteractiveSession(ctx context.Context, cancel context.CancelFunc, stream gatewayInteractiveStream) *gatewayInteractiveSession {
	session := &gatewayInteractiveSession{stream: stream, cancel: cancel, dataCh: make(chan *SandboxExecChunk, 64), done: make(chan struct{})}
	go session.readLoop(ctx)
	return session
}

func (s *gatewayInteractiveSession) readLoop(ctx context.Context) {
	defer s.cancel()
	defer close(s.dataCh)
	defer close(s.done)
	for {
		event, err := s.stream.Recv()
		if err != nil {
			if err != io.EOF {
				s.setError(err)
			} else if !s.hasExitCode {
				s.setError(fmt.Errorf("interactive exec stream ended without exit event"))
			}
			return
		}
		if output := event.GetStdout(); output != nil {
			if !s.sendOutput(ctx, "stdout", output.GetData()) {
				return
			}
			continue
		}
		if output := event.GetStderr(); output != nil {
			if !s.sendOutput(ctx, "stderr", output.GetData()) {
				return
			}
			continue
		}
		if exit := event.GetExit(); exit != nil {
			s.exitCode = int(exit.GetExitCode())
			s.hasExitCode = true
			return
		}
		s.setError(fmt.Errorf("interactive exec returned an empty event"))
		return
	}
}

func (s *gatewayInteractiveSession) sendOutput(ctx context.Context, stream SandboxExecStreamType, data []byte) bool {
	chunk := &SandboxExecChunk{Stream: stream, Data: append([]byte(nil), data...)}
	select {
	case s.dataCh <- chunk:
		return true
	case <-ctx.Done():
		s.setError(ctx.Err())
		return false
	}
}

func (s *gatewayInteractiveSession) setError(err error) {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	if s.err == nil {
		s.err = err
	}
}

func (s *gatewayInteractiveSession) getError() error {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return s.err
}

func (s *gatewayInteractiveSession) Read(p []byte) (int, error) {
	if len(s.buf) > 0 {
		n := copy(p, s.buf)
		s.buf = s.buf[n:]
		return n, nil
	}
	chunk, err := s.NextOutput()
	if err != nil {
		return 0, err
	}
	data := chunk.Data
	n := copy(p, data)
	if n < len(data) {
		s.buf = append(s.buf[:0], data[n:]...)
	}
	return n, nil
}

// NextOutput returns the next stdout/stderr frame without merging the streams.
func (s *gatewayInteractiveSession) NextOutput() (*SandboxExecChunk, error) {
	chunk, ok := <-s.dataCh
	if !ok {
		if err := s.getError(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	return chunk, nil
}

func (s *gatewayInteractiveSession) Write(data []byte) (int, error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if s.inputClosed {
		return 0, io.ErrClosedPipe
	}
	if err := s.stream.Send(&pb.ExecSandboxInput{Payload: &pb.ExecSandboxInput_Stdin{Stdin: append([]byte(nil), data...)}}); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (s *gatewayInteractiveSession) Resize(cols, rows uint32) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if s.inputClosed {
		return io.ErrClosedPipe
	}
	if err := s.stream.Send(&pb.ExecSandboxInput{Payload: &pb.ExecSandboxInput_Resize{Resize: &pb.ExecSandboxWindowResize{Cols: cols, Rows: rows}}}); err != nil {
		return err
	}
	return nil
}

func (s *gatewayInteractiveSession) ExitCode() (int, error) {
	<-s.done
	if !s.hasExitCode {
		return -1, s.getError()
	}
	return s.exitCode, s.getError()
}

func (s *gatewayInteractiveSession) CloseWrite() error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if s.inputClosed {
		return nil
	}
	s.inputClosed = true
	return s.stream.CloseSend()
}

func (s *gatewayInteractiveSession) Close() error {
	s.closeOnce.Do(func() {
		s.cancel()
		s.closeErr = s.CloseWrite()
		<-s.done
	})
	return s.closeErr
}
