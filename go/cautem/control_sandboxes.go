package cautem

import (
	"context"
	"fmt"
	controlv1 "github.com/cautem/quarshell-proto/gen/go/cautem/control/v1"
)

func (c *Client) ListControlSandboxes(ctx context.Context, workspace string, allWorkspaces bool) ([]Sandbox, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	sandboxClient := controlv1.NewSandboxServiceClient(conn)
	workspaces := []string{workspace}
	if allWorkspaces {
		catalog := controlv1.NewCatalogServiceClient(conn)
		response, err := catalog.ListWorkspaces(ctx, &controlv1.ListWorkspacesRequest{})
		if err != nil {
			return nil, fmt.Errorf("list control workspaces: %w", err)
		}
		workspaces = make([]string, 0, len(response.GetWorkspaces()))
		for _, item := range response.GetWorkspaces() {
			if item.GetName() != "" {
				workspaces = append(workspaces, item.GetName())
			}
		}
	} else if workspaces[0] == "" {
		workspaces[0] = "default"
	}

	var result []Sandbox
	for _, name := range workspaces {
		pageToken := ""
		for {
			response, err := sandboxClient.ListSandboxes(ctx, &controlv1.ListSandboxesRequest{
				Workspace: name, PageSize: 100, PageToken: pageToken,
			})
			if err != nil {
				return nil, fmt.Errorf("list control sandboxes in workspace %q: %w", name, err)
			}
			for _, item := range response.GetSandboxes() {
				result = append(result, sandboxFromSummary(item))
			}
			pageToken = response.GetNextPageToken()
			if pageToken == "" {
				break
			}
		}
	}
	return result, nil
}

// GetControlSandbox returns one redacted sandbox summary through the control API.
func (c *Client) GetControlSandbox(ctx context.Context, workspace, name string) (Sandbox, error) {
	conn, err := c.controlConn()
	if err != nil {
		return Sandbox{}, err
	}
	client := controlv1.NewSandboxServiceClient(conn)
	response, err := client.GetSandbox(ctx, &controlv1.GetSandboxRequest{
		Workspace: workspace, Name: name,
	})
	if err != nil {
		return Sandbox{}, fmt.Errorf("get control sandbox %q: %w", name, err)
	}
	return sandboxFromSummary(response.GetSandbox()), nil
}

// GetControlSandboxLogs returns a bounded tail of gateway-buffered logs.
