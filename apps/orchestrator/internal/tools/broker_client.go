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

func Dial(ctx context.Context, address string) (*BrokerClient, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("tool broker address is required")
	}
	conn, err := grpc.DialContext(ctx, address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
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

func (c *BrokerClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}
