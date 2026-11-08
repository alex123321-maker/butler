package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/benchtrace"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This fixture exercises the collector's real SQL, acknowledgement and grading.
// The HTTP producer and model turns are controlled. No provider is contacted.
func TestLiveCollectorWithPostgres(t *testing.T) {
	dsn := os.Getenv("BUTLER_BENCH_TEST_DB")
	if dsn == "" {
		t.Skip("set BUTLER_BENCH_TEST_DB for the disposable PostgreSQL integration")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Path != "/butler_bench_test" || u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" {
		t.Fatal("dedicated local butler_bench_test required")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := "bench_" + newRun(Plan{}, Candidate{}, Setup{}).ID
	schema := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	q := u.Query()
	q.Set("search_path", name)
	u.RawQuery = q.Encode()
	t.Setenv("BUTLER_BENCH_DB", u.String())
	writer, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	_, err = writer.Exec(ctx, `
      CREATE TABLE accepted_inputs(input_id text,event_id text,session_key text,source text,task_id text,run_id text,status text,error_code text);
      CREATE TABLE runs(run_id text,session_key text);
      CREATE TABLE run_final_results(result_id text,run_id text,session_key text,task_id text,content text,answer_state text,content_hash text,privacy_redacted boolean);
      CREATE TABLE delivery_outbox(result_id text,run_id text,session_key text,channel text,status text);
      CREATE TABLE execution_model_attempts(run_id text,provider text,model text,actual_input_tokens bigint,actual_output_tokens bigint);
      CREATE TABLE tool_calls(run_id text,tool_call_id text,tool_name text,status text,args jsonb,result jsonb,started_at timestamptz);
      CREATE TABLE action_intents(run_id text,tool_call_id text,binding jsonb,effect_id text,created_at timestamptz);
      CREATE TABLE action_effects(action_id text,state text);
      ALTER TABLE accepted_inputs ADD accepted_at timestamptz DEFAULT now(), ADD updated_at timestamptz DEFAULT now(), ADD checkpoint text DEFAULT 'done', ADD attempts int DEFAULT 1, ADD checkpoint_json jsonb DEFAULT '{}', ADD owner_id text DEFAULT 'lab';
      ALTER TABLE runs ADD current_state text DEFAULT 'completed', ADD started_at timestamptz DEFAULT now(), ADD finished_at timestamptz DEFAULT now(), ADD autonomy_mode text DEFAULT 'mode_1';
      ALTER TABLE run_final_results ADD artifact_id text DEFAULT 'art', ADD message_id text DEFAULT 'msg', ADD effects_json jsonb DEFAULT '[]', ADD limitations_json jsonb DEFAULT '[]', ADD created_at timestamptz DEFAULT now();
      ALTER TABLE delivery_outbox ADD notification_id text DEFAULT 'notice', ADD chunk_index int DEFAULT 0, ADD attempt_count int DEFAULT 1, ADD error_code text DEFAULT '', ADD privacy_redacted bool DEFAULT false, ADD created_at timestamptz DEFAULT now(), ADD updated_at timestamptz DEFAULT now();
      ALTER TABLE execution_model_attempts ADD operation_id text, ADD input_allowance bigint DEFAULT 32, ADD output_allowance bigint DEFAULT 16, ADD created_at timestamptz DEFAULT now(), ADD observed_at timestamptz;
      ALTER TABLE tool_calls ADD finished_at timestamptz DEFAULT now(), ADD runtime_target text DEFAULT 'doctor', ADD error jsonb;
      ALTER TABLE action_effects ADD dispatch_outcome text DEFAULT 'completed', ADD updated_at timestamptz DEFAULT now();
      CREATE TABLE run_state_transitions(id bigserial,run_id text,from_state text,to_state text,triggered_by text DEFAULT 'core',metadata_json jsonb DEFAULT '{}',transitioned_at timestamptz DEFAULT now());
      CREATE TABLE messages(message_id text,run_id text,session_key text,role text,content text,tool_call_id text,created_at timestamptz DEFAULT now(),metadata jsonb DEFAULT '{}',privacy_redacted bool DEFAULT false);
      CREATE TABLE artifacts(artifact_id text,run_id text,session_key text,artifact_type text,title text,summary text,content_text text,content_json jsonb DEFAULT '{}',content_format text DEFAULT 'text',source_type text DEFAULT 'run',source_ref text DEFAULT '',privacy_redacted bool DEFAULT false,created_at timestamptz DEFAULT now());
      CREATE TABLE guarded_single_tab_requests(approval_id text,request_id text,run_id text,session_key text,task_id text,owner_id text,state text,expires_at timestamptz);
      CREATE TABLE guarded_single_tab_dispatches(request_id text,run_id text);`)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("x", 36)
	tokenFile := filepath.Join(t.TempDir(), "owner")
	if err = os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	var posts atomic.Int32
	traceRoot := t.TempDir()
	answer := "degraded"
	sum := sha256.Sum256([]byte(answer))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/events" || r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "wrong route", 403)
			return
		}
		var event struct {
			ID      string `json:"event_id"`
			Key     string `json:"idempotency_key"`
			Session string `json:"session_key"`
			Source  string
			Payload map[string]string
		}
		if json.NewDecoder(r.Body).Decode(&event) != nil || event.Payload["text"] == "" || event.Payload["intent"] != "conversation" || event.Source != "web" || event.ID != event.Key || event.ID != r.Header.Get("X-Butler-Delivery-ID") {
			http.Error(w, "invalid input", 400)
			return
		}
		posts.Add(1)
		// A durable failure must not be overwritten by a favourable later model response.
		in, run := "input-"+event.ID, "run-"+event.ID
		batch := &pgx.Batch{}
		batch.Queue("INSERT INTO accepted_inputs VALUES ($1,$2,$3,'web',NULL,$4,'completed','')", in, event.ID, event.Session, run)
		batch.Queue("INSERT INTO runs VALUES ($1,$2)", run, event.Session)
		batch.Queue("INSERT INTO run_final_results VALUES ($1,$1,$2,NULL,$3,'final',$4,false)", run, event.Session, answer, hex.EncodeToString(sum[:]))
		batch.Queue("INSERT INTO delivery_outbox VALUES ($1,$1,$2,'web','result_ready')", run, event.Session)
		batch.Queue("INSERT INTO execution_model_attempts VALUES ($1,'fixture','model-a',17,3,'one'),($1,'fixture','model-a',5,NULL,'two')", run)
		batch.Queue("INSERT INTO run_state_transitions(run_id,from_state,to_state) VALUES ($1,'queued','model_running'),($1,'model_running','completed')", run)
		batch.Queue("INSERT INTO messages VALUES ('msg',$1,$2,'assistant',$3)", run, event.Session, answer)
		batch.Queue("INSERT INTO artifacts VALUES ('art',$1,$2,'assistant_final','lab artifact','summary',$3)", run, event.Session, answer)
		bound := benchtrace.Bind(r.Context(), traceRoot, "web", event.Session, event.ID, run)
		benchtrace.Measure(bound, "prompt_assembly")()
		for _, op := range []string{"one", "two"} {
			writer, err := benchtrace.Begin(bound, run, op, "fixture", "model-a", &transport.StartRunRequest{InputItems: []transport.InputItem{{Role: "user", Content: event.Payload["text"]}}})
			if err != nil || writer == nil {
				t.Error("synthetic trace unavailable", err)
				http.Error(w, "trace unavailable", 500)
				return
			}
			writer.Event(transport.NewAssistantFinalEvent(run, "fixture", transport.AssistantFinal{Content: answer}))
			if err := writer.End("stream_closed"); err != nil {
				t.Error(err)
			}
		}
		batch.Queue(`INSERT INTO tool_calls VALUES ($1,'t1','doctor.check_system','completed','{}','{"status":"degraded"}',now())`, run)
		batch.Queue(`INSERT INTO action_intents VALUES ($1,'t1','{"tool_name":"doctor.check_system"}','e1',now())`, run)
		batch.Queue("INSERT INTO action_effects VALUES ('e1','effect_confirmed')")
		if err := writer.SendBatch(r.Context(), batch).Close(); err != nil {
			t.Error(err)
			http.Error(w, "write failed", 500)
			return
		}
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]any{"accepted": true, "input_id": in, "session_key": event.Session})
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	client, err := newClient(ctx, server.URL, tokenFile, Candidate{"fixture", "model-a"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.db.Close()
	defer client.http.CloseIdleConnections()
	client.traceDir = traceRoot
	p, work, err := makePlan(testSetup(), "one", "", "system-health", 1, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	run := newRun(p, client.candidate, testSetup())
	dir := filepath.Join(t.TempDir(), "run")
	if err = execute(ctx, client, dir, run, work); err != nil {
		t.Fatal(err)
	}
	_, rows, err := readRun(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Status != "pass" || posts.Load() != 1 {
		t.Fatalf("bad fixture run: %+v", rows)
	}
	got := rows[0].Turns[0]
	reprocessed, err := replay(dir, run, rows)
	if err != nil || reprocessed.Regraded != 1 {
		t.Fatal("offline grading", reprocessed, err)
	}
	archive := filepath.Join(dir, got.Event)
	raw, err := os.ReadFile(filepath.Join(archive, "timeline.json"))
	var spans []interval
	if err != nil || json.Unmarshal(raw, &spans) != nil {
		t.Fatal("missing timeline", err)
	}
	kinds := map[string]int{}
	for _, span := range spans {
		kinds[span.Lane]++
		if span.Lane == "tool" && (span.US == nil || str(field(span.Details, "result"), "status") != answer) {
			t.Fatal("missing tool timing/result")
		}
		if span.Lane == "artifact" && str(span.Details, "content_text") != answer {
			t.Fatal("missing artifact content")
		}
	}
	if kinds["model_admission"] != 2 || kinds["system_state"] != 2 || kinds["tool"] != 1 || kinds["artifact"] != 1 || kinds["message"] != 1 || got.HTTPUS == nil {
		t.Fatal("timeline coverage", kinds)
	}
	if err := os.Remove(filepath.Join(traceRoot, got.Event, benchtrace.Filename(got.Run, "two"))); err != nil {
		t.Fatal(err)
	}
	if err := client.capture(ctx, t.TempDir(), rows[0].Session, work[0].Steps[0], &got); err == nil || err.Error() != "model_trace_incomplete" {
		t.Fatal("missing model request trace accepted", err)
	}
	if got.ModelCalls == nil || *got.ModelCalls != 2 || got.InputTokens == nil || *got.InputTokens != 22 || got.OutputTokens != nil || got.KnownOutput != 3 {
		t.Fatalf("unknown usage lost: %+v", got)
	}
	if _, err = client.db.Exec(ctx, "INSERT INTO runs VALUES ('x','y')"); err == nil {
		t.Fatal("observer can write")
	}
	client.candidate.Model = "wrong-model"
	shot, err := client.observe(ctx, rows[0].Session, got.Event, got.Input, "")
	if err != nil || verdict(shot, work[0].Steps[0]) != "model_mismatch" {
		t.Fatal("wrong model credited", err)
	}
	if _, err = client.observe(ctx, "foreign", got.Event, got.Input, ""); err == nil {
		t.Fatal("foreign evidence accepted")
	}
	client.candidate.Model = "model-a"
	if _, err = writer.Exec(ctx, "UPDATE run_final_results SET privacy_redacted=true"); err != nil {
		t.Fatal(err)
	}
	shot, err = client.observe(ctx, rows[0].Session, got.Event, got.Input, "")
	if err != nil || verdict(shot, work[0].Steps[0]) != "final_evidence_missing" {
		t.Fatal("redacted final credited", err)
	}
	// A wait is creditable only for the exact task, owner, live approval and dispatch.
	task := "task_" + strings.Repeat("a", 32)
	if _, err = writer.Exec(ctx, `UPDATE accepted_inputs SET task_id=$1,status='waiting_for_user',checkpoint='awaiting_approval',checkpoint_json='{"approval_id":"a","continuation":{"kind":"guarded_tab_selection","version":1,"approval_id":"a"}}'`, task); err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Exec(ctx, "DELETE FROM run_final_results"); err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Exec(ctx, "INSERT INTO guarded_single_tab_requests SELECT 'a','q',run_id,session_key,task_id,owner_id,'pending',now()+interval '1 minute' FROM accepted_inputs"); err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Exec(ctx, "INSERT INTO guarded_single_tab_dispatches SELECT 'q',run_id FROM runs"); err != nil {
		t.Fatal(err)
	}
	shot, err = client.observe(ctx, rows[0].Session, got.Event, got.Input, task)
	if err != nil || !shot.guardedWait {
		t.Fatal("guarded checkpoint", err)
	}
	if _, err = writer.Exec(ctx, "UPDATE guarded_single_tab_requests SET owner_id='other'"); err != nil {
		t.Fatal(err)
	}
	shot, err = client.observe(ctx, rows[0].Session, got.Event, got.Input, task)
	if err != nil || shot.guardedWait {
		t.Fatal("foreign approval credited", err)
	}
	if err = os.WriteFile(filepath.Join(archive, "timeline.json"), []byte("[]"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readRun(dir); err == nil {
		t.Fatal("tampered archive accepted")
	}
}

func TestLostAcknowledgementIsNeverResubmitted(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		conn, _, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		conn.Close()
	}))
	defer s.Close()
	client := liveClient{endpoint: s.URL, token: "synthetic", http: s.Client(), traceDir: t.TempDir()}
	turn := client.turn(t.Context(), "web:bench:x", "bench-"+strings.Repeat("a", 24)+"-0001-01", Step{Prompt: "one request"}, t.TempDir())
	if turn.Reason != "submission_unknown" || turn.ModelCalls != nil || calls.Load() != 1 {
		t.Fatalf("lost acknowledgement: %+v; calls %d", turn, calls.Load())
	}
}
