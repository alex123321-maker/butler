package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type BrokerClient struct {
	conn   *grpc.ClientConn
	client toolbrokerv1.ToolBrokerServiceClient
}

func (c *BrokerClient) PrepareAction(ctx context.Context, call *toolbrokerv1.ToolCall) (actionauth.PreparedAction, error) {
	response, err := c.client.PrepareToolCall(ctx, &toolbrokerv1.PrepareToolCallRequest{ToolCall: call})
	if err != nil {
		return actionauth.PreparedAction{}, err
	}
	if response.GetError() != nil {
		return actionauth.PreparedAction{}, fmt.Errorf("action preparation denied: %s", response.GetError().GetMessage())
	}
	a := response.GetAction()
	if a == nil {
		return actionauth.PreparedAction{}, fmt.Errorf("tool broker omitted exact action policy")
	}
	var binding actionauth.ActionBinding
	if json.Unmarshal([]byte(a.GetBindingJson()), &binding) != nil || binding.ActionID != a.GetActionId() || binding.ActionHash != a.GetActionHash() || binding.RunID != call.GetRunId() || binding.ToolCallID != call.GetToolCallId() {
		return actionauth.PreparedAction{}, fmt.Errorf("tool broker returned incompatible action binding")
	}
	expiry, err := time.Parse(time.RFC3339Nano, a.GetExpiresAt())
	if err != nil {
		return actionauth.PreparedAction{}, fmt.Errorf("tool broker omitted action expiry")
	}
	return actionauth.PreparedAction{Binding: binding, RequiresApproval: a.GetRequiresApproval(), Summary: a.GetSummary(), ExpiresAt: expiry}, nil
}
func (c *BrokerClient) AuthorizeAction(ctx context.Context, call *toolbrokerv1.ToolCall, actionID, approvalID, mandateID string) (string, error) {
	response, err := c.client.AuthorizeToolCall(ctx, &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: actionID, ApprovalId: approvalID, MandateId: mandateID})
	if err != nil {
		return "", err
	}
	if response.GetError() != nil {
		return "", fmt.Errorf("action authorization denied: %s", response.GetError().GetMessage())
	}
	if response.GetAuthorizationId() == "" {
		return "", fmt.Errorf("tool broker omitted dispatch authorization")
	}
	return response.GetAuthorizationId(), nil
}
func (c *BrokerClient) ExecuteAuthorizedToolCall(ctx context.Context, call *toolbrokerv1.ToolCall, grantID string) (*toolbrokerv1.ToolResult, error) {
	resp, err := c.client.ExecuteToolCall(ctx, &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: grantID})
	if err != nil {
		return nil, err
	}
	if resp.GetResult() == nil {
		return nil, fmt.Errorf("tool broker returned empty dispatch result")
	}
	return resp.GetResult(), nil
}

func Dial(address string) (*BrokerClient, error) {
	return dial(address, "")
}

func DialWithTokenFile(address, path string) (*BrokerClient, error) {
	if strings.TrimSpace(path) == "" {
		return Dial(address)
	}
	token, err := actionauth.ReadServiceToken(path)
	if err != nil {
		return nil, err
	}
	return dial(address, token)
}

func dial(address, token string) (*BrokerClient, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("tool broker address is required")
	}
	options := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if token != "" {
		options = append(options, grpc.WithPerRPCCredentials(actionauth.ServiceTokenCredentials{Token: token}))
	}
	conn, err := grpc.NewClient(address, options...)
	if err != nil {
		return nil, fmt.Errorf("dial tool broker %s: %w", address, err)
	}
	return &BrokerClient{conn: conn, client: toolbrokerv1.NewToolBrokerServiceClient(conn)}, nil
}

func (c *BrokerClient) ExecuteToolCall(ctx context.Context, call *toolbrokerv1.ToolCall) (*toolbrokerv1.ToolResult, error) {
	resp, err := c.client.ExecuteToolCall(ctx, &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call})
	if err != nil {
		return nil, err
	}
	if resp.GetResult() == nil {
		return nil, fmt.Errorf("tool broker returned empty result")
	}
	return resp.GetResult(), nil
}

func (c *BrokerClient) ListTools(ctx context.Context) ([]*toolbrokerv1.ToolContract, error) {
	resp, err := c.client.ListTools(ctx, &toolbrokerv1.ListToolsRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetTools(), nil
}

// RequiresApproval checks whether a tool requires approval before execution
// by calling ValidateToolCall and inspecting the returned contract.
func (c *BrokerClient) RequiresApproval(ctx context.Context, toolName string) (bool, error) {
	resp, err := c.client.ValidateToolCall(ctx, &toolbrokerv1.ValidateToolCallRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolName: toolName, ArgsJson: "{}"},
	})
	if err != nil {
		return false, fmt.Errorf("validate tool call for approval check: %w", err)
	}
	if resp.GetContract() == nil {
		return false, nil
	}
	return resp.GetContract().GetRequiresApproval(), nil
}

func (c *BrokerClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}
