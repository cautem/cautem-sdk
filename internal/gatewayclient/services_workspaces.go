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

// ServiceRecord matches gateway store.ServiceRecord.
type ServiceRecord struct {
	Name        string `json:"name"`
	Sandbox     string `json:"sandbox"`
	Port        int    `json:"port"`
	BackendHost string `json:"backend_host"`
	BackendPort int    `json:"backend_port"`
}

// WorkspaceMember matches gateway store.WorkspaceMember.
type WorkspaceMember struct {
	Subject string `json:"subject"`
	Role    string `json:"role"`
}

// WorkspaceRecord matches gateway store.WorkspaceRecord.
type WorkspaceRecord struct {
	ID              string            `json:"id,omitempty"`
	Name            string            `json:"name"`
	ResourceVersion uint64            `json:"resource_version,omitempty"`
	Members         []WorkspaceMember `json:"members,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	CreatedAt       string            `json:"created_at,omitempty"`
	Status          string            `json:"status,omitempty"`
}

// ListServices GET /v1/services.
func (c *Client) ListServices(ctx context.Context) ([]ServiceRecord, error) {
	var out struct {
		Services []ServiceRecord `json:"services"`
	}
	if err := c.get(ctx, "/v1/services", &out); err != nil {
		return nil, err
	}
	return out.Services, nil
}

// GetService GET /v1/services/{name}.
func (c *Client) GetService(ctx context.Context, name string) (ServiceRecord, error) {
	var out ServiceRecord
	if err := c.get(ctx, "/v1/services/"+url.PathEscape(name), &out); err != nil {
		return ServiceRecord{}, err
	}
	return out, nil
}

// PutService PUT /v1/services/{name}.
func (c *Client) PutService(ctx context.Context, rec ServiceRecord) (ServiceRecord, error) {
	b, err := json.Marshal(rec)
	if err != nil {
		return ServiceRecord{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.Base+"/v1/services/"+url.PathEscape(rec.Name), bytes.NewReader(b))
	if err != nil {
		return ServiceRecord{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.auth(req)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return ServiceRecord{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		err = fmt.Errorf("gateway read response body: %w", err)
		return ServiceRecord{}, err
	}
	if res.StatusCode >= 300 {
		return ServiceRecord{}, fmt.Errorf("gateway put service: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	var out ServiceRecord
	if err := json.Unmarshal(body, &out); err != nil {
		return ServiceRecord{}, err
	}
	return out, nil
}

// DeleteService DELETE /v1/services/{name}.
func (c *Client) DeleteService(ctx context.Context, name string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.Base+"/v1/services/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	c.auth(req)
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
		return fmt.Errorf("gateway delete service: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// ListWorkspaces GET /v1/workspaces.
func (c *Client) ListWorkspaces(ctx context.Context) ([]WorkspaceRecord, error) {
	var out struct {
		Workspaces []WorkspaceRecord `json:"workspaces"`
	}
	if err := c.get(ctx, "/v1/workspaces", &out); err != nil {
		return nil, err
	}
	return out.Workspaces, nil
}

// CreateWorkspace POST /v1/workspaces.
func (c *Client) CreateWorkspace(ctx context.Context, name string) (WorkspaceRecord, error) {
	b, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		err = fmt.Errorf("gateway encode request: %w", err)
		return WorkspaceRecord{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/v1/workspaces", bytes.NewReader(b))
	if err != nil {
		return WorkspaceRecord{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.auth(req)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return WorkspaceRecord{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		err = fmt.Errorf("gateway read response body: %w", err)
		return WorkspaceRecord{}, err
	}
	if res.StatusCode >= 300 {
		return WorkspaceRecord{}, fmt.Errorf("gateway create workspace: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	var out WorkspaceRecord
	if err := json.Unmarshal(body, &out); err != nil {
		return WorkspaceRecord{}, err
	}
	return out, nil
}

// GetWorkspace GET /v1/workspaces/{name}.
func (c *Client) GetWorkspace(ctx context.Context, name string) (WorkspaceRecord, error) {
	var out WorkspaceRecord
	if err := c.get(ctx, "/v1/workspaces/"+url.PathEscape(name), &out); err != nil {
		return WorkspaceRecord{}, err
	}
	return out, nil
}

// DeleteWorkspace DELETE /v1/workspaces/{name}.
func (c *Client) DeleteWorkspace(ctx context.Context, name string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.Base+"/v1/workspaces/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	c.auth(req)
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
		return fmt.Errorf("gateway delete workspace: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// WorkspaceMemberAdd PUT /v1/workspaces/{name}/members.
func (c *Client) WorkspaceMemberAdd(ctx context.Context, name, subject, role string) error {
	b, err := json.Marshal(map[string]string{"subject": subject, "role": role})
	if err != nil {
		err = fmt.Errorf("gateway encode request: %w", err)
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.Base+"/v1/workspaces/"+url.PathEscape(name)+"/members", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.auth(req)
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
		return fmt.Errorf("gateway workspace member add: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// WorkspaceMemberRemove DELETE /v1/workspaces/{name}/members/{subject}.
func (c *Client) WorkspaceMemberRemove(ctx context.Context, name, subject string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.Base+"/v1/workspaces/"+url.PathEscape(name)+"/members/"+url.PathEscape(subject), nil)
	if err != nil {
		return err
	}
	c.auth(req)
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
		return fmt.Errorf("gateway workspace member remove: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}
