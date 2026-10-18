package experience

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/ownership"
)

func TestExperienceCapturedToolChangeRevokesAcceptedReadIntegration(t *testing.T) {
	for _, mode := range []string{"update", "delete"} {
		t.Run(mode, func(t *testing.T) {
			f := newLessonFixture(t)
			lesson := f.publish(t)
			f.accept(t, lesson)
			tx, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(f.ctx)
			descendant, err := f.s.Sources.CreateDerivedTx(f.ctx, tx, []catalog.SourceRef{lesson}, "memory_extract", "Dependent experience DATA", "Copied lesson dependent on the original tool observation", json.RawMessage(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(f.ctx); err != nil {
				t.Fatal(err)
			}
			if _, err = f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision)VALUES('consumer','experience','consumer','running','model_running','mode_1','fixture',clock_timestamp(),clock_timestamp(),'task-1',1);INSERT INTO session_ownership(session_key,run_id,owner_id,lease_id,fence_epoch,expires_at)VALUES('experience','consumer','owner','lease',1,clock_timestamp()+interval '1 hour')`); err != nil {
				t.Fatal(err)
			}
			live := ownership.WithToken(context.Background(), nil, ownership.Token{SessionKey: "experience", RunID: "consumer", LeaseID: "lease", FenceEpoch: 1})
			p, err := f.s.Prepare(live, "consumer")
			if err != nil {
				t.Fatal(err)
			}
			tx, err = f.pool.Begin(live)
			if err != nil {
				t.Fatal(err)
			}
			sources, err := f.s.Sources.AccessTx(live, tx, []catalog.SourceRef{lesson}, "memory_read")
			if err != nil {
				t.Fatal(err)
			}
			if yes, err := f.s.EligibleTx(live, tx, sources[0], p, true); err != nil || !yes {
				t.Fatal("current lesson not pinned", err)
			}
			if err = tx.Commit(live); err != nil {
				t.Fatal(err)
			}
			if err = f.s.CheckPinned(live, "consumer"); err != nil {
				t.Fatal(err)
			}
			if mode == "update" {
				_, err = f.pool.Exec(f.ctx, `UPDATE tool_calls SET result='{"changed":true}'::jsonb WHERE tool_call_id=$1`, f.observation.ToolPins[0].ID)
			} else {
				_, err = f.pool.Exec(f.ctx, `DELETE FROM tool_calls WHERE tool_call_id=$1`, f.observation.ToolPins[0].ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{f.snapshot.ID, lesson.ID, descendant.ID} {
				if _, err = f.s.Sources.Get(f.ctx, id); err == nil {
					t.Fatal("changed tool copy or descendant remained readable", id)
				}
			}
			if f.s.CheckPinned(live, "consumer") == nil {
				t.Fatal("late tool mutation did not cancel current read pin")
			}
			var current bool
			if f.pool.QueryRow(f.ctx, `SELECT butler_memory_experience_sources_current(ARRAY[$1::text])`, descendant.ID).Scan(&current) != nil || current {
				t.Fatal("thin/current source predicate accepted changed tool")
			}
			if _, err = f.s.Seal(f.ctx, f.observation, []catalog.SourceRef{ref(f.parent)}); err == nil {
				t.Fatal("stale captured rows resealed")
			}
		})
	}
}

func TestExperienceIdenticalToolReinsertCannotReviveOldObservationIntegration(t *testing.T) {
	f := newLessonFixture(t)
	lesson := f.publish(t)
	f.accept(t, lesson)
	var raw []byte
	id := f.observation.ToolPins[0].ID
	if err := f.pool.QueryRow(f.ctx, `SELECT to_jsonb(t) FROM tool_calls t WHERE tool_call_id=$1`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `DELETE FROM tool_calls WHERE tool_call_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO tool_calls SELECT (jsonb_populate_record(NULL::tool_calls,$1::jsonb)).*`, raw); err != nil {
		t.Fatal(err)
	}
	var digest string
	var revision int64
	var alive bool
	if f.pool.QueryRow(f.ctx, `SELECT row_hash,revision,alive FROM memory_experience_tool_heads WHERE tool_call_id=$1`, id).Scan(&digest, &revision, &alive) != nil || digest != f.observation.ToolPins[0].Hash || revision != f.observation.ToolPins[0].Revision+2 || !alive {
		t.Fatal("same stored bytes did not retain monotonic tombstone history")
	}
	if _, err := f.s.Sources.Get(f.ctx, lesson.ID); err == nil {
		t.Fatal("same ID/identical bytes resurrected old lesson")
	}
	if _, err := f.s.Seal(f.ctx, f.observation, []catalog.SourceRef{ref(f.parent)}); err == nil {
		t.Fatal("old pin reused after delete/reinsert")
	}
}

func TestExperienceToolHeadUpgradeBackfillsExistingRowsIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	_, file, _, _ := runtime.Caller(0)
	migration := filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations")
	apply := func(name string) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(migration, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(raw)); err != nil {
			t.Fatal(name, err)
		}
	}
	// Empty-history rollback creates the real pre-077 schema. Existing primary
	// rows are inserted before applying the exact checked-in upgrade script.
	var payloadMigration bool
	var terminalMigration bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('memory_experience_terminal_attempts') IS NOT NULL`).Scan(&terminalMigration); err != nil {
		t.Fatal(err)
	}
	if terminalMigration {
		apply("080_episodic_terminal_attempts.down.sql")
	}
	if err := pool.QueryRow(ctx, `SELECT to_regclass('memory_run_payload_redactions') IS NOT NULL`).Scan(&payloadMigration); err != nil {
		t.Fatal(err)
	}
	if payloadMigration {
		apply("079_memory_tool_payload_redaction.down.sql")
	}
	apply("077_episodic_experience.down.sql")
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel)VALUES('upgrade','owner','web');INSERT INTO transcript_heads(session_key)VALUES('upgrade');INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at)VALUES('legacy-run','upgrade','legacy','completed','completed','mode_1','fixture',clock_timestamp(),clock_timestamp())`); err != nil {
		t.Fatal(err)
	}
	store := transcript.NewStore(pool)
	if _, err := store.AppendToolCall(ctx, transcript.ToolCall{ToolCallID: "legacy-tool", RunID: "legacy-run", ToolName: "legacy.read", Status: "completed", RuntimeTarget: "fixture", ResultJSON: `{"observed":"legacy original"}`}); err != nil {
		t.Fatal(err)
	}
	var before, after string
	if err := pool.QueryRow(ctx, `SELECT to_jsonb(t)::text FROM tool_calls t WHERE tool_call_id='legacy-tool'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	apply("077_episodic_experience.up.sql")
	if payloadMigration {
		apply("079_memory_tool_payload_redaction.up.sql")
	}
	if terminalMigration {
		apply("080_episodic_terminal_attempts.up.sql")
	}
	if err := pool.QueryRow(ctx, `SELECT to_jsonb(t)::text FROM tool_calls t WHERE tool_call_id='legacy-tool'`).Scan(&after); err != nil || after != before {
		t.Fatal("upgrade changed original tool data", err)
	}
	snapshot, err := store.CaptureRunTools(ctx, "legacy-run", 2*MaxActions)
	if err != nil || len(snapshot.Pins) != 1 || snapshot.Pins[0].Revision != 1 {
		t.Fatal("existing rows were not backfilled", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE tool_calls SET result=result WHERE tool_call_id='legacy-tool'`); err != nil {
		t.Fatal(err)
	}
	var revision int64
	if pool.QueryRow(ctx, `SELECT revision FROM memory_experience_tool_heads WHERE tool_call_id='legacy-tool'`).Scan(&revision) != nil || revision != 2 {
		t.Fatal("upgrade did not install atomic monotonic writer")
	}
}

func TestExperienceUnrelatedAppendsAndDeletionKeepExactToolPinsIntegration(t *testing.T) {
	f := newLessonFixture(t)
	lesson := f.publish(t)
	f.accept(t, lesson)
	store := transcript.NewStore(f.pool)
	if _, err := store.AppendToolCall(f.ctx, transcript.ToolCall{ToolCallID: "future-unrelated-tool", RunID: "run-1", ToolName: "unrelated.read", Status: "completed", RuntimeTarget: "fixture", ResultJSON: `{"later":"not copied"}`}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendMessage(f.ctx, transcript.Message{SessionKey: "experience", RunID: "run-1", Role: "user", Content: "A later unrelated message"}); err != nil {
		t.Fatal(err)
	}
	unrelated, err := f.s.Sources.Put(f.ctx, catalog.PutSource{ExternalKey: "unrelated-source-for-deletion", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "Unrelated source"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Sources.Delete(f.ctx, unrelated.ID, "delete", unrelated.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Sources.Get(f.ctx, lesson.ID); err != nil {
		t.Fatal("unrelated append/global privacy watermark revoked exact original", err)
	}
	var historical, now int64
	if f.pool.QueryRow(f.ctx, `SELECT o.transcript_revision,h.source_revision FROM memory_experience_observations o JOIN transcript_heads h USING(session_key) WHERE o.source_id=$1`, f.snapshot.ID).Scan(&historical, &now) != nil || historical != f.observation.TranscriptRevision || now <= historical {
		t.Fatal("historical revision was overwritten or watermark did not change")
	}
}

func TestExperienceToolFingerprintCoversStoredFieldsIntegration(t *testing.T) {
	f := newLessonFixture(t)
	for _, field := range []string{"tool_name", "args", "status", "runtime_target", "started_at", "finished_at", "result", "error"} {
		t.Run(field, func(t *testing.T) {
			tx, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(f.ctx)
			updates := map[string]string{"tool_name": "'changed.tool'", "args": "'{\"different\":true}'::jsonb", "status": "'failed'", "runtime_target": "'new-runtime'", "started_at": "started_at+interval '1 second'", "finished_at": "clock_timestamp()", "result": "'{\"withdrawn\":true}'::jsonb", "error": "'{\"failure\":true}'::jsonb"}
			if _, err = tx.Exec(f.ctx, "UPDATE tool_calls SET "+field+"="+updates[field]+" WHERE tool_call_id=$1", f.observation.ToolPins[0].ID); err != nil {
				t.Fatal(err)
			}
			var current bool
			if tx.QueryRow(f.ctx, `SELECT butler_memory_experience_sources_current(ARRAY[$1::text])`, f.snapshot.ID).Scan(&current) != nil || current {
				t.Fatal("copied field mutation not fenced", field)
			}
		})
	}
}
