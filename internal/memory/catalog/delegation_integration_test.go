package catalog

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

func delegatedFixture(t *testing.T, project bool) (*Store, context.Context, context.Context, context.Context, Source, Source, delegation.Contract) {
	return delegatedBodyFixture(t, project, "selected bounded summary")
}

func delegatedBodyFixture(t *testing.T, project bool, body string) (*Store, context.Context, context.Context, context.Context, Source, Source, delegation.Contract) {
	t.Helper()
	s, owner := fixture(t)
	parent := seedRun(t, s)
	_, err := s.pool.Exec(owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('child-session','owner','delegation');
 INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('root','web:owner','owner','original owner objective');
 INSERT INTO tasks(task_id,parent_task_id,session_key,owner_id,goal) VALUES('child','root','child-session','owner','bounded source reference');
 UPDATE runs SET task_id='root',task_revision=1 WHERE run_id='run';
 INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,task_id,task_revision,started_at,updated_at) VALUES('child-run','child-session','child-input','running','mode_1','model_running','fixture','child',1,clock_timestamp(),clock_timestamp());
 INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('child-session','child-run','child-lease',1,clock_timestamp()+INTERVAL '1 hour')`)
	if err != nil {
		t.Fatal(err)
	}
	scope := "user"
	if project {
		scope = "project"
	}
	original, err := s.Put(owner, PutSource{ExternalKey: "original", Kind: "document", ScopeType: scope, ScopeID: "owner", Body: "private original provenance"})
	if err != nil {
		t.Fatal(err)
	}
	selected, err := s.Put(owner, PutSource{ExternalKey: "selected", Kind: "model_candidate", ScopeType: "user", ScopeID: "owner", Body: body, Parents: []SourceRef{ref(original)}})
	if err != nil {
		t.Fatal(err)
	}
	proposal := put(t, s, owner, "contract", "bounded candidate instructions remain DATA")
	toPin := func(src Source) delegation.SourceRef {
		return delegation.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}
	}
	c := delegation.Contract{TaskID: "child", RootTaskID: "root", ParentTaskID: "root", ParentRevision: 1, ConstraintsRevision: 1, Revision: 1, Sources: []delegation.SourceRef{toPin(selected)}, ContractSource: toPin(proposal), Deadline: time.Now().UTC().Truncate(time.Microsecond).Add(time.Hour), Depth: 1, State: "queued"}
	contract, _ := json.Marshal(c)
	limits, _ := json.Marshal(c.Budget)
	_, err = s.pool.Exec(owner, `INSERT INTO task_delegations(task_id,root_task_id,parent_task_id,parent_revision,parent_constraints_revision,contract_source_id,contract_json,budget_json,deadline,depth,state) VALUES('child','root','root',1,1,$1,$2,$3,$4,1,'queued')`, proposal.ID, contract, limits, c.Deadline)
	if err != nil {
		t.Fatal(err)
	}
	child := ownership.WithToken(context.Background(), ownership.NewAuthority(s.pool), ownership.Token{SessionKey: "child-session", RunID: "child-run", LeaseID: "child-lease", FenceEpoch: 1})
	return s, owner, parent, child, original, selected, c
}

func TestReadDelegatedRunDerivesOnlySelectedCurrentWorkerContextIntegration(t *testing.T) {
	s, owner, parent, child, original, selected, c := delegatedFixture(t, false)
	if err := bindDelegate(t, s, parent, c); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ReadDelegatedRun(parent, "run"); err != nil || got != nil {
		t.Fatal("ordinary parent inherited child contract", err)
	}
	for _, ctx := range []context.Context{owner, context.Background(), parent, owneridentity.WithActor(child, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "actual-owner"})} {
		if _, err := s.ReadDelegatedRun(ctx, "child-run"); err == nil {
			t.Fatal("worker body reader accepted foreign or ambient owner authority")
		}
	}
	got, err := s.ReadDelegatedRun(child, "child-run")
	if err != nil || got == nil || got.SessionKey != "child-session" || got.Contract.TaskID != "child" || len(got.Sources) != 2 {
		t.Fatal("actual bounded reader", got, err)
	}
	found := false
	for _, source := range got.Sources {
		if source.ID == original.ID {
			t.Fatal("hidden ancestor body copied to selected context")
		}
		if source.ID == selected.ID && source.Body == selected.Body {
			found = true
		}
	}
	if !found {
		t.Fatal("selected body missing")
	}
	originalEdge := false
	for _, dep := range got.Dependencies[selected.ID] {
		if dep.ID == c.ContractSource.ID {
			t.Fatal("sibling contract became invented source ancestor")
		}
		originalEdge = originalEdge || dep.ID == original.ID
	}
	if !originalEdge {
		t.Fatal("selected original ancestry missing")
	}
	if _, err = s.pool.Exec(owner, `UPDATE session_ownership SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE session_key='child-session'`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReadDelegatedRun(child, "child-run"); err == nil {
		t.Fatal("expired worker received selected body")
	}
}

func TestReadDelegatedRunBoundsBodiesAndDeniesStoppedWorkerIntegration(t *testing.T) {
	t.Run("size", func(t *testing.T) {
		s, _, parent, child, _, _, c := delegatedBodyFixture(t, false, strings.Repeat("x", 64<<10))
		if err := bindDelegate(t, s, parent, c); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ReadDelegatedRun(child, "child-run"); err == nil {
			t.Fatal("oversized selected body released")
		}
	})
	for _, state := range []string{"completed", "failed", "cancelled", "timed_out", "finalizing"} {
		t.Run(state, func(t *testing.T) {
			s, owner, parent, child, _, _, c := delegatedFixture(t, false)
			if err := bindDelegate(t, s, parent, c); err != nil {
				t.Fatal(err)
			}
			if _, err := s.pool.Exec(owner, `UPDATE runs SET current_state=$1 WHERE run_id='child-run'`, state); err != nil {
				t.Fatal(err)
			}
			if _, err := s.ReadDelegatedRun(child, "child-run"); err == nil {
				t.Fatal("stopped worker received body")
			}
		})
	}
}

func bindDelegate(t *testing.T, s *Store, ctx context.Context, c delegation.Contract) error {
	t.Helper()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = s.BindDelegationSourcesTx(ctx, tx, "owner", "root", "child", delegationRoots(&c), "memory_read"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func TestDelegatedContextSelectsBodiesWithoutAmbientProfileOrHiddenAncestorIntegration(t *testing.T) {
	s, owner, parent, child, original, selected, c := delegatedFixture(t, false)
	if err := bindDelegate(t, s, child, c); err == nil {
		t.Fatal("child impersonated parent admission")
	}
	if err := bindDelegate(t, s, parent, c); err != nil {
		t.Fatal(err)
	}
	ambient := put(t, s, owner, "ambient-profile", "unselected owner profile and transcript")
	for _, src := range []Source{original, ambient} {
		if _, err := s.Get(child, src.ID); err == nil {
			t.Fatal("unselected body released", src.ExternalKey)
		}
	}
	got, err := s.Get(child, selected.ID)
	if err != nil || got.Body != selected.Body {
		t.Fatal("selected body denied", err)
	}
	var pins int
	if err = s.pool.QueryRow(owner, `SELECT count(*) FROM memory_run_sources WHERE run_id='child-run' AND source_id=$1`, original.ID).Scan(&pins); err != nil || pins != 1 {
		t.Fatal("original privacy edge dropped", pins, err)
	}
	local, err := s.Put(owner, PutSource{ExternalKey: "child-own-response", Kind: "message", ScopeType: "session", ScopeID: "child-session", Body: "own independent response"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(child, local.ID); err != nil {
		t.Fatal("own child context denied", err)
	}
	if _, err = s.pool.Exec(owner, `UPDATE session_ownership SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE session_key='web:owner'; UPDATE runs SET current_state='completed' WHERE run_id='run'`); err != nil {
		t.Fatal(err)
	}
	restarted := New(s.pool, s.manifest)
	if _, err = restarted.Get(child, selected.ID); err != nil {
		t.Fatal("parent process crash revoked durable task authority", err)
	}
	if _, err = s.Delete(owner, original.ID, "delete", original.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.Get(child, selected.ID); err == nil {
		t.Fatal("deleted original released child text")
	}
	if err = restarted.CheckRunSources(child, "child-run"); err == nil {
		t.Fatal("late child release ignored original deletion")
	}
}

func TestDelegationProposalChecksParentGrantEvenWithOwnerBodyAccessIntegration(t *testing.T) {
	s, owner, _, _, original, _, c := delegatedFixture(t, true)
	check := func() error {
		tx, err := s.pool.Begin(owner)
		if err != nil {
			return err
		}
		defer tx.Rollback(owner)
		return s.CheckDelegationProposalSourcesTx(owner, tx, "root", "owner", delegationRoots(&c), "memory_read")
	}
	if _, err := s.Get(owner, original.ID); err != nil {
		t.Fatal("owner body access fixture", err)
	}
	if err := check(); err == nil {
		t.Fatal("owner body read widened parent project permission")
	}
	if err := s.GrantSource(owner, "root", original.ID, "memory_read", 1); err != nil {
		t.Fatal(err)
	}
	if err := check(); err != nil {
		t.Fatal("current exact parent grant", err)
	}
	if _, err := s.pool.Exec(owner, `DELETE FROM memory_scope_grants WHERE task_id='root' AND source_id=$1`, original.ID); err != nil {
		t.Fatal(err)
	}
	if err := check(); err == nil {
		t.Fatal("revoked grant accepted by proposal")
	}
}

func TestDelegatedSourceGrantRevocationAndImmutablePinsIntegration(t *testing.T) {
	s, owner, parent, child, original, selected, c := delegatedFixture(t, true)
	if err := bindDelegate(t, s, owner, c); err == nil {
		t.Fatal("owner admission widened parent task project grants")
	}
	if err := s.GrantSource(owner, "root", original.ID, "memory_read", 1); err != nil {
		t.Fatal(err)
	}
	if err := bindDelegate(t, s, parent, c); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(child, selected.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(owner, `DELETE FROM memory_scope_grants WHERE task_id='root' AND source_id=$1`, original.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := New(s.pool, s.manifest).Get(child, selected.ID); err == nil {
		t.Fatal("copied child grant survived parent revocation")
	}
	tx, err := s.pool.Begin(child)
	if err != nil {
		t.Fatal(err)
	}
	err = s.CheckRunSourcesTx(child, tx, "child-run")
	_ = tx.Rollback(child)
	if err == nil {
		t.Fatal("dispatch/final source guard ignored parent grant revocation")
	}
	for _, q := range []string{`UPDATE task_delegations SET budget_json='{"model_turns":999999}' WHERE task_id='child'`, `UPDATE task_delegations SET contract_json=jsonb_set(contract_json,'{parent_revision}','999'::jsonb) WHERE task_id='child'`} {
		if _, err = s.pool.Exec(owner, q); err == nil {
			t.Fatal("immutable authority pins changed")
		}
	}
	// Deliberately corrupt restored-row fixture, never an owner control API.
	if _, err = s.pool.Exec(owner, `ALTER TABLE task_delegations DISABLE TRIGGER delegation_contract_immutable; UPDATE task_delegations SET contract_json=jsonb_set(contract_json,'{parent_revision}','999'::jsonb) WHERE task_id='child'; ALTER TABLE task_delegations ENABLE TRIGGER delegation_contract_immutable`); err != nil {
		t.Fatal(err)
	}
	tx, err = s.pool.Begin(owner)
	if err != nil {
		t.Fatal(err)
	}
	_, err = delegation.LoadTx(owner, tx, "child")
	_ = tx.Rollback(owner)
	if err == nil {
		t.Fatal("restored JSON disagreed with authoritative columns")
	}
}
