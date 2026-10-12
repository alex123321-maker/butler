// This operator diagnostic gate is separate from production model transport.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	flash         = "gemini-3.8-flash-tiered"
	cliFlash      = "gemini-3.8-flash-medium"
	marker        = "BUTLER_AGY_COMPARE_20261011"
	comparePrompt = "Ответь ровно " + marker + ". Не вызывай инструменты."
	requestLimit  = 65536
	responseLimit = 65536
	outputLimit   = 512
)

var denied = errors.New("diagnostic admission denied")

type admission interface {
	Reserve(context.Context, string) (int, error)
	Observe(context.Context, int, observation) error
}

type observation struct {
	Attempt               int    `json:"attempt"`
	Client                string `json:"client"`
	Host                  string `json:"host"`
	Path                  string `json:"path"`
	Model                 string `json:"model"`
	RequestBytes          int    `json:"request_bytes"`
	OutputCap             int    `json:"output_cap"`
	HTTPStatus            int    `json:"http_status"`
	APIStatus             string `json:"api_status"`
	Reason                string `json:"reason"`
	Finish                string `json:"finish_reason"`
	MarkerMatch           bool   `json:"marker_match"`
	InputTokens           *int64 `json:"input_tokens"`
	OutputTokens          *int64 `json:"output_tokens"`
	ThoughtTokens         *int64 `json:"thought_tokens"`
	TotalTokens           *int64 `json:"total_tokens"`
	ResponseBoundExceeded bool   `json:"response_bound_exceeded"`
	ToolProposal          bool   `json:"tool_proposal"`
	MalformedResponse     bool   `json:"malformed_response"`
	Outcome               string `json:"outcome"`
}

type gate struct {
	mu           sync.Mutex
	mode         string
	client       *http.Client
	token        string
	budget       admission
	onGeneration chan struct{}
	projections  []map[string]any
	results      []observation
}

// inspect admits zero generation writes. execute permits each client once only.
func (g *gate) generate(w http.ResponseWriter, r *http.Request, body []byte) {
	client := "unknown"
	switch ua := r.Header.Get("User-Agent"); {
	case strings.HasPrefix(ua, "antigravity/cli/1.3.3 "):
		client = "cli"
	case ua == "antigravity/1.3.3 butler/0.1":
		client = "go"
	}
	cap := outputLimit
	if g.mode == "native8192" {
		cap = 8192
	}
	normalized, projection, err := normalizeBound(r.Host, r.URL.Path, body, cap)
	projection["authorization_material_absent_from_body"] = !bytes.Contains(body, []byte(g.token))
	if bytes.Contains(body, []byte(g.token)) {
		err = denied
	}
	projection["client"] = client
	projection["content_type_json"] = r.Header.Get("Content-Type") == "application/json"
	projection["accept_sse"] = r.Header.Get("Accept") == "text/event-stream"
	projection["authorized_bearer_matches"] = r.Header.Get("Authorization") == "Bearer "+g.token
	g.mu.Lock()
	g.projections = append(g.projections, projection)
	g.mu.Unlock()
	if client == "cli" {
		select {
		case g.onGeneration <- struct{}{}:
		default:
		}
	}
	live := g.mode == "execute" || (g.mode == "native8192" && client == "go")
	if err != nil || client == "unknown" || !live || r.Method != "POST" || r.URL.RawQuery != "alt=sse" || r.Header.Get("Authorization") != "Bearer "+g.token {
		http.Error(w, "diagnostic gate closed", http.StatusForbidden)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	attempt, err := g.budget.Reserve(ctx, client)
	if err != nil {
		http.Error(w, "diagnostic allowance spent", http.StatusForbidden)
		return
	}
	model, _ := projection["model"].(string)
	o := observation{Attempt: attempt, Client: client, Host: r.Host, Path: r.URL.Path, Model: model, RequestBytes: len(normalized), OutputCap: outputLimit, Reason: "UNKNOWN", APIStatus: "UNKNOWN", Outcome: "UNKNOWN"}
	o.OutputCap = cap
	defer func() {
		// Persistence must outlive request cancellation; never refund unknown usage.
		persist, finish := context.WithTimeout(context.Background(), 5*time.Second)
		defer finish()
		if g.budget.Observe(persist, attempt, o) != nil {
			o.Outcome = "OBSERVATION_PERSIST_FAILED"
		}
		g.mu.Lock()
		g.results = append(g.results, o)
		g.mu.Unlock()
	}()
	u := "https://" + r.Host + r.URL.RequestURI()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(normalized))
	req.Header = r.Header.Clone()
	req.Header.Del("Content-Length")
	req.Header.Del("Proxy-Authorization")
	req.Header.Del("Connection")
	// Let Go negotiate and decode compression. A client-supplied Accept-Encoding
	// disables automatic decoding and would require relaying Content-Encoding.
	req.Header.Del("Accept-Encoding")
	// Prevent net/http from treating this admitted POST as replayable.
	req.Header.Del("Idempotency-Key")
	req.Header.Del("X-Idempotency-Key")
	req.Header.Set("Authorization", "Bearer "+g.token)
	resp, err := g.client.Do(req)
	if err != nil {
		o.Outcome = "CONNECTION_OR_TIMEOUT"
		http.Error(w, "upstream unavailable", 502)
		return
	}
	defer resp.Body.Close()
	o.HTTPStatus = resp.StatusCode
	data, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit+1))
	if err != nil {
		o.Outcome = "READ_OR_TIMEOUT"
		http.Error(w, "upstream incomplete", 502)
		return
	}
	if len(data) > responseLimit {
		o.ResponseBoundExceeded = true
		o.Outcome = "RESPONSE_BOUND"
		http.Error(w, "response bound", 502)
		return
	}
	projectResponse(data, &o)
	if o.MalformedResponse && resp.StatusCode == 200 {
		o.Outcome = "MALFORMED_RESPONSE"
		http.Error(w, "upstream protocol", 502)
		return
	}
	if o.ToolProposal {
		o.Outcome = "TOOL_PROPOSAL_BLOCKED"
		http.Error(w, "tool execution disabled", 502)
		return
	}
	if resp.StatusCode != 200 {
		o.Outcome = "HTTP_REJECTED"
	} else if o.Finish == "STOP" && o.MarkerMatch {
		o.Outcome = "MARKER_COMPLETE"
	} else {
		o.Outcome = "NONTERMINAL_OR_MARKER_MISMATCH"
	}
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(data)
}

func normalize(host, path string, body []byte) ([]byte, map[string]any, error) {
	return normalizeBound(host, path, body, outputLimit)
}

func normalizeBound(host, path string, body []byte, cap int) ([]byte, map[string]any, error) {
	p := map[string]any{"host": "UNKNOWN", "path": "UNKNOWN", "original_request_bytes": len(body), "output_cap": cap, "model_matches": false, "tools_absent": false, "credits_absent": false}
	if cap != 512 && cap != 8192 {
		return nil, p, denied
	}
	if len(body) > requestLimit || !uniqueJSON(body) {
		return nil, p, denied
	}
	var value map[string]any
	if json.Unmarshal(body, &value) != nil {
		return nil, p, denied
	}
	keys := []string{}
	for _, k := range []string{"model", "project", "request", "requestId", "requestType", "userAgent", "enabledCreditTypes", "aicode", "contents", "systemInstruction", "generationConfig", "sessionId", "tools", "toolConfig", "labels", "metadata", "userId", "generationId", "safetySettings", "clientMetadata"} {
		if _, ok := value[k]; ok {
			keys = append(keys, k)
		}
	}
	p["recognized_top_keys"] = keys
	p["top_key_count"] = len(value)
	publicModel, _ := value["model"].(string)
	if a, ok := value["aicode"].(map[string]any); ok {
		publicModel, _ = a["experience"].(string)
	}
	switch publicModel {
	case flash, "gemini-3.8-flash", "gemini-3.8-flash-medium", "gemini-2.5-flash", "gemini-2.0-flash", "gemini-3.1-pro-low", "gemini-3.1-pro-high", "chat-gemini-3.1-pro-low":
		p["model"] = publicModel
	}
	p["model_matches"] = publicModel == flash
	if nested, ok := value["request"].(map[string]any); ok {
		nestedKeys := []string{}
		for _, k := range []string{"contents", "systemInstruction", "generationConfig", "sessionId", "tools", "toolConfig", "enabledCreditTypes", "model", "safetySettings", "labels", "cachedContent"} {
			if _, ok := nested[k]; ok {
				nestedKeys = append(nestedKeys, k)
			}
		}
		p["recognized_request_keys"], p["request_key_count"] = nestedKeys, len(nested)
		if contents, ok := nested["contents"].([]any); ok {
			p["contents_count"] = len(contents)
			if len(contents) == 1 {
				if content, ok := contents[0].(map[string]any); ok {
					if parts, ok := content["parts"].([]any); ok && len(parts) == 1 {
						if part, ok := parts[0].(map[string]any); ok {
							text, _ := part["text"].(string)
							p["prompt_exact"], p["prompt_present"], p["prompt_bytes"] = text == comparePrompt, strings.Contains(text, comparePrompt), len(text)
						}
					}
				}
			}
		}
		if ts, ok := nested["tools"].([]any); ok {
			p["tools_count"] = len(ts)
			declarations := 0
			for _, t := range ts {
				if m, ok := t.(map[string]any); ok {
					if ds, ok := m["functionDeclarations"].([]any); ok {
						declarations += len(ds)
					}
				}
			}
			p["function_declaration_count"] = declarations
		}
	}
	request := value
	model := ""
	switch {
	case host == "aicode.googleapis.com" && path == "/v1:streamGenerateContent":
		p["host"], p["path"] = host, path
		if !knownKeys(value, "aicode", "contents", "systemInstruction", "generationConfig", "sessionId", "tools", "toolConfig", "enabledCreditTypes") {
			return nil, p, denied
		}
		if a, ok := value["aicode"].(map[string]any); ok {
			model, _ = a["experience"].(string)
		}
		if _, exists := value["model"]; exists {
			return nil, p, denied
		}
	case (host == "cloudcode-pa.googleapis.com" || host == "daily-cloudcode-pa.googleapis.com") && path == "/v1internal:streamGenerateContent":
		p["host"], p["path"] = host, path
		if !knownKeys(value, "model", "project", "request", "requestId", "requestType", "userAgent", "enabledCreditTypes") {
			return nil, p, denied
		}
		model, _ = value["model"].(string)
		var ok bool
		request, ok = value["request"].(map[string]any)
		if !ok {
			return nil, p, denied
		}
		if !knownKeys(request, "contents", "systemInstruction", "generationConfig", "sessionId", "tools", "toolConfig", "enabledCreditTypes", "labels") {
			return nil, p, denied
		}
	default:
		return nil, p, denied
	}
	expectedModel := flash
	if host != "aicode.googleapis.com" {
		expectedModel = cliFlash
	}
	p["model_matches"] = model == expectedModel
	if model == flash || model == "chat-gemini-3.1-pro-low" {
		p["model"] = model
	}
	if model != expectedModel {
		return nil, p, denied
	}
	p["session_id_present"] = request["sessionId"] != nil
	p["system_instruction_present"] = request["systemInstruction"] != nil
	p["cloud_wrapper"] = request != nil && host != "aicode.googleapis.com"
	if system, err := json.Marshal(request["systemInstruction"]); err == nil {
		p["system_instruction_bytes"] = len(system)
	}
	for _, m := range []map[string]any{value, request} {
		for _, k := range []string{"tools", "toolConfig", "enabledCreditTypes", "creditTypes", "credits", "creditConfig"} {
			if v, exists := m[k]; exists {
				if k == "tools" && host != "aicode.googleapis.com" && cliDeclarationOnly(v) {
					continue
				}
				if list, ok := v.([]any); !ok || len(list) != 0 {
					return nil, p, denied
				}
			}
		}
	}
	p["credits_absent"] = true
	ts, _ := request["tools"].([]any)
	p["tools_absent"], p["tool_execution_disabled"] = len(ts) == 0, true
	contents, ok := request["contents"].([]any)
	p["contents_count"] = len(contents)
	if !ok || len(contents) != 1 {
		return nil, p, denied
	}
	c, ok := contents[0].(map[string]any)
	if !ok || c["role"] != "user" {
		return nil, p, denied
	}
	parts, ok := c["parts"].([]any)
	if !ok || len(parts) != 1 {
		return nil, p, denied
	}
	part, ok := parts[0].(map[string]any)
	if ok {
		text, _ := part["text"].(string)
		p["prompt_exact"] = text == comparePrompt
		p["prompt_present"] = strings.Contains(text, comparePrompt)
		p["prompt_bytes"] = len(text)
	}
	text, _ := part["text"].(string)
	cliWrapper := host != "aicode.googleapis.com" && strings.Count(text, comparePrompt) == 1 && len(text) <= 2048
	if !ok || len(part) != 1 || (text != comparePrompt && !cliWrapper) {
		return nil, p, denied
	}
	gc, ok := request["generationConfig"].(map[string]any)
	if !ok {
		return nil, p, denied
	}
	if !knownKeys(gc, "maxOutputTokens", "thinkingConfig", "responseMimeType", "temperature", "topP", "topK", "candidateCount", "stopSequences") {
		return nil, p, denied
	}
	configKeys := []string{}
	for _, k := range []string{"maxOutputTokens", "thinkingConfig", "responseMimeType", "temperature", "topP", "topK", "candidateCount", "stopSequences"} {
		if _, exists := gc[k]; exists {
			configKeys = append(configKeys, k)
		}
	}
	p["generation_config_keys"] = configKeys
	if n, ok := gc["maxOutputTokens"].(float64); ok && n >= 0 && n <= 65536 && n == float64(int(n)) {
		p["original_max_output_tokens"] = int(n)
	}
	if thinking, ok := gc["thinkingConfig"].(map[string]any); ok {
		if level, ok := thinking["thinkingLevel"].(string); ok {
			switch strings.ToUpper(level) {
			case "MINIMAL", "LOW", "MEDIUM", "HIGH":
				p["thinking_level"] = strings.ToUpper(level)
			}
		}
	}
	if n, exists := gc["candidateCount"]; exists && n != float64(1) {
		return nil, p, denied
	}
	gc["maxOutputTokens"] = cap
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > requestLimit {
		return nil, p, denied
	}
	p["normalized_request_bytes"] = len(encoded)
	if cap == 8192 {
		if host != "aicode.googleapis.com" || gc["maxOutputTokens"] != 8192 || p["original_max_output_tokens"] != 8192 {
			return nil, p, denied
		}
		p["normalized_request_bytes"], p["original_bytes_preserved"] = len(body), true
		return body, p, nil
	}
	return encoded, p, nil
}

// CLI always advertises one declaration even with the tool-free custom agent.
// Preserve that wire contract; never relay a function proposal to its executor.
func cliDeclarationOnly(value any) bool {
	ts, ok := value.([]any)
	if !ok || len(ts) > 1 {
		return false
	}
	for _, item := range ts {
		m, ok := item.(map[string]any)
		if !ok || !knownKeys(m, "functionDeclarations") {
			return false
		}
		ds, ok := m["functionDeclarations"].([]any)
		if !ok || len(ds) != 1 {
			return false
		}
		d, ok := ds[0].(map[string]any)
		if !ok || !knownKeys(d, "name", "description", "parameters", "parametersJsonSchema") {
			return false
		}
	}
	return true
}

func knownKeys(m map[string]any, keys ...string) bool {
	allowed := map[string]bool{}
	for _, k := range keys {
		allowed[k] = true
	}
	for k := range m {
		if !allowed[k] {
			return false
		}
	}
	return true
}

// Duplicate keys, case aliases, trailing values and malformed JSON refuse admission.
func uniqueJSON(data []byte) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	var parse func() bool
	parse = func() bool {
		t, err := d.Token()
		if err != nil {
			return false
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					k, err := d.Token()
					s, ok := k.(string)
					if err != nil || !ok || seen[strings.ToLower(s)] {
						return false
					}
					seen[strings.ToLower(s)] = true
					if !parse() {
						return false
					}
				}
				t, err = d.Token()
				return err == nil && t == json.Delim('}')
			case '[':
				for d.More() {
					if !parse() {
						return false
					}
				}
				t, err = d.Token()
				return err == nil && t == json.Delim(']')
			default:
				return false
			}
		}
		return true
	}
	if !parse() {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}

func projectResponse(data []byte, o *observation) {
	frames := [][]byte{data}
	if !json.Valid(data) {
		frames = nil
		for _, block := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n\n") {
			lines := []string{}
			for _, line := range strings.Split(block, "\n") {
				if strings.HasPrefix(line, "data:") {
					lines = append(lines, strings.TrimSpace(line[5:]))
				}
			}
			if len(lines) > 0 {
				frames = append(frames, []byte(strings.Join(lines, "\n")))
			}
		}
	}
	text := ""
	if len(frames) == 0 {
		o.MalformedResponse = true
	}
	for _, frame := range frames {
		if strings.TrimSpace(string(frame)) == "[DONE]" {
			continue
		}
		var value map[string]any
		if json.Unmarshal(frame, &value) != nil {
			o.MalformedResponse = true
			continue
		}
		if e, ok := value["error"].(map[string]any); ok {
			if s, ok := e["status"].(string); ok {
				switch s {
				case "RESOURCE_EXHAUSTED", "INVALID_ARGUMENT", "PERMISSION_DENIED", "UNAUTHENTICATED", "UNAVAILABLE", "INTERNAL", "DEADLINE_EXCEEDED", "FAILED_PRECONDITION":
					o.APIStatus = s
				}
			}
			if details, ok := e["details"].([]any); ok {
				for _, d := range details {
					if m, ok := d.(map[string]any); ok {
						if m["reason"] == "HIGH_DEMAND" {
							o.Reason = "HIGH_DEMAND"
						} else if m["reason"] == "QUOTA_EXCEEDED" && o.Reason == "UNKNOWN" {
							o.Reason = "QUOTA_EXCEEDED"
						}
					}
				}
			}
		}
		if nested, ok := value["response"].(map[string]any); ok {
			value = nested
		}
		if usage, ok := value["usageMetadata"].(map[string]any); ok {
			for k, dst := range map[string]**int64{"promptTokenCount": &o.InputTokens, "candidatesTokenCount": &o.OutputTokens, "thoughtsTokenCount": &o.ThoughtTokens, "totalTokenCount": &o.TotalTokens} {
				if f, ok := usage[k].(float64); ok && f >= 0 && f <= 1e9 && f == float64(int64(f)) {
					n := int64(f)
					*dst = &n
				}
			}
		}
		if cs, ok := value["candidates"].([]any); ok {
			if len(cs) > 1 {
				o.MalformedResponse = true
			}
			for _, c := range cs {
				if m, ok := c.(map[string]any); ok {
					if s, ok := m["finishReason"].(string); ok {
						switch s {
						case "STOP", "MAX_TOKENS", "SAFETY", "RECITATION":
							o.Finish = s
						default:
							o.Finish = "UNKNOWN"
						}
					}
					if content, ok := m["content"].(map[string]any); ok {
						if parts, ok := content["parts"].([]any); ok {
							for _, p := range parts {
								if part, ok := p.(map[string]any); ok && part["functionCall"] != nil {
									o.ToolProposal = true
								}
								if part, ok := p.(map[string]any); ok && part["thought"] != true {
									if s, ok := part["text"].(string); ok {
										text += s
									}
								}
							}
						}
					}
				}
			}
		}
	}
	o.MarkerMatch = strings.TrimSpace(text) == marker
}
