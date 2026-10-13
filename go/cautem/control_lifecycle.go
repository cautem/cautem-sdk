package cautem

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	controlv1 "github.com/cautem/cautem-proto/gen/go/cautem/control/v1"
)

const maxControlLogWatches = 24

// ControlMutationResult identifies a lifecycle request so callers can recover
// its authoritative outcome after a timeout or broken connection.
type ControlMutationResult struct {
	Sandbox     Sandbox
	RequestID   string
	OperationID string
	Deleted     bool
}

// ControlOperation is the durable gateway record for one mutating request.
type ControlOperation struct {
	ID                    string
	RequestID             string
	Workspace             string
	Sandbox               string
	Action                string
	State                 string
	ErrorCode             string
	ResultRegistryStatus  string
	ResultResourceVersion uint64
}

// CreateControlSandbox creates a runtime sandbox through the authorized control RPC.
func (c *Client) CreateControlSandbox(ctx context.Context, sandbox Sandbox, command []string) (Sandbox, error) {
	result, err := c.CreateControlSandboxOperation(ctx, sandbox, command)
	return result.Sandbox, err
}

// CreateControlSandboxOperation creates a sandbox and returns durable request
// identifiers even when the RPC outcome is uncertain.
func (c *Client) CreateControlSandboxOperation(ctx context.Context, sandbox Sandbox, command []string) (ControlMutationResult, error) {
	conn, err := c.controlConn()
	if err != nil {
		return ControlMutationResult{}, err
	}
	client := controlv1.NewSandboxServiceClient(conn)
	requestID, err := newRequestID()
	if err != nil {
		return ControlMutationResult{}, err
	}
	result := ControlMutationResult{RequestID: requestID}
	workspace := sandbox.Workspace
	if workspace == "" {
		workspace = "default"
	}
	response, err := client.CreateSandbox(ctx, &controlv1.CreateSandboxRequest{
		Workspace: workspace, Name: sandbox.Name, Image: sandbox.Image,
		Labels: sandbox.Labels, Command: append([]string(nil), command...), RequestId: requestID,
	})
	if err != nil {
		return result, fmt.Errorf("create control sandbox %q (request %s): %w", sandbox.Name, requestID, err)
	}
	result.Sandbox = sandboxFromSummary(response.GetSandbox())
	result.OperationID = response.GetOperationId()
	return result, nil
}

// StartControlSandbox starts one caller-visible runtime sandbox.
func (c *Client) StartControlSandbox(ctx context.Context, workspace, name string) (Sandbox, error) {
	result, err := c.ChangeControlSandboxStateOperation(ctx, workspace, name, "start")
	return result.Sandbox, err
}

// StopControlSandbox stops one caller-visible runtime sandbox.
func (c *Client) StopControlSandbox(ctx context.Context, workspace, name string) (Sandbox, error) {
	result, err := c.ChangeControlSandboxStateOperation(ctx, workspace, name, "stop")
	return result.Sandbox, err
}

// DeleteControlSandbox deletes one caller-visible runtime sandbox.
func (c *Client) DeleteControlSandbox(ctx context.Context, workspace, name string) error {
	_, err := c.DeleteControlSandboxOperation(ctx, workspace, name)
	return err
}

// DeleteControlSandboxOperation deletes a sandbox and exposes its operation ID.
func (c *Client) DeleteControlSandboxOperation(ctx context.Context, workspace, name string) (ControlMutationResult, error) {
	current, err := c.GetControlSandbox(ctx, workspace, name)
	if err != nil {
		return ControlMutationResult{}, err
	}
	conn, err := c.controlConn()
	if err != nil {
		return ControlMutationResult{}, err
	}
	requestID, err := newRequestID()
	if err != nil {
		return ControlMutationResult{}, err
	}
	result := ControlMutationResult{RequestID: requestID}
	response, err := controlv1.NewSandboxServiceClient(conn).DeleteSandbox(ctx, &controlv1.DeleteSandboxRequest{
		Workspace: current.Workspace, Name: current.Name,
		ExpectedResourceVersion: current.ResourceVersion, RequestId: requestID,
	})
	if err != nil {
		return result, fmt.Errorf("delete control sandbox %q (request %s): %w", name, requestID, err)
	}
	result.Deleted = response.GetDeleted()
	result.OperationID = response.GetOperationId()
	if !response.GetDeleted() {
		return result, fmt.Errorf("delete control sandbox %q: gateway did not confirm deletion", name)
	}
	return result, nil
}

// ChangeControlSandboxStateOperation starts or stops a sandbox and exposes the
// request and operation IDs used for conflict and timeout recovery.
func (c *Client) ChangeControlSandboxStateOperation(ctx context.Context, workspace, name, action string) (ControlMutationResult, error) {
	current, err := c.GetControlSandbox(ctx, workspace, name)
	if err != nil {
		return ControlMutationResult{}, err
	}
	conn, err := c.controlConn()
	if err != nil {
		return ControlMutationResult{}, err
	}
	requestID, err := newRequestID()
	if err != nil {
		return ControlMutationResult{}, err
	}
	result := ControlMutationResult{RequestID: requestID}
	request := &controlv1.StartSandboxRequest{Workspace: current.Workspace, Name: current.Name, ExpectedResourceVersion: current.ResourceVersion, RequestId: requestID}
	client := controlv1.NewSandboxServiceClient(conn)
	switch action {
	case "start":
		response, callErr := client.StartSandbox(ctx, request)
		if callErr != nil {
			return result, fmt.Errorf("start control sandbox %q (request %s): %w", name, requestID, callErr)
		}
		result.Sandbox = sandboxFromSummary(response.GetSandbox())
		result.OperationID = response.GetOperationId()
		return result, nil
	case "stop":
		response, callErr := client.StopSandbox(ctx, &controlv1.StopSandboxRequest{
			Workspace: current.Workspace, Name: current.Name,
			ExpectedResourceVersion: current.ResourceVersion, RequestId: requestID,
		})
		if callErr != nil {
			return result, fmt.Errorf("stop control sandbox %q (request %s): %w", name, requestID, callErr)
		}
		result.Sandbox = sandboxFromSummary(response.GetSandbox())
		result.OperationID = response.GetOperationId()
		return result, nil
	default:
		return result, fmt.Errorf("unsupported sandbox action %q", action)
	}
}

// GetControlOperation resolves a lifecycle request by its idempotency key.
func (c *Client) GetControlOperation(ctx context.Context, workspace, requestID string) (ControlOperation, error) {
	conn, err := c.controlConn()
	if err != nil {
		return ControlOperation{}, err
	}
	response, err := controlv1.NewOperationsServiceClient(conn).GetOperation(ctx, &controlv1.GetOperationRequest{Workspace: workspace, RequestId: requestID})
	if err != nil {
		return ControlOperation{}, fmt.Errorf("get control operation %q: %w", requestID, err)
	}
	operation := response.GetOperation()
	if operation == nil {
		return ControlOperation{}, fmt.Errorf("get control operation %q: empty response", requestID)
	}
	return ControlOperation{
		ID: operation.GetId(), RequestID: operation.GetRequestId(), Workspace: operation.GetWorkspace(), Sandbox: operation.GetSandbox(),
		Action: operation.GetAction(), State: operation.GetState(), ErrorCode: operation.GetErrorCode(),
		ResultRegistryStatus: operation.GetResultRegistryStatus(), ResultResourceVersion: operation.GetResultResourceVersion(),
	}, nil
}

func newRequestID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("generate sandbox request id: %w", err)
	}
	return hex.EncodeToString(id[:]), nil
}

// ListControlSandboxes returns the caller-visible sandbox inventory through
// the allowlisted control API. An empty workspace selects the default
// workspace; allWorkspaces uses the catalog to enumerate accessible spaces.
