package task_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/approval"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5/pgconn"
)

// Real PG, pool2, owner Catalog deletion, task controls and permanent journal.
// Run creation and runtime evidence are explicit component fixtures, not model
// generation or a Broker/recipient execution claim.
func TestOwnerControlsAfterSourceDeletionIntegration(t *testing.T) {
	for _, kind := range []string{task.ControlStop, task.ControlAmend} {
		t.Run(kind, func(t *testing.T) {
			pool := memorytest.Database(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			const ownerID = "privacy-control-owner"
			owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: ownerID, Channel: "web", DeliveryID: "delete-then-" + kind})
			leases, tasks := session.NewPostgresLeaseManager(pool), task.NewStore(pool)
			controller := task.NewController(tasks, leases.Authority(), leases, run.NewPostgresRepository(pool), approval.NewPostgresRepository(pool), durableinput.NewStore(pool), transcript.NewStore(pool))
			manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletion-anchor"))
			if err := manifest.Initialize(); err != nil {
				t.Fatal(err)
			}
			sources := catalog.New(pool, manifest)
			source, err := sources.Put(owner, catalog.PutSource{ExternalKey: "private-original", Kind: "document", ScopeType: "user", ScopeID: ownerID, Body: "PRIVATE_SOURCE_BODY_TO_DELETE"})
			if err != nil {
				t.Fatal(err)
			}
			var records []task.Record
			var workers []context.Context
			for _, key := range []string{"privacy-parent"} {
				if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES($1,$2,'web')`, key, ownerID); err != nil {
					t.Fatal(err)
				}
				r, err := tasks.Create(ctx, task.CreateRequest{SessionKey: key, OwnerID: ownerID, Goal: "Retain owner control after privacy deletion", CompletionCriteria: []string{"owner reviewed"}})
				if err != nil {
					t.Fatal(err)
				}
				runID := key + "-run"
				if _, err := pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,task_id,task_revision,started_at,updated_at) VALUES($1,$2,$1,'model_running','model_running','mode_1','fixture',$3,$4,clock_timestamp(),clock_timestamp())`, runID, key, r.TaskID, r.Revision); err != nil {
					t.Fatal(err)
				}
				lease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: key, RunID: runID, OwnerID: "fixture-worker", TTL: time.Minute})
				if err != nil {
					t.Fatal(err)
				}
				live := ownership.WithToken(ctx, leases.Authority(), ownership.Token{SessionKey: key, RunID: runID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
				if err := tasks.AttachRun(live, r.TaskID, r.Revision, runID, key); err != nil {
					t.Fatal(err)
				}
				tx, err := pool.Begin(live)
				if err != nil {
					t.Fatal(err)
				}
				_, err = sources.AccessTx(live, tx, []catalog.SourceRef{{ID: source.ID, Revision: source.Revision, AccessRevision: source.AccessRevision, Hash: source.Hash}}, "memory_read")
				if err != nil {
					_ = tx.Rollback(ctx)
					t.Fatal(err)
				}
				if err := tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				if err := sources.PinTranscript(live, key, 0); err != nil {
					t.Fatal(err)
				}
				r, err = tasks.SaveWorking(live, r.TaskID, r.WorkingRevision, task.WorkingState{Plan: []string{"private plan"}, PendingActions: []string{"pending"}, Scratch: map[string]any{"body": source.Body}})
				if err != nil {
					t.Fatal(err)
				}
				records, workers = append(records, r), append(workers, live)
			}
			root, live := records[0], workers[0]
			token, _ := ownership.TokenFromContext(live)
			journal := actionauth.NewPostgresStore(pool)
			journal.SetRunDataGuard(sources.CheckRunSourcesTx)
			hash := strings.Repeat("a", 64)
			binding := actionauth.ActionBinding{ActionID: "privacy-confirmed-action", ActionHash: hash, RunID: token.RunID, ToolCallID: "privacy-call", TaskID: root.TaskID, TaskRevision: root.Revision, ConstraintsRevision: root.ConstraintsRevision, OwnerID: ownerID, ToolName: "http.request", ContractHash: hash, ArgsHash: hash, Resource: "https://fixture.example/read", Effect: actionauth.EffectRead}
			if err := journal.PutIntent(live, actionauth.Intent{Binding: binding, EffectID: binding.ActionID, EffectFingerprint: hash, EffectPolicy: actionauth.EffectPolicy{RetrySafety: "read_only", VerificationMethod: "read_result"}, ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
				t.Fatal(err)
			}
			grant, err := journal.PutGrant(live, actionauth.Grant{ID: "privacy-grant", Binding: binding, LeaseID: token.LeaseID, LeaseEpoch: token.FenceEpoch, ExpiresAt: time.Now().Add(time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			if err := journal.ClaimGrant(live, grant.ID); err != nil {
				t.Fatal("fixture permanent effect admission failed", err)
			}
			now := time.Now().UTC()
			proof := actionauth.EffectEvidence{EvidenceID: "privacy-confirmed-evidence", ActionID: binding.ActionID, SourceActionID: binding.ActionID, SourceRunID: token.RunID, SourceToolCallID: binding.ToolCallID, Source: "runtime_response", Method: "read_result", Verdict: actionauth.StateConfirmed, ResultDigest: hash, ObservedAt: now, RecordedAt: now}
			if _, err := journal.RecordDispatchOutcome(live, grant.ID, "dispatch_completed", proof); err != nil {
				t.Fatal(err)
			}
			deletion, err := sources.Delete(owner, source.ID, "delete", source.Revision)
			if err != nil {
				t.Fatal(err)
			}
			want := task.WorkingState{Plan: []string{}, PendingActions: []string{}, Scratch: map[string]any{"privacy_redacted": true, "deletion_id": deletion.ID}}
			for i, prior := range records {
				current, err := tasks.Get(ctx, prior.TaskID)
				if err != nil || !reflect.DeepEqual(current.Working, want) {
					t.Fatal("Catalog did not clear private working state", err)
				}
				_, err = tasks.SaveWorking(workers[i], prior.TaskID, current.WorkingRevision, task.WorkingState{Scratch: map[string]any{"revived": source.Body}})
				var pgerr *pgconn.PgError
				if !errors.As(err, &pgerr) || pgerr.Code != "40001" {
					t.Fatal("stale model working publication was not denied by source fence", err)
				}
			}
			request := task.ControlRequest{TaskID: root.TaskID, ExpectedRevision: root.Revision, Kind: kind}
			if kind == task.ControlAmend {
				request.Instruction = "Continue only with current undeleted sources"
			}
			result, err := controller.Apply(owner, request)
			if err != nil {
				t.Fatalf("owner %s after actual deletion failed: %v", kind, err)
			}
			for _, prior := range records {
				current, err := tasks.Get(ctx, prior.TaskID)
				if err != nil || !reflect.DeepEqual(current.Working, want) || current.Revision != prior.Revision+1 {
					t.Fatal("owner control lost privacy marker or branch revision", err)
				}
				if kind == task.ControlStop && current.Status != "cancelled" || kind == task.ControlAmend && prior.TaskID != root.TaskID && current.Status != "paused" {
					t.Fatal("owner control did not apply to independent child")
				}
			}
			if kind == task.ControlAmend {
				var payload string
				if result.ContinuationInputID == "" || pool.QueryRow(ctx, `SELECT event_json::text FROM accepted_inputs WHERE input_id=$1 AND status='queued'`, result.ContinuationInputID).Scan(&payload) != nil || strings.Contains(payload, source.Body) {
					t.Fatal("fresh owner continuation restored old private source")
				}
				if sources.CheckRunSources(live, token.RunID) == nil {
					t.Fatal("owner amendment revived stale worker/source proof")
				}
				const freshRun = "privacy-parent-fresh-run"
				if _, err := pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,task_id,task_revision,started_at,updated_at) VALUES($1,$2,$3,'model_running','model_running','mode_1','fixture',$4,$5,clock_timestamp(),clock_timestamp())`, freshRun, root.SessionKey, result.ContinuationInputID, root.TaskID, result.Task.Revision); err != nil {
					t.Fatal(err)
				}
				freshLease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: root.SessionKey, RunID: freshRun, OwnerID: "fresh-fixture-worker", TTL: time.Minute})
				if err != nil {
					t.Fatal(err)
				}
				fresh := ownership.WithToken(ctx, leases.Authority(), ownership.Token{SessionKey: root.SessionKey, RunID: freshRun, LeaseID: freshLease.LeaseID, FenceEpoch: freshLease.FenceEpoch})
				if err := tasks.AttachRun(fresh, root.TaskID, result.Task.Revision, freshRun, root.SessionKey); err != nil {
					t.Fatal(err)
				}
				var headRevision int64
				if err := pool.QueryRow(fresh, `SELECT source_revision FROM transcript_heads WHERE session_key=$1`, root.SessionKey).Scan(&headRevision); err != nil {
					t.Fatal(err)
				}
				if err := sources.PinTranscript(fresh, root.SessionKey, headRevision); err != nil {
					t.Fatal(err)
				}
				if err := sources.CheckRunSources(fresh, freshRun); err != nil {
					t.Fatal("fresh continuation did not establish current source proof", err)
				}
				tx, err := pool.Begin(fresh)
				if err != nil {
					t.Fatal(err)
				}
				_, denied := sources.AccessTx(fresh, tx, []catalog.SourceRef{{ID: source.ID, Revision: source.Revision, AccessRevision: source.AccessRevision, Hash: source.Hash}}, "memory_read")
				_ = tx.Rollback(ctx)
				if !errors.Is(denied, catalog.ErrDenied) {
					t.Fatal("fresh amendment lease regained deleted original", denied)
				}
			}
			effect, err := journal.GetEffect(ctx, binding.ActionID)
			if err != nil || effect.State != actionauth.StateConfirmed || len(effect.EvidenceRefs) != 1 || effect.EvidenceRefs[0] != proof.EvidenceID {
				t.Fatal("owner control or deletion rewrote confirmed effect evidence", err)
			}
		})
	}
}
