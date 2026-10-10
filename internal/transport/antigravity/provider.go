// Package antigravity implements the AICode subscription HTTP/SSE protocol.
// It proposes functions; only Butler's execution kernel can execute them.
package antigravity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
)

const (
	Name            = "antigravity"
	DefaultModel    = "gemini-3.8-flash-tiered"
	endpoint        = "https://aicode.googleapis.com/v1"
	maxOutputTokens = 8192
)

type Config struct {
	Model      string
	Timeout    time.Duration
	AuthSource providerauth.AntigravityTokenSource
}

type Provider struct {
	config Config
	client *http.Client
	mu     sync.Mutex
	runs   map[string]*runState
}

type runState struct {
	model         string
	ref           transport.ProviderSessionRef
	system        *content
	contents      []content
	tools         []functionDeclaration
	definitions   map[string]transport.ToolDefinition
	pending       *transport.ToolCallRequest
	pendingNative functionCall
	turn          int
	cancel        context.CancelFunc
}

var capabilities = transport.CapabilitySnapshot{SupportsStreaming: true, SupportsToolCalls: true, SupportsCancel: true, SupportsUsageMetadata: true}

func init() {
	transport.RegisterProvider(Name, func(raw any) (transport.ModelProvider, error) {
		cfg, ok := raw.(Config)
		if !ok {
			return nil, fmt.Errorf("antigravity: expected Config")
		}
		return NewProvider(cfg)
	})
}

func NewProvider(cfg Config) (*Provider, error) {
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if cfg.AuthSource == nil {
		return nil, providerError(transport.ErrorTypeProviderUnavailable, "Antigravity authentication source is required")
	}
	client := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Provider{config: cfg, client: client, runs: make(map[string]*runState)}, nil
}

func (p *Provider) Name() string { return Name }
func (p *Provider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return capabilities, nil
}

func (p *Provider) StartRun(ctx context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if req.Context.ProviderName != Name || req.Context.ProviderSessionRef != nil {
		return nil, protocolError("Antigravity requires a fresh Butler-owned context")
	}
	if len(req.TransportOptions) > 0 || req.TransportOptionsJSON != "" {
		return nil, protocolError("Antigravity transport options are unsupported")
	}
	system, contents, err := convertInputs(req.InputItems)
	if err != nil {
		return nil, err
	}
	declarations, definitions, err := convertTools(req.ToolDefinitions)
	if err != nil {
		return nil, err
	}
	ref, err := newReference()
	if err != nil {
		return nil, err
	}
	state := &runState{model: req.Context.ModelName, ref: ref, system: system, contents: contents, tools: declarations, definitions: definitions}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.runs[req.Context.RunID]; exists {
		return nil, protocolError("run is already active")
	}
	p.runs[req.Context.RunID] = state
	return p.executeLocked(ctx, req.Context.RunID, state), nil
}

func (p *Provider) ContinueRun(ctx context.Context, req transport.ContinueRunRequest) (transport.EventStream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if len(req.TransportOptions) > 0 || req.TransportOptionsJSON != "" {
		return nil, protocolError("Antigravity transport options are unsupported")
	}
	system, items, err := convertInputs(req.InputItems)
	if err != nil {
		return nil, err
	}
	if system != nil {
		return nil, protocolError("continuation cannot replace system instructions")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.runs[req.RunID]
	if state == nil || state.cancel != nil || state.pending != nil || !matchesReference(state, req.ProviderSessionRef) {
		return nil, protocolError("run cannot continue with this context")
	}
	state.contents = append(state.contents, items...)
	return p.executeLocked(ctx, req.RunID, state), nil
}

func (p *Provider) SubmitToolResult(ctx context.Context, req transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if len(req.TransportOptions) > 0 || req.TransportOptionsJSON != "" {
		return nil, protocolError("Antigravity transport options are unsupported")
	}
	result, err := strictJSON([]byte(req.ToolResultJSON))
	if err != nil {
		return nil, protocolError("invalid function result")
	}
	if _, ok := result.(map[string]any); !ok {
		result = map[string]any{"result": result}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.runs[req.RunID]
	if state == nil || state.cancel != nil || state.pending == nil || state.pending.ToolCallRef != req.ToolCallRef || !matchesReference(state, req.ProviderSessionRef) {
		return nil, protocolError("tool result does not match the pending Butler proposal")
	}
	encoded, _ := json.Marshal(result)
	response := map[string]any{"name": state.pendingNative.Name, "response": json.RawMessage(encoded)}
	if state.pendingNative.ID != "" {
		response["id"] = state.pendingNative.ID
	}
	state.contents = append(state.contents, content{Role: "user", Parts: []json.RawMessage{marshalPart(map[string]any{"functionResponse": response})}})
	state.pending = nil
	return p.executeLocked(ctx, req.RunID, state), nil
}

func (p *Provider) CancelRun(_ context.Context, req transport.CancelRunRequest) (*transport.TransportEvent, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if state := p.runs[req.RunID]; state != nil {
		if !matchesReference(state, req.ProviderSessionRef) {
			return nil, protocolError("foreign provider context")
		}
		if state.cancel != nil {
			state.cancel()
		}
		delete(p.runs, req.RunID)
	} else if req.ProviderSessionRef != nil {
		return nil, protocolError("provider context is lost")
	}
	event := transport.NewTerminalEvent(req.RunID, transport.EventTypeRunCancelled, Name)
	return &event, nil
}

func (p *Provider) executeLocked(ctx context.Context, runID string, state *runState) transport.EventStream {
	runCtx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	state.cancel = cancel
	state.turn++
	turn := state.turn
	request := generateRequest{Contents: append([]content(nil), state.contents...), SystemInstruction: state.system, GenerationConfig: generationConfig{MaxOutputTokens: maxOutputTokens}}
	if len(state.tools) > 0 {
		request.Tools = []functionTools{{FunctionDeclarations: state.tools}}
		request.ToolConfig = map[string]any{"functionCallingConfig": map[string]any{"mode": "AUTO"}}
	}
	out := make(chan transport.TransportEvent, 8)
	go func() {
		defer close(out)
		defer cancel()
		send := func(event transport.TransportEvent) bool {
			select {
			case out <- event:
				return true
			case <-runCtx.Done():
				return false
			}
		}
		ref := state.ref // Event consumers cannot mutate the provider's private identity.
		if !send(transport.NewRunStartedEvent(runID, Name, capabilities, &ref)) {
			p.discard(runID, state)
			return
		}
		answer, err := p.invoke(runCtx, state.model, request, func(delta string) bool {
			return send(transport.NewAssistantDeltaEvent(runID, Name, transport.AssistantDelta{DeltaType: "text", Content: delta, SequenceNo: turn}))
		})
		p.mu.Lock()
		if p.runs[runID] != state {
			p.mu.Unlock()
			return
		}
		state.cancel = nil
		if err != nil {
			delete(p.runs, runID)
			p.mu.Unlock()
			// Cancellation may already have closed runCtx. Deliver a buffered safe
			// terminal error when possible, without blocking an abandoned consumer.
			select {
			case out <- transport.NewTransportErrorEvent(runID, Name, err):
			default:
			}
			return
		}
		call, err := validateFunctionCalls(answer.Parts, state.definitions)
		if err != nil {
			delete(p.runs, runID)
			p.mu.Unlock()
			send(transport.NewTransportErrorEvent(runID, Name, err))
			return
		}
		if call != nil {
			definition := state.definitions[call.Name]
			proposal := transport.ToolCallRequest{ToolCallRef: fmt.Sprintf("agy-%s-%d", state.ref.SessionRef, turn), ToolName: definition.Name, ArgsJSON: string(call.Args), SequenceNo: turn}
			state.contents = append(state.contents, content{Role: "model", Parts: answer.Parts})
			state.pending = &proposal
			state.pendingNative = *call
			p.mu.Unlock()
			if !send(transport.NewToolCallRequestedEvent(runID, Name, proposal)) {
				p.discard(runID, state)
			}
			return
		}
		delete(p.runs, runID)
		p.mu.Unlock()
		if answer.Text == "" {
			send(transport.NewTransportErrorEvent(runID, Name, protocolError("model returned no usable text or function")))
			return
		}
		send(transport.NewAssistantFinalEvent(runID, Name, transport.AssistantFinal{Content: answer.Text, FinishReason: answer.FinishReason, Usage: answer.Usage}))
		send(transport.NewTerminalEvent(runID, transport.EventTypeRunCompleted, Name))
	}()
	return out
}

func (p *Provider) discard(runID string, state *runState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.runs[runID] == state {
		delete(p.runs, runID)
	}
}
func newReference() (transport.ProviderSessionRef, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return transport.ProviderSessionRef{}, protocolError("cannot create isolated provider context")
	}
	now := time.Now().UTC()
	return transport.ProviderSessionRef{ProviderName: Name, SessionRef: hex.EncodeToString(raw[:]), CreatedAt: now, LastUsedAt: now}, nil
}
func matchesReference(state *runState, ref *transport.ProviderSessionRef) bool {
	return ref == nil || (ref.ProviderName == Name && ref.SessionRef == state.ref.SessionRef && ref.ResponseRef == "")
}
func protocolError(message string) error {
	return providerError(transport.ErrorTypeProviderProtocolError, message)
}
func providerError(kind transport.ErrorType, message string) error {
	return &transport.Error{Type: kind, ProviderName: Name, Message: message, Retryable: false}
}
