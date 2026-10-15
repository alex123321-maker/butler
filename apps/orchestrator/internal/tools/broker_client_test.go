package tools

import (
	"context"
	"errors"
	"testing"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/grpc"
)

func TestRequiresApprovalUsesGetToolContract(t *testing.T) {
	t.Parallel()

	fake := &fakeBrokerServiceClient{
		contract: &toolbrokerv1.ToolContract{ToolName: "http.request", RequiresApproval: true},
	}
	client := &BrokerClient{client: fake}

	needsApproval, err := client.RequiresApproval(context.Background(), "http.request")
	if err != nil {
		t.Fatalf("RequiresApproval returned error: %v", err)
	}
	if !needsApproval {
		t.Fatal("expected requires approval to be true")
	}
	if fake.getCalls != 1 {
		t.Fatalf("expected GetToolContract to be called once, got %d", fake.getCalls)
	}
	if fake.validateCalls != 0 {
		t.Fatalf("expected ValidateToolCall to not be called, got %d", fake.validateCalls)
	}
}

type fakeBrokerServiceClient struct {
	toolbrokerv1.ToolBrokerServiceClient
	contract      *toolbrokerv1.ToolContract
	getCalls      int
	validateCalls int
	err           error
}

func (f *fakeBrokerServiceClient) ValidateToolCall(context.Context, *toolbrokerv1.ValidateToolCallRequest, ...grpc.CallOption) (*toolbrokerv1.ValidateToolCallResponse, error) {
	f.validateCalls++
	return &toolbrokerv1.ValidateToolCallResponse{}, nil
}

func (f *fakeBrokerServiceClient) ExecuteToolCall(context.Context, *toolbrokerv1.ExecuteToolCallRequest, ...grpc.CallOption) (*toolbrokerv1.ExecuteToolCallResponse, error) {
	return &toolbrokerv1.ExecuteToolCallResponse{}, nil
}

func (f *fakeBrokerServiceClient) ListTools(context.Context, *toolbrokerv1.ListToolsRequest, ...grpc.CallOption) (*toolbrokerv1.ListToolsResponse, error) {
	return &toolbrokerv1.ListToolsResponse{}, nil
}

func (f *fakeBrokerServiceClient) GetToolContract(context.Context, *toolbrokerv1.GetToolContractRequest, ...grpc.CallOption) (*toolbrokerv1.GetToolContractResponse, error) {
	f.getCalls++
	return &toolbrokerv1.GetToolContractResponse{Contract: f.contract}, f.err
}

func TestRequiresApprovalDoesNotTreatMissingMetadataAsPermission(t *testing.T) {
	for _, failure := range []error{nil, errors.New("metadata unavailable")} {
		client := &BrokerClient{client: &fakeBrokerServiceClient{err: failure}}
		if _, err := client.RequiresApproval(context.Background(), "http.request"); err == nil {
			t.Fatal("missing metadata accepted")
		}
	}
}
