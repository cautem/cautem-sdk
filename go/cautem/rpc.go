package cautem

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strings"

	compatSDK "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	compatv1 "github.com/NVIDIA/OpenShell/sdk/go/proto/openshellv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type bearerCredentials struct{ token string }

func (c bearerCredentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	if c.token == "" {
		return nil, nil
	}
	return map[string]string{"authorization": "Bearer " + c.token}, nil
}

// compatConnection creates the generated OpenShell transport for methods not
// yet wrapped by the upstream Go facade. The host alias is injected by the
// cautem CLI into sandbox proxy environments and is a trusted local route.
func (c *Client) compatConnection() (*grpc.ClientConn, error) {
	c.rpcMu.Lock()
	defer c.rpcMu.Unlock()
	if c.compatConn != nil {
		return c.compatConn, nil
	}
	endpoint, err := url.Parse(c.base)
	if err != nil || endpoint.Host == "" || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, fmt.Errorf("compatibility RPC: base URL must contain only scheme and authority")
	}
	var transport credentials.TransportCredentials
	switch strings.ToLower(endpoint.Scheme) {
	case "https":
		transport = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	case "http":
		host := strings.ToLower(endpoint.Hostname())
		if !isLoopbackHost(host) && host != "host.cautem.internal" {
			return nil, fmt.Errorf("compatibility RPC: unencrypted gRPC is allowed only for local gateway routes")
		}
		transport = insecure.NewCredentials()
	default:
		return nil, fmt.Errorf("compatibility RPC: unsupported URL scheme %q", endpoint.Scheme)
	}
	conn, err := grpc.NewClient(endpoint.Host,
		grpc.WithTransportCredentials(transport),
		grpc.WithPerRPCCredentials(bearerCredentials{token: c.token}),
	)
	if err != nil {
		return nil, fmt.Errorf("compatibility RPC: connect: %w", err)
	}
	c.compatConn = conn
	return conn, nil
}

func (c *Client) compatClient() (compatv1.OpenShellClient, error) {
	conn, err := c.compatConnection()
	if err != nil {
		return nil, err
	}
	return compatv1.NewOpenShellClient(conn), nil
}

func (bearerCredentials) RequireTransportSecurity() bool { return false }

func (c *Client) controlConn() (*grpc.ClientConn, error) {
	c.rpcMu.Lock()
	defer c.rpcMu.Unlock()
	if c.rpcConn != nil {
		return c.rpcConn, nil
	}
	endpoint, err := url.Parse(c.base)
	if err != nil || endpoint.Host == "" || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, fmt.Errorf("gateway RPC: base URL must contain only scheme and authority")
	}
	var transport credentials.TransportCredentials
	switch strings.ToLower(endpoint.Scheme) {
	case "https":
		transport = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	case "http":
		host := strings.ToLower(endpoint.Hostname())
		if !isLoopbackHost(host) && host != "host.cautem.internal" {
			return nil, fmt.Errorf("gateway RPC: unencrypted gRPC is allowed only for local gateway routes")
		}
		transport = insecure.NewCredentials()
	default:
		return nil, fmt.Errorf("gateway RPC: unsupported URL scheme %q", endpoint.Scheme)
	}
	conn, err := grpc.NewClient(endpoint.Host,
		grpc.WithTransportCredentials(transport),
		grpc.WithPerRPCCredentials(bearerCredentials{token: c.token}),
	)
	if err != nil {
		return nil, fmt.Errorf("gateway RPC: connect: %w", err)
	}
	c.rpcConn = conn
	return conn, nil
}

// compatSDKClient returns the pinned upstream OpenShell SDK client. Runtime
// operations use this contract directly; cautem-specific views use the
// curated control.v1 contract.
func (c *Client) compatSDKClient() (*compatSDK.Client, error) {
	c.rpcMu.Lock()
	defer c.rpcMu.Unlock()
	if c.upstream != nil {
		return c.upstream, nil
	}
	endpoint, err := url.Parse(c.base)
	if err != nil || endpoint.Host == "" || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, fmt.Errorf("compatibility RPC: base URL must contain only scheme and authority")
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return nil, fmt.Errorf("compatibility RPC: unsupported URL scheme %q", endpoint.Scheme)
	}
	client, err := compatSDK.NewClient(types.Config{Address: c.base, Auth: bearerCredentials{token: c.token}})
	if err != nil {
		return nil, fmt.Errorf("compatibility RPC: create client: %w", err)
	}
	c.upstream = client
	return client, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
