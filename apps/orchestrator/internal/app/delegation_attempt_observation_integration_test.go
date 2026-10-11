package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

func failedDelegationFixture(t *testing.T, diagnostic string, release bool, parentConstraints ...int64) (*delegationGuardFixture, delegation.Contract, task.Record, string) {
	t.Helper()
	f := newDelegationGuardFixture(t)
	if len(parentConstraints) > 1 {
		t.Fatal("one parent constraints revision required")
	}
	if len(parentConstraints) == 1 {
		if tag, err := f.pool.Exec(f.ctx, `UPDATE tasks SET constraints_revision=$2 WHERE task_id=$1`, f.parent.TaskID, parentConstraints[0]); err != nil || tag.RowsAffected() != 1 {
			t.Fatal("parent constraints fixture did not apply", err)
		}
		var err error
		f.parent, err = f.store.Get(f.ctx, f.parent.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		f.caps.ConstraintsRevision = f.parent.ConstraintsRevision
		f.saveCaps(t)
	}
	c, child := f.admit(t, "failed-attempt-contract")
	runID := "failed-child:" + child.TaskID
	ctx := f.worker(t, child, runID)
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.sources.AccessTx(ctx, tx, refsForDelegation(c), "memory_read"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	f.budgets.Policy.Models["fixture:fixture"] = budget.ModelPrice{Basis: "local_no_billing"}
	for i := 0; i < 2; i++ {
		a, err := f.budgets.AdmitModel(ctx, runID, "", "fixture", "fixture", 64)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			in, out := int64(7), int64(3)
			if err = f.budgets.ObserveModel(ctx, a.OperationID, &in, &out); err != nil {
				t.Fatal(err)
			}
		}
	}
	journal := actionauth.NewPostgresStore(f.pool)
	journal.SetBudgets(f.budgets)
	journal.SetRunDataGuard(f.sources.CheckRunSourcesTx)
	token, _ := ownership.TokenFromContext(ctx)
	b := actionauth.ActionBinding{ActionID: "attempt-action:" + child.TaskID, ActionHash: validationHash, RunID: runID, ToolCallID: "attempt-call:" + child.TaskID, TaskID: child.TaskID, TaskRevision: child.Revision, ConstraintsRevision: child.ConstraintsRevision, OwnerID: child.OwnerID, ToolName: "http.request", ContractHash: validationHash, ArgsHash: validationHash, Resource: "https://fixture.example/read", Effect: actionauth.EffectRead}
	if err = journal.PutIntent(ctx, actionauth.Intent{Binding: b, EffectID: b.ActionID, EffectFingerprint: validationHash, EffectPolicy: actionauth.EffectPolicy{RetrySafety: "read_only", VerificationMethod: "read_result"}, ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	grant, err := journal.PutGrant(ctx, actionauth.Grant{ID: "attempt-grant:" + child.TaskID, Binding: b, LeaseID: token.LeaseID, LeaseEpoch: token.FenceEpoch, ExpiresAt: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if err = journal.ClaimGrant(ctx, grant.ID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err = journal.RecordDispatchOutcome(ctx, grant.ID, "unknown", actionauth.EffectEvidence{EvidenceID: "attempt-evidence:" + child.TaskID, ActionID: b.ActionID, SourceActionID: b.ActionID, SourceRunID: runID, SourceToolCallID: b.ToolCallID, Source: "runtime_response", Method: "read_result", Verdict: actionauth.StateUnknown, ResultDigest: validationHash, ObservedAt: now, RecordedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `UPDATE runs SET current_state='failed',status='failed',error_type='transport_error',error_message=$2 WHERE run_id=$1`, runID, diagnostic); err != nil {
		t.Fatal(err)
	}
	// Use the actual failed-run lifecycle: it blocks the task and advances its
	// semantic revision. The terminal run and journal retain the old revision.
	if err = f.store.SaveRunOutcome(ctx, runID, "failed", ""); err != nil {
		t.Fatal(err)
	}
	current, err := f.store.Get(f.ctx, child.TaskID)
	if err != nil || current.Status != "blocked" || current.Revision != child.Revision+1 {
		t.Fatal("actual failure lifecycle did not advance task revision", err, current)
	}
	if release {
		if _, err = f.leases.ReleaseLease(f.ctx, token.LeaseID); err != nil {
			t.Fatal(err)
		}
	}
	return f, c, child, runID
}

func TestDelegationFailedAttemptParentConstraintsIndependentIntegration(t *testing.T) {
	f, c, child, _ := failedDelegationFixture(t, "rate_limited", true, 2)
	r, err := f.manager.ObserveFailedAttempts(f.owner, f.parent.TaskID, child.TaskID)
	if err != nil || c.ConstraintsRevision != 2 || r.ConstraintsRevision != 1 || r.TaskRevision != 2 || len(r.Attempts) != 1 || r.Attempts[0].TaskRevision != 1 {
		t.Fatal("parent and child constraints identities were conflated", err, r)
	}
}

func TestDelegationFailedAttemptObservationIntegration(t *testing.T) {
	for _, diagnostic := range []string{"rate_limited", "SECRET_DIAGNOSTIC_MARKER password=fixture-private"} {
		t.Run(diagnostic[:4], func(t *testing.T) {
			f, c, child, runID := failedDelegationFixture(t, diagnostic, true)
			before, err := f.manager.Get(f.ctx, child.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			r, err := f.manager.ObserveFailedAttempts(f.owner, f.parent.TaskID, child.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			if r.TaskStatus != "blocked" || r.TaskRevision != child.Revision+1 || len(r.Attempts) != 1 || r.Attempts[0].TaskRevision != child.Revision || r.Attempts[0].RunID != runID || r.Attempts[0].State != "failed" || len(r.Actions) != 1 || r.Actions[0].TaskRevision != child.Revision || r.Actions[0].State != "effect_unknown" || r.Reserved.ModelTurns != 2 || r.Reserved.ExternalCalls != 1 || len(r.ModelUsage) != 2 || r.ModelUsage[0].InputTokens == nil || *r.ModelUsage[0].InputTokens != 7 || r.ModelUsage[1].InputTokens != nil || r.ModelUsage[1].OutputTokens != nil {
				t.Fatal("lost real attempt/effect/reservation/unknown usage", r)
			}
			if diagnostic == "rate_limited" && r.Attempts[0].DiagnosticCode != "rate_limited" {
				t.Fatal("safe subtype lost")
			}
			raw, _ := json.Marshal(r)
			if strings.Contains(string(raw), "SECRET_DIAGNOSTIC_MARKER") || strings.Contains(string(raw), "fixture-private") || strings.Contains(string(raw), "bounded contract DATA") {
				t.Fatal("private prose/error entered observation")
			}
			if _, err = delegation.ParseReport(raw); err == nil {
				t.Fatal("failed attempt became completed report")
			}
			after, err := f.manager.Get(f.ctx, child.TaskID)
			if err != nil || after.Contract.State != before.Contract.State || after.ResultID != "" || after.Report != nil {
				t.Fatal("observation collected a result or freed execution width")
			}
			// A fresh parent worker reads the same facts without a child lease and
			// pins all current Catalog roots to its own run for release/deletion.
			parentCtx := f.leasedWorker(t, f.parent, "observing-parent:"+child.TaskID)
			if _, err = f.manager.ObserveFailedAttempts(parentCtx, f.parent.TaskID, child.TaskID); err != nil {
				t.Fatal(err)
			}
			var pins int
			if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM memory_run_sources WHERE run_id=$1`, "observing-parent:"+child.TaskID).Scan(&pins); err != nil || pins != len(c.Sources)+1 {
				t.Fatal("parent release did not retain actual source pins", pins, err)
			}
			if _, err = f.manager.CollectGenerated(f.owner, f.parent.TaskID, child.TaskID, "invented-result"); err == nil {
				t.Fatal("failed observation authorized collection")
			}
		})
	}
}

func TestDelegationFailedAttemptObservationDenialsIntegration(t *testing.T) {
	for _, name := range []string{"no-actor", "foreign-owner", "live-child-lease", "parent-stopped", "parent-revised", "child-constraints-revised", "child-cancelled", "source-corrected", "source-deleted", "new-live-attempt", "raw-error-code", "expired-parent-lease", "mixed-actor-worker"} {
		t.Run(name, func(t *testing.T) {
			f, c, child, runID := failedDelegationFixture(t, "rate_limited", name != "live-child-lease")
			ctx := f.owner
			exec := func(sql string, args ...any) {
				t.Helper()
				if tag, err := f.pool.Exec(f.ctx, sql, args...); err != nil || tag.RowsAffected() != 1 {
					t.Fatal("fixture mutation did not apply", err)
				}
			}
			switch name {
			case "no-actor":
				ctx = f.ctx
			case "foreign-owner":
				ctx = owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: "different-owner", Channel: "web", DeliveryID: "foreign"})
			case "parent-stopped":
				exec(`UPDATE tasks SET status='cancelled' WHERE task_id=$1`, f.parent.TaskID)
			case "parent-revised":
				exec(`UPDATE tasks SET revision=revision+1 WHERE task_id=$1`, f.parent.TaskID)
			case "child-constraints-revised":
				exec(`UPDATE tasks SET constraints_revision=constraints_revision+1 WHERE task_id=$1`, child.TaskID)
			case "child-cancelled":
				exec(`UPDATE tasks SET status='cancelled' WHERE task_id=$1`, child.TaskID)
			case "source-corrected":
				if _, err := f.sources.Correct(f.owner, catalog.Correction{SourceID: c.ContractSource.ID, ExpectedRevision: c.ContractSource.Revision, Title: "Corrected source", Body: "corrected", Author: "owner"}); err != nil {
					t.Fatal(err)
				}
			case "source-deleted":
				if _, err := f.sources.Delete(f.owner, c.ContractSource.ID, "delete", c.ContractSource.Revision); err != nil {
					t.Fatal(err)
				}
			case "new-live-attempt":
				// Another unfinished run on the earlier revision must not disappear
				// behind SaveRunOutcome's blocked-task revision increment.
				exec(`INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES('unfinished-old-revision',$1,'fixture','model_running','model_running','mode_1','fixture',clock_timestamp(),clock_timestamp(),$2,$3)`, child.SessionKey, child.TaskID, child.Revision)
			case "raw-error-code":
				exec(`UPDATE runs SET error_type='SECRET_DIAGNOSTIC_MARKER',error_message='fixture-private' WHERE run_id=$1`, runID)
			case "expired-parent-lease":
				ctx = f.leasedWorker(t, f.parent, "expired-parent")
				exec(`UPDATE session_ownership SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE session_key=$1`, f.parent.SessionKey)
			case "mixed-actor-worker":
				ctx = f.leasedWorker(t, f.parent, "mixed-parent")
				ctx = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "web", DeliveryID: "mixed"})
			}
			r, err := f.manager.ObserveFailedAttempts(ctx, f.parent.TaskID, child.TaskID)
			if name == "raw-error-code" {
				if err != nil || r.Attempts[0].ErrorCode != "execution_failed" || r.Attempts[0].DiagnosticCode != "" {
					t.Fatal("unknown code leaked", r, err)
				}
				return
			}
			if err == nil {
				t.Fatal("stale/foreign/live authority admitted observation")
			}
		})
	}
}
