package communication

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	runstore "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

func suspendRunTaskTx(ctx context.Context, tx pgx.Tx, p Principal) error {
	if e := task.WaitInteractionTx(ctx, tx, p.TaskID, p.TaskRevision, p.ConstraintsRevision); e != nil {
		return e
	}
	return runstore.SuspendInteractionTx(ctx, tx, p.RunID)
}

type interaction struct {
	ID, PublicationID, InputID, OwnerID, SessionKey, RunID, TaskID, State string
	TaskRevision, ConstraintsRevision, Version                            int64
	Question                                                              communicationcontrol.Args
	Response                                                              communicationcontrol.Answer
	Expiry                                                                *time.Time
	Hash, Delivery                                                        string
}

func loadInteraction(ctx context.Context, tx pgx.Tx, id string, lock bool) (interaction, error) {
	var v interaction
	var q, answer []byte
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	e := tx.QueryRow(ctx, `SELECT interaction_id,publication_id,input_id,owner_id,session_key,run_id,COALESCE(task_id,''),task_revision,constraints_revision,version,state,question_json,response_json,expires_at,COALESCE(response_hash,''),COALESCE(response_delivery_id,'') FROM communication_interactions WHERE interaction_id=$1 AND NOT privacy_redacted`+suffix, id).Scan(&v.ID, &v.PublicationID, &v.InputID, &v.OwnerID, &v.SessionKey, &v.RunID, &v.TaskID, &v.TaskRevision, &v.ConstraintsRevision, &v.Version, &v.State, &q, &answer, &v.Expiry, &v.Hash, &v.Delivery)
	if e != nil {
		return v, ErrDenied
	}
	if json.Unmarshal(q, &v.Question) != nil {
		return v, ErrDenied
	}
	if len(answer) > 0 {
		_ = json.Unmarshal(answer, &v.Response)
	}
	return v, nil
}

// Respond is the authenticated control ingress, not an accepted FIFO input.
// Actor proof is supplied by an owning adapter; a model/body owner ID is ignored.
func (s *Store) Respond(ctx context.Context, id, bindingID string, bindingVersion, expectedVersion int64, answer communicationcontrol.Answer) (communicationcontrol.Answer, error) {
	_, ok := owneridentity.FromContext(ctx)
	if !ok || s == nil || s.SourceGuard == nil || s.ChannelGuard == nil || s.Inputs == nil {
		return answer, ErrDenied
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return answer, e
	}
	defer tx.Rollback(context.Background())
	result, e := s.RespondTx(ctx, tx, id, bindingID, bindingVersion, expectedVersion, answer)
	if e != nil {
		return answer, e
	}
	if e = tx.Commit(ctx); e != nil {
		return answer, e
	}
	return result, nil
}

// RespondTx joins an owning control transaction. Keeping the entire response,
// transcript and ready marker here also permits transaction-boundary acceptance
// without introducing production fault hooks or a second queue.
func (s *Store) lockInteractionTx(ctx context.Context, tx pgx.Tx, id, bindingID string, bindingVersion int64) (interaction, bool, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || s == nil || s.SourceGuard == nil || s.ChannelGuard == nil || s.Inputs == nil {
		return interaction{}, false, ErrDenied
	}
	v, e := loadInteraction(ctx, tx, id, false)
	if e != nil || v.OwnerID != actor.OwnerID {
		return interaction{}, false, ErrDenied
	}
	var yes bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM sessions WHERE session_key=$1 AND user_id=$2 FOR UPDATE`, v.SessionKey, actor.OwnerID).Scan(&yes) != nil {
		return interaction{}, false, ErrDenied
	}
	// Stabilize run/task before input/interaction, matching the publication writer.
	if tx.QueryRow(ctx, `SELECT TRUE FROM runs WHERE run_id=$1 AND session_key=$2 FOR UPDATE`, v.RunID, v.SessionKey).Scan(&yes) != nil {
		return interaction{}, false, ErrDenied
	}
	paused := false
	if v.TaskID != "" {
		var status string
		var rev, con int64
		if tx.QueryRow(ctx, `SELECT status,revision,constraints_revision FROM tasks WHERE task_id=$1 AND owner_id=$2 FOR UPDATE`, v.TaskID, actor.OwnerID).Scan(&status, &rev, &con) != nil {
			return interaction{}, false, ErrDenied
		}
		if status == "cancelled" || status == "completed" || rev != v.TaskRevision || con != v.ConstraintsRevision {
			return interaction{}, false, ErrDenied
		}
		paused = status == "paused"
	}
	if tx.QueryRow(ctx, `SELECT TRUE FROM accepted_inputs WHERE input_id=$1 FOR UPDATE`, v.InputID).Scan(&yes) != nil {
		return interaction{}, false, ErrDenied
	}
	v, e = loadInteraction(ctx, tx, id, true)
	if e != nil {
		return interaction{}, false, e
	}
	if tx.QueryRow(ctx, `SELECT TRUE FROM communication_bindings b JOIN delivery_outbox d ON d.binding_id=b.binding_id AND d.binding_version=b.version WHERE b.binding_id=$1 AND b.version=$2 AND b.owner_id=$3 AND b.channel=$4 AND b.recipient=$5 AND b.enabled AND b.verified AND d.publication_id=$6 FOR SHARE OF b,d`, bindingID, bindingVersion, actor.OwnerID, actor.Channel, actor.SessionKey, v.PublicationID).Scan(&yes) != nil {
		return interaction{}, false, ErrDenied
	}
	if e = s.SourceGuard(ctx, tx, v.RunID); e != nil {
		return interaction{}, false, ErrDenied
	}
	if e = s.ChannelGuard(ctx, tx, v.RunID, Binding{ID: bindingID, Version: bindingVersion, Channel: actor.Channel, Recipient: actor.SessionKey}, v.Question); e != nil {
		return interaction{}, false, ErrDenied
	}
	return v, paused, nil
}

func (s *Store) RespondTx(ctx context.Context, tx pgx.Tx, id, bindingID string, bindingVersion, expectedVersion int64, answer communicationcontrol.Answer) (communicationcontrol.Answer, error) {
	actor, _ := owneridentity.FromContext(ctx)
	v, paused, e := s.lockInteractionTx(ctx, tx, id, bindingID, bindingVersion)
	if e != nil {
		return answer, e
	}
	raw, _ := json.Marshal(answer)
	digest := hash(string(raw))
	if v.State == "resolved" {
		if v.Hash == digest && v.Delivery == actor.DeliveryID {
			return v.Response, nil
		}
		return answer, ErrResolved
	}
	var now time.Time
	_ = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now)
	if v.State != "open" || v.Version != expectedVersion || v.Expiry != nil && !v.Expiry.After(now) || v.Question.Input == nil {
		return answer, ErrDenied
	}
	if communicationcontrol.ValidateAnswer(*v.Question.Input, answer) != nil {
		return answer, ErrDenied
	}
	_, e = tx.Exec(ctx, `UPDATE communication_interactions SET state='resolved',version=version+1,response_json=$2::jsonb,response_hash=$3,response_delivery_id=$4,response_binding_id=$5,resolved_at=clock_timestamp() WHERE interaction_id=$1 AND state='open'`, id, raw, digest, actor.DeliveryID, bindingID)
	if e != nil {
		return answer, e
	}
	meta, _ := json.Marshal(map[string]any{"kind": "interaction_response", "interaction_ref": id, "channel": actor.Channel})
	_, e = tx.Exec(ctx, `INSERT INTO messages(message_id,session_key,run_id,role,content,metadata) VALUES($1,$2,$3,'user',$4,$5::jsonb)`, "response:"+id, v.SessionKey, v.RunID, string(raw), meta)
	if e != nil {
		return answer, e
	}
	// The owner response is new DATA, but its interpretation depends on this
	// question. Catalog deletion must erase this canonical copy as well.
	_, e = tx.Exec(ctx, `INSERT INTO memory_source_dependencies(child_source_id,parent_source_id,parent_revision) SELECT $1,source_id,head_revision FROM memory_sources WHERE source_id=$2`, "message:response:"+id, "message:"+v.PublicationID)
	if e != nil {
		return answer, e
	}
	// Pause has revoked/cancelled the old execution through ordinary owner
	// controls. Store DATA now; the owner's later resume creates a fresh Input.
	if !paused {
		if e = s.Inputs.ReadyInteractionTx(ctx, tx, v.InputID, id, false); e != nil {
			return answer, e
		}
	}
	_, e = tx.Exec(ctx, `UPDATE communication_admissions SET obligation_state='open' WHERE input_id=$1 AND obligation_state='waiting_for_user'`, v.InputID)
	if e != nil {
		return answer, e
	}
	if e = s.eventTx(ctx, tx, v.OwnerID, v.SessionKey, id, "interaction.resolved"); e != nil {
		return answer, e
	}
	return answer, nil
}

// ResumeInteraction runs only after the old lease/claim has been replaced by a
// fresh worker. Preferences do not create approval and never execute a tool.
func (s *Store) ResumeInteraction(ctx context.Context, runID, id string) (string, error) {
	tok, ok := ownership.TokenFromContext(ctx)
	if !ok || tok.RunID != runID || s.SourceGuard == nil {
		return "", ErrDenied
	}
	ctx, release, e := ownership.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer release()
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(context.Background())
	var yes bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM sessions WHERE session_key=$1 FOR UPDATE`, tok.SessionKey).Scan(&yes) != nil {
		return "", ErrDenied
	}
	if tx.QueryRow(ctx, `SELECT TRUE FROM runs r JOIN session_ownership o USING(session_key) WHERE r.run_id=$1 AND o.run_id=$1 AND o.lease_id=$2 AND o.fence_epoch=$3 AND o.expires_at>clock_timestamp() AND r.current_state IN ('awaiting_approval','awaiting_model_resume','model_running') FOR UPDATE OF r`, runID, tok.LeaseID, tok.FenceEpoch).Scan(&yes) != nil {
		return "", ErrDenied
	}
	v, e := loadInteraction(ctx, tx, id, false)
	if e != nil || v.State != "resolved" || v.RunID != runID || v.SessionKey != tok.SessionKey {
		return "", ErrDenied
	}
	if v.TaskID != "" {
		if e = task.ResumeInteractionTx(ctx, tx, v.TaskID, v.TaskRevision, v.ConstraintsRevision); e != nil {
			return "", ErrDenied
		}
		var now time.Time
		_ = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now)
		if _, e = actionauth.CheckTaskAuthorityTx(ctx, tx, v.TaskID, v.OwnerID, nil, now); e != nil {
			return "", ErrDenied
		}
	}
	if e = s.Inputs.CheckInteractionClaimTx(ctx, tx, runID, id); e != nil {
		return "", ErrDenied
	}
	if e = s.SourceGuard(ctx, tx, runID); e != nil {
		return "", ErrDenied
	}
	raw, _ := json.Marshal(map[string]any{"interaction_ref": id, "answer": v.Response, "preference_data": true})
	if e = tx.Commit(ctx); e != nil {
		return "", e
	}
	return string(raw), nil
}

// PendingInteraction is also used after a lost owning-endpoint ACK. A durable
// question stops the old provider before generic tool-result writes can erase it.
func (s *Store) PendingInteraction(ctx context.Context, runID string) (string, error) {
	t, ok := ownership.TokenFromContext(ctx)
	if !ok || t.RunID != runID {
		return "", ErrDenied
	}
	var id string
	e := s.Pool.QueryRow(ctx, `SELECT x.interaction_id FROM communication_interactions x JOIN accepted_inputs i USING(input_id) WHERE x.run_id=$1 AND x.session_key=$2 AND i.checkpoint IN ('awaiting_interaction','interaction_ready') AND i.checkpoint_json->'interaction'->>'interaction_id'=x.interaction_id AND x.state IN ('open','resolved')`, runID, t.SessionKey).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		return "", nil
	}
	return id, e
}
