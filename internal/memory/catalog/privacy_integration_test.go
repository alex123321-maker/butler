package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/summary"
	"github.com/butler/butler/internal/ownership"
)

func seedRun(t *testing.T, s *Store) context.Context {
	t.Helper()
	_, e := s.pool.Exec(context.Background(), `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:owner','owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('run','web:owner','input','running','mode_1','model_running','fixture',clock_timestamp(),clock_timestamp());INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('web:owner','run','lease',1,clock_timestamp()+INTERVAL '1 hour')`)
	if e != nil {
		t.Fatal(e)
	}
	return ownership.WithToken(context.Background(), ownership.NewAuthority(s.pool), ownership.Token{SessionKey: "web:owner", RunID: "run", LeaseID: "lease", FenceEpoch: 1})
}

type faultCleaner struct {
	seen []CacheRef
	fail bool
}

func (c *faultCleaner) PurgeWorking(_ context.Context, ref CacheRef) error {
	c.seen = append(c.seen, ref)
	if c.fail {
		return errors.New("cache unavailable")
	}
	return nil
}

func TestPrivacyCleanupRestartAndStaleSummaryWorkingPublicationIntegration(t *testing.T) {
	s, ownerCtx := fixture(t)
	live := seedRun(t, s)
	_, e := s.pool.Exec(ownerCtx, `INSERT INTO messages(message_id,session_key,run_id,role,content) VALUES('original','web:owner','run','user','private text');INSERT INTO tasks(task_id,session_key,owner_id,goal,working_json,last_run_id) VALUES('task','web:owner','owner','preserved owner goal','{"scratch":{"private":"text"}}','run')`)
	if e != nil {
		t.Fatal(e)
	}
	var revision, sequence int64
	if e = s.pool.QueryRow(ownerCtx, `SELECT source_revision,last_sequence FROM transcript_heads WHERE session_key='web:owner'`).Scan(&revision, &sequence); e != nil {
		t.Fatal(e)
	}
	old := summary.Snapshot{SessionKey: "web:owner", Summary: "private derived summary", ThroughMessageID: "original", ThroughSequence: sequence, SourceRevision: revision, FormatVersion: 1}
	if ok, e := summary.NewStore(s.pool).SaveCoveredSummary(ownerCtx, old); e != nil || !ok {
		t.Fatal(ok, e)
	}
	if e = s.PinTranscript(live, "web:owner", revision); e != nil {
		t.Fatal(e)
	}
	src := put(t, s, ownerCtx, "source", "source text")
	if _, e = s.Get(live, src.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Delete(ownerCtx, src.ID, "delete", src.Revision); e != nil {
		t.Fatal(e)
	}
	if ok, e := summary.NewStore(s.pool).SaveCoveredSummary(ownerCtx, old); e != nil || ok {
		t.Fatal("late summary resurrected", ok, e)
	}
	called := false
	if e = s.GuardCacheWrite(live, "run", func(context.Context) error { called = true; return nil }); e == nil || called {
		t.Fatal("stale cache writer authorized", called, e)
	}
	if _, e = s.pool.Exec(ownerCtx, `UPDATE tasks SET working_json='{"scratch":{"private":"late"}}' WHERE task_id='task'`); e == nil {
		t.Fatal("stale task scratch restored")
	}
	var goal string
	var workingRevision int64
	if e = s.pool.QueryRow(ownerCtx, `SELECT goal,working_revision FROM tasks WHERE task_id='task'`).Scan(&goal, &workingRevision); e != nil || goal != "preserved owner goal" || workingRevision < 2 {
		t.Fatal(goal, workingRevision, e)
	}
	cleaner := &faultCleaner{fail: true}
	bounded, stop := context.WithTimeout(ownerCtx, 3*time.Second)
	defer stop()
	if found, e := s.CleanupPending(bounded, cleaner); !found || e == nil {
		t.Fatal("fault was not pending", found, e)
	}
	// Reconstructed Store models process restart; the PG cursor remains durable.
	restarted := New(s.pool, s.manifest)
	cleaner.fail = false
	for i := 0; i < 3; i++ {
		if _, e = restarted.CleanupPending(bounded, cleaner); e != nil {
			t.Fatal(e)
		}
	}
	if len(cleaner.seen) != 2 || cleaner.seen[0] != (CacheRef{"web:owner", "run"}) || cleaner.seen[1] != cleaner.seen[0] {
		t.Fatal("cleanup was not scoped/idempotent", cleaner.seen)
	}
	var status string
	if e = s.pool.QueryRow(ownerCtx, `SELECT cache_status FROM memory_deletions`).Scan(&status); e != nil || status != "done" {
		t.Fatal(status, e)
	}
}
func TestDeletionRedactsImmutableFinalWithoutInventingUndoIntegration(t *testing.T) {
	s, ctx := fixture(t)
	_ = seedRun(t, s)
	_, e := s.pool.Exec(ctx, `INSERT INTO messages(message_id,session_key,run_id,role,content) VALUES('original','web:owner','run','user','private input');INSERT INTO messages(message_id,session_key,run_id,role,content) VALUES('answer','web:owner','run','assistant','derived private answer');INSERT INTO artifacts(artifact_id,run_id,session_key,artifact_type,content_text) VALUES('artifact','run','web:owner','assistant_final','derived private answer');INSERT INTO run_final_results(result_id,run_id,session_key,artifact_id,message_id,content,content_hash,answer_state,effects_json) VALUES('result','run','web:owner','artifact','answer','derived private answer','original-hash','final','[{"action_id":"effect","effect_state":"effect_confirmed","evidence_refs":["receipt-hash"]}]');INSERT INTO delivery_outbox(notification_id,result_id,run_id,session_key,channel,chunk_index,content,status,deadline_at,claim_id,claim_until) VALUES('notification','result','run','web:owner','telegram',0,'derived private answer','sending',clock_timestamp()+INTERVAL '1 hour','cached-claim',clock_timestamp()+INTERVAL '30 seconds')`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.pool.Exec(ctx, `UPDATE run_final_results SET content='malicious rewrite' WHERE result_id='result'`); e == nil {
		t.Fatal("result immutability weakened")
	}
	if _, e = s.Delete(ctx, "message:original", "delete", 1); e != nil {
		t.Fatal(e)
	}
	var body, sum, verdict, effects string
	var redacted bool
	if e = s.pool.QueryRow(ctx, `SELECT content,content_hash,verdict,effects_json::text,privacy_redacted FROM run_final_results`).Scan(&body, &sum, &verdict, &effects, &redacted); e != nil || body != "" || sum != "original-hash" || verdict != "unverified" || !redacted {
		t.Fatal(body, sum, verdict, redacted, e)
	}
	var status string
	if e = s.pool.QueryRow(ctx, `SELECT content,status FROM delivery_outbox`).Scan(&body, &status); e != nil || body != "" || status != "delivery_unknown" {
		t.Fatal("in-flight receipt was falsely undone", body, status, e)
	}
	if _, e = s.pool.Exec(ctx, `UPDATE run_final_results SET content='resurrect private answer',privacy_redacted=FALSE WHERE result_id='result'`); e == nil {
		t.Fatal("privacy redaction reversed")
	}
	var n int
	if e = s.pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE content<>''`).Scan(&n); e != nil || n != 0 {
		t.Fatal("derived transcript survived", n, e)
	}
}
func TestLiveSourceAndTranscriptPinsRejectLatePublicationIntegration(t *testing.T) {
	s, ownerCtx := fixture(t)
	runCtx := seedRun(t, s)
	src := put(t, s, ownerCtx, "private-doc", "private data")
	if _, e := s.Get(runCtx, src.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Delete(ownerCtx, src.ID, "delete", 1); e != nil {
		t.Fatal(e)
	}
	if _, e := s.pool.Exec(runCtx, `INSERT INTO messages(message_id,session_key,run_id,role,content) VALUES('late','web:owner','run','assistant','late private data')`); e == nil {
		t.Fatal("late final reintroduced deleted source")
	}
	// A source amendment also fences a prepared run, before deletion.
	s2, ctx2 := fixture(t)
	live := seedRun(t, s2)
	if _, e := s2.pool.Exec(ctx2, `INSERT INTO messages(message_id,session_key,run_id,role,content) VALUES('input','web:owner','run','user','original text')`); e != nil {
		t.Fatal(e)
	}
	if e := s2.PinTranscript(live, "web:owner", 1); e != nil {
		t.Fatal(e)
	}
	if _, e := s2.pool.Exec(ctx2, `UPDATE messages SET content='corrected' WHERE message_id='input'`); e != nil {
		t.Fatal(e)
	}
	if e := s2.PinTranscript(live, "web:owner", 1); !errors.Is(e, ErrStale) {
		t.Fatal("old transcript coverage accepted", e)
	}
	if _, e := s2.pool.Exec(live, `INSERT INTO artifacts(artifact_id,run_id,session_key,artifact_type,content_text) VALUES('late','run','web:owner','assistant_final','old cached context')`); e == nil {
		t.Fatal("old prepared context persisted")
	}
}
