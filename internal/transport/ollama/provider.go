// Package ollama implements a pinned, local, text-only observation transport.
// It neither executes tools nor grants local-only mandate authority by pricing.
package ollama

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

const (
	Name          = "ollama-observation"
	Model         = "qwen3:0.6b"
	ModelDigest   = "7df6b6e09427a769808717c0a93cadc4ae99ed4eb8bf5ca557c90846becea435"
	ContextTokens = 4096
	OutputTokens  = 256
)

// Config is private operator composition, never model transport options. The
// supplied service must have immutable weights, one runner, bounded CPU/RAM and
// a private network. DNS alias butler-generative is reserved for that service.
type Config struct {
	Endpoint   string
	Digest     string
	Timeout    time.Duration
	JSONOutput bool
}

type Provider struct {
	config Config
	client *http.Client
	mu     sync.Mutex
	runs   map[string]*activeRun
	slot   chan struct{}
}
type activeRun struct{ cancel context.CancelFunc }

var capabilities = transport.CapabilitySnapshot{SupportsStreaming: true, SupportsCancel: true, SupportsUsageMetadata: true}

func NewProvider(cfg Config) (*Provider, error) {
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.Port() == "" {
		return nil, fail(transport.ErrorTypeProviderUnavailable)
	}
	ip := net.ParseIP(u.Hostname())
	if (ip == nil || !ip.IsLoopback()) && (u.Hostname() != "butler-generative" || u.Port() != "11434") {
		return nil, fail(transport.ErrorTypeProviderUnavailable)
	}
	if cfg.Digest != ModelDigest || cfg.Timeout <= 0 || cfg.Timeout > 2*time.Minute {
		return nil, fail(transport.ErrorTypeProviderUnavailable)
	}
	clientTransport := http.DefaultTransport.(*http.Transport).Clone()
	clientTransport.Proxy = nil
	clientTransport.MaxResponseHeaderBytes = 16 << 10
	client := &http.Client{Transport: clientTransport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Provider{config: cfg, client: client, runs: map[string]*activeRun{}, slot: make(chan struct{}, 1)}, nil
}

func (*Provider) Name() string                    { return Name }
func (*Provider) InferenceLimits() (int64, int64) { return ContextTokens, OutputTokens }
func (*Provider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return capabilities, nil
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type chatRequest struct {
	Model     string    `json:"model"`
	Messages  []message `json:"messages"`
	Stream    bool      `json:"stream"`
	Think     bool      `json:"think"`
	KeepAlive string    `json:"keep_alive"`
	Format    string    `json:"format,omitempty"`
	Options   options   `json:"options"`
}
type options struct {
	NumCtx      int `json:"num_ctx"`
	NumPredict  int `json:"num_predict"`
	NumThread   int `json:"num_thread"`
	NumGPU      int `json:"num_gpu"`
	Temperature int `json:"temperature"`
	Seed        int `json:"seed"`
}

func (p *Provider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if r.Validate() != nil || r.Context.ProviderName != Name || r.Context.ModelName != Model || r.Context.ProviderSessionRef != nil || len(r.ToolDefinitions) != 0 || len(r.TransportOptions) != 0 || r.TransportOptionsJSON != "" {
		return nil, fail(transport.ErrorTypeCapabilityMismatch)
	}
	items, err := content.Normalize(r.InputItems)
	if err != nil {
		return nil, fail(transport.ErrorTypeProviderProtocolError)
	}
	if len(items) < 1 || len(items) > 16 {
		return nil, fail(transport.ErrorTypeInputBytesExceeded)
	}
	body := chatRequest{Model: Model, Stream: true, KeepAlive: "5m", Options: options{NumCtx: ContextTokens, NumPredict: OutputTokens, NumThread: 2, Seed: 123}}
	if p.config.JSONOutput {
		body.Format = "json"
	}
	// For this pinned byte-level tokenizer, bytes plus a conservative template
	// allowance bound prompt tokens. Refuse excess before HTTP; do not depend on
	// server-side context truncation to silently discard source evidence.
	bound := 256
	for _, item := range items {
		if !utf8.ValidString(item.Content) || (item.Role != "system" && item.Role != "user" && item.Role != "assistant") {
			return nil, fail(transport.ErrorTypeProviderProtocolError)
		}
		bound += len(item.Content) + 128
		body.Messages = append(body.Messages, message{Role: item.Role, Content: item.Content})
	}
	if bound+OutputTokens > ContextTokens {
		return nil, fail(transport.ErrorTypeInputBytesExceeded)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fail(transport.ErrorTypeProviderProtocolError)
	}
	if err = transport.CheckSerializedInput(ctx, len(encoded)); err != nil {
		return nil, err
	}
	turn, cancel := context.WithTimeout(ctx, p.config.Timeout)
	state := &activeRun{cancel: cancel}
	p.mu.Lock()
	if p.runs[r.Context.RunID] != nil || len(p.runs) >= 4 {
		p.mu.Unlock()
		cancel()
		return nil, fail(transport.ErrorTypeProviderUnavailable)
	}
	p.runs[r.Context.RunID] = state
	p.mu.Unlock()
	out := make(chan transport.TransportEvent, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer func() {
			p.mu.Lock()
			if p.runs[r.Context.RunID] == state {
				delete(p.runs, r.Context.RunID)
			}
			p.mu.Unlock()
		}()
		send := func(e transport.TransportEvent) bool {
			if turn.Err() != nil {
				return false
			}
			select {
			case out <- e:
				return true
			case <-turn.Done():
				return false
			}
		}
		emitError := func(e error) {
			select {
			case out <- transport.NewTransportErrorEvent(r.Context.RunID, Name, e):
			default:
			}
		}
		select {
		case p.slot <- struct{}{}:
		case <-turn.Done():
			emitError(fail(transport.ErrorTypeProviderTimeout))
			return
		}
		defer func() { <-p.slot }()
		if !send(transport.NewRunStartedEvent(r.Context.RunID, Name, capabilities, nil)) {
			return
		}
		answer, err := p.invoke(turn, encoded, func(delta string) bool {
			if !r.StreamingEnabled {
				return true
			}
			return send(transport.NewAssistantDeltaEvent(r.Context.RunID, Name, transport.AssistantDelta{DeltaType: "text", Content: delta, SequenceNo: 1}))
		})
		if err != nil {
			emitError(err)
			return
		}
		if !send(transport.NewAssistantFinalEvent(r.Context.RunID, Name, answer)) {
			return
		}
		send(transport.NewTerminalEvent(r.Context.RunID, transport.EventTypeRunCompleted, Name))
	}()
	return out, nil
}

func (*Provider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, fail(transport.ErrorTypeCapabilityMismatch)
}
func (*Provider) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return nil, fail(transport.ErrorTypeCapabilityMismatch)
}
func (p *Provider) CancelRun(_ context.Context, r transport.CancelRunRequest) (*transport.TransportEvent, error) {
	if r.Validate() != nil || r.ProviderSessionRef != nil {
		return nil, fail(transport.ErrorTypeStatefulSessionLost)
	}
	p.mu.Lock()
	if state := p.runs[r.RunID]; state != nil {
		state.cancel()
	}
	p.mu.Unlock()
	event := transport.NewTerminalEvent(r.RunID, transport.EventTypeRunCancelled, Name)
	return &event, nil
}

func fail(kind transport.ErrorType) error {
	return &transport.Error{Type: kind, ProviderName: Name, Message: "local observation transport refused or unavailable", Retryable: false}
}
