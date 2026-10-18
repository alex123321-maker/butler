package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

// ModelPolicyRequest is an explicit, finite owner permission for exact source
// identities. Source changes may change content, never access or ancestry.
// Handler is a code-owned, versioned consumer that must pass its own gates.
type ModelPolicyRequest struct {
	ID              string            `json:"policy_id"`
	Handler         string            `json:"handler"`
	Trigger         string            `json:"trigger"`
	IntervalSeconds int               `json:"interval_seconds"`
	MaxJobs         int               `json:"max_jobs"`
	Privacy         string            `json:"privacy"`
	Job             Request           `json:"job"`
	Contexts        []ModelContextRef `json:"contexts,omitempty"`
}
type ModelPolicy struct {
	ModelPolicyRequest
	OwnerID             string                 `json:"owner_id"`
	TaskRevision        int64                  `json:"task_revision"`
	ConstraintsRevision int64                  `json:"constraints_revision"`
	Authority           actionauth.Assignment  `json:"authority"`
	ContextPins         []ModelContextPin      `json:"context_pins,omitempty"`
	TerminalExperience  *terminalExperiencePin `json:"terminal_experience,omitempty"`
}

var ErrWaiting = errors.New("background memory admission is waiting")

type waitReason struct{ reason string }

func (e *waitReason) Error() string        { return e.reason }
func (e *waitReason) Is(target error) bool { return target == ErrWaiting }

func validAllowance(u budget.Units) bool {
	return u.ModelTurns > 0 && u.ModelTurns <= 128 && u.Tokens > 0 && u.Tokens <= 1<<32 && u.CostMicros >= 0 && u.CostMicros <= 1<<40 && u.ActiveMillis > 0 && u.ActiveMillis <= 86400000 && u.ProviderInputBytes > 0 && u.ProviderInputBytes <= 1<<32 && u.ExternalCalls == 0 && u.EgressBytes == 0 && u.Children == 0
}

func (s *Store) CreateModelPolicy(ctx context.Context, q ModelPolicyRequest) (ModelPolicy, error) {
	if _, ok := owneridentity.FromContext(ctx); !ok {
		return ModelPolicy{}, ErrClaim
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ModelPolicy{}, err
	}
	defer tx.Rollback(ctx)
	p, err := s.CreateModelPolicyTx(ctx, tx, q)
	if err != nil {
		return p, err
	}
	return p, tx.Commit(ctx)
}

// CreateModelPolicyTx is the same owner-only finite policy admission in a caller
// transaction. A trusted snapshot adapter can commit its source/pins and policy
// atomically; it acquires domain/authority locks before creating source rows.
func (s *Store) CreateModelPolicyTx(ctx context.Context, tx pgx.Tx, q ModelPolicyRequest) (ModelPolicy, error) {
	return s.createModelPolicyTx(ctx, tx, q, nil)
}

func (s *Store) createModelPolicyTx(ctx context.Context, tx pgx.Tx, q ModelPolicyRequest, terminal *terminalExperiencePin) (ModelPolicy, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || tx == nil || len(q.ID) < 8 || len(q.ID) > 128 || len(q.Handler) < 1 || len(q.Handler) > 128 || q.MaxJobs < 1 || q.MaxJobs > 32 || q.IntervalSeconds < 60 || q.IntervalSeconds > 86400 || (q.Privacy != "cloud_allowed" && q.Privacy != "local_only") || !validAllowance(q.Job.Allowance) || len(q.Job.Sources) > 64 {
		return ModelPolicy{}, ErrClaim
	}
	switch q.Trigger {
	case "source_changed", "session_completed", "session_inactive", "schedule":
	default:
		return ModelPolicy{}, ErrClaim
	}
	if (q.Trigger == "session_completed" || q.Trigger == "session_inactive") && q.Job.OriginalRunID == "" {
		return ModelPolicy{}, ErrClaim
	}
	contexts, contextErr := orderedContextRefs(q.Contexts)
	if contextErr != nil || len(contexts) > 0 && (q.Handler != ConsolidationHandler || q.Job.TaskID != "" || q.Job.OriginalRunID != "" || q.Privacy != "cloud_allowed") {
		return ModelPolicy{}, ErrClaim
	}
	q.Contexts = contexts
	q.Job.ID = ""
	q.Job.Kind = "model"
	q.Job.Purpose = "memory_model"
	q.Job.Deadline = q.Job.Deadline.UTC().Truncate(time.Microsecond)
	if err := s.catalog.CheckRestoreTx(ctx, tx); err != nil {
		return ModelPolicy{}, err
	}
	var e error
	var now time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return ModelPolicy{}, e
	}
	if !q.Job.Deadline.After(now) || q.Job.Deadline.After(now.Add(7*24*time.Hour)) {
		return ModelPolicy{}, ErrClaim
	}
	p := ModelPolicy{ModelPolicyRequest: q, OwnerID: actor.OwnerID, TerminalExperience: terminal}
	initial, e := s.catalog.CanonicalSourcesTx(ctx, tx, q.Job.Sources)
	if e != nil {
		return p, e
	}
	p.Job.Sources = initial
	if e = lockPolicySessionsTx(ctx, tx, p); e != nil {
		return p, e
	}
	if e = lockContextTasksTx(ctx, tx, p.Contexts); e != nil {
		return p, e
	}
	for _, ref := range p.Contexts {
		pin, err := resolveContextPinTx(ctx, tx, p.OwnerID, ref, p.IntervalSeconds)
		if err != nil {
			return p, err
		}
		p.ContextPins = append(p.ContextPins, pin)
	}
	if e = validContextShape(p); e != nil {
		return p, e
	}
	if q.Job.TaskID != "" {
		var owner, status, parent string
		e = tx.QueryRow(ctx, `SELECT owner_id,status,COALESCE(parent_task_id,''),revision,constraints_revision FROM tasks WHERE task_id=$1 FOR SHARE`, q.Job.TaskID).Scan(&owner, &status, &parent, &p.TaskRevision, &p.ConstraintsRevision)
		if e != nil || owner != p.OwnerID || parent != "" {
			return p, ErrClaim
		}
		if e = s.checkExperienceTerminalTaskTx(ctx, tx, p, status); e != nil {
			return p, e
		}
		p.Authority, e = actionauth.CheckTaskAuthorityTx(ctx, tx, q.Job.TaskID, p.OwnerID, nil, now)
		if e != nil {
			return p, e
		}
	}
	if q.Job.OriginalRunID != "" {
		var owner, task string
		e = tx.QueryRow(ctx, `SELECT s.user_id,COALESCE(r.task_id,'') FROM runs r JOIN sessions s USING(session_key) WHERE r.run_id=$1`, q.Job.OriginalRunID).Scan(&owner, &task)
		if e != nil || owner != p.OwnerID || task != q.Job.TaskID {
			return p, ErrClaim
		}
	}
	refs, e := s.catalog.CanonicalSourcesTx(ctx, tx, q.Job.Sources)
	if e != nil {
		return p, e
	}
	sources, e := s.catalog.AccessTx(ctx, tx, refs, "memory_job_create")
	if e != nil {
		return p, e
	}
	if e = sourcePolicyPrivacyTx(ctx, tx, p, sources); e != nil {
		return p, e
	}
	refs = nil
	for _, src := range sources {
		refs = append(refs, catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash})
	}
	p.Job.Sources, _, e = canonical(refs)
	if e != nil || len(refs) > 64 {
		return p, ErrClaim
	}
	if e = s.checkPolicyTaskTx(ctx, tx, p); e != nil {
		return p, e
	}
	// Finite global authorization registry, serialized independently of claims.
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(820071)`); e != nil {
		return p, e
	}
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM memory_model_policies WHERE state<>'revoked' AND (policy->'job'->>'deadline')::timestamptz>clock_timestamp()`).Scan(&count); e != nil {
		return p, e
	}
	var old []byte
	e = tx.QueryRow(ctx, `SELECT policy FROM memory_model_policies WHERE policy_id=$1`, q.ID).Scan(&old)
	if e == nil {
		var previous ModelPolicy
		if json.Unmarshal(old, &previous) != nil || string(jsonBytes(previous)) != string(jsonBytes(p)) {
			return p, ErrClaim
		}
		return previous, nil
	}
	if e != pgx.ErrNoRows {
		return p, e
	}
	if count >= 128 {
		return p, ErrWaiting
	}
	_, e = tx.Exec(ctx, `INSERT INTO memory_model_policies(policy_id,owner_id,policy) VALUES($1,$2,$3)`, p.ID, p.OwnerID, jsonBytes(p))
	if e != nil {
		return p, e
	}
	return p, nil
}

func readPolicy(ctx context.Context, tx pgx.Tx, id string, lock bool) (ModelPolicy, string, error) {
	var p ModelPolicy
	var raw []byte
	var state string
	clause := ""
	if lock {
		clause = " FOR SHARE"
	}
	e := tx.QueryRow(ctx, `SELECT policy,state FROM memory_model_policies WHERE policy_id=$1`+clause, id).Scan(&raw, &state)
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return p, state, ErrClaim
		}
		return p, state, e
	}
	if json.Unmarshal(raw, &p) != nil || p.ID != id {
		return p, state, ErrClaim
	}
	return p, state, nil
}
func (s *Store) checkPolicyTaskTx(ctx context.Context, tx pgx.Tx, p ModelPolicy) error {
	if err := validContextShape(p); err != nil {
		return err
	}
	if e := lockPolicySessionsTx(ctx, tx, p); e != nil {
		return e
	}
	var now time.Time
	if e := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return e
	}
	if !p.Job.Deadline.After(now) {
		return ErrClaim
	}
	if e := checkContextPinsTx(ctx, tx, p.OwnerID, p.ContextPins); e != nil {
		return e
	}
	if p.Job.TaskID == "" {
		if p.TerminalExperience != nil {
			return ErrClaim
		}
		return nil
	}
	var owner, status, parent string
	var rev, cr int64
	if e := tx.QueryRow(ctx, `SELECT owner_id,status,COALESCE(parent_task_id,''),revision,constraints_revision FROM tasks WHERE task_id=$1 FOR SHARE`, p.Job.TaskID).Scan(&owner, &status, &parent, &rev, &cr); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrClaim
		}
		return e
	}
	if owner != p.OwnerID || parent != "" || rev != p.TaskRevision || cr != p.ConstraintsRevision {
		return ErrClaim
	}
	if e := s.checkExperienceTerminalTaskTx(ctx, tx, p, status); e != nil {
		return e
	}
	if p.Authority.MandateID != "" {
		var raw []byte
		if e := tx.QueryRow(ctx, `SELECT mandate_data FROM action_mandates WHERE mandate_id=$1 FOR SHARE`, p.Authority.MandateID).Scan(&raw); e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				return ErrClaim
			}
			return e
		}
		var m actionauth.Mandate
		if json.Unmarshal(raw, &m) != nil {
			return ErrClaim
		}
		if !actionauth.WithinActiveHours(m.Hours, now) {
			return ErrWaiting
		}
	}
	a, e := actionauth.CheckTaskAuthorityTx(ctx, tx, p.Job.TaskID, p.OwnerID, &p.Authority, now)
	if e != nil {
		return e
	}
	if a != p.Authority {
		return ErrClaim
	}
	if a.TaskID != "" {
		m, _, e := actionauth.CheckAssignmentTx(ctx, tx, a, now)
		if e != nil {
			return e
		}
		if p.Privacy == "cloud_allowed" && m.Privacy != "cloud_allowed" {
			return ErrClaim
		}
	}
	return nil
}
func (s *Store) checkModelPolicyTx(ctx context.Context, tx pgx.Tx, j Job) (ModelPolicy, error) {
	p, _, e := readPolicy(ctx, tx, j.PolicyID, false)
	if e != nil {
		return p, e
	}
	if p.OwnerID != j.OwnerID || p.Job.TaskID != j.TaskID || p.Job.OriginalRunID != j.OriginalRunID || p.TaskRevision != j.TaskRevision || p.ConstraintsRevision != j.ConstraintsRevision || p.Authority != j.Authority || p.Job.Allowance != j.Allowance || !p.Job.Deadline.Equal(j.Deadline) || j.Kind != "model" || string(jsonBytes(p.ContextPins)) != string(jsonBytes(j.Contexts)) {
		return p, ErrClaim
	}
	if e = s.checkPolicyTaskTx(ctx, tx, p); e != nil {
		return p, e
	}
	var control string
	if e := tx.QueryRow(ctx, `SELECT state FROM memory_model_control WHERE singleton FOR SHARE`).Scan(&control); e != nil {
		return p, e
	}
	if control != "enabled" {
		return p, ErrWaiting
	}
	_, state, e := readPolicy(ctx, tx, p.ID, true)
	if e != nil {
		return p, e
	}
	if state != "active" {
		return p, ErrWaiting
	}
	return p, nil
}

// PollModelPolicies is a bounded scan in the existing worker loop. It neither
// creates another scheduler nor participates in user-run completion.
func (s *Store) PollModelPolicies(ctx context.Context, enabled func(string) bool, preparers ...map[string]ModelScheduleHandler) error {
	if e := s.reapModels(ctx); e != nil {
		return e
	}
	rows, e := s.pool.Query(ctx, `SELECT policy_id,policy->>'handler' FROM memory_model_policies WHERE state='active' AND next_event_at<=clock_timestamp() ORDER BY next_event_at,policy_id LIMIT 32`)
	if e != nil {
		return e
	}
	type item struct{ id, handler string }
	var ids []item
	for rows.Next() {
		var i item
		if e = rows.Scan(&i.id, &i.handler); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, i)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, i := range ids {
		if !enabled(i.handler) {
			_, _ = s.pool.Exec(ctx, `UPDATE memory_model_policies SET reason='consumer_disabled',next_event_at=clock_timestamp()+INTERVAL '10 seconds' WHERE policy_id=$1`, i.id)
			continue
		}
		var prepare ModelScheduleHandler
		if len(preparers) > 0 {
			prepare = preparers[0][i.handler]
		}
		if e = s.scheduleModelPolicy(ctx, i.id, prepare); e != nil {
			return e
		}
	}
	return nil
}
func (s *Store) scheduleModelPolicy(ctx context.Context, id string, preparers ...ModelScheduleHandler) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	p, _, e := readPolicy(ctx, tx, id, false)
	if e != nil {
		return e
	}
	if e = s.checkPolicyTaskTx(ctx, tx, p); e != nil {
		state, reason := "active", "authority_window"
		if !errors.Is(e, ErrWaiting) {
			if !errors.Is(e, ErrClaim) && !errors.Is(e, actionauth.ErrOwnerAuthority) {
				return e
			}
			state = "revoked"
			reason = "authority_stale"
		}
		_, e = tx.Exec(ctx, `UPDATE memory_model_policies SET state=$2,reason=$3,next_event_at=clock_timestamp()+INTERVAL '10 seconds' WHERE policy_id=$1`, id, state, reason)
		if e != nil {
			return e
		}
		return tx.Commit(ctx)
	}
	var control string
	if e = tx.QueryRow(ctx, `SELECT state FROM memory_model_control WHERE singleton FOR SHARE`).Scan(&control); e != nil {
		return e
	}
	var state, last string
	var created int
	if e = tx.QueryRow(ctx, `SELECT state,last_event,jobs_created FROM memory_model_policies WHERE policy_id=$1 FOR UPDATE`, id).Scan(&state, &last, &created); e != nil {
		return e
	}
	if state != "active" {
		return tx.Commit(ctx)
	}
	wait := func(reason string) error {
		_, e := tx.Exec(ctx, `UPDATE memory_model_policies SET reason=$2,next_event_at=clock_timestamp()+INTERVAL '10 seconds' WHERE policy_id=$1`, id, reason)
		if e != nil {
			return e
		}
		return tx.Commit(ctx)
	}
	if control != "enabled" {
		return wait("worker_" + control)
	}
	if created >= p.MaxJobs {
		return wait("event_limit")
	}
	busy, e := interactiveTx(ctx, tx)
	if e != nil {
		return e
	}
	if busy {
		return wait("interactive_priority")
	}
	if p.Trigger == "session_completed" || p.Trigger == "session_inactive" {
		var eligible bool
		e = tx.QueryRow(ctx, `SELECT r.current_state='completed' AND NOT EXISTS(SELECT 1 FROM runs x WHERE x.session_key=r.session_key AND x.current_state NOT IN ('completed','failed','cancelled','timed_out')) AND ($2='session_completed' OR r.updated_at<=clock_timestamp()-$3*INTERVAL '1 second') FROM runs r WHERE r.run_id=$1`, p.Job.OriginalRunID, p.Trigger, p.IntervalSeconds).Scan(&eligible)
		if e != nil {
			return e
		}
		if !eligible {
			return wait("session_active")
		}
	}
	// Owner policy permits these exact stable IDs, with unchanged access rights.
	ownerCtx := catalog.WithAccessChecker(owneridentity.WithActor(ctx, owneridentity.Actor{}), &policyChecker{s: s, p: p})
	if len(preparers) > 0 {
		if refresher, ok := preparers[0].(ModelPolicyRefreshHandler); ok {
			reason, err := refresher.RefreshPolicySourcesTx(ownerCtx, tx, s.catalog, p)
			if err != nil {
				return err
			}
			if reason != "" {
				return wait(reason)
			}
		}
	}
	refs := make([]catalog.SourceRef, len(p.Job.Sources))
	for i, r := range p.Job.Sources {
		refs[i] = catalog.SourceRef{ID: r.ID, AccessRevision: r.AccessRevision}
	}
	sources, e := s.catalog.AccessTx(ownerCtx, tx, refs, "memory_job_create")
	if e != nil {
		return wait("source_unavailable")
	}
	if e = sourcePolicyPrivacyTx(ctx, tx, p, sources); e != nil {
		return wait("source_privacy")
	}
	if len(preparers) > 0 && preparers[0] != nil {
		prepared, reason, err := preparers[0].PrepareSourcesTx(ownerCtx, tx, s.catalog, p, sources)
		if err != nil {
			return err
		}
		if reason != "" {
			return wait(reason)
		}
		if len(prepared) != len(sources) {
			return ErrClaim
		}
		for i, source := range prepared {
			if source.ID != sources[i].ID || source.AccessRevision != sources[i].AccessRevision || source.OwnerID != p.OwnerID {
				return ErrClaim
			}
		}
		sources = prepared
	}
	current := make([]catalog.SourceRef, 0, len(sources))
	for _, src := range sources {
		current = append(current, catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash})
	}
	current, sum, e := canonical(current)
	if e != nil {
		return e
	}
	sum, e = modelFingerprint(current, p.ContextPins)
	if e != nil {
		return e
	}
	graph, e := s.catalog.CanonicalSourcesTx(ctx, tx, current)
	if e != nil || len(graph) != len(current) {
		return wait("source_graph_changed")
	}
	for i := range current {
		if graph[i] != current[i] {
			return wait("source_graph_changed")
		}
	}
	event := sum
	if p.Trigger == "session_completed" || p.Trigger == "session_inactive" {
		event = p.Job.OriginalRunID
	}
	if p.Trigger == "schedule" {
		var slot int64
		if e = tx.QueryRow(ctx, `SELECT floor(extract(epoch FROM clock_timestamp())/$1)::bigint`, p.IntervalSeconds).Scan(&slot); e != nil {
			return e
		}
		event = fmt.Sprintf("%s:%d", sum, slot)
	}
	if event == last {
		return wait("coalesced")
	}
	// Global bound is serialized only after domain/source locks, before job rows.
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(820072)`); e != nil {
		return e
	}
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM memory_aux_jobs WHERE kind='model' AND status IN ('pending','running')`).Scan(&count); e != nil {
		return e
	}
	if count >= 64 {
		return wait("queue_full")
	}
	digest := sha256.Sum256([]byte(id + ":" + event))
	jid := "model_" + hex.EncodeToString(digest[:])
	_, e = tx.Exec(ctx, `INSERT INTO memory_aux_jobs(job_id,owner_id,kind,purpose,task_id,task_revision,constraints_revision,original_run_id,authority,sources,source_fingerprint,allowance,deadline,policy_id,context_manifest) VALUES($1,$2,'model',$13,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$14) ON CONFLICT DO NOTHING`, jid, p.OwnerID, p.Job.TaskID, p.TaskRevision, p.ConstraintsRevision, p.Job.OriginalRunID, jsonBytes(p.Authority), jsonBytes(current), sum, jsonBytes(p.Job.Allowance), p.Job.Deadline, p.ID, p.Handler, contextManifestJSON(p.ContextPins))
	if e != nil {
		return e
	}
	if e = s.checkPolicyTaskTx(ctx, tx, p); e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE memory_model_policies SET jobs_created=jobs_created+1,last_event=$2,reason='',next_event_at=clock_timestamp()+$3*INTERVAL '1 second' WHERE policy_id=$1`, id, event, p.IntervalSeconds)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func lockPolicySessionsTx(ctx context.Context, tx pgx.Tx, p ModelPolicy) error {
	ids := make([]string, 0, len(p.Job.Sources))
	for _, r := range p.Job.Sources {
		ids = append(ids, r.ID)
	}
	// Sessions precede tasks/sources, matching deletion. FOR UPDATE also blocks
	// FK inserts of a concurrent task that would invalidate the privacy predicate.
	contextSessions := make([]string, len(p.Contexts))
	for i, c := range p.Contexts {
		contextSessions[i] = c.SessionKey
	}
	rows, e := tx.Query(ctx, `SELECT session_key FROM sessions WHERE session_key IN (SELECT scope_id FROM memory_sources WHERE source_id=ANY($1) AND scope_type='session') OR session_key=ANY($2) ORDER BY session_key FOR UPDATE`, ids, contextSessions)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return e
		}
	}
	return rows.Err()
}

func interactiveTx(ctx context.Context, tx pgx.Tx) (bool, error) {
	var busy bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM session_ownership WHERE expires_at>clock_timestamp())`).Scan(&busy)
	return busy, e
}

func sourcePolicyPrivacyTx(ctx context.Context, tx pgx.Tx, p ModelPolicy, sources []catalog.Source) error {
	if p.Privacy != "cloud_allowed" {
		return nil
	}
	for _, src := range sources {
		if len(p.ContextPins) > 0 && contextPermits(p, src) {
			continue
		}
		switch src.ScopeType {
		case "task":
			if p.Job.TaskID == "" || p.Job.TaskID != src.ScopeID {
				return ErrClaim
			}
		case "session":
			var permitted bool
			// Session history may mix tasks with distinct privacy; no implicit union of
			// their permissions. A future multi-task consumer needs a separate contract.
			if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runs WHERE run_id=$1 AND session_key=$2 AND COALESCE(task_id,'')=$3) AND NOT EXISTS(SELECT 1 FROM tasks WHERE session_key=$2 AND task_id<>$3)`, p.Job.OriginalRunID, src.ScopeID, p.Job.TaskID).Scan(&permitted); e != nil {
				return e
			}
			if !permitted {
				return ErrClaim
			}
		case "user":
			if src.ScopeID != p.OwnerID {
				return ErrClaim
			}
		default:
			return ErrClaim
		}
	}
	return nil
}

func (s *Store) reapModels(ctx context.Context) error {
	if _, e := s.pool.Exec(ctx, `UPDATE memory_model_policies SET state='revoked',reason='deadline' WHERE state<>'revoked' AND (policy->'job'->>'deadline')::timestamptz<=clock_timestamp()`); e != nil {
		return e
	}
	_, e := s.pool.Exec(ctx, `UPDATE memory_aux_jobs j SET status=CASE WHEN p.state='revoked' THEN 'cancelled' WHEN j.deadline<=clock_timestamp() OR j.attempts>=j.max_attempts THEN 'failed' ELSE 'pending' END,claim_id='',claim_until=NULL,error_code=CASE WHEN p.state='revoked' THEN 'policy_revoked' ELSE 'claim_or_deadline_expired' END,updated_at=clock_timestamp() FROM memory_model_policies p WHERE j.policy_id=p.policy_id AND j.status IN ('pending','running') AND (p.state='revoked' OR j.deadline<=clock_timestamp() OR j.status='running' AND j.claim_until<=clock_timestamp())`)
	return e
}

// A stored owner policy supplies independent restricted creation authority;
// the scheduler never impersonates an authenticated channel owner.
type policyChecker struct {
	s *Store
	p ModelPolicy
}

func (c *policyChecker) PinnedMemorySources() []catalog.SourceRef {
	refs := make([]catalog.SourceRef, len(c.p.Job.Sources))
	for i, r := range c.p.Job.Sources {
		refs[i] = catalog.SourceRef{ID: r.ID, AccessRevision: r.AccessRevision}
	}
	return refs
}
func (c *policyChecker) CheckMemoryAccessTx(ctx context.Context, tx pgx.Tx, refs []catalog.SourceRef, purpose string) (string, error) {
	if purpose != "memory_job_create" {
		return "", ErrClaim
	}
	allowed := map[string]int64{}
	for _, r := range c.p.Job.Sources {
		allowed[r.ID] = r.AccessRevision
	}
	for _, r := range refs {
		if allowed[r.ID] == 0 || r.AccessRevision != 0 && r.AccessRevision != allowed[r.ID] {
			return "", ErrClaim
		}
	}
	if e := c.FinalizeMemoryAccessTx(ctx, tx, refs, purpose); e != nil {
		return "", e
	}
	return c.p.OwnerID, nil
}
func (c *policyChecker) FinalizeMemoryAccessTx(ctx context.Context, tx pgx.Tx, _ []catalog.SourceRef, _ string) error {
	if e := c.s.checkPolicyTaskTx(ctx, tx, c.p); e != nil {
		return e
	}
	_, state, e := readPolicy(ctx, tx, c.p.ID, true)
	if e != nil {
		return e
	}
	if state != "active" {
		return ErrClaim
	}
	return nil
}

// ControlModel is owner-proved; process/operator disable is additionally enforced
// by the composition's worker lifecycle. Pauses do not reset any allowance.
func (s *Store) ControlModel(ctx context.Context, id, state string) error {
	a, ok := owneridentity.FromContext(ctx)
	if !ok {
		return ErrClaim
	}
	if state != "active" && state != "paused" && state != "revoked" {
		return ErrClaim
	}
	tag, e := s.pool.Exec(ctx, `UPDATE memory_model_policies SET state=$3,reason=$3 WHERE policy_id=$1 AND owner_id=$2 AND state<>'revoked'`, id, a.OwnerID, state)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrClaim
	}
	return nil
}
func (s *Store) CancelModelJob(ctx context.Context, id string) error {
	a, ok := owneridentity.FromContext(ctx)
	if !ok {
		return ErrClaim
	}
	tag, e := s.pool.Exec(ctx, `UPDATE memory_aux_jobs SET status='cancelled',error_code='owner_cancelled',updated_at=clock_timestamp() WHERE job_id=$1 AND owner_id=$2 AND kind='model' AND status IN ('pending','running')`, id, a.OwnerID)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrClaim
	}
	return nil
}
func (s *Store) ModelView(ctx context.Context) ([]json.RawMessage, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok {
		return nil, ErrClaim
	}
	rows, e := s.pool.Query(ctx, `SELECT jsonb_build_object('policy_id',p.policy_id,'state',p.state,'reason',p.reason,'jobs_created',p.jobs_created,'next_event_at',p.next_event_at,'handler',p.policy->>'handler','jobs',COALESCE((SELECT jsonb_agg(jsonb_build_object('job_id',j.job_id,'status',j.status,'attempts',j.attempts,'reason',j.error_code,'attempt_observations',COALESCE((SELECT jsonb_agg(jsonb_build_object('operation_id',a.operation_id,'state',a.state,'usage',a.usage,'quote',a.quote,'reason',a.reason,'created_at',a.created_at)) FROM memory_model_attempts a WHERE a.job_id=j.job_id),'[]'),'result_source_id',(SELECT result_source_id FROM memory_model_attempts a WHERE a.job_id=j.job_id AND a.state='published'))) FROM memory_aux_jobs j WHERE j.policy_id=p.policy_id),'[]')) FROM memory_model_policies p WHERE p.owner_id=$1 ORDER BY p.created_at DESC LIMIT 100`, a.OwnerID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var r json.RawMessage
		if e = rows.Scan(&r); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
