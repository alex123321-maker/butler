package workspaceexec

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/workspace"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrRecoveryConflict = errors.New("workspace recovery state is unavailable or changed")

// Public owning-control DTOs contain only opaque identity and current CAS.
// No executor/lease/fence/root/source body or caller receipt is accepted.
type DiscardRequest struct {
	WorkspaceID         string `json:"workspace_id"`
	ResourceVersion     int64  `json:"resource_version"`
	WorkspaceGeneration int64  `json:"workspace_generation"`
	TaskRevision        int64  `json:"task_revision"`
	ConstraintsRevision int64  `json:"constraints_revision"`
	PreviousRecoveryID  string `json:"previous_recovery_id,omitempty"`
}
type RecoveryStatus struct {
	WorkspaceID         string `json:"workspace_id"`
	TaskID              string `json:"task_id"`
	TaskRevision        int64  `json:"task_revision"`
	ConstraintsRevision int64  `json:"constraints_revision"`
	WorkspaceGeneration int64  `json:"workspace_generation"`
	ResourceVersion     int64  `json:"resource_version"`
	RecoveryID          string `json:"recovery_id,omitempty"`
	Phase               string `json:"phase"`
	CanDiscard          bool   `json:"can_discard"`
	OriginalOutcome     string `json:"original_outcome"`
}
type RecoveryController struct {
	pool    *pgxpool.Pool
	sources *catalog.Store
}

func NewRecoveryController(pool *pgxpool.Pool, sources *catalog.Store) (*RecoveryController, error) {
	if pool == nil || sources == nil || sources.Pool() != pool {
		return nil, ErrDenied
	}
	return &RecoveryController{pool, sources}, nil
}

type recoveryTarget struct {
	ticket, root                                  string
	source                                        SourceContext
	proof                                         resources.Proof
	generation, taskRevision, constraintsRevision int64
}
type recoveryRecord struct {
	id, owner, channel, delivery, phase, requestHash string
	req                                              DiscardRequest
	target                                           recoveryTarget
}

// Locks the same tree barrier as child admission and owner Stop, then its
// ordered ownership/session/task rows. A stopped row alone is not quiescence;
// Runtime separately proves the old physical inode and drains the copy lane.
func (c *RecoveryController) stoppedTree(ctx context.Context, tx pgx.Tx, t recoveryTarget, owner string) error {
	b := t.source.ref.Binding
	var root string
	if tx.QueryRow(ctx, `WITH RECURSIVE ancestors AS(SELECT task_id,parent_task_id FROM tasks WHERE task_id=$1 UNION SELECT t.task_id,t.parent_task_id FROM tasks t JOIN ancestors a ON t.task_id=a.parent_task_id) SELECT task_id FROM ancestors WHERE parent_task_id IS NULL`, b.TaskID).Scan(&root) != nil {
		return ErrDenied
	}
	// Shared validation excludes child admission/owner mutation (exclusive
	// tree barrier) while preserving independent per-copy recovery concurrency.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared(hashtextextended('butler-task-tree:'||$1,0))`, root); err != nil {
		return ErrDenied
	}
	rows, err := tx.Query(ctx, `WITH RECURSIVE tree AS(SELECT task_id,session_key FROM tasks WHERE task_id=$1 UNION SELECT t.task_id,t.session_key FROM tasks t JOIN tree b ON t.parent_task_id=b.task_id) SELECT DISTINCT session_key FROM tree ORDER BY session_key`, root)
	if err != nil {
		return ErrDenied
	}
	var sessions []string
	for rows.Next() {
		var key string
		if rows.Scan(&key) != nil {
			rows.Close()
			return ErrDenied
		}
		sessions = append(sessions, key)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(sessions) == 0 {
		return ErrDenied
	}
	for _, session := range sessions {
		var valid bool
		if tx.QueryRow(ctx, `SELECT lease_id='' OR expires_at<=clock_timestamp() FROM session_ownership WHERE session_key=$1 FOR SHARE`, session).Scan(&valid) != nil || !valid {
			return ErrDenied
		}
		if tx.QueryRow(ctx, `SELECT user_id=$2 FROM sessions WHERE session_key=$1 FOR SHARE`, session, owner).Scan(&valid) != nil || !valid {
			return ErrDenied
		}
	}
	rows, err = tx.Query(ctx, `WITH RECURSIVE tree AS(SELECT task_id FROM tasks WHERE task_id=$1 UNION SELECT t.task_id FROM tasks t JOIN tree b ON t.parent_task_id=b.task_id) SELECT t.owner_id=$2 AND t.status IN ('cancelled','completed') FROM tasks t JOIN tree USING(task_id) ORDER BY t.task_id FOR SHARE OF t`, root, owner)
	if err != nil {
		return ErrDenied
	}
	for rows.Next() {
		var valid bool
		if rows.Scan(&valid) != nil || !valid {
			rows.Close()
			return ErrDenied
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return ErrDenied
	}
	var valid bool
	if tx.QueryRow(ctx, `SELECT owner_id=$2 AND session_key=$3 AND revision=$4 AND constraints_revision=$5 FROM tasks WHERE task_id=$1`, b.TaskID, owner, b.SessionKey, t.taskRevision, t.constraintsRevision).Scan(&valid) != nil || !valid {
		return ErrRecoveryConflict
	}
	return nil
}
func (c *RecoveryController) target(ctx context.Context, tx pgx.Tx, id, owner string) (recoveryTarget, error) {
	var t recoveryTarget
	var raw []byte
	// The inflight lease selects one immutable old authorization, never a
	// caller-supplied run, lease or inferred filesystem name.
	err := tx.QueryRow(ctx, `SELECT d.ticket_id,x.root_identity_hash,a.source_context,d.resource_id,d.resource_lease_id,d.resource_version,d.resource_fence,t.revision,t.constraints_revision,COALESCE(w.generation,0) FROM execution_resources r JOIN code_workspace_dispatches d ON d.resource_id=r.resource_id AND d.resource_lease_id=r.inflight_lease_id JOIN code_workspace_authorizations a USING(authorization_id) JOIN code_workspace_executor_bindings x USING(ticket_id) JOIN tasks t ON t.task_id=a.source_context->>'task_id' LEFT JOIN code_workspaces w ON w.workspace_id=a.workspace_id WHERE r.resource_id=$1 AND r.owner_id=$2 AND a.workspace_id=$3 AND a.tool_name IN ('workspace.create','workspace.write') AND d.phase IN ('running','unknown')`, workspace.ResourceID(id), owner, id).Scan(&t.ticket, &t.root, &raw, &t.proof.ResourceID, &t.proof.LeaseID, &t.proof.ExpectedResourceVersion, &t.proof.FenceEpoch, &t.taskRevision, &t.constraintsRevision, &t.generation)
	if err != nil {
		return t, ErrRecoveryConflict
	}
	t.source, err = decodeContext(raw)
	if err != nil || t.source.ref.ID != id || t.source.ref.Binding.OwnerID != owner || !sha256Digest(t.root) {
		return t, ErrDenied
	}
	b := t.source.ref.Binding
	t.proof.RunID, t.proof.SessionKey, t.proof.TaskID = t.source.run, b.SessionKey, b.TaskID
	return t, nil
}
func recoveryHistoryTx(ctx context.Context, tx pgx.Tx) error {
	var intact bool
	if tx.QueryRow(ctx, `WITH expected(rel,name,fn,events) AS(VALUES ('code_workspace_executor_bindings','code_workspace_executor_bindings_immutable','butler_workspace_authorization_immutable()',27),('code_workspace_dispatches','code_workspace_dispatches_immutable','butler_workspace_dispatch_immutable()',27),('code_workspace_recoveries','code_workspace_recoveries_immutable','butler_workspace_recovery_immutable()',27),('code_workspace_erasure_receipts','code_workspace_erasure_receipts_immutable','butler_workspace_authorization_immutable()',27)) SELECT NOT EXISTS(SELECT 1 FROM expected e WHERE NOT EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=to_regclass(e.rel) AND t.tgname=e.name AND t.tgfoid=to_regprocedure(e.fn) AND t.tgtype=e.events AND t.tgenabled IN ('O','A') AND NOT t.tgisinternal))`).Scan(&intact) != nil || !intact {
		return ErrDenied
	}
	return nil
}
func (c *RecoveryController) current(ctx context.Context, tx pgx.Tx, t recoveryTarget, owner string) error {
	if recoveryHistoryTx(ctx, tx) != nil {
		return ErrDenied
	}
	if c == nil || c.sources.CheckRestoreTx(ctx, tx) != nil || c.stoppedTree(ctx, tx, t, owner) != nil {
		return ErrDenied
	}
	// Owning source access checks the frozen complete original graph and privacy
	// anchor. Results stay private; no body is stored in recovery or exported.
	if _, err := c.sources.AccessTx(ctx, tx, t.source.parents, "memory_read"); err != nil {
		return ErrDenied
	}
	b := t.source.ref.Binding
	var unknown bool
	if tx.QueryRow(ctx, `SELECT d.phase IN ('running','unknown') AND e.authorization_id=d.authorization_id AND e.state IN ('executing','effect_unknown') FROM code_workspace_dispatches d JOIN action_effects e ON e.action_id=d.effect_id WHERE d.ticket_id=$1 FOR SHARE OF d,e`, t.ticket).Scan(&unknown) != nil || !unknown {
		return ErrRecoveryConflict
	}
	var exact bool
	var generation int64
	err := tx.QueryRow(ctx, `SELECT generation,owner_id=$2 AND session_key=$3 AND task_id=$4 AND task_revision=$5 AND constraints_revision=$6 AND blocked_reason<>'owner_discarded' FROM code_workspaces WHERE workspace_id=$1 FOR UPDATE`, t.source.ref.ID, b.OwnerID, b.SessionKey, b.TaskID, b.TaskRevision, b.ConstraintsRevision).Scan(&generation, &exact)
	if errors.Is(err, pgx.ErrNoRows) {
		if t.generation != 0 {
			return ErrRecoveryConflict
		}
	} else if err != nil || !exact || generation != t.generation {
		return ErrRecoveryConflict
	}
	if tx.QueryRow(ctx, `SELECT r.owner_id=$2 AND r.version=$3 AND r.fence_epoch=$4 AND r.inflight_lease_id=$5 AND l.owner_id=$2 AND l.resource_id=r.resource_id AND l.run_id=$6 AND l.session_key=$7 AND l.task_id=$8 AND l.task_revision=$9 AND l.constraints_revision=$10 AND l.resource_version=$3 AND l.fence_epoch=$4 AND l.mode='write' AND l.state IN ('claimed','unknown') AND l.expires_at<=clock_timestamp() FROM execution_resources r JOIN execution_resource_leases l ON l.lease_id=$5 WHERE r.resource_id=$1 FOR UPDATE OF r,l`, t.proof.ResourceID, owner, t.proof.ExpectedResourceVersion, t.proof.FenceEpoch, t.proof.LeaseID, t.proof.RunID, b.SessionKey, b.TaskID, b.TaskRevision, b.ConstraintsRevision).Scan(&exact) != nil || !exact {
		return ErrRecoveryConflict
	}
	// Fresh time after all waits; expiry alone never resolves the inflight copy.
	if tx.QueryRow(ctx, `SELECT expires_at<=clock_timestamp() FROM execution_resource_leases WHERE lease_id=$1`, t.proof.LeaseID).Scan(&exact) != nil || !exact {
		return ErrRecoveryConflict
	}
	if c.sources.CheckRestoreTx(ctx, tx) != nil {
		return ErrDenied
	}
	return nil
}
func statusFor(t recoveryTarget) RecoveryStatus {
	return RecoveryStatus{WorkspaceID: t.source.ref.ID, TaskID: t.source.ref.Binding.TaskID, TaskRevision: t.taskRevision, ConstraintsRevision: t.constraintsRevision, WorkspaceGeneration: t.generation, ResourceVersion: t.proof.ExpectedResourceVersion, Phase: "quarantined", OriginalOutcome: "unknown"}
}
func (c *RecoveryController) latest(ctx context.Context, tx pgx.Tx, id, owner string) (recoveryRecord, error) {
	var r recoveryRecord
	err := tx.QueryRow(ctx, `SELECT recovery_id,owner_id,channel,delivery_id,phase,request_hash,workspace_id,resource_version,workspace_generation,task_revision,constraints_revision,previous_recovery_id FROM code_workspace_recoveries WHERE workspace_id=$1 AND owner_id=$2 ORDER BY created_at DESC,recovery_id DESC LIMIT 1`, id, owner).Scan(&r.id, &r.owner, &r.channel, &r.delivery, &r.phase, &r.requestHash, &r.req.WorkspaceID, &r.req.ResourceVersion, &r.req.WorkspaceGeneration, &r.req.TaskRevision, &r.req.ConstraintsRevision, &r.req.PreviousRecoveryID)
	return r, err
}
func (c *RecoveryController) Get(ctx context.Context, id string) (RecoveryStatus, error) {
	ctx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || c == nil || !opaqueID(id) {
		return RecoveryStatus{}, ErrDenied
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return RecoveryStatus{}, ErrRecoveryConflict
	}
	defer tx.Rollback(context.Background())
	latest, latestErr := c.latest(ctx, tx, id, actor.OwnerID)
	if latestErr == nil && latest.phase == "discarded" {
		// Terminal history still requires current exact owner/task and privacy anchor.
		t, err := c.recordTarget(ctx, tx, latest)
		if err != nil || latest.requestHash != requestDigest(latest.req) || recoveryHistoryTx(ctx, tx) != nil || c.stoppedTree(ctx, tx, t, actor.OwnerID) != nil || c.sources.CheckRestoreTx(ctx, tx) != nil {
			return RecoveryStatus{}, ErrDenied
		}
		digest := sha256.Sum256([]byte("workspace-erased:" + id))
		var proved bool
		if tx.QueryRow(ctx, `SELECT e.root_identity_hash=$2 AND e.observation_hash=$3 AND r.owner_id=$4 AND r.version=$5 AND r.fence_epoch=$6 AND r.inflight_lease_id='' AND l.state='reconciled' AND l.evidence_id=$7 AND w.owner_id=$4 AND w.generation=$8 AND w.state='blocked' AND w.blocked_reason='owner_discarded' FROM code_workspace_erasure_receipts e JOIN execution_resources r ON r.resource_id=$9 JOIN execution_resource_leases l ON l.lease_id=$10 JOIN code_workspaces w ON w.workspace_id=$11 WHERE e.recovery_id=$1 FOR SHARE OF e,r,l,w`, latest.id, t.root, hex.EncodeToString(digest[:]), actor.OwnerID, t.proof.ExpectedResourceVersion+1, t.proof.FenceEpoch+1, "workspace-discard:"+latest.id, t.generation+1, t.proof.ResourceID, t.proof.LeaseID, id).Scan(&proved) != nil || !proved {
			return RecoveryStatus{}, ErrRecoveryConflict
		}
		s := statusFor(t)
		s.ResourceVersion++
		s.WorkspaceGeneration++
		s.RecoveryID = latest.id
		s.Phase = "discarded"
		if tx.Commit(ctx) != nil {
			return RecoveryStatus{}, ErrRecoveryConflict
		}
		return s, nil
	}
	t, err := c.target(ctx, tx, id, actor.OwnerID)
	if err != nil {
		return RecoveryStatus{}, ErrDenied
	}
	s := statusFor(t)
	if latestErr == nil {
		s.RecoveryID = latest.id
		s.Phase = latest.phase
	}
	s.CanDiscard = c.current(ctx, tx, t, actor.OwnerID) == nil && s.Phase != "requested"
	if tx.Commit(ctx) != nil {
		return RecoveryStatus{}, ErrRecoveryConflict
	}
	return s, nil
}
func requestDigest(req DiscardRequest) string {
	raw, _ := json.Marshal(req)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ListForTask lets the authenticated owner discover opaque unknown-Create IDs
// that were intentionally absent from a failed tool result. This is metadata
// observation only. An incomplete/missing executor proof remains unavailable;
// it cannot manufacture CanDiscard or reveal a private authorization/lease.
func (c *RecoveryController) ListForTask(ctx context.Context, taskID string) ([]RecoveryStatus, error) {
	ctx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || c == nil || taskID == "" || len(taskID) > 256 {
		return nil, ErrDenied
	}
	var own bool
	if c.pool.QueryRow(ctx, `SELECT owner_id=$2 FROM tasks WHERE task_id=$1`, taskID, actor.OwnerID).Scan(&own) != nil || !own {
		return nil, ErrDenied
	}
	rows, err := c.pool.Query(ctx, `WITH copies AS(SELECT DISTINCT ON(a.workspace_id) a.workspace_id,t.task_id,t.revision,t.constraints_revision,COALESCE(w.generation,0) AS workspace_generation,r.version AS resource_version,COALESCE(q.recovery_id,'') AS recovery_id,COALESCE(q.phase,'quarantined') AS phase,r.inflight_lease_id<>'' AS unresolved FROM code_workspace_authorizations a JOIN code_workspace_dispatches d USING(authorization_id) JOIN execution_resources r USING(resource_id) JOIN tasks t ON t.task_id=a.source_context->>'task_id' LEFT JOIN code_workspaces w ON w.workspace_id=a.workspace_id LEFT JOIN LATERAL(SELECT recovery_id,phase FROM code_workspace_recoveries WHERE workspace_id=a.workspace_id AND owner_id=$2 ORDER BY created_at DESC,recovery_id DESC LIMIT 1) q ON TRUE WHERE t.task_id=$1 AND t.owner_id=$2 AND r.owner_id=$2 AND a.source_context->>'owner_id'=$2 AND (r.inflight_lease_id<>'' OR q.recovery_id IS NOT NULL) ORDER BY a.workspace_id,d.created_at DESC) SELECT workspace_id,task_id,revision,constraints_revision,workspace_generation,resource_version,recovery_id,phase FROM copies ORDER BY unresolved DESC,workspace_id LIMIT 50`, taskID, actor.OwnerID)
	if err != nil {
		return nil, ErrRecoveryConflict
	}
	result := []RecoveryStatus{}
	for rows.Next() {
		var s RecoveryStatus
		if rows.Scan(&s.WorkspaceID, &s.TaskID, &s.TaskRevision, &s.ConstraintsRevision, &s.WorkspaceGeneration, &s.ResourceVersion, &s.RecoveryID, &s.Phase) != nil {
			rows.Close()
			return nil, ErrRecoveryConflict
		}
		s.OriginalOutcome = "unknown"
		// A terminal enum alone is insufficient without the independent receipt
		// and exact retired resource proof checked below by Get.
		if s.Phase == "discarded" {
			s.Phase = "quarantined"
		}
		result = append(result, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, ErrRecoveryConflict
	}
	// Release the listing connection before calling any current-proof reader.
	for i, s := range result {
		if current, err := c.Get(ctx, s.WorkspaceID); err == nil {
			result[i] = current
		}
	}
	return result, nil
}
func (c *RecoveryController) RequestDiscard(ctx context.Context, req DiscardRequest) (RecoveryStatus, error) {
	ctx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || c == nil || !opaqueID(req.WorkspaceID) || req.ResourceVersion < 1 || req.WorkspaceGeneration < 0 || req.TaskRevision < 1 || req.ConstraintsRevision < 1 || req.PreviousRecoveryID != "" && !opaqueID(req.PreviousRecoveryID) || len(actor.DeliveryID) > 256 {
		return RecoveryStatus{}, ErrDenied
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return RecoveryStatus{}, ErrRecoveryConflict
	}
	defer tx.Rollback(context.Background())
	var previousID, previousHash, previousPhase string
	err = tx.QueryRow(ctx, `SELECT recovery_id,request_hash,phase FROM code_workspace_recoveries WHERE owner_id=$1 AND channel=$2 AND delivery_id=$3`, actor.OwnerID, actor.Channel, actor.DeliveryID).Scan(&previousID, &previousHash, &previousPhase)
	if err == nil {
		if previousHash != requestDigest(req) {
			return RecoveryStatus{}, ErrRecoveryConflict
		}
		// No physical replay: current observation is read after releasing this TX.
		if tx.Commit(ctx) != nil {
			return RecoveryStatus{}, ErrRecoveryConflict
		}
		return c.Get(ctx, req.WorkspaceID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return RecoveryStatus{}, ErrRecoveryConflict
	}
	t, err := c.target(ctx, tx, req.WorkspaceID, actor.OwnerID)
	if err != nil {
		return RecoveryStatus{}, ErrRecoveryConflict
	}
	if t.proof.ExpectedResourceVersion != req.ResourceVersion || t.generation != req.WorkspaceGeneration || t.taskRevision != req.TaskRevision || t.constraintsRevision != req.ConstraintsRevision {
		return RecoveryStatus{}, ErrRecoveryConflict
	}
	if err = c.current(ctx, tx, t, actor.OwnerID); err != nil {
		return RecoveryStatus{}, err
	}
	// A concurrent identical delivery may have committed while resource rows
	// were awaited. Return only its current observation, with no second intent.
	err = tx.QueryRow(ctx, `SELECT request_hash FROM code_workspace_recoveries WHERE owner_id=$1 AND channel=$2 AND delivery_id=$3`, actor.OwnerID, actor.Channel, actor.DeliveryID).Scan(&previousHash)
	if err == nil {
		if previousHash != requestDigest(req) {
			return RecoveryStatus{}, ErrRecoveryConflict
		}
		if tx.Commit(ctx) != nil {
			return RecoveryStatus{}, ErrRecoveryConflict
		}
		return c.Get(ctx, req.WorkspaceID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return RecoveryStatus{}, ErrRecoveryConflict
	}
	latest, err := c.latest(ctx, tx, req.WorkspaceID, actor.OwnerID)
	if err == nil {
		if latest.id != req.PreviousRecoveryID || latest.phase != "erasing" {
			return RecoveryStatus{}, ErrRecoveryConflict
		}
		if _, err = tx.Exec(ctx, `UPDATE code_workspace_recoveries SET phase='superseded',finished_at=clock_timestamp() WHERE recovery_id=$1 AND phase='erasing'`, latest.id); err != nil {
			return RecoveryStatus{}, ErrRecoveryConflict
		}
	} else if !errors.Is(err, pgx.ErrNoRows) || req.PreviousRecoveryID != "" {
		return RecoveryStatus{}, ErrRecoveryConflict
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return RecoveryStatus{}, ErrRecoveryConflict
	}
	id := hex.EncodeToString(random[:])
	if _, err = tx.Exec(ctx, `INSERT INTO code_workspace_recoveries(recovery_id,owner_id,channel,delivery_id,request_hash,workspace_id,ticket_id,resource_version,workspace_generation,task_revision,constraints_revision,previous_recovery_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, actor.OwnerID, actor.Channel, actor.DeliveryID, requestDigest(req), req.WorkspaceID, t.ticket, req.ResourceVersion, req.WorkspaceGeneration, req.TaskRevision, req.ConstraintsRevision, req.PreviousRecoveryID); err != nil {
		return RecoveryStatus{}, ErrRecoveryConflict
	}
	if tx.Commit(ctx) != nil {
		return RecoveryStatus{}, ErrRecoveryConflict
	}
	s := statusFor(t)
	s.RecoveryID = id
	s.Phase = "requested"
	return s, nil
}
func (c *RecoveryController) recordTarget(ctx context.Context, tx pgx.Tx, r recoveryRecord) (recoveryTarget, error) {
	var t recoveryTarget
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT d.ticket_id,x.root_identity_hash,a.source_context,d.resource_id,d.resource_lease_id,d.resource_version,d.resource_fence FROM code_workspace_recoveries q JOIN code_workspace_dispatches d USING(ticket_id) JOIN code_workspace_executor_bindings x USING(ticket_id) JOIN code_workspace_authorizations a USING(authorization_id) WHERE q.recovery_id=$1 AND q.owner_id=$2`, r.id, r.owner).Scan(&t.ticket, &t.root, &raw, &t.proof.ResourceID, &t.proof.LeaseID, &t.proof.ExpectedResourceVersion, &t.proof.FenceEpoch)
	if err != nil {
		return t, ErrRecoveryConflict
	}
	t.source, err = decodeContext(raw)
	if err != nil || t.source.ref.Binding.OwnerID != r.owner || t.source.ref.ID != r.req.WorkspaceID || t.proof.ExpectedResourceVersion != r.req.ResourceVersion {
		return t, ErrDenied
	}
	b := t.source.ref.Binding
	t.proof.RunID, t.proof.SessionKey, t.proof.TaskID = t.source.run, b.SessionKey, b.TaskID
	t.generation, t.taskRevision, t.constraintsRevision = r.req.WorkspaceGeneration, r.req.TaskRevision, r.req.ConstraintsRevision
	return t, nil
}
