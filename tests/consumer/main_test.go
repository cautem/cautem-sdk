package consumer

import (
	"context"
	"testing"

	"github.com/cautem/quarshell-sdk/go/cautem"
)

func TestPublicClientConstructionAndLocalMethod(t *testing.T) {
	client := cautem.New("http://127.0.0.1:7443")
	t.Cleanup(func() { _ = client.Close() })

	if client.Workspace != "" {
		t.Fatalf("new client Workspace = %q, want empty default", client.Workspace)
	}
	if err := client.Connect(context.Background(), "demo"); err == nil {
		t.Fatal("Connect error = nil, want the documented unsupported-method error")
	}
}
