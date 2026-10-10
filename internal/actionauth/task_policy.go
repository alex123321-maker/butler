package actionauth

import "fmt"

// CheckExecutionPolicy checks a server-derived owner restriction. Exact approval
// or a broader mandate cannot override a current prepare-only restriction.
func CheckExecutionPolicy(c ContextSnapshot, effect Effect) error {
	switch c.ExecutionPolicy {
	case "", "normal":
		return nil
	case "prepare_only":
		if effect == EffectRead {
			return nil
		}
		return fmt.Errorf("task permits preparation only; external or unknown effects are denied")
	default:
		return fmt.Errorf("unsupported trusted task execution policy")
	}
}
