package cautem

import (
	"context"
	"net"
	"sync"
	"testing"

	compatv1 "github.com/NVIDIA/OpenShell/sdk/go/proto/openshellv1"
	"google.golang.org/grpc"
)

type providerEnvironmentCapabilityServer struct {
	compatv1.UnimplementedOpenShellServer
	mu      sync.Mutex
	request *compatv1.GetSandboxProviderEnvironmentRequest
}

func (s *providerEnvironmentCapabilityServer) GetSandboxProviderEnvironment(_ context.Context, req *compatv1.GetSandboxProviderEnvironmentRequest) (*compatv1.GetSandboxProviderEnvironmentResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.request = req
	return &compatv1.GetSandboxProviderEnvironmentResponse{Environment: map[string]string{"API_TOKEN": "bound-value"}}, nil
}

func TestResolveProxySecretsAdvertisesOnlyStaticEndpointBindingCapability(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	service := &providerEnvironmentCapabilityServer{}
	compatv1.RegisterOpenShellServer(server, service)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	client := NewWithToken("http://"+listener.Addr().String(), "sandbox-token")
	t.Cleanup(func() { _ = client.Close() })
	got, err := client.ResolveProxySecrets(context.Background(), "sandbox-a")
	if err != nil || got["API_TOKEN"] != "bound-value" {
		t.Fatalf("resolved credentials=%v err=%v", got, err)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.request == nil || service.request.GetSandboxId() != "sandbox-a" || !service.request.GetSupportsStaticCredentialBindings() || service.request.GetSupportsStablePlaceholderEnvironmentKeys() {
		t.Fatalf("capability request=%v", service.request)
	}
}
