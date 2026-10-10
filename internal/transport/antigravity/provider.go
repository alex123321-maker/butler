// Package antigravity normalizes the official Google Antigravity CLI's
// schema-constrained output. The CLI never receives executable Butler tools.
package antigravity

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/butler/butler/internal/transport"
)

const Name = "antigravity"

//go:embed agent.md
var agentDefinition []byte

type Config struct {
	Model         string
	BinaryPath    string
	ProfileDir    string
	WorkspaceRoot string
	Timeout       time.Duration
}

type Provider struct {
	config Config
	mu     sync.Mutex
	runs   map[string]*runState
}

type runState struct {
	model   string
	inputs  []transport.InputItem
	tools   []transport.ToolDefinition
	pending *transport.ToolCallRequest
	turn    int
	cancel  context.CancelFunc
}

type decision struct {
	Kind      string          `json:"kind"`
	Answer    string          `json:"answer"`
	ToolName  string          `json:"tool_name"`
	Arguments json.RawMessage `json:"arguments"`
}

type cliEvent struct {
	Event string `json:"event"`
	Init  *struct {
		Tools []string `json:"tools"`
		Agent string   `json:"agent"`
	} `json:"init"`
	Step *struct {
		StepType     string          `json:"step_type"`
		SubagentInfo json.RawMessage `json:"subagent_info"`
	} `json:"step_update"`
	Result *struct {
		Status           string          `json:"status"`
		Error            string          `json:"error"`
		StructuredOutput json.RawMessage `json:"structured_output"`
		Usage            struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	} `json:"result"`
}

var capabilities = transport.CapabilitySnapshot{SupportsToolCalls: true, SupportsCancel: true, SupportsUsageMetadata: true}

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
		cfg.Model = "gemini-3.8-flash-tiered"
	}
	if cfg.BinaryPath == "" {
		cfg.BinaryPath = "agy"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if cfg.WorkspaceRoot == "" {
		cfg.WorkspaceRoot = filepath.Join(os.TempDir(), "butler-antigravity")
	}
	if cfg.ProfileDir == "" {
		cfg.ProfileDir = "/home/butler"
	}
	return &Provider{config: cfg, runs: make(map[string]*runState)}, nil
}

func (p *Provider) Name() string { return Name }
func (p *Provider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return capabilities, nil
}

func (p *Provider) StartRun(ctx context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	if _, exists := p.runs[req.Context.RunID]; exists {
		p.mu.Unlock()
		return nil, protocolError("run is already active")
	}
	model := req.Context.ModelName
	if model == "" {
		model = p.config.Model
	}
	state := &runState{model: model, inputs: append([]transport.InputItem(nil), req.InputItems...), tools: append([]transport.ToolDefinition(nil), req.ToolDefinitions...)}
	p.runs[req.Context.RunID] = state
	p.mu.Unlock()
	return p.execute(ctx, req.Context.RunID)
}

func (p *Provider) ContinueRun(ctx context.Context, req transport.ContinueRunRequest) (transport.EventStream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	state := p.runs[req.RunID]
	if state == nil || state.cancel != nil || state.pending != nil {
		p.mu.Unlock()
		return nil, protocolError("run cannot continue")
	}
	state.inputs = append(state.inputs, req.InputItems...)
	p.mu.Unlock()
	return p.execute(ctx, req.RunID)
}

func (p *Provider) SubmitToolResult(ctx context.Context, req transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	state := p.runs[req.RunID]
	if state == nil || state.cancel != nil || state.pending == nil || state.pending.ToolCallRef != req.ToolCallRef {
		p.mu.Unlock()
		return nil, protocolError("tool result does not match the pending proposal")
	}
	state.inputs = append(state.inputs, transport.InputItem{Role: "tool", Name: state.pending.ToolName, Content: req.ToolResultJSON, ContentType: "application/json"})
	state.pending = nil
	p.mu.Unlock()
	return p.execute(ctx, req.RunID)
}

func (p *Provider) CancelRun(_ context.Context, req transport.CancelRunRequest) (*transport.TransportEvent, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	if state := p.runs[req.RunID]; state != nil {
		if state.cancel != nil {
			state.cancel()
		}
		delete(p.runs, req.RunID)
	}
	p.mu.Unlock()
	event := transport.NewTerminalEvent(req.RunID, transport.EventTypeRunCancelled, Name)
	return &event, nil
}

func (p *Provider) execute(ctx context.Context, runID string) (transport.EventStream, error) {
	p.mu.Lock()
	state := p.runs[runID]
	if state == nil || state.cancel != nil {
		p.mu.Unlock()
		return nil, protocolError("run is absent or already executing")
	}
	runCtx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	state.cancel = cancel
	state.turn++
	model, turn := state.model, state.turn
	inputs, tools := append([]transport.InputItem(nil), state.inputs...), append([]transport.ToolDefinition(nil), state.tools...)
	p.mu.Unlock()
	out := make(chan transport.TransportEvent, 4)
	go func() {
		defer close(out)
		defer cancel()
		out <- transport.NewRunStartedEvent(runID, Name, capabilities, nil)
		choice, usage, err := p.invoke(runCtx, model, inputs, tools)
		p.mu.Lock()
		current := p.runs[runID]
		if current != state {
			p.mu.Unlock()
			return
		}
		state.cancel = nil
		if err != nil {
			delete(p.runs, runID)
			p.mu.Unlock()
			out <- transport.NewTransportErrorEvent(runID, Name, err)
			return
		}
		if choice.Kind == "tool_call" {
			call := transport.ToolCallRequest{ToolCallRef: fmt.Sprintf("agy-%s-%d", runID, turn), ToolName: choice.ToolName, ArgsJSON: string(choice.Arguments), SequenceNo: turn}
			state.pending = &call
			encoded, _ := json.Marshal(choice)
			state.inputs = append(state.inputs, transport.InputItem{Role: "assistant", Content: string(encoded), ContentType: "application/json"})
			p.mu.Unlock()
			out <- transport.NewToolCallRequestedEvent(runID, Name, call)
			return
		}
		delete(p.runs, runID)
		p.mu.Unlock()
		out <- transport.NewAssistantFinalEvent(runID, Name, transport.AssistantFinal{Content: choice.Answer, FinishReason: "stop", Usage: usage})
		out <- transport.NewTerminalEvent(runID, transport.EventTypeRunCompleted, Name)
	}()
	return out, nil
}

func (p *Provider) invoke(ctx context.Context, model string, inputs []transport.InputItem, tools []transport.ToolDefinition) (decision, *transport.Usage, error) {
	if err := validateProfile(p.config.ProfileDir); err != nil {
		return decision{}, nil, err
	}
	if err := os.MkdirAll(p.config.WorkspaceRoot, 0700); err != nil {
		return decision{}, nil, protocolError("cannot create isolated CLI workspace")
	}
	workspace, err := os.MkdirTemp(p.config.WorkspaceRoot, "turn-")
	if err != nil {
		return decision{}, nil, protocolError("cannot create isolated CLI workspace")
	}
	defer os.RemoveAll(workspace)
	agentDir := filepath.Join(workspace, ".agents", "agents")
	if err := os.MkdirAll(agentDir, 0700); err != nil {
		return decision{}, nil, protocolError("cannot create model-only agent")
	}
	if err := os.WriteFile(filepath.Join(agentDir, "butler-model.md"), agentDefinition, 0400); err != nil {
		return decision{}, nil, protocolError("cannot create model-only agent")
	}
	envelope, err := json.Marshal(struct {
		Messages []transport.InputItem      `json:"messages"`
		Tools    []transport.ToolDefinition `json:"tool_definitions"`
	}{inputs, tools})
	if err != nil {
		return decision{}, nil, protocolError("invalid model input")
	}
	message, _ := json.Marshal(map[string]any{"event": "user", "message": map[string]string{"content": string(envelope)}})
	cmd := exec.CommandContext(ctx, p.config.BinaryPath, "--agent", "butler-model", "--model", model, "--input-format", "stream-json", "--output-format", "stream-json", "--json-schema", decisionSchema(tools), "--disable-slash-commands", "--print-timeout", p.config.Timeout.String())
	cmd.Dir = workspace
	cmd.Env = cliEnvironment()
	cmd.Stdin = bytes.NewReader(append(message, '\n'))
	cmd.Stderr = io.Discard // CLI diagnostics can contain credential/account material.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return decision{}, nil, protocolError("cannot open CLI stream")
	}
	if err := cmd.Start(); err != nil {
		return decision{}, nil, &transport.Error{Type: transport.ErrorTypeProviderUnavailable, ProviderName: Name, Message: "Antigravity CLI could not start; check its installed binary"}
	}
	choice, usage, streamErr := readResult(stdout, tools)
	if streamErr != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return decision{}, nil, ctx.Err()
	}
	if streamErr != nil {
		return decision{}, nil, streamErr
	}
	if waitErr != nil {
		return decision{}, nil, protocolError("Antigravity CLI exited unsuccessfully")
	}
	return choice, usage, nil
}

func readResult(reader io.Reader, tools []transport.ToolDefinition) (decision, *transport.Usage, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 2*1024*1024)
	initialized := false
	var result *cliEvent
	for scanner.Scan() {
		var event cliEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return decision{}, nil, protocolError("invalid CLI event")
		}
		switch event.Event {
		case "init":
			if initialized || event.Init == nil || event.Init.Tools == nil {
				return decision{}, nil, protocolError("CLI did not establish the model-only agent with an empty toolkit")
			}
			if len(event.Init.Tools) != 0 {
				return decision{}, nil, protocolError("CLI advertised executable tools instead of an empty toolkit")
			}
			if event.Init.Agent != "butler-model" {
				return decision{}, nil, protocolError("CLI did not report the expected model-only agent identity")
			}
			initialized = true
		case "step_update":
			if !initialized || event.Step == nil || event.Step.StepType == "tool" || len(event.Step.SubagentInfo) != 0 {
				return decision{}, nil, protocolError("CLI attempted an unsupported autonomous step")
			}
		case "result":
			if event.Result == nil || result != nil {
				return decision{}, nil, protocolError("invalid terminal CLI event")
			}
			if event.Result.Status != "SUCCESS" {
				return decision{}, nil, cliFailure(event.Result.Error)
			}
			if !initialized {
				return decision{}, nil, protocolError("CLI result arrived before isolation confirmation")
			}
			copyEvent := event
			result = &copyEvent
		default:
			return decision{}, nil, protocolError("unsupported CLI event")
		}
	}
	if scanner.Err() != nil || result == nil {
		return decision{}, nil, protocolError("CLI stream ended without a complete result")
	}
	var choice decision
	decoder := json.NewDecoder(bytes.NewReader(result.Result.StructuredOutput))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&choice); err != nil {
		return decision{}, nil, protocolError("invalid structured model decision")
	}
	if err := validateDecision(choice, tools); err != nil {
		return decision{}, nil, err
	}
	u := result.Result.Usage
	return choice, &transport.Usage{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, TotalTokens: u.TotalTokens}, nil
}

func validateDecision(choice decision, tools []transport.ToolDefinition) error {
	switch choice.Kind {
	case "answer":
		var args map[string]json.RawMessage
		if strings.TrimSpace(choice.Answer) == "" || choice.ToolName != "" || json.Unmarshal(choice.Arguments, &args) != nil || args == nil || len(args) != 0 {
			return protocolError("invalid answer decision")
		}
	case "tool_call":
		if choice.Answer != "" || !json.Valid(choice.Arguments) || len(bytes.TrimSpace(choice.Arguments)) == 0 || bytes.TrimSpace(choice.Arguments)[0] != '{' {
			return protocolError("invalid tool proposal")
		}
		for _, tool := range tools {
			if tool.Name == choice.ToolName {
				return nil
			}
		}
		return &transport.Error{Type: transport.ErrorTypeInvalidToolRequest, ProviderName: Name, Message: "Antigravity proposed a tool outside the supplied tool definitions"}
	default:
		return protocolError("unknown model decision")
	}
	return nil
}

func decisionSchema(tools []transport.ToolDefinition) string {
	kinds := []string{"answer"}
	if len(tools) > 0 {
		kinds = append(kinds, "tool_call")
	}
	encoded, _ := json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "required": []string{"kind", "answer", "tool_name", "arguments"}, "properties": map[string]any{
		"kind": map[string]any{"type": "string", "enum": kinds}, "answer": map[string]any{"type": "string"}, "tool_name": map[string]any{"type": "string"}, "arguments": map[string]any{"type": "object"},
	}})
	return string(encoded)
}

func cliEnvironment() []string {
	env := []string{"AGY_CLI_DISABLE_AUTO_UPDATE=true"}
	for _, key := range []string{"HOME", "PATH", "LANG", "LC_ALL", "TMPDIR", "SSH_CONNECTION", "SSL_CERT_FILE", "SSL_CERT_DIR"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func cliFailure(raw string) error {
	message := strings.ToLower(raw)
	if strings.Contains(message, "auth") {
		return &transport.Error{Type: transport.ErrorTypeProviderUnavailable, ProviderName: Name, ProviderCode: "authentication_required", Message: "Antigravity requires Google account sign-in in its private CLI profile"}
	}
	if strings.Contains(message, "quota") || strings.Contains(message, "rate limit") {
		return &transport.Error{Type: transport.ErrorTypeRateLimited, ProviderName: Name, Message: "Antigravity subscription quota is unavailable", Retryable: false}
	}
	return protocolError("Antigravity CLI failed; review diagnostics outside Butler chat")
}

func protocolError(message string) error {
	return &transport.Error{Type: transport.ErrorTypeProviderProtocolError, ProviderName: Name, Message: message}
}
