package antigravity

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/butler/butler/internal/transport"
)

type content struct {
	Role  string            `json:"role,omitempty"`
	Parts []json.RawMessage `json:"parts"`
}
type functionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parametersJsonSchema"`
}
type functionTools struct {
	FunctionDeclarations []functionDeclaration `json:"functionDeclarations"`
}
type functionCall struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}
type generateRequest struct {
	Contents          []content        `json:"contents"`
	SystemInstruction *content         `json:"systemInstruction,omitempty"`
	Tools             []functionTools  `json:"tools,omitempty"`
	ToolConfig        map[string]any   `json:"toolConfig,omitempty"`
	GenerationConfig  generationConfig `json:"generationConfig"`
}
type generationConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens"`
}
type aicodeInference struct {
	Experience string `json:"experience"`
}
type requestEnvelope struct {
	AICode aicodeInference `json:"aicode"`
	generateRequest
}
type responseEnvelope struct {
	Candidates []struct {
		Index        int     `json:"index"`
		Content      content `json:"content"`
		FinishReason string  `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
	ModelVersion                    string            `json:"modelVersion"`
	AutomaticFunctionCallingHistory []json.RawMessage `json:"automaticFunctionCallingHistory"`
	Error                           json.RawMessage   `json:"error"`
}
type modelAnswer struct {
	Parts        []json.RawMessage
	Text         string
	FinishReason string
	Usage        *transport.Usage
}

func (p *Provider) invoke(ctx context.Context, model string, body generateRequest, onText func(string) bool) (modelAnswer, error) {
	auth, err := p.config.AuthSource.ResolveAntigravity(ctx)
	if err != nil {
		return modelAnswer{}, providerError(transport.ErrorTypeProviderUnavailable, "Antigravity authentication is unavailable; reconnect through the private control plane")
	}
	if strings.TrimSpace(auth.AccessToken) == "" || strings.ContainsAny(auth.AccessToken, "\r\n") || !auth.ExpiresAt.After(time.Now()) {
		return modelAnswer{}, providerError(transport.ErrorTypeProviderUnavailable, "Antigravity authentication is incomplete or expired")
	}
	// AICode and model are a protobuf oneof. Subscription inference uses only
	// aicode.experience. Cloud Code wrappers are a separate protocol. Optional
	// CLI session/entitlement fields remain omitted after earlier v1 rejections;
	// an offline CLI capture is not proof of production field acceptance.
	envelope := requestEnvelope{AICode: aicodeInference{Experience: model}, generateRequest: body}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return modelAnswer{}, protocolError("invalid model request")
	}
	if err = transport.CheckSerializedInput(ctx, len(encoded)); err != nil {
		return modelAnswer{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+":streamGenerateContent?alt=sse", bytes.NewReader(encoded))
	if err != nil {
		return modelAnswer{}, protocolError("cannot build model request")
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "antigravity/1.3.3 butler/0.1")
	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return modelAnswer{}, ctx.Err()
		}
		return modelAnswer{}, providerError(transport.ErrorTypeTransportConnectionError, "Antigravity HTTP connection failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return modelAnswer{}, statusError(resp.StatusCode, resp.Body)
	}
	if !strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return modelAnswer{}, protocolError("Antigravity returned a non-SSE response")
	}
	return readStream(resp.Body, onText)
}

func statusError(status int, body io.Reader) error {
	switch status {
	case http.StatusTooManyRequests:
		// Only a fixed reason changes the safe diagnostic. Raw error messages,
		// metadata and account identifiers never leave this transport boundary.
		if hasHighDemandReason(body) {
			return providerError(transport.ErrorTypeModelCapacityUnavailable, "Antigravity model capacity is temporarily unavailable (HTTP 429)")
		}
		return providerError(transport.ErrorTypeRateLimited, "Antigravity upstream rejected the request (HTTP 429)")
	case http.StatusUnauthorized, http.StatusForbidden:
		return providerError(transport.ErrorTypeProviderUnavailable, "Antigravity account access was rejected; reconnect through the private control plane")
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return providerError(transport.ErrorTypeProviderTimeout, "Antigravity request timed out")
	default:
		if status >= 500 {
			return providerError(transport.ErrorTypeProviderUnavailable, "Antigravity backend is unavailable")
		}
		return protocolError("Antigravity rejected the model request")
	}
}

func hasHighDemandReason(body io.Reader) bool {
	const limit = 64 * 1024
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil || len(data) > limit {
		return false
	}
	var envelope struct {
		Error struct {
			Details []struct {
				Reason string `json:"reason"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return false
	}
	for _, detail := range envelope.Error.Details {
		if detail.Reason == "HIGH_DEMAND" {
			return true
		}
	}
	return false
}

// Decode complete SSE frames before exposing any function. Incomplete streams
// never yield a tool proposal, even if a function appeared before the disconnect.
func readStream(reader io.Reader, onText func(string) bool) (modelAnswer, error) {
	limited := &io.LimitedReader{R: reader, N: 16*1024*1024 + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var answer modelAnswer
	var frame strings.Builder
	finished := false
	flush := func() error {
		if frame.Len() == 0 {
			return nil
		}
		data := frame.String()
		frame.Reset()
		if data == "[DONE]" {
			if !finished {
				return protocolError("stream ended before a terminal candidate")
			}
			return nil
		}
		if _, err := strictJSON([]byte(data)); err != nil {
			return protocolError("invalid SSE response JSON")
		}
		var envelope responseEnvelope
		if json.Unmarshal([]byte(data), &envelope) != nil || (len(envelope.Error) > 0 && string(envelope.Error) != "null") {
			return protocolError("invalid Antigravity response envelope")
		}
		response := envelope
		if len(response.AutomaticFunctionCallingHistory) > 0 {
			return protocolError("provider attempted automatic function execution")
		}
		if len(response.Candidates) > 1 {
			return protocolError("multiple candidates are unsupported")
		}
		for _, candidate := range response.Candidates {
			if candidate.Index != 0 || (candidate.Content.Role != "" && candidate.Content.Role != "model") || finished {
				return protocolError("unexpected candidate after completion")
			}
			for _, part := range candidate.Content.Parts {
				text, err := validatePart(part)
				if err != nil {
					return err
				}
				answer.Parts = append(answer.Parts, append(json.RawMessage(nil), part...))
				if text != "" {
					answer.Text += text
					if !onText(text) {
						return context.Canceled
					}
				}
			}
			if candidate.FinishReason != "" {
				if candidate.FinishReason != "STOP" && candidate.FinishReason != "MAX_TOKENS" {
					return protocolError("model response was blocked or failed")
				}
				answer.FinishReason = candidate.FinishReason
				finished = true
			}
		}
		u := response.UsageMetadata
		if u.PromptTokenCount < 0 || u.CandidatesTokenCount < 0 || u.TotalTokenCount < 0 {
			return protocolError("invalid usage metadata")
		}
		if u.TotalTokenCount > 0 {
			answer.Usage = &transport.Usage{InputTokens: u.PromptTokenCount, OutputTokens: u.CandidatesTokenCount, TotalTokens: u.TotalTokenCount}
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return modelAnswer{}, err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if frame.Len() > 0 {
				frame.WriteByte('\n')
			}
			frame.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			if frame.Len() > 1024*1024 {
				return modelAnswer{}, protocolError("SSE frame exceeds limit")
			}
		}
	}
	if scanner.Err() != nil || limited.N <= 0 || frame.Len() != 0 || !finished {
		return modelAnswer{}, protocolError("Antigravity stream ended without a complete terminal response")
	}
	if answer.FinishReason == "MAX_TOKENS" {
		for _, raw := range answer.Parts {
			var part map[string]json.RawMessage
			_ = json.Unmarshal(raw, &part)
			if _, ok := part["functionCall"]; ok {
				return modelAnswer{}, protocolError("truncated function proposal")
			}
		}
	}
	return answer, nil
}

func validatePart(raw json.RawMessage) (string, error) {
	value, err := strictJSON(raw)
	if err != nil {
		return "", protocolError("invalid model content part")
	}
	part, ok := value.(map[string]any)
	if !ok {
		return "", protocolError("invalid model content part")
	}
	for key := range part {
		switch key {
		case "text", "thought", "thoughtSignature", "functionCall":
		default:
			return "", protocolError("unsupported model content part")
		}
	}
	if thought, ok := part["thought"]; ok {
		if _, valid := thought.(bool); !valid {
			return "", protocolError("invalid thought marker")
		}
	}
	if signature, ok := part["thoughtSignature"]; ok {
		if _, valid := signature.(string); !valid {
			return "", protocolError("invalid thought signature")
		}
	}
	if _, function := part["functionCall"]; function {
		if _, text := part["text"]; text {
			return "", protocolError("ambiguous function content")
		}
		return "", nil
	}
	text, ok := part["text"].(string)
	if !ok {
		return "", protocolError("model content is missing text or function")
	}
	if thought, _ := part["thought"].(bool); thought {
		return "", nil
	}
	return text, nil
}

func validateFunctionCalls(parts []json.RawMessage, definitions map[string]transport.ToolDefinition) (*functionCall, error) {
	var call *functionCall
	for _, raw := range parts {
		var part map[string]json.RawMessage
		_ = json.Unmarshal(raw, &part)
		encoded, ok := part["functionCall"]
		if !ok {
			continue
		}
		if call != nil {
			return nil, providerError(transport.ErrorTypeCapabilityMismatch, "parallel function proposals are unsupported")
		}
		decoder := json.NewDecoder(bytes.NewReader(encoded))
		decoder.DisallowUnknownFields()
		var proposed functionCall
		if decoder.Decode(&proposed) != nil {
			return nil, providerError(transport.ErrorTypeInvalidToolRequest, "invalid native function proposal")
		}
		definition, ok := definitions[proposed.Name]
		if !ok {
			return nil, providerError(transport.ErrorTypeInvalidToolRequest, "model proposed a function outside the Butler catalog")
		}
		if err := validateArguments(definition.SchemaJSON, proposed.Args); err != nil {
			return nil, providerError(transport.ErrorTypeInvalidToolRequest, "model function arguments violate the supplied schema")
		}
		call = &proposed
	}
	return call, nil
}

func convertInputs(inputs []transport.InputItem) (*content, []content, error) {
	var system *content
	contents := make([]content, 0, len(inputs))
	for _, item := range inputs {
		part := marshalPart(map[string]any{"text": item.Content})
		switch item.Role {
		case "system", "developer":
			if system == nil {
				system = &content{Parts: []json.RawMessage{}}
			}
			system.Parts = append(system.Parts, part)
		case "assistant":
			contents = append(contents, content{Role: "model", Parts: []json.RawMessage{part}})
		case "user":
			contents = append(contents, content{Role: "user", Parts: []json.RawMessage{part}})
		case "tool": // Durable Butler history is data, not provider-generated authority.
			contents = append(contents, content{Role: "user", Parts: []json.RawMessage{marshalPart(map[string]any{"text": "Recorded Butler tool result (" + item.Name + "): " + item.Content})}})
		default:
			return nil, nil, protocolError("unsupported Butler input role")
		}
	}
	if len(contents) == 0 {
		return nil, nil, protocolError("model request requires conversational input")
	}
	return system, contents, nil
}
func convertTools(tools []transport.ToolDefinition) ([]functionDeclaration, map[string]transport.ToolDefinition, error) {
	declarations := make([]functionDeclaration, 0, len(tools))
	definitions := make(map[string]transport.ToolDefinition, len(tools))
	names := make(map[string]bool, len(tools))
	for i, tool := range tools {
		if tool.Name == "" || names[tool.Name] {
			return nil, nil, protocolError("duplicate or empty Butler tool name")
		}
		names[tool.Name] = true
		if err := validateSchema(tool.SchemaJSON); err != nil {
			return nil, nil, protocolError("unsupported Butler tool schema")
		}
		alias := fmt.Sprintf("butler_tool_%d", i)
		definitions[alias] = tool
		declarations = append(declarations, functionDeclaration{Name: alias, Description: tool.Name + ": " + tool.Description, Parameters: json.RawMessage(tool.SchemaJSON)})
	}
	return declarations, definitions, nil
}
func marshalPart(value any) json.RawMessage { encoded, _ := json.Marshal(value); return encoded }
