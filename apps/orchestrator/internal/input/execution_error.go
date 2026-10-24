package input

import "github.com/butler/butler/internal/transport"

// Only the fixed transport vocabulary survives into owner execution states.
// Provider messages/details are not diagnostic identifiers or channel content.
func executionFailure(err error) (string, string) {
	if code := transport.DiagnosticCode(err); code != "" {
		return code, code
	}
	return "execution_stopped", err.Error()
}
