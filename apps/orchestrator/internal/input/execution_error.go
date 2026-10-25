package input

import (
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/internal/transport"
)

// Only the fixed transport vocabulary survives into owner execution states.
// Provider messages/details are not diagnostic identifiers or channel content.
func executionFailure(err error) (string, string) {
	if flow.AuthorityObservationTimeout(err) {
		return "execution_authority_observation_timeout", "execution_authority_observation_timeout"
	}
	if code := transport.DiagnosticCode(err); code != "" {
		return code, code
	}
	return "execution_stopped", err.Error()
}
