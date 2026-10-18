package experience

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
)

func terminalFixture(t *testing.T) (*lessonFixture, catalog.SourceRef) {
	t.Helper()
	f := newLessonFixture(t)
	return f, stopFixture(t, f)
}

func stopFixture(t *testing.T, f *lessonFixture) catalog.SourceRef {
	t.Helper()
	_, err := f.pool.Exec(f.ctx, `UPDATE tasks SET status='blocked' WHERE task_id='task-1';UPDATE runs SET current_state='tool_running',status='tool_running' WHERE run_id='run-1';
INSERT INTO accepted_inputs(input_id,session_key,event_id,dedup_key,source,owner_id,event_json,task_id,run_id,status,checkpoint,checkpoint_json,attempts,error_code)
VALUES('stopped-input','experience','original','original','web','owner','{}','task-1','run-1','blocked','tool_result_recorded','{"actual":"unknown"}',1,'effect_unknown')`)
	if err != nil {
		t.Fatal(err)
	}
	o := f.observation
	o.ResultID, o.AgentReport, o.OwnerResultID, o.OwnerVerdict = "", "", "", "unknown"
	o.VerifiedCriteria = []string{}
	o.TerminalAttempt = &TerminalAttempt{InputID: "stopped-input", InputStatus: "blocked", Attempts: 1, Checkpoint: "tool_result_recorded", Reason: "effect_unknown", RunState: "tool_running"}
	if err = f.pool.QueryRow(f.ctx, `SELECT row_hash,revision FROM memory_experience_input_heads WHERE input_id='stopped-input' AND alive`).Scan(&o.TerminalAttempt.InputHash, &o.TerminalAttempt.InputRevision); err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.s.Seal(f.ctx, o, []catalog.SourceRef{ref(f.parent)})
	if err != nil {
		t.Fatal("actual stopped fixture seal", err)
	}
	f.observation = o
	f.snapshot = snapshot
	return snapshot
}

func TestExperienceStoppedInputTombstoneCannotReviveObservationIntegration(t *testing.T) {
	f, snapshot := terminalFixture(t)
	if _, err := f.s.Sources.Get(f.ctx, snapshot.ID); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := f.pool.QueryRow(f.ctx, `SELECT row_hash FROM memory_experience_input_heads WHERE input_id='stopped-input'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	_, err := f.pool.Exec(f.ctx, `DELETE FROM accepted_inputs WHERE input_id='stopped-input';
INSERT INTO accepted_inputs(input_id,session_key,event_id,dedup_key,source,owner_id,event_json,task_id,run_id,status,checkpoint,checkpoint_json,attempts,error_code)
VALUES('stopped-input','experience','original','original','web','owner','{}','task-1','run-1','blocked','tool_result_recorded','{"actual":"unknown"}',1,'effect_unknown')`)
	if err != nil {
		t.Fatal(err)
	}
	var after string
	var revision int64
	if f.pool.QueryRow(f.ctx, `SELECT row_hash,revision FROM memory_experience_input_heads WHERE input_id='stopped-input' AND alive`).Scan(&after, &revision) != nil || before != after || revision <= f.observation.TerminalAttempt.InputRevision {
		t.Fatal("identical input reinsertion discarded its tombstone")
	}
	if _, err = f.s.Sources.Get(f.ctx, snapshot.ID); err == nil {
		t.Fatal("same input ID/payload revived withdrawn terminal evidence")
	}
	if _, err = f.s.Seal(f.ctx, f.observation, []catalog.SourceRef{ref(f.parent)}); err == nil {
		t.Fatal("old input head could be resealed")
	}
}

func TestExperienceStoppedInputAndRunWritersWaitForSourceReleaseIntegration(t *testing.T) {
	for _, target := range []string{"checkpoint", "run"} {
		t.Run(target, func(t *testing.T) {
			f, snapshot := terminalFixture(t)
			ctx, cancel := context.WithTimeout(f.ctx, 10*time.Second)
			defer cancel()
			read, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer read.Rollback(ctx)
			if _, err = f.s.Sources.AccessTx(ctx, read, []catalog.SourceRef{snapshot}, "memory_read"); err != nil {
				t.Fatal(err)
			}
			pid := make(chan uint32, 1)
			done := make(chan error, 1)
			go func() {
				connection, acquireErr := f.pool.Acquire(ctx)
				if acquireErr != nil {
					done <- acquireErr
					return
				}
				defer connection.Release()
				pid <- connection.Conn().PgConn().PID()
				query := `UPDATE accepted_inputs SET checkpoint_json=checkpoint_json||'{"changed":true}'::jsonb WHERE input_id='stopped-input'`
				if target == "run" {
					query = `UPDATE runs SET current_state='failed' WHERE run_id='run-1'`
				}
				_, updateErr := connection.Exec(ctx, query)
				done <- updateErr
			}()
			var writer uint32
			select {
			case writer = <-pid:
			case err = <-done:
				t.Fatal("writer admission", err)
			case <-ctx.Done():
				t.Fatal("writer admission deadline")
			}
			waiting := false
			for until := time.Now().Add(2 * time.Second); time.Now().Before(until); {
				if err = read.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE pid=$1 AND locktype='advisory' AND NOT granted)`, writer).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if !waiting {
				t.Fatal("material writer crossed the source publication fence")
			}
			if err = read.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err = <-done:
				if err != nil {
					t.Fatal("writer after source release", err)
				}
			case <-ctx.Done():
				t.Fatal("writer did not resume after source release")
			}
			if _, err = f.s.Sources.Get(f.ctx, snapshot.ID); err == nil {
				t.Fatal("changed stopped original remained current")
			}
		})
	}
}

func TestExperienceStoppedObservationIgnoresUncopiedErrorTextAndTimesIntegration(t *testing.T) {
	f, snapshot := terminalFixture(t)
	var before, after int64
	if f.pool.QueryRow(f.ctx, `SELECT revision FROM memory_experience_input_heads WHERE input_id='stopped-input'`).Scan(&before) != nil {
		t.Fatal("input head unavailable")
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET error_message='uncopied private diagnostic',updated_at=clock_timestamp()+interval '1 second' WHERE input_id='stopped-input'`); err != nil {
		t.Fatal(err)
	}
	if f.pool.QueryRow(f.ctx, `SELECT revision FROM memory_experience_input_heads WHERE input_id='stopped-input'`).Scan(&after) != nil || before != after {
		t.Fatal("unobserved metadata rewrote the material input head")
	}
	if _, err := f.s.Sources.Get(f.ctx, snapshot.ID); err != nil {
		t.Fatal("unobserved diagnostic/time revoked an unchanged observation", err)
	}
}

func TestExperienceStoppedInputUpgradeBackfillsExistingRowsIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	_, file, _, _ := runtime.Caller(0)
	migrations := filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations")
	apply := func(name string) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(migrations, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(raw)); err != nil {
			t.Fatal("exact input-head migration", name, err)
		}
	}
	apply("080_episodic_terminal_attempts.down.sql")
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel)VALUES('upgrade','owner','web');
INSERT INTO accepted_inputs(input_id,session_key,event_id,dedup_key,source,owner_id,event_json,status,checkpoint,checkpoint_json,attempts,error_code)
VALUES('legacy-input','upgrade','legacy','legacy','web','owner','{}','blocked','tool_result_recorded','{"original":"retained"}',1,'effect_unknown')`); err != nil {
		t.Fatal(err)
	}
	var before, after string
	if pool.QueryRow(ctx, `SELECT to_jsonb(i)::text FROM accepted_inputs i WHERE input_id='legacy-input'`).Scan(&before) != nil {
		t.Fatal("legacy primary input unavailable")
	}
	apply("080_episodic_terminal_attempts.up.sql")
	var correct bool
	if pool.QueryRow(ctx, `SELECT to_jsonb(i)::text,h.revision=1 AND h.alive AND h.row_hash=butler_memory_experience_input_hash(i) FROM accepted_inputs i JOIN memory_experience_input_heads h USING(input_id) WHERE i.input_id='legacy-input'`).Scan(&after, &correct) != nil || before != after || !correct {
		t.Fatal("input-head upgrade changed primary input or missed its exact backfill")
	}
}
