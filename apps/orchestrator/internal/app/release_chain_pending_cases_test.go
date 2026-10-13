package app

import "reflect"

// Nil retains the original five-case declaration and its order. The new
// explicit declaration skips only the completed normal-work portion directed
// by the user; no arbitrary caller subset, duplicate or ordering is accepted.
func releaseChainHighDeclaredScenarios(entry releaseChainHighEntry) ([]string, error) {
	if entry.ScenarioNames == nil {
		return releaseChainV5Scenarios(), nil
	}
	fixed := []string{"restart_sent", "explicit_absence", "lost_ack", "owner_stop"}
	if entry.PreserveInvalidOriginalEvidence {
		// A new frozen scope starts with the previously unattempted faults. The
		// complete restart/result/notification chain remains mandatory last.
		fixed = []string{"owner_stop", "explicit_absence", "lost_ack", "restart_sent"}
	}
	if entry.SkipVerifiedOwnerStop {
		if !entry.PreserveInvalidOriginalEvidence || !entry.ContinueAfterCaseFailure {
			return nil, errNativeFixtureDenied
		}
		fixed = []string{"explicit_absence", "lost_ack", "restart_sent"}
	}
	if entry.SkipVerifiedExplicitAbsence {
		if !entry.SkipVerifiedOwnerStop || !entry.FailFastTerminalContinuation || entry.PostgreSQLPoolSize != 10 {
			return nil, errNativeFixtureDenied
		}
		fixed = []string{"lost_ack", "restart_sent"}
	}
	if !reflect.DeepEqual(entry.ScenarioNames, fixed) {
		return nil, errNativeFixtureDenied
	}
	return append([]string(nil), fixed...), nil
}

// This is an explicit fixture profile. Production default/max8MiB, the old
// 128KiB fixture, conservative token quotes and every other resource stay put.
func releaseChainHighApplyInputAllowance(entry releaseChainHighEntry, cfg *releaseChainModelConfig) error {
	if entry.ProviderInputBytes == 0 {
		return nil
	}
	if cfg == nil || entry.ProviderInputBytes != 4<<20 || entry.Maximum != 120 || entry.Policy == (releaseChainHighWindowPolicy{}) || !entry.Policy.valid() {
		return errNativeFixtureDenied
	}
	cfg.Policy.ModelInputBytes = entry.ProviderInputBytes
	cfg.Policy.Day.ProviderInputBytes = int64(entry.Maximum) * entry.ProviderInputBytes
	return cfg.Policy.Validate()
}
