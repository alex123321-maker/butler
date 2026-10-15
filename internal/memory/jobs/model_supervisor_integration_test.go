package jobs

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
)

func TestModelSupervisorCurrentPinnedSourceAndAuthorityIntegration(t *testing.T) {
	for _, change := range []string{"source_revision", "source_access", "source_secret", "ancestor_revision", "new_ancestor", "delete", "policy_pause", "policy_revoke", "worker_disable", "job_cancel", "claim_takeover", "job_manifest", "interactive", "task_revision", "task_privacy", "missing_summary_guard"} {
		t.Run(change, func(t *testing.T) {
			w, owner, src := modelFixture(t)
			parent := src
			if change == "ancestor_revision" || change == "new_ancestor" {
				var err error
				src, err = w.Jobs.catalog.Put(owner, catalog.PutSource{ExternalKey: "derived-source", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "derived text", Title: "source", Parents: request(parent).Sources})
				if err != nil {
					t.Fatal(err)
				}
			}
			q := policyRequest(src)
			if change == "task_revision" || change == "task_privacy" {
				_, e := w.Jobs.pool.Exec(owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('supervisor-session','owner','web'); INSERT INTO tasks(task_id,session_key,owner_id,goal,status) VALUES('supervisor-task','supervisor-session','owner','finished','completed'); INSERT INTO runs(run_id,session_key,task_id,task_revision,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('supervisor-run','supervisor-session','supervisor-task',1,'i','completed','mode_1','completed','fixture',clock_timestamp(),clock_timestamp())`)
				if e != nil {
					t.Fatal(e)
				}
				q.Job.TaskID = "supervisor-task"
				q.Job.OriginalRunID = "supervisor-run"
				if change == "task_privacy" {
					var e error
					src, e = w.Jobs.catalog.Put(owner, catalog.PutSource{ExternalKey: "session-source", Kind: "document", ScopeType: "session", ScopeID: "supervisor-session", Body: "private source", Title: "source"})
					if e != nil {
						t.Fatal(e)
					}
					q.Job.Sources = request(src).Sources
				}
			}
			j := schedule(t, w, owner, q)
			if _, _, e := w.Admit(context.Background(), j); e != nil {
				t.Fatal(e)
			}
			if e := w.Jobs.checkModelSupervisor(context.Background(), j); e != nil {
				t.Fatal("current supervisor denied", e)
			}
			var err error
			switch change {
			case "source_revision":
				_, err = w.Jobs.catalog.Correct(owner, catalog.Correction{SourceID: src.ID, ExpectedRevision: src.Revision, Body: "changed"})
			case "source_access":
				_, err = w.Jobs.pool.Exec(owner, `UPDATE memory_sources SET access_revision=access_revision+1 WHERE source_id=$1`, src.ID)
			case "source_secret":
				_, err = w.Jobs.pool.Exec(owner, `UPDATE memory_source_revisions SET origin=jsonb_set(origin,'{sensitivity}','"secret"') WHERE source_id=$1 AND revision=$2`, src.ID, src.Revision)
			case "ancestor_revision":
				_, err = w.Jobs.catalog.Correct(owner, catalog.Correction{SourceID: parent.ID, ExpectedRevision: parent.Revision, Body: "changed parent"})
			case "new_ancestor":
				var extra catalog.Source
				extra, err = w.Jobs.catalog.Put(owner, catalog.PutSource{ExternalKey: "new-parent", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "new parent", Title: "source"})
				if err == nil {
					_, err = w.Jobs.pool.Exec(owner, `INSERT INTO memory_source_dependencies(child_source_id,parent_source_id,parent_revision) VALUES($1,$2,$3)`, src.ID, extra.ID, extra.Revision)
				}
			case "delete":
				_, err = w.Jobs.catalog.Delete(owner, src.ID, "delete", src.Revision)
			case "policy_pause":
				err = w.Jobs.ControlModel(owner, q.ID, "paused")
			case "policy_revoke":
				err = w.Jobs.ControlModel(owner, q.ID, "revoked")
			case "worker_disable":
				err = w.Jobs.SetModelControl(owner, "disabled")
			case "job_cancel":
				err = w.Jobs.CancelModelJob(owner, j.ID)
			case "claim_takeover":
				_, err = w.Jobs.pool.Exec(owner, `UPDATE memory_aux_jobs SET claim_epoch=claim_epoch+1 WHERE job_id=$1`, j.ID)
			case "job_manifest":
				j.Fingerprint = "forged-manifest"
			case "interactive":
				_, err = w.Jobs.pool.Exec(owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('active','owner','web'); INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('active','r','l',1,clock_timestamp()+INTERVAL '1 minute')`)
			case "task_revision":
				_, err = w.Jobs.pool.Exec(owner, `UPDATE tasks SET revision=revision+1 WHERE task_id='supervisor-task'`)
			case "task_privacy":
				_, err = w.Jobs.pool.Exec(owner, `INSERT INTO tasks(task_id,session_key,owner_id,goal,status) VALUES('other-task','supervisor-session','owner','other','active')`)
			case "missing_summary_guard":
				_, err = w.Jobs.pool.Exec(owner, `DROP FUNCTION butler_memory_summary_sources_current(TEXT[]) CASCADE`)
			}
			if err != nil {
				t.Fatal(err)
			}
			if e := w.Jobs.checkModelSupervisor(context.Background(), j); e == nil {
				t.Fatal("stale supervisor accepted", change)
			}
		})
	}
}

type modelSecondAuthorityProof struct {
	modelFixtureProvider
	store  *Store
	change string
	stop   context.CancelFunc
	http   atomic.Int32
}

func (p *modelSecondAuthorityProof) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	blocker, err := p.store.pool.Begin(context.Background())
	if err != nil {
		return nil, err
	}
	if _, err = blocker.Exec(context.Background(), `SELECT session_key FROM sessions WHERE session_key='proof-session' FOR UPDATE`); err != nil {
		blocker.Rollback(context.Background())
		return nil, err
	}
	released := make(chan error, 1)
	go func() {
		time.Sleep(600 * time.Millisecond)
		var err error
		switch p.change {
		case "policy_revoke":
			_, err = blocker.Exec(context.Background(), `UPDATE memory_model_policies SET state='revoked'`)
		case "worker_disable":
			_, err = blocker.Exec(context.Background(), `UPDATE memory_model_control SET state='disabled' WHERE singleton`)
		case "source_change":
			_, err = blocker.Exec(context.Background(), `UPDATE memory_sources SET access_revision=access_revision+1 WHERE source_id=$1`, p.source.ID)
		}
		if p.change == "stop" {
			p.stop()
		}
		if err == nil {
			err = blocker.Commit(context.Background())
		} else {
			blocker.Rollback(context.Background())
		}
		released <- err
	}()
	// Native transport repeats authority after auth resolution and immediately
	// before HTTP. This full proof must not race its own periodic session lock.
	err = transport.CheckInferenceAuthority(ctx)
	if releaseErr := <-released; releaseErr != nil {
		return nil, releaseErr
	}
	if err != nil {
		return nil, err
	}
	if err = transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	p.http.Add(1)
	ch := make(chan transport.TransportEvent, 1)
	ch <- transport.NewAssistantFinalEvent(r.Context.RunID, p.Name(), transport.AssistantFinal{Content: string(jsonBytes(map[string]string{"source_id": p.source.ID, "quote": p.source.Body})), Usage: &transport.Usage{InputTokens: 20, OutputTokens: 12, TotalTokens: 32}})
	close(ch)
	return ch, nil
}

func TestModelSupervisorYieldsOnlyToOwningFullHTTPProofIntegration(t *testing.T) {
	for _, change := range []string{"unchanged", "stop", "policy_revoke", "worker_disable", "source_change"} {
		t.Run(change, func(t *testing.T) {
			w, owner, _ := modelFixture(t)
			w.Jobs.budgets.Policy.ModelMillis = 3000
			_, err := w.Jobs.pool.Exec(owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('proof-session','owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('proof-run','proof-session','i','completed','mode_1','completed','fixture',clock_timestamp(),clock_timestamp())`)
			if err != nil {
				t.Fatal(err)
			}
			src, err := w.Jobs.catalog.Put(owner, catalog.PutSource{ExternalKey: "proof-source", Kind: "document", ScopeType: "session", ScopeID: "proof-session", Body: "source", Title: "source"})
			if err != nil {
				t.Fatal(err)
			}
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			p := &modelSecondAuthorityProof{modelFixtureProvider: modelFixtureProvider{source: src}, store: w.Jobs, change: change, stop: stop}
			w.Provider = p
			q := policyRequest(src)
			q.Job.OriginalRunID = "proof-run"
			j := schedule(t, w, owner, q)
			err = w.execute(ctx, j, QuoteProbe{})
			if change == "unchanged" {
				if err != nil || p.http.Load() != 1 {
					t.Fatal("mandatory current full proof was self-cancelled", err, p.http.Load())
				}
			} else if err == nil || p.http.Load() != 0 {
				t.Fatal("changed authority sent HTTP despite full proof", change, err, p.http.Load())
			}
		})
	}
}

func TestModelSupervisorDoesNotWaitForPublicationJobLockIntegration(t *testing.T) {
	w, owner, src := modelFixture(t)
	j := schedule(t, w, owner, policyRequest(src))
	if _, _, err := w.Admit(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	// The full publication proof intentionally locks the job. Periodic proof
	// must observe its committed claim without waiting behind its own publisher.
	tx, err := w.Jobs.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), `SELECT job_id FROM memory_aux_jobs WHERE job_id=$1 FOR UPDATE`, j.ID); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer stop()
	if err = w.Jobs.checkModelSupervisor(ctx, j); err != nil {
		t.Fatal("supervisor waited for publication job lock", err)
	}
	full, finish := context.WithTimeout(w.Jobs.Context(context.Background(), j), 200*time.Millisecond)
	defer finish()
	if err = w.Jobs.catalog.CheckObjects(full, nil, j.Purpose); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("full publication proof no longer waits for its mandatory job lock", err)
	}
}

func TestModelSupervisorRequiresInstalledSummaryOriginalGuardIntegration(t *testing.T) {
	w, owner, src := modelFixture(t)
	j := schedule(t, w, owner, policyRequest(src))
	if _, _, err := w.Admit(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	// A test predicate with the production contract proves that copied-original
	// denial is never skipped, independently of direct source-pin validity.
	if _, err := w.Jobs.pool.Exec(owner, `CREATE OR REPLACE FUNCTION butler_memory_summary_sources_current(ids TEXT[]) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$ SELECT FALSE $$`); err != nil {
		t.Fatal(err)
	}
	if err := w.Jobs.checkModelSupervisor(context.Background(), j); err == nil {
		t.Fatal("changed summary originals accepted")
	}
}

type modelSlowPublication struct{ QuoteProbe }

func (modelSlowPublication) PublishTx(ctx context.Context, tx pgx.Tx, _ *catalog.Store, _ Job, _ ModelAttempt, _ []catalog.Source, _ catalog.Source, _ string) error {
	_, err := tx.Exec(ctx, `SELECT pg_sleep(0.8)`)
	return err
}

func TestModelSupervisorJoinsBeforeSlowSessionPublicationIntegration(t *testing.T) {
	w, owner, _ := modelFixture(t)
	w.Jobs.budgets.Policy.ModelMillis = 3000
	_, err := w.Jobs.pool.Exec(owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('slow-session','owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('slow-run','slow-session','i','completed','mode_1','completed','fixture',clock_timestamp(),clock_timestamp())`)
	if err != nil {
		t.Fatal(err)
	}
	src, err := w.Jobs.catalog.Put(owner, catalog.PutSource{ExternalKey: "slow-session-source", Kind: "document", ScopeType: "session", ScopeID: "slow-session", Body: "source", Title: "source"})
	if err != nil {
		t.Fatal(err)
	}
	w.Provider.(*modelFixtureProvider).source = src
	q := policyRequest(src)
	q.Job.OriginalRunID = "slow-run"
	j := schedule(t, w, owner, q)
	if err = w.execute(context.Background(), j, modelSlowPublication{}); err != nil {
		t.Fatal("own periodic observer cancelled slow publication", err)
	}
}

func TestModelSupervisorReportsPolicySessionLockDeadlineIntegration(t *testing.T) {
	w, owner, _ := modelFixture(t)
	_, err := w.Jobs.pool.Exec(owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('locked-session','owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('locked-run','locked-session','i','completed','mode_1','completed','fixture',clock_timestamp(),clock_timestamp())`)
	if err != nil {
		t.Fatal(err)
	}
	src, err := w.Jobs.catalog.Put(owner, catalog.PutSource{ExternalKey: "locked-session-source", Kind: "document", ScopeType: "session", ScopeID: "locked-session", Body: "source", Title: "source"})
	if err != nil {
		t.Fatal(err)
	}
	q := policyRequest(src)
	q.Job.OriginalRunID = "locked-run"
	j := schedule(t, w, owner, q)
	if _, _, err = w.Admit(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	tx, err := w.Jobs.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), `SELECT session_key FROM sessions WHERE session_key='locked-session' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer stop()
	phase := ""
	if err = w.Jobs.checkModelSupervisor(ctx, j, func(p string) { phase = p }); !errors.Is(err, context.DeadlineExceeded) || phase != "supervisor_policy" {
		t.Fatal("precise policy-lock deadline not diagnosed", phase, err)
	}
}

type modelInvalidOutput struct{ QuoteProbe }

func (modelInvalidOutput) Validate([]catalog.Source, string) (string, error) {
	return "", errors.New("PRIVATE_CONSUMER_DETAIL")
}

func TestModelInvalidConsumerOutputHasFixedDurableDiagnosticIntegration(t *testing.T) {
	w, owner, src := modelFixture(t)
	j := schedule(t, w, owner, policyRequest(src))
	if err := w.execute(context.Background(), j, modelInvalidOutput{}); !errors.Is(err, ErrConsumerOutputInvalid) || err.Error() != ErrConsumerOutputInvalid.Error() {
		t.Fatal("consumer detail leaked or authority diagnostic substituted", err)
	}
	var jobCode, reason string
	var known, published bool
	if err := w.Jobs.pool.QueryRow(owner, `SELECT j.error_code,a.reason,a.usage IS NOT NULL,a.state='published' FROM memory_aux_jobs j JOIN memory_model_attempts a USING(job_id) WHERE j.job_id=$1`, j.ID).Scan(&jobCode, &reason, &known, &published); err != nil {
		t.Fatal(err)
	}
	if jobCode != "consumer_output_invalid" || reason != jobCode || !known || published {
		t.Fatal(jobCode, reason, known, published)
	}
}
