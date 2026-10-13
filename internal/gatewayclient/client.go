package gatewayclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RequestTimeout bounds ordinary gateway requests; RelayTimeout allows relay execution.
const (
	RequestTimeout = 10 * time.Second
	RelayTimeout   = 70 * time.Second
)

// Labels maps metadata names to their values.
type Labels = map[string]string

// Credentials maps credential environment keys to secret values.
type Credentials = map[string]string

// ProviderConfig maps provider option names to their values.
type ProviderConfig = map[string]string

// RefreshMaterial maps refresh input names to their values.
type RefreshMaterial = map[string]string

// CredentialBindings maps refresh inputs or outputs to credential keys.
type CredentialBindings = map[string]string

// CredentialExpiry maps credential keys to Unix millisecond expiration times.
type CredentialExpiry = map[string]int64

// Client is a tiny HTTP client for cautem-gateway.
type Client struct {
	Base  string
	Token string // optional Bearer
	HTTP  *http.Client
}

// New returns a client for base URL (for example http://127.0.0.1:7443).
// The gateway requires a bearer on every /v1 route; set Token (or use
// NewWithToken) — it is attached to every request by the transport.
func New(base string) *Client {
	c := &Client{Base: strings.TrimRight(base, "/")}
	c.HTTP = &http.Client{Timeout: RequestTimeout, Transport: &authTransport{c: c}}
	return c
}

// authTransport adds the bearer to requests that do not carry one, so no
// call site can forget authentication.
type authTransport struct {
	c    *Client
	base http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	if tok := t.c.Token; tok != "" && req.Header.Get("Authorization") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return base.RoundTrip(req)
}

// NewWithToken returns a client with bearer auth.
func NewWithToken(base, token string) *Client {
	c := New(base)
	c.Token = strings.TrimSpace(token)
	return c
}

func (c *Client) auth(req *http.Request) {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
}

// Healthz hits /healthz.
func (c *Client) Healthz(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.get(ctx, "/healthz", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, path string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+path, nil)
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
		return fmt.Errorf("gateway %s: %s: %s", path, res.Status, bytes.TrimSpace(body))
	}
	return json.NewDecoder(res.Body).Decode(dest)
}
