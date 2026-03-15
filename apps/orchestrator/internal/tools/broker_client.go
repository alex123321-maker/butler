package tools

import (
	"context"
	"fmt"
	"strings"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type BrokerClient struct {
	conn   *grpc.ClientConn
	client toolbrokerv1.ToolBrokerServiceClient
}

func Dial(address string) (*BrokerClient, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("tool broker address is required")
	}
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
