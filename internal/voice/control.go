package voice

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

// ControlSink calls the existing owner controller with this current-source
// guard in the controller's transaction, after its authority locks. The voice
// module does not choose a lifecycle transition or create an action grant.
type ControlSink interface {
	ApplyReviewed(context.Context, ReviewedCommand, func(context.Context, pgx.Tx) error) (string, error)
}

// ConfirmControl first persists the exact reviewed source and target. The
// controller has a separate transaction and its own stable delivery receipt;
// a lost ACK can be reconciled from control_pending without executing twice.
func (s *Store) ConfirmControl(ctx context.Context, r Review, sink ControlSink) (Clip, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || sink == nil || !r.CheckedCriticalDetails || !utf8.ValidString(r.Text) || strings.TrimSpace(r.Text) == "" || len(r.Text) > 16384 || sanitize.Text(r.Text) != r.Text || r.Target.TaskID == "" || r.Target.SessionKey == "" || r.Target.ExpectedTaskRevision < 1 {
		return Clip{}, ErrDenied
	}
	c, e := s.Get(ctx, r.ClipID)
	if e != nil || c.OwnerID != a.OwnerID || c.Role != OwnerCommand || c.Transcript == nil || c.Revision != r.ExpectedRevision || (c.State != "review" && c.State != "control_pending") {
		return Clip{}, ErrDenied
	}
	tx, e := s.Catalog.Pool().Begin(ctx)
	if e != nil {
		return Clip{}, e
	}
	defer tx.Rollback(ctx)
	parents := []catalog.SourceRef{c.Source, *c.Transcript}
	if c.State == "review" {
		child, e := s.Catalog.CreateDerivedTx(ctx, tx, parents, "voice_owner_review", "owner-reviewed voice control", r.Text, encode(map[string]any{"version": Version, "critical_details_reviewed": true}))
		if e != nil {
			return Clip{}, e
		}
		tag, e := tx.Exec(ctx, `UPDATE voice_clips SET state='control_pending',reviewed_ref=$2,control_target=$3,revision=revision+1,updated_at=clock_timestamp() WHERE clip_id=$1 AND revision=$4 AND state='review' AND expires_at>clock_timestamp()`, c.ID, encode(ref(child)), encode(r.Target), c.Revision)
		if e != nil || tag.RowsAffected() != 1 {
			return Clip{}, ErrDenied
		}
		c.State = "control_pending"
		c.Revision++
		cr := ref(child)
		c.Reviewed = &cr
		c.ControlTarget = &r.Target
	} else if c.Reviewed == nil || c.ControlTarget == nil || *c.ControlTarget != r.Target {
		return Clip{}, ErrDenied
	}
	srcs, e := s.Catalog.AccessTx(ctx, tx, []catalog.SourceRef{*c.Reviewed}, "voice_owner_review")
	if e != nil || len(srcs) != 1 || srcs[0].Body != r.Text {
		return Clip{}, ErrDenied
	}
	if e = tx.Commit(ctx); e != nil {
		return Clip{}, e
	}
	command := ReviewedCommand{ClipID: c.ID, Text: r.Text, Source: *c.Reviewed, Original: c.Source, Target: r.Target}
	guard := func(ctx context.Context, tx pgx.Tx) error {
		srcs, e := s.Catalog.AccessTx(ctx, tx, []catalog.SourceRef{command.Source, command.Original}, "voice_owner_review")
		if e != nil {
			return e
		}
		found := false
		for _, src := range srcs {
			if src.ID == command.Source.ID && src.Body == command.Text {
				found = true
			}
		}
		if !found {
			return ErrDenied
		}
		current, e := load(ctx, tx, c.ID, true)
		if e != nil || current.OwnerID != a.OwnerID || current.Role != OwnerCommand || current.Revision != c.Revision || current.State != "control_pending" || current.Reviewed == nil || *current.Reviewed != command.Source || current.ControlTarget == nil || *current.ControlTarget != command.Target {
			return ErrDenied
		}
		var beforeExpiry bool
		if tx.QueryRow(ctx, `SELECT clock_timestamp()<$1`, current.Expires).Scan(&beforeExpiry) != nil || !beforeExpiry {
			return ErrDenied
		}
		return nil
	}
	receipt, e := sink.ApplyReviewed(ctx, command, guard)
	if e != nil || receipt == "" {
		return Clip{}, ErrDenied
	}
	// This is a projection of an already committed controller receipt. A crash
	// here leaves control_pending and the stable controller delivery resolves it.
	tag, e := s.Catalog.Pool().Exec(ctx, `UPDATE voice_clips SET state='submitted',input_id=$2,revision=revision+1,updated_at=clock_timestamp() WHERE clip_id=$1 AND revision=$3 AND state='control_pending'`, c.ID, receipt, c.Revision)
	if e != nil || tag.RowsAffected() != 1 {
		return Clip{}, ErrDenied
	}
	return s.Get(ctx, c.ID)
}
