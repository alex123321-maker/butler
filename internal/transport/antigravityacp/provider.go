package antigravityacp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/butler/butler/internal/acp"
	"github.com/butler/butler/internal/jsonstrict"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
)

const Name = "antigravity-acp"

type Config struct {
	Enabled    bool
	Model      string
	Process    acp.AntigravityProcess
	SocketPath string
	Timeout    time.Duration
}

type runner func(context.Context, acp.AntigravityProcess, string, string, func(context.Context) error) (acp.Result, error)

type Provider struct {
	config Config
	run    runner
	mu     sync.Mutex
	runs   map[string]*runState
	slot   chan struct{}
}

type runState struct {
	ref     transport.ProviderSessionRef
	items   []transport.InputItem
	tools   []transport.ToolDefinition
	pending *transport.ToolCallRequest
	cancel  context.CancelFunc
	expiry  *time.Timer
	turn    int
}

type decision struct {
	Type      string          `json:"type"`
	Answer    *string         `json:"answer,omitempty"`
	Name      *string         `json:"name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

var capabilities = transport.CapabilitySnapshot{SupportsToolCalls: true, SupportsCancel: true}

func NewProvider(config Config) (*Provider, error) {
	if !config.Enabled || runtime.GOOS != "linux" || !filepath.IsAbs(config.SocketPath) {
		return nil, failure(transport.ErrorTypeProviderUnavailable)
	}
	return newProvider(config, func(ctx context.Context, _ acp.AntigravityProcess, model, prompt string, before func(context.Context) error) (acp.Result, error) {
		return acp.RunRemoteAntigravity(ctx, config.SocketPath, model, prompt, before)
	})
}

func newProvider(config Config, execute runner) (*Provider, error) {
	if !strings.HasPrefix(config.Model, "gemini-") || len(config.Model) > 256 || config.Timeout <= 0 || config.Timeout > 300*time.Second || execute == nil {
		return nil, failure(transport.ErrorTypeCapabilityMismatch)
	}
	return &Provider{config: config, run: execute, runs: map[string]*runState{}, slot: make(chan struct{}, 1)}, nil
}

func (*Provider) Name() string { return Name }

func (provider *Provider) AgentExecutionLimits() transport.AgentExecutionLimits {
	timeout := provider.config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	} // Identity-only zero values cannot execute.
	return transport.AgentExecutionLimits{InputBytes: 2 << 20, OutputBytes: 64 << 10, WallTime: timeout}
}

func (*Provider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return capabilities, nil
}

func (provider *Provider) StartRun(ctx context.Context, request transport.StartRunRequest) (transport.EventStream, error) {
	if request.Validate() != nil || request.Context.ProviderName != Name || request.Context.ModelName != provider.config.Model || request.Context.ProviderSessionRef != nil || len(request.TransportOptions) != 0 || request.TransportOptionsJSON != "" {
		return nil, failure(transport.ErrorTypeCapabilityMismatch)
	}
	items, err := normalize(request.InputItems, true)
	if err != nil {
		return nil, err
	}
	if err := validateTools(request.ToolDefinitions); err != nil {
		return nil, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, failure(transport.ErrorTypeInternalTransportError)
	}
	now := time.Now().UTC()
	state := &runState{
		ref:   transport.ProviderSessionRef{ProviderName: Name, SessionRef: hex.EncodeToString(random[:]), CreatedAt: now, LastUsedAt: now},
		items: items, tools: append([]transport.ToolDefinition{}, request.ToolDefinitions...),
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.runs[request.Context.RunID] != nil || len(provider.runs) >= 16 {
		return nil, failure(transport.ErrorTypeProviderUnavailable)
	}
	provider.runs[request.Context.RunID] = state
	return provider.executeLocked(ctx, request.Context.RunID, state), nil
}

func (provider *Provider) ContinueRun(ctx context.Context, request transport.ContinueRunRequest) (transport.EventStream, error) {
	if request.Validate() != nil || len(request.TransportOptions) != 0 || request.TransportOptionsJSON != "" {
		return nil, failure(transport.ErrorTypeCapabilityMismatch)
	}
	items, err := normalize(request.InputItems, false)
	if err != nil {
		return nil, err
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	state := provider.runs[request.RunID]
	if !matches(state, request.ProviderSessionRef) || state.cancel != nil || state.pending != nil {
		return nil, failure(transport.ErrorTypeStatefulSessionLost)
	}
	if err := replaceTools(state, request.ToolSet); err != nil {
		return nil, err
	}
	state.items = append(state.items, items...)
	return provider.executeLocked(ctx, request.RunID, state), nil
}

func (provider *Provider) SubmitToolResult(ctx context.Context, request transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if request.Validate() != nil || len(request.TransportOptions) != 0 || request.TransportOptionsJSON != "" {
		return nil, failure(transport.ErrorTypeCapabilityMismatch)
	}
	var observation any
	if jsonstrict.Decode([]byte(request.ToolResultJSON), &observation) != nil {
		return nil, failure(transport.ErrorTypeInvalidToolRequest)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	state := provider.runs[request.RunID]
	if !matches(state, request.ProviderSessionRef) || state.cancel != nil || state.pending == nil || state.pending.ToolCallRef != request.ToolCallRef {
		return nil, failure(transport.ErrorTypeStatefulSessionLost)
	}
	if err := replaceTools(state, request.ToolSet); err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(map[string]any{"tool_call_ref": state.pending.ToolCallRef, "tool_name": state.pending.ToolName, "result": observation})
	state.items = append(state.items, transport.InputItem{Role: "tool", Name: state.pending.ToolName, Content: string(encoded), ContentType: "application/json"})
	state.pending = nil
	return provider.executeLocked(ctx, request.RunID, state), nil
}

func (provider *Provider) CancelRun(_ context.Context, request transport.CancelRunRequest) (*transport.TransportEvent, error) {
	if request.Validate() != nil {
		return nil, failure(transport.ErrorTypeCapabilityMismatch)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if state := provider.runs[request.RunID]; state != nil {
		if request.ProviderSessionRef != nil && !matches(state, request.ProviderSessionRef) {
			return nil, failure(transport.ErrorTypeStatefulSessionLost)
		}
		if state.cancel != nil {
			state.cancel()
		}
		if state.expiry != nil {
			state.expiry.Stop()
		}
		delete(provider.runs, request.RunID)
	} else if request.ProviderSessionRef != nil {
		return nil, failure(transport.ErrorTypeStatefulSessionLost)
	}
	event := transport.NewTerminalEvent(request.RunID, transport.EventTypeRunCancelled, Name)
	return &event, nil
}

func (provider *Provider) executeLocked(parent context.Context, runID string, state *runState) transport.EventStream {
	if state.expiry != nil {
		state.expiry.Stop()
		state.expiry = nil
	}
	ctx, cancel := context.WithTimeout(parent, provider.config.Timeout)
	state.cancel = cancel
	state.turn++
	turn := state.turn
	ref := state.ref
	encoded, encodeErr := json.Marshal(struct {
		Messages []transport.InputItem      `json:"messages"`
		Tools    []transport.ToolDefinition `json:"tools"`
	}{state.items, state.tools})
	out := make(chan transport.TransportEvent, 4)
	go func() {
		defer close(out)
		defer cancel()
		send := func(event transport.TransportEvent) bool {
			if ctx.Err() != nil {
				return false
			}
			select {
			case out <- event:
				return true
			case <-ctx.Done():
				return false
			}
		}
		finishError := func(err error) {
			provider.mu.Lock()
			owned := provider.runs[runID] == state
			if owned {
				delete(provider.runs, runID)
			}
			provider.mu.Unlock()
			if owned {
				out <- transport.NewTransportErrorEvent(runID, Name, err)
			}
		}
		if encodeErr != nil || len(encoded) > 2<<20 {
			finishError(failure(transport.ErrorTypeInputBytesExceeded))
			return
		}
		if err := transport.CheckSerializedInput(ctx, len(encoded)); err != nil {
			finishError(err)
			return
		}
		select {
		case provider.slot <- struct{}{}:
		case <-ctx.Done():
			finishError(failure(transport.ErrorTypeProviderTimeout))
			return
		}
		defer func() { <-provider.slot }()
		if err := transport.CheckInferenceAuthority(ctx); err != nil {
			finishError(err)
			return
		}
		if !send(transport.NewRunStartedEvent(runID, Name, capabilities, &ref)) {
			finishError(failure(transport.ErrorTypeProviderTimeout))
			return
		}
		answer, err := provider.run(ctx, provider.config.Process, provider.config.Model, string(encoded), func(callCtx context.Context) error {
			if err := transport.CheckInferenceAuthority(callCtx); err != nil {
				return err
			}
			return transport.CheckInferenceDispatch(callCtx)
		})
		if err != nil {
			kind := transport.ErrorTypeProviderProtocolError
			if errors.Is(err, acp.ErrProcess) || errors.Is(err, acp.ErrProcessUnsupported) || errors.Is(err, acp.ErrAuthenticationRequired) {
				kind = transport.ErrorTypeProviderUnavailable
			}
			if ctx.Err() != nil || errors.Is(err, acp.ErrBudget) {
				kind = transport.ErrorTypeProviderTimeout
			}
			finishError(failure(kind))
			return
		}
		if answer.ModelID != provider.config.Model || answer.StopReason != "end_turn" || answer.UsageState != "unknown" {
			finishError(failure(transport.ErrorTypeProviderProtocolError))
			return
		}
		if err := transport.CheckInferenceAuthority(ctx); err != nil {
			finishError(err)
			return
		}
		var value decision
		if len(answer.Text) > 64<<10 || jsonstrict.Decode([]byte(answer.Text), &value) != nil {
			finishError(failure(transport.ErrorTypeProviderProtocolError))
			return
		}
		if err := validateDecision(value, state.tools); err != nil {
			finishError(err)
			return
		}
		provider.mu.Lock()
		if provider.runs[runID] != state || ctx.Err() != nil {
			provider.mu.Unlock()
			finishError(failure(transport.ErrorTypeProviderTimeout))
			return
		}
		state.cancel = nil
		state.items = append(state.items, transport.InputItem{Role: "assistant", Content: answer.Text, ContentType: "application/json"})
		state.expiry = time.AfterFunc(15*time.Minute, func() {
			provider.mu.Lock()
			defer provider.mu.Unlock()
			if provider.runs[runID] == state && state.turn == turn && state.cancel == nil {
				delete(provider.runs, runID)
			}
		})
		if value.Type == "tool_call" {
			proposal := transport.ToolCallRequest{ToolCallRef: fmt.Sprintf("acp-%s-%d", ref.SessionRef, turn), ToolName: *value.Name, ArgsJSON: string(value.Arguments), SequenceNo: turn}
			state.pending = &proposal
			provider.mu.Unlock()
			send(transport.NewToolCallRequestedEvent(runID, Name, proposal))
			return
		}
		provider.mu.Unlock()
		send(transport.NewAssistantFinalEvent(runID, Name, transport.AssistantFinal{Content: *value.Answer, FinishReason: "stop"}))
		send(transport.NewTerminalEvent(runID, transport.EventTypeRunCompleted, Name))
	}()
	return out
}

func matches(state *runState, ref *transport.ProviderSessionRef) bool {
	return state != nil && ref != nil && ref.ProviderName == Name && ref.SessionRef == state.ref.SessionRef && ref.ResponseRef == ""
}

func normalize(input []transport.InputItem, systemAllowed bool) ([]transport.InputItem, error) {
	for _, item := range input {
		if item.Role != "user" && item.Role != "assistant" && !(systemAllowed && (item.Role == "system" || item.Role == "developer")) {
			return nil, failure(transport.ErrorTypeProviderProtocolError)
		}
	}
	items, err := content.Normalize(input)
	if err != nil || len(items) == 0 {
		return nil, failure(transport.ErrorTypeProviderProtocolError)
	}
	for index := range items {
		if items[index].Role != "user" && items[index].Role != "assistant" && !(systemAllowed && (items[index].Role == "system" || items[index].Role == "developer")) {
			return nil, failure(transport.ErrorTypeProviderProtocolError)
		}
		items[index].Metadata = nil
	}
	return items, nil
}

func validateTools(tools []transport.ToolDefinition) error {
	if len(tools) > 256 || transport.NewToolSetSnapshot(tools).Validate() != nil {
		return failure(transport.ErrorTypeInvalidToolRequest)
	}
	for _, tool := range tools {
		if antigravity.ValidateLogicalToolSchema(tool.SchemaJSON) != nil {
			return failure(transport.ErrorTypeInvalidToolRequest)
		}
	}
	return nil
}

func replaceTools(state *runState, snapshot *transport.ToolSetSnapshot) error {
	if snapshot == nil {
		return nil
	}
	if snapshot.Validate() != nil || validateTools(snapshot.Definitions) != nil {
		return failure(transport.ErrorTypeInvalidToolRequest)
	}
	state.tools = append([]transport.ToolDefinition{}, snapshot.Definitions...)
	return nil
}

func validateDecision(value decision, tools []transport.ToolDefinition) error {
	switch value.Type {
	case "answer":
		if value.Answer != nil && strings.TrimSpace(*value.Answer) != "" && value.Name == nil && value.Arguments == nil {
			return nil
		}
	case "tool_call":
		if value.Answer == nil && value.Name != nil && len(value.Arguments) > 0 && strings.HasPrefix(strings.TrimSpace(string(value.Arguments)), "{") {
			for _, tool := range tools {
				if tool.Name == *value.Name && antigravity.ValidateLogicalToolArguments(tool.SchemaJSON, value.Arguments) == nil {
					return nil
				}
			}
		}
	}
	return failure(transport.ErrorTypeInvalidToolRequest)
}

func failure(kind transport.ErrorType) error {
	return &transport.Error{Type: kind, ProviderName: Name, Message: "Antigravity ACP request refused or unavailable", Retryable: false}
}
