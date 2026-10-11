package app

import (
	"context"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/voice"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type VoiceControlAdapter struct {
	Pool       *pgxpool.Pool
	Controller *task.Controller
}

func (a *VoiceControlAdapter) ApplyReviewed(ctx context.Context, c voice.ReviewedCommand, guard func(context.Context, pgx.Tx) error) (string, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || a == nil || a.Controller == nil || a.Pool == nil || guard == nil || c.Target.ExpectedTaskRevision < 1 {
		return "", voice.ErrDenied
	}
	kind, policy := task.ClassifyOwnerText(c.Text)
	if kind == "" {
		return "", voice.ErrDenied
	}
	// Explicit target belongs to this current authenticated owner. Body fields
	// never supply owner proof, and recognition never chooses a target task.
	var own, session string
	if a.Pool.QueryRow(ctx, `SELECT owner_id,session_key FROM tasks WHERE task_id=$1`, c.Target.TaskID).Scan(&own, &session) != nil || own != actor.OwnerID || session != c.Target.SessionKey {
		return "", voice.ErrDenied
	}
	actor.SessionKey = session
	actor.DeliveryID = "voice-control:" + c.ClipID
	bounded := owneridentity.WithActor(ctx, actor)
	// Lifecycle controls need the typed restriction, not another durable copy of
	// recognized speech. The immutable review remains catalog-owned and deletable.
	result, e := a.Controller.ApplyWithGuard(bounded, task.ControlRequest{TaskID: c.Target.TaskID, ExpectedRevision: c.Target.ExpectedTaskRevision, Kind: kind, ExecutionPolicy: policy, Instruction: "Owner-reviewed voice source: " + c.Source.ID}, task.ControlGuard(guard))
	if e != nil {
		return "", e
	}
	return result.InputID, nil
}
