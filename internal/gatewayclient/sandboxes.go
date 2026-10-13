package gatewayclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Sandbox is the registry payload.
type Sandbox struct {
	Name              string   `json:"name"`
	ID                string   `json:"id,omitempty"`
	Image             string   `json:"image,omitempty"`
	Workspace         string   `json:"workspace,omitempty"`
	ResourceVersion   uint64   `json:"resource_version,omitempty"`
	Network           string   `json:"network,omitempty"`
	Status            string   `json:"status,omitempty"`
	Labels            Labels   `json:"labels,omitempty"`
	BasePolicyYAML    string   `json:"base_policy_yaml,omitempty"`
	AttachedProviders []string `json:"attached_providers,omitempty"`
}

// UpsertSandbox PUT /v1/sandboxes/{name}.
func (c *Client) UpsertSandbox(ctx context.Context, sb Sandbox) error {
	b, err := json.Marshal(sb)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.Base+"/v1/sandboxes/"+url.PathEscape(sb.Name), bytes.NewReader(b))
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
		return fmt.Errorf("gateway upsert: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// DeleteSandbox DELETE /v1/sandboxes/{name}.
func (c *Client) DeleteSandbox(ctx context.Context, name string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.Base+"/v1/sandboxes/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 && res.StatusCode != http.StatusNotFound {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			err = fmt.Errorf("gateway read response body: %w", err)
			return err
		}
		return fmt.Errorf("gateway delete: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// ListSandboxes GET /v1/sandboxes.
func (c *Client) ListSandboxes(ctx context.Context) ([]Sandbox, error) {
	var out struct {
		Sandboxes []Sandbox `json:"sandboxes"`
	}
	if err := c.get(ctx, "/v1/sandboxes", &out); err != nil {
		return nil, err
	}
	return out.Sandboxes, nil
}

// ResolveSecrets GET /v1/sandboxes/{name}/secrets — sidecar credential map.
func (c *Client) ResolveSecrets(ctx context.Context, sandbox string) (map[string]string, error) {
	var out struct {
		Secrets map[string]string `json:"secrets"`
	}
	if err := c.get(ctx, "/v1/sandboxes/"+url.PathEscape(sandbox)+"/secrets", &out); err != nil {
		return nil, err
	}
	if out.Secrets == nil {
		out.Secrets = map[string]string{}
	}
	return out.Secrets, nil
}
