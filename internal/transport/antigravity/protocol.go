package antigravity

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/butler/butler/internal/providerauth"
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
	SessionID         string           `json:"sessionId,omitempty"`
	Contents          []content        `json:"contents"`
	SystemInstruction *content         `json:"systemInstruction,omitempty"`
	Tools             []functionTools  `json:"tools,omitempty"`
	ToolConfig        map[string]any   `json:"toolConfig,omitempty"`
	GenerationConfig  generationConfig `json:"generationConfig"`
}
type generationConfig struct {
	MaxOutputTokens    int             `json:"maxOutputTokens"`
	ThinkingConfig     *thinkingConfig `json:"thinkingConfig,omitempty"`
	ResponseMIMEType   string          `json:"responseMimeType,omitempty"`
	ResponseJSONSchema json.RawMessage `json:"responseJsonSchema,omitempty"`
	ResponseSchema     json.RawMessage `json:"responseSchema,omitempty"`
}
type thinkingConfig struct {
	ThinkingBudget int    `json:"thinkingBudget,omitempty"`
	ThinkingLevel  string `json:"thinkingLevel,omitempty"`
}
type cloudRequestEnvelope struct {
	Model       string          `json:"model"`
	Project     string          `json:"project"`
	Request     generateRequest `json:"request"`
	RequestID   string          `json:"requestId"`
	RequestType string          `json:"requestType"`
	UserAgent   string          `json:"userAgent"`
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
	UsageMetadata                   *nativeUsage      `json:"usageMetadata"`
	ModelVersion                    string            `json:"modelVersion"`
	AutomaticFunctionCallingHistory []json.RawMessage `json:"automaticFunctionCallingHistory"`
	Error                           json.RawMessage   `json:"error"`
}
type modelAnswer struct {
	Parts        []json.RawMessage
	Text         string
	FinishReason string
	Usage        *transport.Usage
	ModelVersion *string
}

var nativeModelVersion = regexp.MustCompile(`^gemini-[a-zA-Z0-9][a-zA-Z0-9._-]{0,110}$`)

type nativeUsage struct {
	PromptTokenCount, CandidatesTokenCount, TotalTokenCount *int
	ThoughtsTokenCount                                      *int
}

// Count names are exact. Other provider usage dimensions may be ignored, but
// encoding/json case/Unicode aliases must not override a measured count.
func (u *nativeUsage) UnmarshalJSON(data []byte) error {
	if _, err := strictJSON(data); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return fmt.Errorf("invalid usage metadata")
	}
	counts := map[string]**int{"promptTokenCount": &u.PromptTokenCount, "candidatesTokenCount": &u.CandidatesTokenCount, "totalTokenCount": &u.TotalTokenCount, "thoughtsTokenCount": &u.ThoughtsTokenCount}
	for key, raw := range fields {
		if target, ok := counts[key]; ok {
			if err := json.Unmarshal(raw, target); err != nil {
				return err
			}
			continue
		}
		for canonical := range counts {
			if strings.EqualFold(key, canonical) {
				return fmt.Errorf("ambiguous usage count")
			}
		}
	}
	return nil
}

func (p *Provider) invoke(ctx context.Context, model, sessionID string, body generateRequest, onText func(string) bool) (modelAnswer, error) {
	if err := checkInferenceAuthority(ctx); err != nil {
		return modelAnswer{}, err
	}
	cloud := model == FlashHighModel || model == FlashLowModel
	resolve := p.config.AuthSource.ResolveAntigravity
	if cloud {
		source, ok := p.config.AuthSource.(providerauth.AntigravityCloudCodeTokenSource)
		if !ok {
			return modelAnswer{}, providerError(transport.ErrorTypeProviderUnavailable, "Antigravity subscription discovery is required for the selected Flash profile")
		}
		resolve = source.ResolveAntigravityCloudCode
	}
	transport.ObserveProviderStage(ctx, transport.ProviderStageAuthentication)
	auth, err := resolve(ctx)
	if err != nil {
		return modelAnswer{}, providerError(transport.ErrorTypeProviderUnavailable, "Antigravity authentication is unavailable; reconnect through the private control plane")
	}
	if strings.TrimSpace(auth.AccessToken) == "" || strings.ContainsAny(auth.AccessToken, "\r\n") || !auth.ExpiresAt.After(time.Now()) {
		return modelAnswer{}, providerError(transport.ErrorTypeProviderUnavailable, "Antigravity authentication is incomplete or expired")
	}
	var envelope any = requestEnvelope{AICode: aicodeInference{Experience: model}, generateRequest: body}
	url, userAgent := endpoint, "antigravity/1.3.3 butler/0.1"
	if cloud {
		if !providerauth.ValidAntigravityProject(auth.ProjectID) {
			return modelAnswer{}, providerError(transport.ErrorTypeProviderUnavailable, "Antigravity subscription project is unavailable")
		}
		requestID, err := newCloudRequestID()
		if err != nil {
			return modelAnswer{}, err
		}
		body.SessionID = sessionID
		body.GenerationConfig.ThinkingConfig = &thinkingConfig{ThinkingBudget: -1}
		if p.config.ThinkingLevel != "" {
			body.GenerationConfig.ThinkingConfig = &thinkingConfig{ThinkingLevel: p.config.ThinkingLevel}
		}
		envelope = cloudRequestEnvelope{Model: model, Project: auth.ProjectID, Request: body, RequestID: requestID, RequestType: "agent", UserAgent: "antigravity"}
		url, userAgent = cloudEndpoint, "antigravity/2.22.0 butler/0.1"
	}
	transport.ObserveProviderStage(ctx, transport.ProviderStageSerialization)
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return modelAnswer{}, protocolError("invalid model request")
	}
	if err = transport.CheckSerializedInput(ctx, len(encoded)); err != nil {
		return modelAnswer{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+":streamGenerateContent?alt=sse", bytes.NewReader(encoded))
	if err != nil {
		return modelAnswer{}, protocolError("cannot build model request")
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", userAgent)
	transport.ObserveProviderStage(ctx, transport.ProviderStageHTTP)
	if err := checkInferenceAuthority(ctx); err != nil {
		return modelAnswer{}, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		if ctx.Err() != nil {
			return modelAnswer{}, ctx.Err()
		}
		return modelAnswer{}, providerError(transport.ErrorTypeMemorySourceStale, "memory_source_stale")
	}
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
	transport.ObserveProviderStage(ctx, transport.ProviderStageStream)
	return readStreamProtocol(resp.Body, onText, cloud)
}

func checkInferenceAuthority(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return providerError(transport.ErrorTypeMemorySourceTimeout, "memory_source_check_timeout")
		}
		return providerError(transport.ErrorTypeMemorySourceStale, "memory_source_stale")
	}
	return nil
}

func statusError(status int, body io.Reader) error {
	err := statusErrorKind(status, body)
	if failure, ok := err.(*transport.Error); ok {
		failure.HTTPStatus = status
	}
	return err
}

func statusErrorKind(status int, body io.Reader) error {
	switch status {
	case http.StatusTooManyRequests:
		// Only a fixed reason changes the safe diagnostic. Raw error messages,
		// metadata and account identifiers never leave this transport boundary.
		if hasHighDemandReason(body) {
			return providerError(transport.ErrorTypeModelCapacityUnavailable, "Antigravity model capacity is temporarily unavailable (HTTP 429)")
		}
		return providerError(transport.ErrorTypeRateLimited, "Antigravity upstream rejected the request (HTTP 429)")
	case http.StatusUnauthorized, http.StatusForbidden:
		failure := providerError(transport.ErrorTypeProviderUnavailable, "Antigravity account access was rejected; inspect private control-plane diagnostics").(*transport.Error)
		failure.SafeReason = authRejectionReason(body)
		return failure
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return providerError(transport.ErrorTypeProviderTimeout, "Antigravity request timed out")
	default:
		if status >= 500 {
			return providerError(transport.ErrorTypeProviderUnavailable, "Antigravity backend is unavailable")
		}
		failure := protocolError("Antigravity rejected the model request").(*transport.Error)
		if status == http.StatusBadRequest || status == http.StatusUnprocessableEntity {
			failure.SafeReason = invalidArgumentReason(body)
		}
		return failure
	}
}

// Only exact upstream classifications cross the boundary. No message, project,
// account, metadata, field path or response bytes survive this projection.
func authRejectionReason(body io.Reader) string {
	data, err := io.ReadAll(io.LimitReader(body, 64*1024+1))
	defer clear(data)
	if err != nil || len(data) > 64*1024 {
		return ""
	}
	if _, err = strictJSON(data); err != nil {
		return ""
	}
	var envelope struct {
		Error struct {
			Status  string `json:"status"`
			Details []struct {
				Type   string `json:"@type"`
				Reason string `json:"reason"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return ""
	}
	for _, d := range envelope.Error.Details {
		if d.Type != "type.googleapis.com/google.rpc.ErrorInfo" {
			continue
		}
		switch d.Reason {
		case "ACCESS_TOKEN_EXPIRED":
			return "upstream_access_token_expired"
		case "ACCESS_TOKEN_SCOPE_INSUFFICIENT":
			return "upstream_scope_insufficient"
		case "SERVICE_DISABLED":
			return "upstream_service_disabled"
		case "BILLING_DISABLED":
			return "upstream_billing_disabled"
		case "IAM_PERMISSION_DENIED":
			return "upstream_iam_denied"
		case "LOCATION_POLICY_VIOLATED":
			return "upstream_location_policy"
		}
	}
	if envelope.Error.Status == "UNAUTHENTICATED" {
		return "upstream_unauthenticated"
	}
	if envelope.Error.Status == "PERMISSION_DENIED" {
		return "upstream_permission_denied"
	}
	return ""
}

// Like HIGH_DEMAND, this reads a bounded private envelope and exports only a
// fixed class. It never returns provider text, a field path or account metadata.
// This is an upstream diagnostic claim, not independent proof of its cause.
func invalidArgumentReason(body io.Reader) string {
	const limit = 64 * 1024
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	defer clear(data)
	if err != nil || len(data) > limit {
		return ""
	}
	data = bytes.TrimSpace(data)
	if bytes.HasPrefix(data, []byte("data:")) {
		data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte("data:")))
	}
	decoded, err := strictJSON(data)
	if err != nil {
		if json.Valid(data) {
			return ""
		}
		return "upstream_bad_request_non_json"
	}
	// Some Google SSE error responses use a singleton JSON array even when
	// the HTTP status is already400. Never export that private envelope.
	if list, ok := decoded.([]any); ok {
		if len(list) != 1 {
			return ""
		}
		data, err = json.Marshal(list[0])
		defer clear(data)
		if err != nil {
			return ""
		}
	}
	var envelope struct {
		Error struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return ""
	}
	if envelope.Error.Status == "FAILED_PRECONDITION" {
		return "upstream_failed_precondition"
	}
	if envelope.Error.Status != "INVALID_ARGUMENT" {
		return ""
	}
	message := envelope.Error.Message
	switch {
	case strings.Contains(strings.ToLower(message), "model") && (strings.Contains(strings.ToLower(message), "not found") || strings.Contains(strings.ToLower(message), "not supported") || strings.Contains(strings.ToLower(message), "invalid model")):
		return "upstream_model_rejected"
	case strings.Contains(message, "thinkingBudget") || strings.Contains(message, "thinking_budget"):
		return "upstream_thinking_rejected"
	case strings.Contains(message, "maxOutputTokens") || strings.Contains(message, "max_output_tokens"):
		return "upstream_output_limit_rejected"
	case strings.Contains(message, "systemInstruction") || strings.Contains(message, "system_instruction"):
		return "upstream_system_instruction_rejected"
	case strings.Contains(message, `Unknown name "minItems"`):
		return "unsupported_schema_min_items"
	case strings.Contains(message, `Unknown name "maxItems"`):
		return "unsupported_schema_max_items"
	case strings.Contains(message, "responseSchema") || strings.Contains(message, "response_schema"):
		return "response_schema_rejected"
	default:
		return "upstream_invalid_argument"
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
	return readStreamProtocol(reader, onText, false)
}

func readStreamProtocol(reader io.Reader, onText func(string) bool, cloud bool) (modelAnswer, error) {
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
		decoded, decodeErr := strictJSON([]byte(data))
		if decodeErr != nil {
			return protocolError("invalid SSE response JSON")
		}
		// The typed envelope decoder folds key case/Unicode. Reject aliases at
		// the usage boundary so a second field cannot replace canonical counts.
		if object, ok := decoded.(map[string]any); ok {
			usageAlias := func(fields map[string]any) bool {
				for key := range fields {
					for _, canonical := range []string{"usageMetadata", "modelVersion"} {
						if key != canonical && strings.EqualFold(key, canonical) {
							return true
						}
					}
				}
				return false
			}
			if usageAlias(object) {
				return protocolError("ambiguous usage metadata")
			}
			if cloud {
				for key, value := range object {
					if strings.EqualFold(key, "response") {
						if key != "response" {
							return protocolError("ambiguous Cloud Code response envelope")
						}
						if wrapped, ok := value.(map[string]any); ok && usageAlias(wrapped) {
							return protocolError("ambiguous usage metadata")
						}
					}
				}
			}
		}
		var envelope responseEnvelope
		if json.Unmarshal([]byte(data), &envelope) != nil || (len(envelope.Error) > 0 && string(envelope.Error) != "null") {
			return protocolError("invalid Antigravity response envelope")
		}
		response := envelope
		if cloud {
			var wrapped struct {
				Response *responseEnvelope `json:"response"`
			}
			if len(envelope.Candidates) > 0 || len(envelope.AutomaticFunctionCallingHistory) > 0 || json.Unmarshal([]byte(data), &wrapped) != nil || wrapped.Response == nil {
				return protocolError("invalid Cloud Code response envelope")
			}
			response = *wrapped.Response
			if len(response.Error) > 0 && string(response.Error) != "null" {
				return protocolError("invalid Cloud Code response envelope")
			}
		}
		if len(response.AutomaticFunctionCallingHistory) > 0 {
			return protocolError("provider attempted automatic function execution")
		}
		if response.ModelVersion != "" {
			if !nativeModelVersion.MatchString(response.ModelVersion) || answer.ModelVersion != nil && *answer.ModelVersion != response.ModelVersion {
				return protocolError("invalid or conflicting observed model version")
			}
			version := response.ModelVersion
			answer.ModelVersion = &version
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
		if u != nil {
			for _, count := range []*int{u.PromptTokenCount, u.CandidatesTokenCount, u.TotalTokenCount, u.ThoughtsTokenCount} {
				if count != nil && *count < 0 {
					return protocolError("invalid usage metadata")
				}
			}
		}
		if finished && u != nil && u.PromptTokenCount != nil && u.CandidatesTokenCount != nil && u.TotalTokenCount != nil {
			usage := &transport.Usage{InputTokens: *u.PromptTokenCount, OutputTokens: *u.CandidatesTokenCount, TotalTokens: *u.TotalTokenCount, ThinkingTokens: u.ThoughtsTokenCount}
			if usage.Validate() != nil {
				return protocolError("invalid usage metadata")
			}
			if usage.TotalTokens < usage.InputTokens+usage.OutputTokens {
				return protocolError("invalid usage metadata")
			}
			if previous := answer.Usage; previous != nil {
				if previous.InputTokens != usage.InputTokens || previous.OutputTokens != usage.OutputTokens || previous.TotalTokens != usage.TotalTokens || previous.ThinkingTokens != nil && usage.ThinkingTokens != nil && *previous.ThinkingTokens != *usage.ThinkingTokens {
					return protocolError("conflicting complete usage metadata")
				}
				if usage.ThinkingTokens == nil {
					usage.ThinkingTokens = previous.ThinkingTokens
				}
			}
			answer.Usage = usage
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
				// EOF and every candidate/trailer/metadata check have completed.
				// Rejection does not erase this request's measured cost, but its
				// model body must never accompany the failed outcome.
				return modelAnswer{Usage: answer.Usage, ModelVersion: answer.ModelVersion}, protocolError("truncated function proposal")
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
			return nil, &transport.Error{Type: transport.ErrorTypeCapabilityMismatch, ProviderName: Name, Message: "parallel function proposals are unsupported", SafeReason: "batch_tool_calls_unsupported"}
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
		schema, err := nativeToolSchema(tool.SchemaJSON)
		if err != nil {
			return nil, nil, protocolError("unsupported Butler tool schema")
		}
		alias := fmt.Sprintf("butler_tool_%d", i)
		definitions[alias] = tool
		declarations = append(declarations, functionDeclaration{Name: alias, Description: tool.Name + ": " + tool.Description, Parameters: schema})
	}
	return declarations, definitions, nil
}
func marshalPart(value any) json.RawMessage { encoded, _ := json.Marshal(value); return encoded }
