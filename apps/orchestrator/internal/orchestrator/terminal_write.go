package orchestrator

// Keep the actual cause inspectable by trusted callers without copying raw
// database/transient-store diagnostics into logs, run records or owner errors.
type terminalWriteError struct {
	stage     string
	cause     error
	preceding error
}

func (e *terminalWriteError) Error() string { return "terminal_" + e.stage + "_persistence_failed" }
func (e *terminalWriteError) Unwrap() []error {
	if e.preceding == nil {
		return []error{e.cause}
	}
	return []error{e.preceding, e.cause}
}

func (e *terminalWriteError) withPreceding(cause error) *terminalWriteError {
	return &terminalWriteError{stage: e.stage, cause: e.cause, preceding: cause}
}
