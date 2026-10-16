package cautem

import (
	"context"
	"fmt"
	controlv1 "github.com/cautem/quarshell-proto/gen/go/cautem/control/v1"
	"io"
	"sync"
	"time"
)

func (c *Client) GetControlSandboxLogs(ctx context.Context, workspace, name string, limit uint32) ([]LogLine, error) {
	return c.GetControlSandboxLogsFiltered(ctx, workspace, name, limit, "", "", "")
}

// GetControlSandboxLogsFiltered returns a bounded tail with optional time,
// source, and level filters. since accepts a Go duration or RFC3339 timestamp.
func (c *Client) GetControlSandboxLogsFiltered(ctx context.Context, workspace, name string, limit uint32, since, source, level string) ([]LogLine, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	client := controlv1.NewSandboxServiceClient(conn)
	response, err := client.GetSandboxLogs(ctx, &controlv1.GetSandboxLogsRequest{
		Workspace: workspace, Name: name, Limit: limit, SinceUnixMs: parseLogSince(since),
		Source: source, Level: level,
	})
	if err != nil {
		return nil, fmt.Errorf("get control sandbox logs %q: %w", name, err)
	}
	lines := make([]LogLine, 0, len(response.GetLines()))
	for _, line := range response.GetLines() {
		lines = append(lines, LogLine{
			TS: time.UnixMilli(line.GetTimestampUnixMs()), Source: line.GetSource(),
			Level: line.GetLevel(), Text: line.GetMessage(),
		})
	}
	return lines, nil
}

// WatchControlSandboxLogs follows the gateway's bounded log stream. The
// gateway sends a fresh bounded tail on a new watch and the caller should
// reconnect with a new watch after a stream error.
func (c *Client) WatchControlSandboxLogs(ctx context.Context, workspace, name string, initialLimit uint32, w io.Writer) error {
	return c.WatchControlSandboxLogsFiltered(ctx, workspace, name, initialLimit, "", "", "", w)
}

// WatchControlSandboxLogsFiltered follows a filtered workspace-scoped log stream.
func (c *Client) WatchControlSandboxLogsFiltered(ctx context.Context, workspace, name string, initialLimit uint32, since, source, level string, w io.Writer) error {
	return c.watchControlSandboxLogs(ctx, workspace, name, initialLimit, parseLogSince(since), source, level, "", w)
}

// WatchControlSandboxSet follows every supplied sandbox concurrently.
func (c *Client) WatchControlSandboxSet(ctx context.Context, sandboxes []Sandbox, initialLimit uint32, since, source, level string, w io.Writer) error {
	if w == nil {
		return fmt.Errorf("watch control sandbox logs: writer required")
	}
	if len(sandboxes) == 0 {
		return nil
	}
	if len(sandboxes) > maxControlLogWatches {
		return fmt.Errorf("watch control sandbox logs: %d sandboxes exceed the concurrent watch limit of %d", len(sandboxes), maxControlLogWatches)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var writers sync.Mutex
	var wait sync.WaitGroup
	errCh := make(chan error, len(sandboxes))
	for _, sandbox := range sandboxes {
		sandbox := sandbox
		wait.Add(1)
		go func() {
			defer wait.Done()
			label := sandbox.Workspace + "/" + sandbox.Name
			err := c.watchControlSandboxLogs(watchCtx, sandbox.Workspace, sandbox.Name, initialLimit, parseLogSince(since), source, level, label, writerLocked{mu: &writers, w: w})
			if err != nil && watchCtx.Err() == nil {
				select {
				case errCh <- err:
				default:
				}
				cancel()
			}
		}()
	}
	wait.Wait()
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

type writerLocked struct {
	mu *sync.Mutex
	w  io.Writer
}

func (w writerLocked) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}

func (c *Client) watchControlSandboxLogs(ctx context.Context, workspace, name string, initialLimit uint32, sinceUnixMS int64, source, level, label string, w io.Writer) error {
	if w == nil {
		return fmt.Errorf("watch control sandbox logs: writer required")
	}
	conn, err := c.controlConn()
	if err != nil {
		return err
	}
	client := controlv1.NewSandboxServiceClient(conn)
	stream, err := client.WatchSandboxLogs(ctx, &controlv1.WatchSandboxLogsRequest{
		Workspace: workspace, Name: name, InitialLimit: initialLimit, SinceUnixMs: sinceUnixMS,
		Source: source, Level: level,
	})
	if err != nil {
		return fmt.Errorf("watch control sandbox logs %q: %w", name, err)
	}
	for {
		event, recvErr := stream.Recv()
		if recvErr == io.EOF {
			return nil
		}
		if recvErr != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("watch control sandbox logs %q: %w", name, recvErr)
		}
		line := event.GetLine()
		if line == nil {
			continue
		}
		prefix := line.GetSource()
		if prefix == "" {
			prefix = "proxy"
		}
		if label != "" {
			prefix = label + " " + prefix
		}
		if _, err := fmt.Fprintf(w, "[%s] %s\n", prefix, line.GetMessage()); err != nil {
			return err
		}
	}
}

func parseLogSince(since string) int64 {
	if since == "" {
		return 0
	}
	if duration, err := time.ParseDuration(since); err == nil {
		return time.Now().UTC().Add(-duration).UnixMilli()
	}
	if timestamp, err := time.Parse(time.RFC3339Nano, since); err == nil {
		return timestamp.UnixMilli()
	}
	return 0
}

func sandboxFromSummary(item *controlv1.SandboxSummary) Sandbox {
	if item == nil {
		return Sandbox{}
	}
	return Sandbox{
		Name: item.GetName(), ID: item.GetId(), Image: item.GetImage(),
		Workspace: item.GetWorkspace(), ResourceVersion: item.GetResourceVersion(), Status: item.GetRegistryStatus(),
		ComputeDriver: item.GetComputeDriver(),
		Labels:        item.GetLabels(),
	}
}
