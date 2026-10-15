// Read-only isolated Broker readiness check. Never executes a tool or model.
package main

import (
	"context"
	"encoding/json"
	"os"
	"time"

	broker "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if os.Getenv("BUTLER_TOOL_BROKER_ADDR") != "tool-broker:9090" {
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.NewClient("tool-broker:9090", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		os.Exit(1)
	}
	defer conn.Close()
	result, err := broker.NewToolBrokerServiceClient(conn).ListTools(ctx, &broker.ListToolsRequest{Limit: 16})
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"list_tools_succeeded": err == nil, "tool_count": len(result.GetTools()), "inference_calls": 0, "tool_executions": 0})
	if err != nil {
		os.Exit(1)
	}
}
