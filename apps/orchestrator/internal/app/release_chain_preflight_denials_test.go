package app

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
)

type releaseChainEntrySpy struct {
	*converterWorkerProvider
	calls atomic.Int32
}

func (s *releaseChainEntrySpy) StartRun(context.Context, transport.StartRunRequest) (transport.EventStream, error) {
	s.calls.Add(1)
	return nil, errors.New("synthetic provider spy entered")
}
func (s *releaseChainEntrySpy) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	s.calls.Add(1)
	return nil, errors.New("synthetic provider spy entered")
}
func (s *releaseChainEntrySpy) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	s.calls.Add(1)
	return nil, errors.New("synthetic provider spy entered")
}

func TestReleaseChainEntryCheckerDenialsIntegration(t *testing.T) {
	for _, failure := range []string{"owner_actor", "foreign_task", "unadmitted_child", "old_lease", "expired_lease", "stopped", "changed_source", "bad_expected_pin", "continue_denied", "submit_after_stop"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			pool := memorytest.Database(t)
			key := "web:entry-owner"
			owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "entry-owner", SessionKey: key, Channel: "web", DeliveryID: "entry-fixture"})
			sessions := session.NewPostgresRepository(pool)
			if _, _, e := sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: "entry-owner", Channel: "web", MetadataJSON: "{}"}); e != nil {
				t.Fatal(e)
			}
			store := task.NewStore(pool)
			root, e := store.Create(owner, task.CreateRequest{SessionKey: key, OwnerID: "entry-owner", Goal: "Current original entry proof", CompletionCriteria: []string{"original exact"}})
			if e != nil {
				t.Fatal(e)
			}
			actualTask := root
			if failure == "foreign_task" || failure == "unadmitted_child" {
				request := task.CreateRequest{SessionKey: key, OwnerID: "entry-owner", Goal: "Independent trusted owner task", CompletionCriteria: []string{"own original"}}
				if failure == "unadmitted_child" {
					request.ParentTaskID = root.TaskID
				}
				actualTask, e = store.Create(owner, request)
				if e != nil {
					t.Fatal(e)
				}
			}
			manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions.log"))
			if e = manifest.Initialize(); e != nil {
				t.Fatal(e)
			}
			sources := catalog.New(pool, manifest)
			if e = sources.Reconcile(ctx); e != nil {
				t.Fatal(e)
			}
			refs := []catalog.SourceRef{}
			for _, name := range []string{"document", "need", "spec"} {
				s, e := sources.Put(owner, catalog.PutSource{ExternalKey: name, Kind: "document", ScopeType: "user", ScopeID: "entry-owner", Title: "Synthetic current DATA", Body: "exact " + name})
				if e != nil {
					t.Fatal(e)
				}
				refs = append(refs, catalog.SourceRef{ID: s.ID, Revision: s.Revision, AccessRevision: s.AccessRevision, Hash: s.Hash})
			}
			runs := run.NewService(run.NewPostgresRepository(pool), nil)
			attempt, e := runs.CreateRun(ctx, &sessionv1.CreateRunRequest{SessionKey: key, InputEvent: &runv1.InputEvent{EventId: "entry-attempt"}, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2, ModelProvider: "fixture", MetadataJson: "{}"})
			if e != nil {
				t.Fatal(e)
			}
			leases := session.NewPostgresLeaseManager(pool)
			ttl := time.Minute
			if failure == "expired_lease" {
				ttl = time.Second
			}
			lease, e := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: key, RunID: attempt.RunId, OwnerID: "entry-worker", TTL: ttl})
			if e != nil {
				t.Fatal(e)
			}
			token := ownership.Token{SessionKey: key, RunID: attempt.RunId, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch}
			live := ownership.WithToken(ctx, leases.Authority(), token)
			if e = store.AttachRun(owner, actualTask.TaskID, actualTask.Revision, attempt.RunId, key); e != nil {
				t.Fatal(e)
			}
			states := []commonv1.RunState{commonv1.RunState_RUN_STATE_CREATED, commonv1.RunState_RUN_STATE_QUEUED, commonv1.RunState_RUN_STATE_ACQUIRED, commonv1.RunState_RUN_STATE_PREPARING, commonv1.RunState_RUN_STATE_MODEL_RUNNING}
			for i := 1; i < len(states); i++ {
				if _, e = runs.TransitionRun(live, &sessionv1.UpdateRunStateRequest{RunId: attempt.RunId, FromState: states[i-1], ToState: states[i], LeaseId: lease.LeaseID}); e != nil {
					t.Fatal(e)
				}
			}
			if e = sources.PinTranscript(live, key, 0); e != nil {
				t.Fatal(e)
			}
			tx, e := pool.Begin(live)
			if e != nil {
				t.Fatal(e)
			}
			_, e = sources.AccessTx(live, tx, refs, "memory_read")
			if e != nil {
				_ = tx.Rollback(ctx)
				t.Fatal(e)
			}
			if e = tx.Commit(live); e != nil {
				t.Fatal(e)
			}
			spy := &releaseChainEntrySpy{converterWorkerProvider: &converterWorkerProvider{}}
			checker := &releaseChainEntryChecker{ModelProvider: spy, pool: pool, sources: sources, root: root.TaskID, document: refs[0], need: refs[1], spec: refs[2]}
			switch failure {
			case "owner_actor":
				live = owneridentity.WithActor(live, owneridentity.Actor{OwnerID: "entry-owner", Channel: "web", DeliveryID: "forged"})
			case "old_lease", "continue_denied":
				token.FenceEpoch++
				live = ownership.WithToken(ctx, nil, token)
			case "expired_lease":
				for {
					var expired bool
					if e = pool.QueryRow(ctx, `SELECT expires_at<=clock_timestamp() FROM session_ownership WHERE lease_id=$1`, lease.LeaseID).Scan(&expired); e != nil {
						t.Fatal(e)
					}
					if expired {
						break
					}
					time.Sleep(20 * time.Millisecond)
				}
			case "stopped", "submit_after_stop":
				f := &developerFixture{f: &delegationGuardFixture{ctx: ctx, pool: pool, store: store, leases: leases, parent: root}}
				if _, e = releaseChainController(f).Apply(owner, task.ControlRequest{TaskID: root.TaskID, ExpectedRevision: root.Revision, Kind: task.ControlStop}); e != nil {
					t.Fatal(e)
				}
			case "changed_source":
				if _, e = sources.Correct(owner, catalog.Correction{SourceID: refs[0].ID, ExpectedRevision: refs[0].Revision, Title: "Changed original", Body: "different original"}); e != nil {
					t.Fatal(e)
				}
			case "bad_expected_pin":
				checker.document.Hash = "wrong-expected-content-pin"
			}
			if failure == "continue_denied" {
				if _, e = checker.ContinueRun(live, transport.ContinueRunRequest{RunID: attempt.RunId}); e == nil {
					t.Fatal("stale continuation entered provider")
				}
			} else if failure == "submit_after_stop" {
				if _, e = checker.SubmitToolResult(live, transport.SubmitToolResultRequest{RunID: attempt.RunId, ToolCallRef: "stopped-tool", ToolResultJSON: `{"status":"completed"}`}); e == nil {
					t.Fatal("stopped result submission entered provider")
				}
			} else {
				if _, e = checker.StartRun(live, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: attempt.RunId, SessionKey: key}}); e == nil {
					t.Fatal("invalid entry reached provider")
				}
			}
			if checker.starts.Load() != 0 || checker.submits.Load() != 0 || checker.continues.Load() != 0 || spy.calls.Load() != 0 {
				t.Fatal("denied entry incremented provider calls")
			}
		})
	}
}
