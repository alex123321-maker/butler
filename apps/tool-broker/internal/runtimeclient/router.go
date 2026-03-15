package runtimeclient

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Router struct {
	log   *slog.Logger
	mu    sync.Mutex
	conns map[string]*grpc.ClientConn
}

func New(log *slog.Logger) *Router {
	if log == nil {
		log = slog.Default()
	}
	return &Router{log: logger.WithComponent(log, "runtime-router"), conns: make(map[string]*grpc.ClientConn)}
}

func (r *Router) Execute(ctx context.Context, toolCall *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract) (*toolbrokerv1.ToolResult, error) {
	if toolCall == nil {
		return nil, fmt.Errorf("tool_call is required")
	}
	if contract == nil {
		return nil, fmt.Errorf("tool contract is required")
	}
	target := strings.TrimSpace(contract.GetRuntimeTarget())
	if target == "" {
		target = strings.TrimSpace(toolCall.GetRuntimeTarget())
	}
	if target == "" {
		return nil, fmt.Errorf("runtime target is required")
	}

	client, err := r.clientFor(target)
	if err != nil {
		return nil, err
	}
	resp, err := client.Execute(ctx, &runtimev1.ExecuteRequest{
		Context:  executionContext(ctx, toolCall, contract, target),
		ToolCall: toolCall,
		Contract: contract,
	})
	if err != nil {
		return nil, fmt.Errorf("execute runtime tool via %s: %w", target, err)
	}
	if resp.GetResult() == nil {
		return nil, fmt.Errorf("runtime %s returned empty result", target)
	}
	result := resp.GetResult()
	if strings.TrimSpace(result.GetRunId()) == "" {
		result.RunId = toolCall.GetRunId()
	}
	if strings.TrimSpace(result.GetToolCallId()) == "" {
		result.ToolCallId = toolCall.GetToolCallId()
	}
	if strings.TrimSpace(result.GetToolName()) == "" {
		result.ToolName = toolCall.GetToolName()
	}
	return result, nil
}

func (r *Router) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var closeErr error
	for target, conn := range r.conns {
		if err := conn.Close(); err != nil && closeErr == nil {
			closeErr = fmt.Errorf("close runtime connection %s: %w", target, err)
		}
		delete(r.conns, target)
	}
	return closeErr
}

func (r *Router) clientFor(target string) (runtimev1.ToolRuntimeServiceClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if conn, ok := r.conns[target]; ok {
		return runtimev1.NewToolRuntimeServiceClient(conn), nil
	}
	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(dialCtx, target, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("dial runtime target %s: %w", target, err)
	}
	r.conns[target] = conn
	r.log.Info("runtime target connected", slog.String("runtime_target", target))
	return runtimev1.NewToolRuntimeServiceClient(conn), nil
}

func executionContext(ctx context.Context, toolCall *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, target string) *runtimev1.ExecutionContext {
	timeoutMS := int64(0)
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 {
			timeoutMS = remaining.Milliseconds()
		}
	}
	return &runtimev1.ExecutionContext{
		RequestId:     toolCall.GetRunId() + ":" + toolCall.GetToolCallId(),
		RunId:         toolCall.GetRunId(),
		ToolCallId:    toolCall.GetToolCallId(),
		ToolName:      contract.GetToolName(),
		RuntimeTarget: target,
		TimeoutMs:     timeoutMS,
	}
}
