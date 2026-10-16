package cautem

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"

	controlv1 "github.com/cautem/quarshell-proto/gen/go/cautem/control/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type operationTestServer struct {
	controlv1.UnimplementedOperationsServiceServer
}

type sandboxCreateTestServer struct {
	controlv1.UnimplementedSandboxServiceServer
	request *controlv1.CreateSandboxRequest
}

type mainProcessAttachTestServer struct {
	controlv1.UnimplementedSandboxServiceServer
	start chan *controlv1.AttachMainProcessStart
	input chan *controlv1.AttachMainProcessRequest
}

func (s *mainProcessAttachTestServer) AttachMainProcess(stream grpc.BidiStreamingServer[controlv1.AttachMainProcessRequest, controlv1.AttachMainProcessResponse]) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	s.start <- first.GetStart()
	for i := 0; i < 2; i++ {
		request, err := stream.Recv()
		if err != nil {
			return err
		}
		s.input <- request
	}
	if err := stream.Send(&controlv1.AttachMainProcessResponse{Payload: &controlv1.AttachMainProcessResponse_Output{Output: []byte("attached output")}}); err != nil {
		return err
	}
	return stream.Send(&controlv1.AttachMainProcessResponse{Payload: &controlv1.AttachMainProcessResponse_ExitCode{ExitCode: 19}})
}

func (s *sandboxCreateTestServer) CreateSandbox(_ context.Context, request *controlv1.CreateSandboxRequest) (*controlv1.CreateSandboxResponse, error) {
	s.request = request
	return &controlv1.CreateSandboxResponse{Completed: true, OperationId: "op-create", Sandbox: &controlv1.SandboxSummary{
		Name: request.GetName(), Workspace: request.GetWorkspace(), Image: request.GetImage(),
	}}, nil
}

type lifecycleTestServer struct {
	controlv1.UnimplementedSandboxServiceServer
	completed bool
	starts    int
	stops     int
}

func (s *lifecycleTestServer) GetSandbox(_ context.Context, request *controlv1.GetSandboxRequest) (*controlv1.GetSandboxResponse, error) {
	return &controlv1.GetSandboxResponse{Sandbox: &controlv1.SandboxSummary{
		Name: request.GetName(), Workspace: request.GetWorkspace(), ComputeDriver: "kubernetes", ResourceVersion: 7,
	}}, nil
}

func (s *lifecycleTestServer) StartSandbox(_ context.Context, request *controlv1.StartSandboxRequest) (*controlv1.StartSandboxResponse, error) {
	s.starts++
	return &controlv1.StartSandboxResponse{
		Sandbox:   &controlv1.SandboxSummary{Name: request.GetName(), Workspace: request.GetWorkspace(), ComputeDriver: "kubernetes"},
		Completed: s.completed, OperationId: "start-operation",
	}, nil
}

func (s *lifecycleTestServer) StopSandbox(_ context.Context, request *controlv1.StopSandboxRequest) (*controlv1.StopSandboxResponse, error) {
	s.stops++
	return &controlv1.StopSandboxResponse{
		Sandbox:   &controlv1.SandboxSummary{Name: request.GetName(), Workspace: request.GetWorkspace(), ComputeDriver: "kubernetes"},
		Completed: s.completed, OperationId: "stop-operation",
	}, nil
}

func (operationTestServer) GetOperation(_ context.Context, request *controlv1.GetOperationRequest) (*controlv1.GetOperationResponse, error) {
	return &controlv1.GetOperationResponse{Operation: &controlv1.OperationSummary{
		Id: "operation-1", RequestId: request.GetRequestId(), Workspace: request.GetWorkspace(), Sandbox: "demo",
		Action: "sandbox.start", State: "succeeded", ResultRegistryStatus: "running", ResultResourceVersion: 8,
	}}, nil
}

func TestGetControlOperationMapsRecoveryRecord(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	controlv1.RegisterOperationsServiceServer(server, operationTestServer{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient("passthrough:///bufnet", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := &Client{rpcConn: connection}
	operation, err := client.GetControlOperation(context.Background(), "team", "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if operation.ID != "operation-1" || operation.RequestID != "request-1" || operation.Workspace != "team" || operation.State != "succeeded" || operation.ResultResourceVersion != 8 {
		t.Fatalf("unexpected operation: %+v", operation)
	}
}

func TestNewRequestIDIsOpaqueAndUnique(t *testing.T) {
	first, err := newRequestID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newRequestID()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 32 || len(second) != 32 || first == second {
		t.Fatalf("invalid request IDs: %q %q", first, second)
	}
}

func TestCreateControlSandboxCarriesSharedTemplateName(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	create := &sandboxCreateTestServer{}
	controlv1.RegisterSandboxServiceServer(server, create)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient("passthrough:///bufnet", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := &Client{rpcConn: connection}
	result, err := client.CreateControlSandboxOperation(context.Background(), Sandbox{
		Name: "agent", Workspace: "team", WorkloadTemplateName: "shared-agent",
		Labels:    Labels{"purpose": "test"},
		CPUMillis: 500, MemoryBytes: 2 * 1024 * 1024 * 1024, GPUCount: 2,
		Environment: map[string]string{"FEATURE_FLAG": "on"}, DriverConfigJSON: `{"docker":{"shm_size":67108864}}`, TTY: true,
		PidsLimit: 512, CDIDevices: []string{"nvidia.com/gpu=0", "nvidia.com/gpu=1"},
	}, []string{"sleep", "infinity"})
	if err != nil {
		t.Fatal(err)
	}
	if create.request == nil || create.request.GetWorkspace() != "team" || create.request.GetWorkloadTemplateName() != "shared-agent" || create.request.GetImage() != "" || len(create.request.GetCommand()) != 2 || create.request.GetCpuMillis() != 500 || create.request.GetMemoryBytes() != 2*1024*1024*1024 || create.request.GetGpuCount() != 2 || create.request.GetEnvironment()["FEATURE_FLAG"] != "on" || create.request.GetDriverConfigJson() == "" || !create.request.GetTty() || create.request.GetPidsLimit() != 512 || strings.Join(create.request.GetCdiDevices(), ",") != "nvidia.com/gpu=0,nvidia.com/gpu=1" {
		t.Fatalf("unexpected Control create request: %+v", create.request)
	}
	if result.Sandbox.Name != "agent" || result.Sandbox.Workspace != "team" || result.OperationID != "op-create" {
		t.Fatalf("unexpected create result: %+v", result)
	}
}

func TestAttachControlMainProcessStreamsInputOutputAndExit(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	backend := &mainProcessAttachTestServer{start: make(chan *controlv1.AttachMainProcessStart, 1), input: make(chan *controlv1.AttachMainProcessRequest, 2)}
	controlv1.RegisterSandboxServiceServer(server, backend)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient("passthrough:///bufnet", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	session, err := (&Client{rpcConn: connection}).AttachControlMainProcess(context.Background(), "team", "agent", true, 120, 40)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	start := <-backend.start
	if start == nil || start.GetWorkspace() != "team" || start.GetName() != "agent" || !start.GetTty() || start.GetCols() != 120 || start.GetRows() != 40 {
		t.Fatalf("attach start frame = %+v", start)
	}
	if n, err := session.Write([]byte("hello\n")); err != nil || n != 6 {
		t.Fatalf("write stdin = %d, %v", n, err)
	}
	if err := session.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	stdin := (<-backend.input).GetStdin()
	resize := (<-backend.input).GetResize()
	if string(stdin) != "hello\n" || resize == nil || resize.GetCols() != 100 || resize.GetRows() != 30 {
		t.Fatalf("attach input frames stdin=%q resize=%+v", stdin, resize)
	}
	output, err := io.ReadAll(session)
	if err != nil || string(output) != "attached output" {
		t.Fatalf("attach output = %q, %v", output, err)
	}
	if code, err := session.ExitCode(); err != nil || code != 19 {
		t.Fatalf("attach exit = %d, %v", code, err)
	}
}

func TestSandboxSummaryPreservesComputeDriver(t *testing.T) {
	sandbox := sandboxFromSummary(&controlv1.SandboxSummary{
		Name: "demo", Workspace: "team", ComputeDriver: "cli-managed", RegistryStatus: "running",
	})
	if sandbox.Name != "demo" || sandbox.Workspace != "team" || sandbox.ComputeDriver != "cli-managed" {
		t.Fatalf("sandbox summary mapping lost lifecycle routing fields: %+v", sandbox)
	}
}

func TestControlLifecycleRequiresAndPreservesCompletion(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	backend := &lifecycleTestServer{completed: true}
	controlv1.RegisterSandboxServiceServer(server, backend)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient("passthrough:///bufnet", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := &Client{rpcConn: connection}

	started, err := client.ChangeControlSandboxStateOperation(context.Background(), "team", "demo", "start")
	if err != nil || !started.Completed || started.OperationID != "start-operation" || started.Sandbox.ComputeDriver != "kubernetes" {
		t.Fatalf("start result = %+v, %v", started, err)
	}
	stopped, err := client.StopControlSandbox(context.Background(), "team", "demo")
	if err != nil || stopped.ComputeDriver != "kubernetes" || backend.starts != 1 || backend.stops != 1 {
		t.Fatalf("stop result = %+v, %v; calls start=%d stop=%d", stopped, err, backend.starts, backend.stops)
	}

	backend.completed = false
	if _, err := client.StartControlSandbox(context.Background(), "team", "demo"); err == nil {
		t.Fatal("StartControlSandbox accepted a response without completion confirmation")
	}
}
