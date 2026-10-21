package communication

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

var ErrOwnerControlRequired = errors.New("owner control ingress required")

type replyControlTarget struct {
	publication, task, run string
	revision               int64
}

// controlTargetTx proves only the owner's explicit control target. In contrast
// to execution admission, stopping an expired/paused task needs no mandate.
func (s *Store) controlTargetTx(ctx context.Context, tx pgx.Tx, q ReplyRequest) (replyControlTarget, error) {
	var out replyControlTarget
	a, ok := owneridentity.FromContext(ctx)
	if !ok || s.SourceGuard == nil || s.ChannelGuard == nil || q.BindingID == "" || q.BindingVersion < 1 {
		return out, ErrDenied
	}
	if (q.PublicationRef == "") == (q.ChannelACK == "") {
		return out, ErrAmbiguous
	}
	if a.Channel == "telegram" && q.ChannelACK == "" || a.Channel == "web" && q.PublicationRef == "" {
		return out, ErrDenied
	}
	rows, e := tx.Query(ctx, `SELECT DISTINCT p.publication_id,COALESCE(p.task_id,''),p.run_id,p.task_revision FROM communication_publications p JOIN delivery_outbox d USING(publication_id) JOIN communication_bindings b ON b.binding_id=d.binding_id AND b.version=d.binding_version WHERE b.binding_id=$1 AND b.version=$2 AND b.owner_id=$3 AND b.channel=$4 AND b.recipient=$5 AND b.enabled AND b.verified AND d.recipient_snapshot=b.recipient AND NOT p.privacy_redacted AND NOT d.privacy_redacted AND (($6<>'' AND p.publication_id=$6) OR ($7<>'' AND d.status='sent' AND d.channel_ack=$7)) LIMIT 2`, q.BindingID, q.BindingVersion, a.OwnerID, a.Channel, a.SessionKey, q.PublicationRef, q.ChannelACK)
	if e != nil {
		return out, e
	}
	n := 0
	for rows.Next() {
		n++
		if e = rows.Scan(&out.publication, &out.task, &out.run, &out.revision); e != nil {
			rows.Close()
			return out, e
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if n > 1 {
		return out, ErrAmbiguous
	}
	if n != 1 || out.task == "" {
		return out, ErrDenied
	}
	var payload []byte
	var kind string
	if tx.QueryRow(ctx, `SELECT p.kind,p.payload FROM communication_publications p JOIN delivery_outbox d USING(publication_id) JOIN communication_bindings b ON b.binding_id=d.binding_id AND b.version=d.binding_version WHERE p.publication_id=$1 AND b.binding_id=$2 AND b.version=$3 AND b.enabled AND b.verified AND b.owner_id=$4 AND b.channel=$5 AND b.recipient=$6 AND d.recipient_snapshot=b.recipient AND NOT p.privacy_redacted AND NOT d.privacy_redacted AND ($7='' OR (d.status='sent' AND d.channel_ack=$7)) FOR SHARE OF p,b,d`, out.publication, q.BindingID, q.BindingVersion, a.OwnerID, a.Channel, a.SessionKey, q.ChannelACK).Scan(&kind, &payload) != nil {
		return out, ErrDenied
	}
	if e = s.SourceGuard(ctx, tx, out.run); e != nil {
		return out, ErrDenied
	}
	args, e := communicationcontrol.DecodeArgs("communication."+kind, string(payload))
	if e != nil {
		return out, ErrDenied
	}
	if e = s.ChannelGuard(ctx, tx, out.run, Binding{ID: q.BindingID, Version: q.BindingVersion, Channel: a.Channel, Recipient: a.SessionKey}, args); e != nil {
		return out, ErrDenied
	}
	return out, nil
}

// ControlReply uses the existing Task Controller, outside the waiting FIFO.
// The guard repeats target/source proof inside its owning transaction. No
// execution permission or caller-selected TaskID crosses this boundary.
func (s *Store) ControlReply(ctx context.Context, c *task.Controller, q ReplyRequest) (task.ControlResult, error) {
	var out task.ControlResult
	a, ok := owneridentity.FromContext(ctx)
	kind, policy := task.ClassifyOwnerText(q.Text)
	if !ok || s == nil || s.Pool == nil || c == nil || kind == "" || !utf8.ValidString(q.Text) || utf8.RuneCountInString(q.Text) > 4000 || len(a.DeliveryID) > 512 || len(a.SessionKey) > 512 || len(q.BindingID) > 128 || len(q.PublicationRef) > 128 || len(q.ChannelACK) > 512 {
		return out, ErrDenied
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return out, e
	}
	target, e := s.controlTargetTx(ctx, tx, q)
	_ = tx.Rollback(context.Background())
	if e != nil {
		return out, e
	}
	// The controller already owns canonical task/session locks. Retain private
	// channel proof in its guard, without fabricating/merging a channel session.
	original := a
	a.SessionKey = ""
	controlCtx := owneridentity.WithActor(ctx, a)
	return c.ApplyWithGuard(controlCtx, task.ControlRequest{TaskID: target.task, ExpectedRevision: target.revision, Kind: kind, Instruction: q.Text, ExecutionPolicy: policy}, func(owned context.Context, tx pgx.Tx) error {
		current, e := s.controlTargetTx(owneridentity.WithActor(owned, original), tx, q)
		if e != nil {
			return e
		}
		if current != target {
			return ErrConflict
		}
		return nil
	})
}
