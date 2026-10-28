package orchestrator

import (
	"context"
	"errors"

	"github.com/butler/butler/internal/transport"
)

// ownedCancellation contains only a fixed Core reason, never storage/provider
// details. It retains the cancelled classification and the existing ownership
// sentinel independently. Source text and transport events cannot construct it.
type ownedCancellation struct {
	reason             string
	failure            error
	observationTimeout bool
}

// Only the owning watchdog's preserved plain cause qualifies. Joined independent
// failures retain their own diagnosis and cannot acquire retry eligibility.
func AuthorityObservationTimeout(err error) bool {
	cause, ok := err.(*ownedCancellation)
	return ok && cause.observationTimeout
}

func (e *ownedCancellation) Error() string          { return e.reason }
func (e *ownedCancellation) DiagnosticCode() string { return e.reason }
func (e *ownedCancellation) Unwrap() []error {
	return []error{context.Canceled, e.failure}
}

func preserveOwnedCancellation(ctx context.Context, err error) error {
	if !errors.Is(err, context.Canceled) {
		return err
	}
	// Joined or wrapped independent failures retain their own diagnosis even
	// when cancellation is also present. Only a plain cancelled stream acquires
	// the owning supervisor's cause.
	var providerFailure *transport.Error
	var terminalFailure *terminalWriteError
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &providerFailure) || errors.As(err, &terminalFailure) {
		return err
	}
	var cause *ownedCancellation
	if errors.As(context.Cause(ctx), &cause) {
		return cause
	}
	return err
}
