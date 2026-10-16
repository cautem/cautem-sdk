package cautem

import (
	"context"
	"fmt"

	compatSDK "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	gc "github.com/cautem/quarshell-sdk/internal/gatewayclient"
)

// WorkspaceListOptions selects one page of workspaces.
type WorkspaceListOptions struct {
	PageSize      int
	PageToken     string
	LabelSelector string
}

// WorkspaceListPage contains one page and its opaque continuation token.
type WorkspaceListPage struct {
	Workspaces    []gc.WorkspaceRecord `json:"workspaces"`
	NextPageToken string               `json:"next_page_token,omitempty"`
}

// ListWorkspacesPage lists one compatibility-contract workspace page.
func (c *Client) ListWorkspacesPage(ctx context.Context, options WorkspaceListOptions) (WorkspaceListPage, error) {
	client, err := c.compatSDKClient()
	if err != nil {
		return WorkspaceListPage{}, err
	}
	pager, err := client.Workspaces().List(compatSDK.ListOptions{
		PageSize: options.PageSize, PageToken: options.PageToken, LabelSelector: options.LabelSelector,
	})
	if err != nil {
		return WorkspaceListPage{}, err
	}
	page, err := pager.NextPage(ctx)
	if err != nil {
		return WorkspaceListPage{}, fmt.Errorf("list workspace page: %w", err)
	}
	result := WorkspaceListPage{Workspaces: make([]gc.WorkspaceRecord, 0)}
	if page == nil {
		return result, nil
	}
	result.NextPageToken = page.NextPageToken
	for _, workspace := range page.Items {
		if workspace == nil {
			continue
		}
		record := gc.WorkspaceRecord{
			ID: workspace.ID, Name: workspace.Name, ResourceVersion: workspace.ResourceVersion,
			Labels: cloneStringMap(workspace.Labels), Status: string(workspace.Phase),
		}
		if !workspace.CreatedAt.IsZero() {
			record.CreatedAt = workspace.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		result.Workspaces = append(result.Workspaces, record)
	}
	return result, nil
}
