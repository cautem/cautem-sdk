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

// ProfileInfo is a catalog entry summary.
type ProfileInfo struct {
	ID              string `json:"id"`
	Category        string `json:"category,omitempty"`
	Source          string `json:"source"`
	Scope           string `json:"scope,omitempty"`
	ResourceVersion uint64 `json:"resource_version,omitempty"`
}

// ProviderRecord is a gateway provider instance (env key names only on GET).
type ProviderRecord struct {
	Name                  string                           `json:"name"`
	Type                  string                           `json:"type"`
	Workspace             string                           `json:"workspace,omitempty"`
	EnvVars               []string                         `json:"env_vars,omitempty"`
	Credentials           Credentials                      `json:"credentials,omitempty"` // write-only on PUT
	CredentialExpiresAtMS CredentialExpiry                 `json:"credential_expires_at_ms,omitempty"`
	RuntimeCredentials    bool                             `json:"runtime_credentials,omitempty"`
	Config                ProviderConfig                   `json:"config,omitempty"`
	Refresh               map[string]ProviderRefreshConfig `json:"refresh,omitempty"`
}

// ProviderRefreshConfig is gateway-side credential rotation metadata.
type ProviderRefreshConfig struct {
	CredentialKey          string             `json:"credential_key"`
	Strategy               string             `json:"strategy"`
	Material               RefreshMaterial    `json:"material,omitempty"`
	MaterialSecretKeys     []string           `json:"material_secret_keys,omitempty"`
	MaterialCredentialKeys CredentialBindings `json:"material_credential_keys,omitempty"`
	Outputs                CredentialBindings `json:"outputs,omitempty"`
	RefreshBeforeSeconds   int64              `json:"refresh_before_seconds,omitempty"`
	MaxLifetimeSeconds     int64              `json:"max_lifetime_seconds,omitempty"`
	ExpiresAtMS            int64              `json:"expires_at_ms,omitempty"`
}

// PutProvider PUT /v1/providers/{name}. Credentials values are stored encrypted on the gateway.
func (c *Client) PutProvider(ctx context.Context, rec ProviderRecord) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.Base+"/v1/providers/"+url.PathEscape(rec.Name), bytes.NewReader(b))
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
		return fmt.Errorf("gateway put provider: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// ListProviders GET /v1/providers.
func (c *Client) ListProviders(ctx context.Context) ([]ProviderRecord, error) {
	var out struct {
		Providers []ProviderRecord `json:"providers"`
	}
	if err := c.get(ctx, "/v1/providers", &out); err != nil {
		return nil, err
	}
	return out.Providers, nil
}

// GetProvider GET /v1/providers/{name} (metadata only; no secret values).
func (c *Client) GetProvider(ctx context.Context, name string) (ProviderRecord, error) {
	var out ProviderRecord
	if err := c.get(ctx, "/v1/providers/"+url.PathEscape(name), &out); err != nil {
		return ProviderRecord{}, err
	}
	return out, nil
}

// DeleteProvider DELETE /v1/providers/{name}.
func (c *Client) DeleteProvider(ctx context.Context, name string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.Base+"/v1/providers/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
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
		return fmt.Errorf("gateway delete provider: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// AttachProvider PUT /v1/sandboxes/{sandbox}/providers/{provider}.
func (c *Client) AttachProvider(ctx context.Context, sandbox, provider string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.Base+"/v1/sandboxes/"+url.PathEscape(sandbox)+"/providers/"+url.PathEscape(provider), nil)
	if err != nil {
		return err
	}
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
		return fmt.Errorf("gateway attach provider: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// DetachProvider DELETE /v1/sandboxes/{sandbox}/providers/{provider}.
func (c *Client) DetachProvider(ctx context.Context, sandbox, provider string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.Base+"/v1/sandboxes/"+url.PathEscape(sandbox)+"/providers/"+url.PathEscape(provider), nil)
	if err != nil {
		return err
	}
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
		return fmt.Errorf("gateway detach provider: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// SandboxProviderAttachment is metadata for sandbox provider list.
type SandboxProviderAttachment struct {
	Name    string   `json:"name"`
	Type    string   `json:"type,omitempty"`
	EnvVars []string `json:"env_vars,omitempty"`
}

// ListSandboxProviders GET /v1/sandboxes/{name}/providers.
func (c *Client) ListSandboxProviders(ctx context.Context, sandbox string) ([]SandboxProviderAttachment, error) {
	var out struct {
		Providers []SandboxProviderAttachment `json:"providers"`
	}
	if err := c.get(ctx, "/v1/sandboxes/"+url.PathEscape(sandbox)+"/providers", &out); err != nil {
		return nil, err
	}
	return out.Providers, nil
}
