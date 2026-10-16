package cautem

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	compatv1 "github.com/NVIDIA/OpenShell/sdk/go/proto/openshellv1"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
)

type rootfsStagingServer struct {
	compatv1.UnimplementedOpenShellServer
	gotWorkspace string
	gotFileName  string
	gotSize      uint64
}

func (s *rootfsStagingServer) BeginRootfsTarStaging(_ context.Context, request *compatv1.BeginRootfsTarStagingRequest) (*compatv1.BeginRootfsTarStagingResponse, error) {
	s.gotWorkspace = request.GetWorkspaceScope().GetWorkspace()
	s.gotFileName = request.GetFileName()
	s.gotSize = request.GetSizeBytes()
	return &compatv1.BeginRootfsTarStagingResponse{StagingToken: "opaque-token"}, nil
}

func TestUploadRootfsTarStreamsFromBeginToHTTP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	grpcServer := grpc.NewServer()
	staging := &rootfsStagingServer{}
	compatv1.RegisterOpenShellServer(grpcServer, staging)
	var uploadedBody, uploadedAuth, uploadedType string
	var uploadedLength int64
	handler := h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/v1/rootfs-tar/opaque-token" {
			http.NotFound(w, r)
			return
		}
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			http.Error(w, readErr.Error(), http.StatusBadRequest)
			return
		}
		uploadedBody = string(body)
		uploadedAuth = r.Header.Get("Authorization")
		uploadedType = r.Header.Get("Content-Type")
		uploadedLength = r.ContentLength
		w.WriteHeader(http.StatusCreated)
	}), &http2.Server{})
	httpServer := &http.Server{Handler: handler}
	go func() { _ = httpServer.Serve(listener) }()
	t.Cleanup(func() {
		_ = httpServer.Close()
		grpcServer.Stop()
	})

	client := NewWithToken("http://"+listener.Addr().String(), "test-bearer")
	client.Workspace = "team-a"
	t.Cleanup(func() { _ = client.Close() })
	token, err := client.UploadRootfsTar(context.Background(), "rootfs.tar", uint64(len("tar-bytes")), strings.NewReader("tar-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if token != "opaque-token" {
		t.Fatalf("token=%q", token)
	}
	if staging.gotWorkspace != "team-a" || staging.gotFileName != "rootfs.tar" || staging.gotSize != uint64(len("tar-bytes")) {
		t.Fatalf("staging request workspace=%q file=%q size=%d", staging.gotWorkspace, staging.gotFileName, staging.gotSize)
	}
	if uploadedBody != "tar-bytes" || uploadedAuth != "Bearer test-bearer" || uploadedType != "application/x-tar" || uploadedLength != int64(len("tar-bytes")) {
		t.Fatalf("upload body=%q auth=%q type=%q length=%d", uploadedBody, uploadedAuth, uploadedType, uploadedLength)
	}
}
