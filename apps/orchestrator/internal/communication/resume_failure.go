package communication

import (
	"context"
	"errors"

	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// Only fixed owning phases/classifications escape. Raw SQL, source and private
// control errors are deliberately discarded, including through Unwrap.
type resumeFailure struct {
	code  string
	cause error
}

func (e *resumeFailure) Error() string                 { return e.code }
func (e *resumeFailure) InteractionResumeCode() string { return e.code }
func (e *resumeFailure) Unwrap() []error               { return []error{ErrDenied, e.cause} }

func resumeFailed(ctx context.Context, phase string, err error) error {
	kind, cause := "unavailable", ErrDenied
	switch {
	case errors.Is(err, catalog.ErrRestore):
		kind, cause = "restore", catalog.ErrRestore
	case errors.Is(err, catalog.ErrStale):
		kind, cause = "stale", catalog.ErrStale
	case errors.Is(err, catalog.ErrDenied):
		kind, cause = "denied", catalog.ErrDenied
	case errors.Is(err, context.DeadlineExceeded):
		kind, cause = "deadline", context.DeadlineExceeded
	case errors.Is(err, context.Canceled):
		kind, cause = "canceled", context.Canceled
	case ctx.Err() != nil:
		cause = ctx.Err()
		kind = "canceled"
		if errors.Is(cause, context.DeadlineExceeded) {
			kind = "deadline"
		}
	case errors.Is(err, input.ErrClaimLost):
		kind, cause = "lost", input.ErrClaimLost
	case errors.Is(err, ownership.ErrStale):
		kind, cause = "stale", ownership.ErrStale
	case err == nil || errors.Is(err, ErrDenied) || errors.Is(err, pgx.ErrNoRows):
		kind = "denied"
	}
	return &resumeFailure{code: "interaction_resume_" + phase + "_" + kind, cause: cause}
}
