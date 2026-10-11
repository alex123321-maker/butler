package voice

import (
	"context"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
	"strings"
	"unicode/utf8"
)

type Target struct {
	SessionKey           string `json:"session_key"`
	TaskID               string `json:"task_id"`
	ExpectedTaskRevision int64  `json:"expected_task_revision"`
}
type ReviewedCommand struct {
	ClipID   string
	Text     string
	Source   catalog.SourceRef
	Original catalog.SourceRef
	Target   Target
}

// An orchestrator-owned adapter implements ordinary durable input acceptance,
// with current session/task admission first in the shared transaction. Nil
// composition is disabled; the voice module cannot execute or grant actions.
type InputSink interface {
	LockTargetTx(context.Context, pgx.Tx, Target) error
	SubmitReviewedTx(context.Context, pgx.Tx, ReviewedCommand) (string, error)
}
type Review struct {
	ClipID                 string `json:"clip_id"`
	ExpectedRevision       int64  `json:"expected_revision"`
	Text                   string `json:"text"`
	CheckedCriticalDetails bool   `json:"checked_critical_details"`
	Target                 Target `json:"target"`
}

func (s *Store) Confirm(ctx context.Context, r Review, sink InputSink) (Clip, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || sink == nil || !r.CheckedCriticalDetails || !utf8.ValidString(r.Text) || strings.TrimSpace(r.Text) == "" || len(r.Text) > 16384 || sanitize.Text(r.Text) != r.Text {
		return Clip{}, ErrDenied
	}
	c, e := s.Get(ctx, r.ClipID)
	if e != nil || c.OwnerID != a.OwnerID || c.Role != OwnerCommand || c.State != "review" || c.Revision != r.ExpectedRevision || c.Transcript == nil {
		return Clip{}, ErrDenied
	}
	tx, e := s.Catalog.Pool().Begin(ctx)
	if e != nil {
		return Clip{}, e
	}
	defer tx.Rollback(ctx)
	// Session/task locks precede source graph locks, as in ordinary input and
	// owner Stop. The adapter rechecks current owner and task status here.
	if e = sink.LockTargetTx(ctx, tx, r.Target); e != nil {
		return Clip{}, e
	}
	parents := []catalog.SourceRef{c.Source, *c.Transcript}
	child, e := s.Catalog.CreateDerivedTx(ctx, tx, parents, "voice_owner_review", "owner-reviewed voice text", r.Text, encode(map[string]any{"version": Version, "review_delivery": a.DeliveryID, "critical_details_reviewed": true}))
	if e != nil {
		return Clip{}, e
	}
	current, e := load(ctx, tx, c.ID, true)
	if e != nil || current.Revision != c.Revision || current.State != "review" {
		return Clip{}, ErrDenied
	}
	inputID, e := sink.SubmitReviewedTx(ctx, tx, ReviewedCommand{ClipID: c.ID, Text: r.Text, Source: ref(child), Original: c.Source, Target: r.Target})
	if e != nil || inputID == "" {
		return Clip{}, ErrDenied
	}
	tag, e := tx.Exec(ctx, `UPDATE voice_clips SET state='submitted',reviewed_ref=$2,input_id=$3,revision=revision+1,updated_at=clock_timestamp() WHERE clip_id=$1 AND revision=$4 AND state='review' AND expires_at>clock_timestamp()`, c.ID, encode(ref(child)), inputID, c.Revision)
	if e != nil || tag.RowsAffected() != 1 {
		return Clip{}, ErrDenied
	}
	if _, e = s.Catalog.AccessTx(ctx, tx, parents, "voice_owner_review"); e != nil {
		return Clip{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Clip{}, e
	}
	return s.Get(ctx, c.ID)
}
