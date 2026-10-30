package openai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/butler/butler/internal/transport"
	"strconv"
	"strings"
	"time"
)

func (p *Provider) InferenceLimits() (int64, int64) {
	return int64(p.config.MaximumInputTokens), int64(p.config.MaximumOutputTokens)
}
func (p *Provider) InferenceTimeout() time.Duration { return p.config.Timeout }

// UTF-8 serialized bytes overestimate ordinary text tokens. Include the actual
// previous input/output token count, rather than counting incremental bytes only.
// A recovered unknown provider prefix must be rebuilt by the owning kernel.
func (p *Provider) checkContextAllowance(runID string, body []byte) error {
	var q map[string]any
	if json.Unmarshal(body, &q) != nil {
		return transport.ProviderProtocolError(providerName)
	}
	var prefix int64
	if id := stringValue(q["previous_response_id"]); id != "" {
		p.mu.Lock()
		n, ok := p.prefixTokens[runID][id]
		p.mu.Unlock()
		if !ok {
			return &transport.Error{Type: transport.ErrorTypeStatefulSessionLost, ProviderName: providerName, Message: "openai response prefix unavailable"}
		}
		prefix = n
	}
	if prefix > int64(p.config.MaximumInputTokens)-int64(len(body)) {
		return &transport.Error{Type: transport.ErrorTypeInputBytesExceeded, ProviderName: providerName, Message: "openai complete context allowance exceeded"}
	}
	return nil
}

func (p *Provider) encodeRequest(payload map[string]any) ([]byte, error) {
	// A requested privacy setting must never be silently dropped. Stateless
	// recovery is not implemented by this provider's measured-prefix contract.
	if _, present := payload["store"]; present {
		return nil, &transport.Error{Type: transport.ErrorTypeCapabilityMismatch, ProviderName: providerName, Message: "openai per-request storage override is unsupported"}
	}
	output := p.config.MaximumOutputTokens
	if v, ok := payload["max_output_tokens"]; ok {
		raw, err := json.Marshal(v)
		n, parseErr := strconv.Atoi(string(raw))
		if err != nil || parseErr != nil || n < 1 || n > output {
			return nil, fmt.Errorf("openai: output allowance exceeded")
		}
		output = n
	}
	payload["max_output_tokens"] = output
	if p.config.ReasoningEffort != "" {
		payload["reasoning"] = map[string]any{"effort": p.config.ReasoningEffort}
	}
	payload["stream"] = true
	payload["parallel_tool_calls"] = false // Core's submit contract accepts one result.
	if _, ok := payload["service_tier"]; !ok {
		payload["service_tier"] = "default"
	}
	return json.Marshal(payload)
}

func toolAlias(name string) string {
	valid := len(name) > 0 && len(name) <= 64
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			valid = false
		}
	}
	if valid && !strings.HasPrefix(name, "butler_") {
		return name
	}
	hash := sha256.Sum256([]byte(name))
	var label strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			label.WriteRune(r)
		} else {
			label.WriteByte('_')
		}
		if label.Len() >= 40 {
			break
		}
	}
	return "butler_" + label.String() + "_" + hex.EncodeToString(hash[:8])
}
func (p *Provider) resolveToolName(runID, name string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if value := p.aliases[runID][name]; value != "" {
		return value
	}
	return name
}

func (p *Provider) expireInstructions(runID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if old := p.expiry[runID]; old != nil {
		old.Stop()
	}
	var timer *time.Timer
	timer = time.AfterFunc(time.Minute, func() {
		p.mu.Lock()
		if p.expiry[runID] != timer {
			p.mu.Unlock()
			return
		}
		delete(p.expiry, runID)
		delete(p.instructions, runID)
		delete(p.toolSets, runID)
		delete(p.aliases, runID)
		delete(p.prefixTokens, runID)
		p.mu.Unlock()
		p.CloseRun(runID)
	})
	p.expiry[runID] = timer
}
