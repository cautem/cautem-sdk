package cautem

import (
	"context"
	"fmt"
	controlv1 "github.com/cautem/quarshell-proto/gen/go/cautem/control/v1"
)

// ControlOverview returns caller-visible gateway summary data through the
// curated control API.
func (c *Client) ControlOverview(ctx context.Context) (map[string]any, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	console := controlv1.NewConsoleServiceClient(conn)
	viewer, err := console.GetViewer(ctx, &controlv1.GetViewerRequest{})
	if err != nil {
		return nil, fmt.Errorf("get control viewer: %w", err)
	}
	workspace := c.workspace()
	overview, err := console.GetOverview(ctx, &controlv1.GetOverviewRequest{Workspace: workspace})
	if err != nil {
		return nil, fmt.Errorf("get control overview: %w", err)
	}
	capabilities, err := console.GetConsoleCapabilities(ctx, &controlv1.GetConsoleCapabilitiesRequest{})
	if err != nil {
		return nil, fmt.Errorf("get control capabilities: %w", err)
	}
	return map[string]any{
		"gateway_id":                  overview.GetGatewayId(),
		"workspace":                   workspace,
		"sandbox_count":               overview.GetSandboxCount(),
		"registry_running_count":      overview.GetRegistryRunningCount(),
		"snapshot_at_unix_ms":         overview.GetSnapshotAtUnixMs(),
		"auth_mode":                   viewer.GetIdentityProvider(),
		"roles":                       append([]string(nil), viewer.GetRoles()...),
		"scopes":                      append([]string(nil), viewer.GetScopes()...),
		"compute_drivers":             append([]string(nil), capabilities.GetComputeDrivers()...),
		"sandbox_lifecycle_available": capabilities.GetSandboxLifecycleAvailable(),
	}, nil
}

// Whoami returns the authenticated viewer from the curated control API.
func (c *Client) Whoami(ctx context.Context) (map[string]any, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	response, err := controlv1.NewConsoleServiceClient(conn).GetViewer(ctx, &controlv1.GetViewerRequest{})
	if err != nil {
		return nil, fmt.Errorf("get control viewer: %w", err)
	}
	return map[string]any{
		"auth":              "authenticated",
		"subject":           response.GetSubject(),
		"roles":             append([]string(nil), response.GetRoles()...),
		"scopes":            append([]string(nil), response.GetScopes()...),
		"identity_provider": response.GetIdentityProvider(),
	}, nil
}

// ListProposals returns visible policy proposals through the Control API.
