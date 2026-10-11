package app

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/voice"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type VoiceInputAdapter struct {
	Pool    *pgxpool.Pool
	Sources *catalog.Store
	Inputs  *input.Store
}

func (a *VoiceInputAdapter) LockTargetTx(ctx context.Context, tx pgx.Tx, target voice.Target) error {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || a == nil || a.Inputs == nil || a.Sources == nil || target.SessionKey == "" {
		return voice.ErrDenied
	}
	var own string
	if tx.QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key=$1 FOR UPDATE`, target.SessionKey).Scan(&own) != nil || own != actor.OwnerID {
		return voice.ErrDenied
	}
	if target.TaskID != "" {
		var current int64
		var state string
		if target.ExpectedTaskRevision < 1 || tx.QueryRow(ctx, `SELECT revision,status FROM tasks WHERE task_id=$1 AND session_key=$2 AND owner_id=$3 FOR SHARE`, target.TaskID, target.SessionKey, own).Scan(&current, &state) != nil || state != "active" || current != target.ExpectedTaskRevision {
			return voice.ErrDenied
		}
	}
	return nil
}
func (a *VoiceInputAdapter) SubmitReviewedTx(ctx context.Context, tx pgx.Tx, c voice.ReviewedCommand) (string, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok {
		return "", voice.ErrDenied
	}
	if kind, _ := task.ClassifyOwnerText(c.Text); kind != "" {
		return "", voice.ErrDenied
	}
	payload, _ := json.Marshal(map[string]any{"user_id": actor.OwnerID, "task_id": c.Target.TaskID, "voice_review_binding": c.ClipID})
	event := ingress.InputEvent{EventID: "voice-reviewed:" + c.ClipID, IdempotencyKey: "voice-reviewed:" + c.ClipID, SessionKey: c.Target.SessionKey, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(payload)}
	record, _, e := a.Inputs.AcceptTx(ctx, tx, event, actor.OwnerID, c.Target.TaskID)
	if e != nil {
		return "", e
	}
	source, _ := json.Marshal(c.Source)
	original, _ := json.Marshal(c.Original)
	if _, e = tx.Exec(ctx, `INSERT INTO voice_input_sources(input_id,event_id,owner_id,session_key,task_id,task_revision,source_ref,original_ref) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, record.InputID, event.EventID, actor.OwnerID, event.SessionKey, c.Target.TaskID, c.Target.ExpectedTaskRevision, source, original); e != nil {
		return "", e
	}
	return record.InputID, nil
}

// Prepare is composed only into the trusted post-lease hook. It never honors
// a source reference in an arbitrary request body or borrows an old owner actor.
func (a *VoiceInputAdapter) Prepare(ctx context.Context, event ingress.InputEvent) (ingress.InputEvent, error) {
	if !strings.HasPrefix(event.EventID, "voice-reviewed:") {
		return event, nil
	}
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.SessionKey != event.SessionKey || a == nil || a.Pool == nil || a.Sources == nil {
		return ingress.InputEvent{}, voice.ErrDenied
	}
	ctx = owneridentity.WithActor(ctx, owneridentity.Actor{})
	tx, e := a.Pool.Begin(ctx)
	if e != nil {
		return ingress.InputEvent{}, e
	}
	defer tx.Rollback(ctx)
	if _, e = a.Sources.AccessTx(ctx, tx, nil, "memory_read"); e != nil {
		return ingress.InputEvent{}, e
	}
	var own, session, taskID, body, channel string
	var revision int64
	var raw, original []byte
	e = tx.QueryRow(ctx, `SELECT v.owner_id,v.session_key,v.task_id,v.task_revision,v.source_ref,v.original_ref,i.event_json->>'PayloadJSON',i.source FROM voice_input_sources v JOIN accepted_inputs i USING(input_id) JOIN voice_clips c ON c.input_id=v.input_id WHERE v.event_id=$1 AND i.event_id=$1 AND i.status='running' AND i.run_id=$2 AND c.state='submitted' AND c.expires_at>clock_timestamp() AND c.reviewed_ref=v.source_ref AND c.source_ref=v.original_ref FOR SHARE OF v,i`, event.EventID, token.RunID).Scan(&own, &session, &taskID, &revision, &raw, &original, &body, &channel)
	if e != nil || session != event.SessionKey || channel != event.Source || body != event.PayloadJSON {
		return ingress.InputEvent{}, voice.ErrDenied
	}
	if taskID != "" {
		var current int64
		var state string
		if tx.QueryRow(ctx, `SELECT revision,status FROM tasks WHERE task_id=$1 AND owner_id=$2 AND session_key=$3 FOR SHARE`, taskID, own, session).Scan(&current, &state) != nil || state != "active" || current != revision {
			return ingress.InputEvent{}, voice.ErrDenied
		}
	}
	var source, origin catalog.SourceRef
	if json.Unmarshal(raw, &source) != nil || json.Unmarshal(original, &origin) != nil {
		return ingress.InputEvent{}, voice.ErrDenied
	}
	sources, e := a.Sources.AccessTx(ctx, tx, []catalog.SourceRef{source, origin}, "memory_read")
	if e != nil {
		return ingress.InputEvent{}, e
	}
	text := ""
	for _, src := range sources {
		if src.ID == source.ID && src.OwnerID == own {
			text = src.Body
		}
	}
	if text == "" {
		return ingress.InputEvent{}, voice.ErrDenied
	}
	var payload map[string]any
	if json.Unmarshal([]byte(body), &payload) != nil {
		return ingress.InputEvent{}, voice.ErrDenied
	}
	payload["text"] = text
	encoded, _ := json.Marshal(payload)
	event.PayloadJSON = string(encoded)
	if e = tx.Commit(ctx); e != nil {
		return ingress.InputEvent{}, e
	}
	return event, nil
}
