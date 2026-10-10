package approval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrUnauthorized = errors.New("authenticated owner and authorized approval channel required")
var ErrActionInvalid = errors.New("approval action changed, expired or cancelled")

type Actor struct {
	OwnerID    string
	Channel    string
	SessionKey string
	DeliveryID string
}
type actorKey struct{}

// WithActor is for an authenticated channel adapter, never model/caller args.
func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, actor)
}
func ActorFromContext(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok
}

type CheckpointHooks interface {
	SuspendApprovalTx(context.Context, pgx.Tx, string, string, string) error
	ReadyApprovalTx(context.Context, pgx.Tx, string, string) error
}

func (r *PostgresRepository) SetCheckpointHooks(h CheckpointHooks) { r.checkpoints = h }

type approvalTxKey struct{}
type approvalDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func (r *PostgresRepository) db(ctx context.Context) approvalDB {
	if tx, ok := ctx.Value(approvalTxKey{}).(pgx.Tx); ok {
		return tx
	}
	return r.pool
}

func (r *PostgresRepository) CreateDurable(ctx context.Context, p CreatePendingParams) (Record, error) {
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer release()
	b := p.ActionBinding
	if b.ActionID == "" || b.ActionHash == "" || b.RunID != p.RunID || b.ToolCallID != p.ToolCallID || b.ToolName != p.ToolName || b.OwnerID == "" || b.ArgsHash == "" || r.checkpoints == nil {
		return Record{}, fmt.Errorf("exact durable action and checkpoint hooks required")
	}
	var continuation struct {
		Version int `json:"version"`
	}
	if len(p.ContinuationJSON) > 1024*1024 || json.Unmarshal([]byte(p.ContinuationJSON), &continuation) != nil || continuation.Version != 1 {
		return Record{}, fmt.Errorf("version 1 data-only continuation required")
	}
	if p.ExpiresAt == nil || !p.ExpiresAt.After(time.Now()) {
		return Record{}, ErrActionInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(context.Background())
	ctx = context.WithValue(ctx, approvalTxKey{}, tx)
	var owner, runState string
	var pinned int64
	var taskID string
	err = tx.QueryRow(ctx, `SELECT s.user_id,r.current_state,COALESCE(r.task_id,''),COALESCE(r.task_revision,0) FROM runs r JOIN sessions s USING(session_key) WHERE r.run_id=$1 AND r.session_key=$2 FOR UPDATE OF r`, p.RunID, p.SessionKey).Scan(&owner, &runState, &taskID, &pinned)
	if err != nil {
		return Record{}, err
	}
	if owner != b.OwnerID || runState != "tool_pending" || taskID != b.TaskID || pinned != b.TaskRevision {
		return Record{}, ErrActionInvalid
	}
	if b.TaskID != "" {
		var revision, constraints int64
		var status string
		err = tx.QueryRow(ctx, `SELECT revision,constraints_revision,status FROM tasks WHERE task_id=$1 FOR UPDATE`, b.TaskID).Scan(&revision, &constraints, &status)
		if err != nil || revision != b.TaskRevision || constraints != b.ConstraintsRevision || status != "active" {
			return Record{}, ErrActionInvalid
		}
	}
	record, err := r.CreateApproval(ctx, CreateParams{ApprovalID: newApprovalID(), RunID: p.RunID, SessionKey: p.SessionKey, ToolCallID: p.ToolCallID, ApprovalType: ApprovalTypeToolCall, RequestedVia: p.RequestedVia, ToolName: p.ToolName, ArgsJSON: p.ArgsJSON, PayloadJSON: p.PayloadJSON, RiskLevel: p.RiskLevel, Summary: p.Summary, DetailsJSON: p.DetailsJSON, ExpiresAt: p.ExpiresAt, RequestedAt: p.RequestedAt})
	if err != nil {
		return Record{}, err
	}
	binding, _ := json.Marshal(b)
	_, err = tx.Exec(ctx, `INSERT INTO approval_actions(approval_id,action_id,action_hash,task_id,task_revision,constraints_revision,owner_id,channel,binding_json,continuation_version,continuation_json) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9::jsonb,1,$10::jsonb)`, record.ApprovalID, b.ActionID, b.ActionHash, b.TaskID, b.TaskRevision, b.ConstraintsRevision, b.OwnerID, p.RequestedVia, binding, p.ContinuationJSON)
	if err != nil {
		return Record{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE runs SET status='awaiting_approval',current_state='awaiting_approval',updated_at=clock_timestamp() WHERE run_id=$1`, p.RunID); err != nil {
		return Record{}, err
	}
	if b.TaskID != "" {
		if _, err = tx.Exec(ctx, `UPDATE tasks SET status='waiting_for_user',updated_at=clock_timestamp() WHERE task_id=$1`, b.TaskID); err != nil {
			return Record{}, err
		}
	}
	if err = r.checkpoints.SuspendApprovalTx(ctx, tx, p.RunID, record.ApprovalID, p.ContinuationJSON); err != nil {
		return Record{}, err
	}
	if err = r.InsertEvent(ctx, Event{ApprovalID: record.ApprovalID, RunID: p.RunID, SessionKey: p.SessionKey, EventType: "approval_requested", StatusAfter: StatusPending, ActorType: "system", ActorID: "orchestrator", Reason: "durable exact action prepared; execution suspended", MetadataJSON: `{"continuation_version":1}`}); err != nil {
		return Record{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO run_state_transitions(run_id,from_state,to_state,triggered_by,metadata_json,transitioned_at) VALUES($1,'tool_pending','awaiting_approval','orchestrator',$2::jsonb,clock_timestamp())`, p.RunID, fmt.Sprintf(`{"approval_id":%q,"durable":true}`, record.ApprovalID)); err != nil {
		return Record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s *Service) VerifyActionApproval(ctx context.Context, request actionauth.ApprovalRequest) (actionauth.ApprovalSnapshot, error) {
	repo, ok := s.repo.(*PostgresRepository)
	if !ok {
		return actionauth.ApprovalSnapshot{}, ErrActionInvalid
	}
	return repo.verifyAction(ctx, request)
}
func (r *PostgresRepository) verifyAction(ctx context.Context, request actionauth.ApprovalRequest) (actionauth.ApprovalSnapshot, error) {
	var snapshot actionauth.ApprovalSnapshot
	var binding []byte
	var currentRev, constraints, pinned int64
	var status, runState, owner, taskID string
	var validExpiry bool
	err := r.db(ctx).QueryRow(ctx, `SELECT a.approval_id,a.status,d.binding_json,a.expires_at,COALESCE(a.resolved_at,a.requested_at),COALESCE(a.resolved_via,''),a.expires_at>clock_timestamp(),s.user_id,r.current_state,COALESCE(r.task_id,''),COALESCE(r.task_revision,0),COALESCE(t.revision,0),COALESCE(t.constraints_revision,0),COALESCE(t.status,'')
 FROM approvals a JOIN approval_actions d USING(approval_id) JOIN runs r ON r.run_id=a.run_id JOIN sessions s ON s.session_key=a.session_key LEFT JOIN tasks t ON t.task_id=r.task_id WHERE a.approval_id=$1`, request.ApprovalID).Scan(&snapshot.ApprovalID, &snapshot.Status, &binding, &snapshot.ExpiresAt, &snapshot.ResolvedAt, &snapshot.ResolvedVia, &validExpiry, &owner, &runState, &taskID, &pinned, &currentRev, &constraints, &status)
	if err != nil {
		return snapshot, ErrActionInvalid
	}
	if json.Unmarshal(binding, &snapshot.Binding) != nil {
		return snapshot, ErrActionInvalid
	}
	b := snapshot.Binding
	if !reflect.DeepEqual(b, request.Binding) || !validExpiry || owner != b.OwnerID || taskID != b.TaskID || pinned != b.TaskRevision || (b.TaskID != "" && (currentRev != b.TaskRevision || constraints != b.ConstraintsRevision || (status != "active" && status != "waiting_for_user"))) || runState == "cancelled" || runState == "completed" || runState == "failed" || runState == "timed_out" {
		return snapshot, ErrActionInvalid
	}
	return snapshot, nil
}

func (r *PostgresRepository) ResolveDurable(ctx context.Context, record Record, approved bool, actor Actor, reason string) (Record, bool, error) {
	if actor.OwnerID == "" || actor.Channel == "" || actor.DeliveryID == "" {
		return Record{}, false, ErrUnauthorized
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Record{}, false, err
	}
	defer tx.Rollback(context.Background())
	ctx = context.WithValue(ctx, approvalTxKey{}, tx)
	var binding []byte
	var owner, channel, sessionKey, runState string
	var taskID string
	err = tx.QueryRow(ctx, `SELECT d.binding_json,d.owner_id,d.channel,a.session_key,r.current_state,COALESCE(d.task_id,'') FROM approval_actions d JOIN approvals a USING(approval_id) JOIN runs r ON r.run_id=a.run_id WHERE d.approval_id=$1 FOR UPDATE OF d,a,r`, record.ApprovalID).Scan(&binding, &owner, &channel, &sessionKey, &runState, &taskID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, false, ErrApprovalNotFound
	}
	if err != nil {
		return Record{}, false, err
	}
	if actor.OwnerID != owner || actor.Channel != channel || (channel == ResolvedViaTelegram && actor.SessionKey != sessionKey) {
		return Record{}, false, ErrUnauthorized
	}
	current, err := r.GetApprovalByID(ctx, record.ApprovalID)
	if err != nil {
		return Record{}, false, err
	}
	if current.Status != StatusPending {
		return current, false, nil
	}
	var b actionauth.ActionBinding
	if json.Unmarshal(binding, &b) != nil {
		return Record{}, false, ErrActionInvalid
	}
	snapshot, verificationErr := r.verifyAction(ctx, actionauth.ApprovalRequest{ApprovalID: record.ApprovalID, Binding: b})
	status := StatusRejected
	if approved {
		status = StatusApproved
	}
	if verificationErr != nil {
		status = StatusExpired
		reason = "action/task revision changed, cancelled or approval expired"
	}
	if runState != "awaiting_approval" {
		status = StatusExpired
		reason = "run no longer awaits this action"
	}
	decisionID := newApprovalID()
	tag, err := tx.Exec(ctx, `INSERT INTO approval_decision_inputs(decision_input_id,approval_id,source,delivery_id,owner_id,approved) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(source,delivery_id) DO NOTHING`, decisionID, record.ApprovalID, actor.Channel, actor.DeliveryID, actor.OwnerID, approved)
	if err != nil {
		return Record{}, false, err
	}
	if tag.RowsAffected() == 0 {
		return current, false, nil
	}
	_ = snapshot
	updated, err := r.ResolveApproval(ctx, ResolveParams{ApprovalID: record.ApprovalID, Status: status, ResolvedVia: actor.Channel, ResolvedBy: actor.OwnerID, ResolutionReason: reason, ResolvedAt: time.Now().UTC()})
	if err != nil {
		return Record{}, false, err
	}
	state := "ready"
	if status != StatusApproved {
		state = "blocked"
	}
	if _, err = tx.Exec(ctx, `UPDATE approval_actions SET state=$2,decision_input_id=$3,updated_at=clock_timestamp() WHERE approval_id=$1`, record.ApprovalID, state, decisionID); err != nil {
		return Record{}, false, err
	}
	if taskID != "" {
		taskStatus := "active"
		if status != StatusApproved {
			taskStatus = "blocked"
		}
		if _, err = tx.Exec(ctx, `UPDATE tasks SET status=$2,updated_at=clock_timestamp() WHERE task_id=$1 AND revision=$3 AND status='waiting_for_user'`, taskID, taskStatus, b.TaskRevision); err != nil {
			return Record{}, false, err
		}
	}
	if err = r.checkpoints.ReadyApprovalTx(ctx, tx, record.RunID, record.ApprovalID); err != nil {
		return Record{}, false, err
	}
	if err = r.InsertEvent(ctx, Event{ApprovalID: record.ApprovalID, RunID: record.RunID, SessionKey: record.SessionKey, EventType: "approval_resolved", StatusBefore: StatusPending, StatusAfter: status, ActorType: actor.Channel, ActorID: actor.OwnerID, Reason: reason, MetadataJSON: `{"durable_decision":true}`}); err != nil {
		return Record{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Record{}, false, err
	}
	return updated, true, nil
}
func (r *PostgresRepository) HasDurableAction(ctx context.Context, id string) (bool, error) {
	var yes bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM approval_actions WHERE approval_id=$1)`, id).Scan(&yes)
	return yes, err
}
func (s *Service) LoadContinuationByRun(ctx context.Context, runID string) (Record, string, error) {
	repo, ok := s.repo.(*PostgresRepository)
	if !ok {
		return Record{}, "", ErrActionInvalid
	}
	var id, payload string
	err := repo.pool.QueryRow(ctx, `SELECT d.approval_id,d.continuation_json::text FROM approval_actions d JOIN approvals a USING(approval_id) WHERE a.run_id=$1 ORDER BY a.requested_at DESC LIMIT 1`, runID).Scan(&id, &payload)
	if err != nil {
		return Record{}, "", err
	}
	record, err := repo.GetApprovalByID(ctx, id)
	return record, payload, err
}

// NormalizeTelegramOwner matches the numeric identity stored by ingress.
func NormalizeTelegramOwner(value string) string { return strings.TrimPrefix(value, "telegram_user:") }
