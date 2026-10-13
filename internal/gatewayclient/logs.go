package gatewayclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PostLogs POST /v1/sandboxes/{name}/logs — ingest observation lines.
func (c *Client) PostLogs(ctx context.Context, sandbox string, lines []LogLine) error {
	b, err := json.Marshal(map[string]any{"lines": lines})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/v1/sandboxes/"+url.PathEscape(sandbox)+"/logs", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			err = fmt.Errorf("gateway read response body: %w", err)
			return err
		}
		return fmt.Errorf("gateway post logs: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// LogLine is one observation event for ingest/SSE.
type LogLine struct {
	TS     time.Time `json:"ts"`
	Source string    `json:"source"`
	Level  string    `json:"level"`
	Text   string    `json:"text"`
}

// FollowLogs streams SSE log lines to w. names may be multiple; all=true uses gateway ?all=1.
func (c *Client) FollowLogs(ctx context.Context, names []string, all bool, since, source, level string, w io.Writer) error {
	path := "/v1/logs"
	q := url.Values{"follow": {"1"}}
	if all {
		q.Set("all", "1")
	}
	for _, name := range names {
		q.Add("name", name)
	}
	if since != "" {
		q.Set("since", since)
	}
	if source != "" {
		q.Set("source", source)
	}
	if level != "" {
		q.Set("level", level)
	}
	if !all && len(names) == 1 {
		path = "/v1/sandboxes/" + url.PathEscape(names[0]) + "/logs"
		q.Del("name")
	}
	u := c.Base + path + "?" + q.Encode()

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	// SSE needs no overall timeout
	sseClient := *httpClient
	sseClient.Timeout = 0

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	res, err := sseClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			err = fmt.Errorf("gateway read response body: %w", err)
			return err
		}
		return fmt.Errorf("gateway follow logs: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if after, ok := strings.CutPrefix(line, "data: "); ok {
			if _, err := fmt.Fprintln(w, after); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

// GetLogsSnapshot returns recent log lines (non-follow JSON).
func (c *Client) GetLogsSnapshot(ctx context.Context, name, since, source, level string) ([]LogLine, error) {
	q := url.Values{}
	if since != "" {
		q.Set("since", since)
	}
	if source != "" {
		q.Set("source", source)
	}
	if level != "" {
		q.Set("level", level)
	}
	path := "/v1/sandboxes/" + url.PathEscape(name) + "/logs?" + q.Encode()
	var out struct {
		Lines []LogLine `json:"lines"`
	}
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	return out.Lines, nil
}
