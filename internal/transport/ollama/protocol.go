package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/butler/butler/internal/transport"
)

func (p *Provider) request(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, p.config.Endpoint+path, bytes.NewReader(body))
	if err != nil {
		return nil, fail(transport.ErrorTypeProviderProtocolError)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fail(transport.ErrorTypeProviderTimeout)
		}
		return nil, fail(transport.ErrorTypeTransportConnectionError)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fail(transport.ErrorTypeProviderUnavailable)
	}
	return resp, nil
}

func (p *Provider) checkPin(ctx context.Context) error {
	resp, err := p.request(ctx, http.MethodGet, "/api/tags", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(raw) > 65536 || !uniqueJSON(raw) {
		return fail(transport.ErrorTypeProviderProtocolError)
	}
	var tags struct {
		Models []struct {
			Name   string `json:"name"`
			Model  string `json:"model"`
			Digest string `json:"digest"`
		} `json:"models"`
	}
	if json.Unmarshal(raw, &tags) != nil || len(tags.Models) != 1 || tags.Models[0].Name != Model || tags.Models[0].Model != Model || tags.Models[0].Digest != p.config.Digest {
		return fail(transport.ErrorTypeProviderUnavailable)
	}
	return nil
}

type frame struct {
	Model      string `json:"model"`
	Done       *bool  `json:"done"`
	DoneReason string `json:"done_reason"`
	Message    struct {
		Role      string            `json:"role"`
		Content   string            `json:"content"`
		Thinking  string            `json:"thinking"`
		ToolCalls []json.RawMessage `json:"tool_calls"`
	} `json:"message"`
	Input  *int            `json:"prompt_eval_count"`
	Output *int            `json:"eval_count"`
	Error  json.RawMessage `json:"error"`
}

func (p *Provider) invoke(ctx context.Context, body []byte, onText func(string) bool) (transport.AssistantFinal, error) {
	if err := p.checkPin(ctx); err != nil {
		return transport.AssistantFinal{}, err
	}
	resp, err := p.request(ctx, http.MethodPost, "/api/chat", body)
	if err != nil {
		return transport.AssistantFinal{}, err
	}
	defer resp.Body.Close()
	if !strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "application/x-ndjson") {
		return transport.AssistantFinal{}, fail(transport.ErrorTypeProviderProtocolError)
	}
	reader := io.LimitReader(resp.Body, (2<<20)+1)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 65536)
	var final transport.AssistantFinal
	done, total := false, 0
	for scanner.Scan() {
		raw := scanner.Bytes()
		total += len(raw) + 1
		var f frame
		if done || total > 2<<20 || !uniqueJSON(raw) || json.Unmarshal(raw, &f) != nil || f.Model != Model || f.Done == nil || len(f.Error) != 0 || len(f.Message.ToolCalls) != 0 || f.Message.Thinking != "" || f.Message.Role != "assistant" || !utf8.ValidString(f.Message.Content) {
			return transport.AssistantFinal{}, fail(transport.ErrorTypeProviderProtocolError)
		}
		if len(final.Content)+len(f.Message.Content) > 32768 {
			return transport.AssistantFinal{}, fail(transport.ErrorTypeProviderProtocolError)
		}
		final.Content += f.Message.Content
		if f.Message.Content != "" && !onText(f.Message.Content) {
			return transport.AssistantFinal{}, fail(transport.ErrorTypeProviderTimeout)
		}
		if !*f.Done {
			continue
		}
		done = true
		if f.DoneReason != "stop" && f.DoneReason != "length" {
			return transport.AssistantFinal{}, fail(transport.ErrorTypeProviderProtocolError)
		}
		final.FinishReason = f.DoneReason
		if f.Input != nil && (*f.Input < 0 || *f.Input > ContextTokens) || f.Output != nil && (*f.Output < 0 || *f.Output > OutputTokens) {
			return transport.AssistantFinal{}, fail(transport.ErrorTypeProviderProtocolError)
		}
		if f.Input != nil && f.Output != nil {
			final.Usage = &transport.Usage{InputTokens: *f.Input, OutputTokens: *f.Output, TotalTokens: *f.Input + *f.Output}
		}
	}
	if scanner.Err() != nil || !done || final.Content == "" || ctx.Err() != nil {
		return transport.AssistantFinal{}, fail(transport.ErrorTypeProviderProtocolError)
	}
	return final, nil
}

// Refuse duplicate envelope/measurement keys, including escaped aliases. Model
// output remains an opaque string: its JSON is checked by the owning consumer.
func uniqueJSON(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if !uniqueValue(d, 0) {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}
func uniqueValue(d *json.Decoder, depth int) bool {
	if depth > 16 {
		return false
	}
	t, err := d.Token()
	if err != nil {
		return false
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return true
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			name, ok := key.(string)
			if err != nil || !ok || seen[name] {
				return false
			}
			seen[name] = true
			if !uniqueValue(d, depth+1) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && end == json.Delim('}')
	case '[':
		for d.More() {
			if !uniqueValue(d, depth+1) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && end == json.Delim(']')
	default:
		return false
	}
}
