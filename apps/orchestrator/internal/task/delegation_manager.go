package task

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

type DelegationManager struct {
	store    *Store
	sessions DelegationSessions
	inputs   DelegationInputs
	guard    DelegationGuard
	limits   DelegationLimits
}

func NewDelegationManager(s *Store, sessions DelegationSessions, inputs DelegationInputs, guard DelegationGuard, limits DelegationLimits) (*DelegationManager, error) {
	if s == nil || s.pool == nil || s.budgets == nil || sessions == nil || inputs == nil || guard == nil || limits.MaxDepth < 1 || limits.MaxDepth > 3 || limits.MaxChildren < 1 || limits.MaxChildren > 8 || limits.MaxParallel < 1 || limits.MaxParallel > 2 || limits.MaxParallel > limits.MaxChildren || limits.MaxDependencies < 0 || limits.MaxDependencies > 8 {
		return nil, delegation.ErrDenied
	}
	return &DelegationManager{s, sessions, inputs, guard, limits}, nil
}

// begin stabilizes admission/controls first, then locks the tree's existing
// identities in the same order as owner stop. No completed parent lease is
// borrowed for lifecycle maintenance; worker mutation checks are explicit below.
func (m *DelegationManager) begin(ctx context.Context, parentID string) (pgx.Tx, string, Record, error) {
	tx, err := m.store.pool.Begin(ctx)
	if err != nil {
		return nil, "", Record{}, err
	}
	fail := func(err error) (pgx.Tx, string, Record, error) { delegationRollback(tx); return nil, "", Record{}, err }
	root, err := lockTreeTx(ctx, tx, parentID)
	if err != nil {
		return fail(err)
	}
	rows, err := tx.Query(ctx, `WITH RECURSIVE branch AS(SELECT task_id,session_key FROM tasks WHERE task_id=$1 UNION SELECT t.task_id,t.session_key FROM tasks t JOIN branch b ON t.parent_task_id=b.task_id) SELECT DISTINCT session_key FROM branch ORDER BY session_key`, root)
	if err != nil {
		return fail(err)
	}
	var keys []string
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			break
		}
		keys = append(keys, key)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return fail(err)
	}
	for _, key := range keys {
		if _, err = tx.Exec(ctx, `INSERT INTO session_ownership(session_key) VALUES($1) ON CONFLICT(session_key) DO NOTHING`, key); err != nil {
			return fail(err)
		}
		var locked string
		if err = tx.QueryRow(ctx, `SELECT session_key FROM session_ownership WHERE session_key=$1 FOR SHARE`, key).Scan(&locked); err != nil {
			return fail(err)
		}
	}
	for _, key := range keys {
		var owner string
		if err = tx.QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key=$1 FOR UPDATE`, key).Scan(&owner); err != nil {
			return fail(err)
		}
	}
	rows, err = tx.Query(ctx, `SELECT r.run_id FROM runs r JOIN session_ownership o ON o.run_id=r.run_id WHERE o.session_key=ANY($1) ORDER BY r.run_id FOR SHARE OF r`, keys)
	if err != nil {
		return fail(err)
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return fail(err)
	}
	// All task locks are shared here. Workers take their own task write lock
	// before delegation authority; lifecycle does not upgrade a shared task lock.
	rows, err = tx.Query(ctx, `WITH RECURSIVE branch AS(SELECT task_id FROM tasks WHERE task_id=$1 UNION SELECT t.task_id FROM tasks t JOIN branch b ON t.parent_task_id=b.task_id) SELECT t.task_id FROM tasks t JOIN branch b USING(task_id) ORDER BY t.task_id FOR SHARE OF t`, root)
	if err != nil {
		return fail(err)
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return fail(err)
	}
	parent, err := m.store.GetTx(ctx, tx, parentID)
	if err != nil {
		return fail(err)
	}
	if parent.Status != "active" {
		return fail(ErrConflict)
	}
	return tx, root, parent, nil
}

func delegationRollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

// workerTx checks the private current parent/child token on this transaction.
// It must not use ownership.Begin, whose extra connection would invert the tree
// barrier and can deadlock owner controls or a pool with two connections.
func workerTx(ctx context.Context, tx pgx.Tx, record Record) error {
	t, ok := ownership.TokenFromContext(ctx)
	if !ok || t.SessionKey != record.SessionKey {
		return ownership.ErrStale
	}
	var valid bool
	err := tx.QueryRow(ctx, `SELECT lease_id=$2 AND run_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp() FROM session_ownership WHERE session_key=$1 FOR SHARE`, t.SessionKey, t.LeaseID, t.RunID, t.FenceEpoch).Scan(&valid)
	if err != nil || !valid {
		return ownership.ErrStale
	}
	err = tx.QueryRow(ctx, `SELECT task_id=$2 AND task_revision=$3 AND current_state NOT IN ('completed','failed','cancelled','timed_out') FROM runs WHERE run_id=$1 AND session_key=$4 FOR SHARE`, t.RunID, record.TaskID, record.Revision, record.SessionKey).Scan(&valid)
	if err != nil || !valid {
		return ownership.ErrStale
	}
	return nil
}

func (m *DelegationManager) Admit(ctx context.Context, request DelegationRequest) (DelegationRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Copy caller slices before any lock wait. Model proposals are immutable data.
	raw, err := json.Marshal(request)
	if err != nil {
		return DelegationRecord{}, ErrInvalid
	}
	if err = json.Unmarshal(raw, &request); err != nil {
		return DelegationRecord{}, ErrInvalid
	}
	request.Deadline = request.Deadline.UTC().Truncate(time.Microsecond)
	if !validProposal(request, m.limits) {
		return DelegationRecord{}, delegation.ErrDenied
	}
	fingerprint := proposalFingerprint(request)
	tx, root, parent, err := m.begin(ctx, request.ParentTaskID)
	if err != nil {
		return DelegationRecord{}, err
	}
	defer delegationRollback(tx)
	if err = delegationCallerTx(ctx, tx, parent); err != nil {
		return DelegationRecord{}, err
	}
	if parent.Revision != request.ParentRevision || parent.ConstraintsRevision != request.ConstraintsRevision {
		return DelegationRecord{}, ErrConflict
	}
	if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, parent.TaskID, parent.OwnerID, nil, time.Now()); err != nil {
		return DelegationRecord{}, err
	}
	// Immutable source identity is the idempotency key for one prepared proposal.
	// Tree admission barrier makes this repeat-safe across processes and restarts.
	var existing string
	err = tx.QueryRow(ctx, `SELECT task_id FROM task_delegations WHERE parent_task_id=$1 AND parent_revision=$2 AND contract_source_id=$3 AND (contract_json->'contract_source'->>'revision')::bigint=$4 ORDER BY task_id LIMIT 1`, parent.TaskID, parent.Revision, request.ContractSource.ID, request.ContractSource.Revision).Scan(&existing)
	if err == nil {
		var pinned string
		if err = tx.QueryRow(ctx, `SELECT contract_json->>'proposal_fingerprint' FROM task_delegations WHERE task_id=$1`, existing).Scan(&pinned); err != nil {
			return DelegationRecord{}, err
		}
		if pinned != fingerprint {
			return DelegationRecord{}, ErrConflict
		}
		value, err := m.loadTx(ctx, tx, existing)
		if err != nil {
			return value, err
		}
		child, err := m.store.GetTx(ctx, tx, existing)
		if err != nil {
			return value, err
		}
		if err = m.guard.CheckLifecycleTx(ctx, tx, parent, child, value.Contract); err != nil {
			return value, err
		}
		// This is still an admission request, even when the child already exists.
		// Recheck its current proposal/dispatch after all lifecycle lock waits;
		// ordinary lifecycle maintenance remains independent of the old worker.
		if err = m.guard.CheckProposalTx(ctx, tx, parent, request); err != nil {
			return value, err
		}
		return value, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return DelegationRecord{}, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return DelegationRecord{}, err
	}
	parentContract, err := delegation.LoadTx(ctx, tx, parent.TaskID)
	if err != nil {
		return DelegationRecord{}, err
	}
	depth := 1
	if parentContract != nil {
		depth = parentContract.Depth + 1
		if !narrower(request, *parentContract) {
			return DelegationRecord{}, delegation.ErrDenied
		}
	}
	if depth > m.limits.MaxDepth || !request.Deadline.After(now) || request.Deadline.After(now.Add(24*time.Hour)) || !budgetWithin(request.Budget, limitsOf(m.store.budgets.Policy.Task)) {
		return DelegationRecord{}, delegation.ErrDenied
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM task_delegations WHERE root_task_id=$1`, root).Scan(&count); err != nil {
		return DelegationRecord{}, err
	}
	if count >= m.limits.MaxChildren {
		return DelegationRecord{}, delegation.ErrDenied
	}
	if err = m.dependenciesTx(ctx, tx, root, parent.TaskID, "", request.Dependencies); err != nil {
		return DelegationRecord{}, err
	}
	if err = m.guard.CheckProposalTx(ctx, tx, parent, request); err != nil {
		return DelegationRecord{}, err
	}
	// A fresh session has no parent transcript or provider references. Production
	// preparation must additionally exclude ambient owner profile/retrieval.
	key, err := newID("delegation:")
	if err != nil {
		return DelegationRecord{}, err
	}
	if _, created, e := m.sessions.CreateSessionTx(ctx, tx, session.CreateSessionParams{SessionKey: key, UserID: parent.OwnerID, Channel: "internal", MetadataJSON: `{"kind":"bounded_delegation"}`}); e != nil || !created {
		if e == nil {
			e = ErrConflict
		}
		return DelegationRecord{}, e
	}
	if _, err = tx.Exec(ctx, `SELECT session_key FROM sessions WHERE session_key=$1 FOR UPDATE`, key); err != nil {
		return DelegationRecord{}, err
	}
	child, err := m.store.createTx(ctx, tx, CreateRequest{SessionKey: key, OwnerID: parent.OwnerID, ParentTaskID: parent.TaskID, Goal: "Delegated contract " + request.ContractSource.ID, Source: map[string]any{"type": "bounded_delegation", "contract_source": request.ContractSource}, CompletionCriteria: []string{"contract-source:" + request.ContractSource.ID}}, true)
	if err != nil {
		return DelegationRecord{}, err
	}
	c := delegation.Contract{TaskID: child.TaskID, RootTaskID: root, ParentTaskID: parent.TaskID, ParentRevision: parent.Revision, ConstraintsRevision: parent.ConstraintsRevision, Revision: 1, Permissions: request.Permissions, Sources: request.Sources, ContractSource: request.ContractSource, Budget: request.Budget, Deadline: request.Deadline, Depth: depth, State: "waiting"}
	if err = delegation.Validate(c, now); err != nil {
		return DelegationRecord{}, err
	}
	contractRaw, err := json.Marshal(struct {
		delegation.Contract
		Fingerprint string `json:"proposal_fingerprint"`
	}{c, fingerprint})
	if err != nil {
		return DelegationRecord{}, err
	}
	budgetRaw, _ := json.Marshal(c.Budget)
	_, err = tx.Exec(ctx, `INSERT INTO task_delegations(task_id,root_task_id,parent_task_id,parent_revision,parent_constraints_revision,contract_source_id,contract_json,budget_json,deadline,depth,state) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb,$9,$10,'waiting')`, c.TaskID, c.RootTaskID, c.ParentTaskID, c.ParentRevision, c.ConstraintsRevision, c.ContractSource.ID, contractRaw, budgetRaw, c.Deadline, c.Depth)
	if err != nil {
		return DelegationRecord{}, err
	}
	if err = m.dependenciesTx(ctx, tx, root, parent.TaskID, c.TaskID, request.Dependencies); err != nil {
		return DelegationRecord{}, err
	}
	if err = m.guard.BindContractSourcesTx(ctx, tx, parent, child, c); err != nil {
		return DelegationRecord{}, err
	}
	if err = m.queueTx(ctx, tx, parent, child, &c); err != nil {
		return DelegationRecord{}, err
	}
	if err = delegationCallerTx(ctx, tx, parent); err != nil {
		return DelegationRecord{}, err
	}
	if err = m.guard.CheckProposalTx(ctx, tx, parent, request); err != nil {
		return DelegationRecord{}, err
	}
	return DelegationRecord{Contract: c}, tx.Commit(ctx)
}

func validProposal(r DelegationRequest, l DelegationLimits) bool {
	if r.ParentTaskID == "" || r.ParentRevision < 1 || r.ConstraintsRevision < 1 || !delegation.ValidSource(r.ContractSource) || len(r.Sources) > 64 || len(r.Permissions) > 32 || len(r.Dependencies) > l.MaxDependencies {
		return false
	}
	seen := map[string]bool{}
	for _, s := range r.Sources {
		if !delegation.ValidSource(s) || seen[s.ID] || s.ID == r.ContractSource.ID {
			return false
		}
		seen[s.ID] = true
	}
	seen = map[string]bool{}
	for _, id := range r.Dependencies {
		if id == "" || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
func proposalFingerprint(r DelegationRequest) string {
	sort.Strings(r.Dependencies)
	b, _ := json.Marshal(r)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
