package budget

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// ModelPrice is operator configuration. Zero monetary charge is meaningful only
// with an explicitly declared local/subscription basis, never an inferred price.
type ModelPrice struct {
	Basis                string `json:"basis"`
	InputMicrosPerToken  int64  `json:"input_micros_per_token"`
	OutputMicrosPerToken int64  `json:"output_micros_per_token"`
}
type Policy struct {
	DisableOwnerDay    bool                  `json:"disable_owner_day,omitempty"`
	Task               Units                 `json:"task"`
	Mandate            Units                 `json:"mandate"`
	Day                Units                 `json:"day"`
	Conversation       Units                 `json:"conversation"`
	TaskLifetimeMillis int64                 `json:"task_lifetime_millis"`
	ModelMillis        int64                 `json:"model_millis"`
	ToolMillis         int64                 `json:"tool_millis"`
	OutputTokens       int64                 `json:"output_tokens"`
	Models             map[string]ModelPrice `json:"models"`
	RepeatLimit        int64                 `json:"repeat_limit"`
	ToolEgressBytes    int64                 `json:"tool_egress_bytes"`
	ModelInputBytes    int64                 `json:"model_input_bytes"`
}

func DefaultPolicy() Policy {
	u := Units{ModelTurns: 64, Tokens: 67108864, ExternalCalls: 128, Children: 8, ActiveMillis: 3600000, EgressBytes: 134217728, ProviderInputBytes: 268435456}
	return Policy{Task: u, Mandate: u, Day: Units{ModelTurns: 256, Tokens: 268435456, ExternalCalls: 512, Children: 32, ActiveMillis: 14400000, EgressBytes: 536870912, ProviderInputBytes: 1073741824}, Conversation: u, TaskLifetimeMillis: 604800000, ModelMillis: 120000, ToolMillis: 120000, OutputTokens: 8192, RepeatLimit: 3, Models: map[string]ModelPrice{}, ToolEgressBytes: 4194304, ModelInputBytes: 8388608}
}
func LoadPolicy() (Policy, error) {
	return ParsePolicy(os.Getenv("BUTLER_EXECUTION_BUDGET_POLICY_JSON"))
}

func ParsePolicy(raw string) (Policy, error) {
	p := DefaultPolicy()
	if raw != "" {
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&p); err != nil {
			return Policy{}, fmt.Errorf("invalid execution budget policy JSON")
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return Policy{}, fmt.Errorf("execution budget policy must be one JSON object")
		}
	}
	return p, p.Validate()
}
func (p Policy) Validate() error {
	for _, u := range []Units{p.Task, p.Mandate, p.Day, p.Conversation} {
		if err := u.validate(); err != nil {
			return err
		}
	}
	if p.TaskLifetimeMillis <= 0 || p.TaskLifetimeMillis > 31536000000 || p.ModelMillis <= 0 || p.ModelMillis > 86400000 || p.ToolMillis <= 0 || p.ToolMillis > 86400000 || p.OutputTokens <= 0 || p.OutputTokens > 65536 || p.RepeatLimit < 2 || p.RepeatLimit > 100 || p.ToolEgressBytes < 4194304 || p.ModelInputBytes <= 0 || p.ModelInputBytes > 8388608 {
		return ErrInvalid
	}
	for _, price := range p.Models {
		if price.InputMicrosPerToken < 0 || price.OutputMicrosPerToken < 0 {
			return ErrInvalid
		}
		switch price.Basis {
		case "operator_rate":
			if price.InputMicrosPerToken == 0 && price.OutputMicrosPerToken == 0 {
				return ErrUnknownCost
			}
		case "subscription_no_overage", "local_no_billing":
			if price.InputMicrosPerToken != 0 || price.OutputMicrosPerToken != 0 {
				return ErrInvalid
			}
		default:
			return ErrUnknownCost
		}
	}
	return nil
}
func (p Policy) ModelQuote(provider, model string, inputTokens int64) (Units, error) {
	price, ok := p.Models[provider+":"+model]
	if !ok {
		return Units{}, ErrUnknownCost
	}
	if inputTokens < 0 || inputTokens > 100000000 {
		return Units{}, ErrInvalid
	}
	// Check multiplication before integer monetary arithmetic.
	const max = int64(^uint64(0) >> 1)
	if price.InputMicrosPerToken > max/(inputTokens+1) || price.OutputMicrosPerToken > max/(p.OutputTokens+1) {
		return Units{}, ErrInvalid
	}
	i, o := inputTokens*price.InputMicrosPerToken, p.OutputTokens*price.OutputMicrosPerToken
	if i > max-o {
		return Units{}, ErrInvalid
	}
	return Units{ModelTurns: 1, Tokens: inputTokens + p.OutputTokens, CostMicros: i + o, ActiveMillis: p.ModelMillis, ProviderInputBytes: p.ModelInputBytes}, nil
}
