package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/task"
)

func TestBehavior76EvidenceExportIntegration(t *testing.T) {
	f := newBehavior76Fixture(t)
	episode := behavior76DevelopmentEpisode()
	episode.Initial.Tasks[0].ExecutionPolicy = "normal"
	f.seed(episode)
	provider := &behavior76SetupProvider{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, fixture: f}
	if outcome, err := f.evaluate(provider, episode.Steps[0]); err != nil || outcome.State != "completed" {
		t.Fatal("public initial evaluation failed", err)
	}
	if _, err := f.apply(behavior76Step{ID: "public-export-delete", Event: behavior76Event{Type: "profile_change", SourceKey: "public-pref", Operation: "delete"}}); err != nil {
		t.Fatal(err)
	}
	before := f.snapshot()
	if before.DeletionWatermark <= 0 {
		t.Fatal("actual deletion watermark absent")
	}
	if _, err := f.apply(behavior76Step{ID: "public-export-restart", Event: behavior76Event{Type: "service_restart"}}); err != nil {
		t.Fatal(err)
	}
	if outcome, err := f.evaluate(provider, episode.Steps[1]); err != nil || outcome.State != "completed" {
		t.Fatal("public post-deletion evaluation failed", err)
	}
	after := f.snapshot()
	anchor, err := os.ReadFile(f.manifestPath)
	if err != nil || after.DeletionManifestSHA256 != behavior76Hash(anchor) || before.DeletionManifestSHA256 != after.DeletionManifestSHA256 || before.DeletionWatermark != after.DeletionWatermark {
		t.Fatal("actual deletion anchor changed across restart", err)
	}
	dir := t.TempDir()
	f.export(dir)
	tables := []string{"tasks", "task_results", "task_control_events", "task_control_targets", "run_final_results", "accepted_inputs", "runs", "action_intents", "action_authorizations", "action_mandates", "action_effects", "action_effect_dispatches", "action_effect_evidence", "tool_calls", "execution_model_attempts", "execution_budget_scopes", "execution_budget_reservations", "behavior76_objects", "behavior76_object_events", "behavior76_recipient_receipts", "behavior76_aliases", "behavior76_observations", "behavior76_task_mandates", "memory_profile", "memory_claims", "memory_sources", "memory_source_revisions", "memory_source_dependencies", "memory_run_sources", "memory_scope_grants", "memory_fact_heads", "session_ownership", "sessions", "messages", "memory_run_transcripts", "transcript_heads", "memory_restore_state", "memory_deletions", "memory_summary_sources", "memory_summary_dependencies", "session_summaries", "memory_temporal_run_pins", "memory_temporal_source_pins", "memory_claim_temporal_heads", "memory_claim_temporal_revisions", "memory_fact_decision_history", "task_authority_bindings", "owner_goals"}
	for _, table := range tables {
		raw, err := os.ReadFile(filepath.Join(dir, table+".jsonl"))
		if err != nil {
			t.Fatal("required actual table export missing", table, err)
		}
		scanner := bufio.NewScanner(bytes.NewReader(raw))
		scanner.Buffer(make([]byte, 4096), 4<<20)
		for scanner.Scan() {
			if !json.Valid(scanner.Bytes()) {
				t.Fatal("invalid exported JSON row", table)
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
	}
	var schema string
	if err := f.pool.QueryRow(f.ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	name := "public-evidence-" + schema
	if !f.backupPrivate(name) {
		t.Fatal("public private-backup smoke failed")
	}
	backupAnchor, err := os.ReadFile(filepath.Join("/private/behavior76/backups", name+".deletions.log"))
	if err != nil || !bytes.Equal(anchor, backupAnchor) || behavior76Hash(backupAnchor) != after.DeletionManifestSHA256 {
		t.Fatal("private anchor backup is not exact", err)
	}
	cmd := exec.CommandContext(f.ctx, "/usr/bin/pg_restore", "--file=/dev/null", filepath.Join("/private/behavior76/backups", name+".dump"))
	if err := cmd.Run(); err != nil {
		t.Fatal("private PG archive unreadable", err)
	}
	t.Logf("public evidence smoke: %d actual tables; exact deletion anchor and readable PG archive; native/auth=0", len(tables))
}

type behavior76Snapshot struct {
	Tasks                  []task.Record      `json:"tasks"`
	Objects                []behavior76Object `json:"objects"`
	Profiles               []json.RawMessage  `json:"profiles"`
	Claims                 []json.RawMessage  `json:"claims"`
	Sources                []json.RawMessage  `json:"sources"`
	DeletionManifestSHA256 string             `json:"deletion_manifest_sha256"`
	DeletionWatermark      int64              `json:"deletion_watermark"`
}

func (f *behavior76Fixture) backupPrivate(name string) bool {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var schema string
	if err := f.pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		f.t.Error("backup schema identity unavailable")
		return false
	}
	path := filepath.Join("/private/behavior76/backups", name+".dump")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		f.t.Error("private backup cannot overwrite earlier bytes")
		return false
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/pg_dump", "-Fc", "--no-owner", "--no-acl", "--schema="+schema, "--dbname=postgres://postgres@127.0.0.1:55451/behavior76_test?sslmode=disable")
	cmd.Stdout = file
	err = cmd.Run()
	syncErr := file.Sync()
	closeErr := file.Close()
	if err != nil || syncErr != nil || closeErr != nil {
		f.t.Error("private schema backup unavailable")
		return false
	}
	anchor, err := os.ReadFile(f.manifestPath)
	if err != nil {
		f.t.Error("original deletion anchor unavailable for private backup")
		return false
	}
	anchorFile, err := os.OpenFile(filepath.Join("/private/behavior76/backups", name+".deletions.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		f.t.Error("private deletion anchor cannot overwrite earlier bytes")
		return false
	}
	_, writeErr := anchorFile.Write(anchor)
	syncErr = anchorFile.Sync()
	closeErr = anchorFile.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		f.t.Error("private exact deletion anchor backup unavailable")
		return false
	}
	return true
}

func (f *behavior76Fixture) retainOnEvidenceFailure() {
	if !f.t.Failed() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var schema string
	if err := f.pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		f.t.Error("failed-schema retention identity unavailable")
		return
	}
	// memorytest cleans its original random schema name after the test. Rename
	// only after execution/export failure to retain the original facts for audit.
	if _, err := f.pool.Exec(ctx, fmt.Sprintf("ALTER SCHEMA %s RENAME TO %s", schema, "retained_"+schema)); err != nil {
		f.t.Error("failed-schema retention unavailable")
	}
}

type behavior76Outcome struct {
	State           string   `json:"state"`
	OwnerInputID    string   `json:"owner_input_id"`
	OwnerRunID      string   `json:"owner_run_id"`
	RunIDs          []string `json:"run_ids"`
	AutomaticRunIDs []string `json:"automatic_run_ids"`
	NativeEntryIDs  []int    `json:"native_entry_ids"`
	ErrorCode       string   `json:"safe_error_category,omitempty"`
	FailurePhase    string   `json:"failure_phase,omitempty"`
	EventReceipt    any      `json:"event_receipt"`
}
type behavior76Index struct {
	Version       string                   `json:"version"`
	StimuliSHA256 string                   `json:"stimuli_sha256"`
	Episodes      []behavior76IndexEpisode `json:"episodes"`
}
type behavior76IndexEpisode struct {
	EpisodeID   string                `json:"episode_id"`
	MaskedArmID string                `json:"masked_arm_id"`
	AliasesPath string                `json:"aliases_path"`
	TablesDir   string                `json:"tables_dir"`
	Steps       []behavior76IndexStep `json:"steps"`
}
type behavior76IndexStep struct {
	StepID  string `json:"step_id"`
	Before  string `json:"before_snapshot_path"`
	After   string `json:"after_snapshot_path"`
	Outcome string `json:"outcome_path"`
}

func behavior76Write(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("evidence cannot overwrite prior bytes", err)
	}
	_, err = file.Write(append(raw, '\n'))
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatal("durable evidence unavailable", err, closeErr)
	}
}
func (f *behavior76Fixture) snapshot() behavior76Snapshot {
	copy := *f
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	copy.ctx = ctx
	f = &copy
	t := f.t
	t.Helper()
	s := behavior76Snapshot{Tasks: []task.Record{}, Objects: []behavior76Object{}, Profiles: []json.RawMessage{}}
	anchor, err := os.ReadFile(f.manifestPath)
	if err != nil {
		t.Fatal("actual deletion anchor unavailable for snapshot")
	}
	s.DeletionManifestSHA256 = behavior76Hash(anchor)
	if err = f.pool.QueryRow(f.ctx, `SELECT watermark FROM memory_restore_state WHERE singleton`).Scan(&s.DeletionWatermark); err != nil {
		t.Fatal("actual deletion watermark unavailable")
	}
	rows, err := f.pool.Query(f.ctx, `SELECT value FROM behavior76_aliases WHERE kind='task' ORDER BY key`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		r, err := f.tasks.Get(f.ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		s.Tasks = append(s.Tasks, r)
	}
	rows, err = f.pool.Query(f.ctx, `SELECT key FROM behavior76_objects ORDER BY key`)
	if err != nil {
		t.Fatal(err)
	}
	ids = nil
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		o, err := behavior76ObjectQuery(f.ctx, f.pool, id, "")
		if err != nil {
			t.Fatal(err)
		}
		s.Objects = append(s.Objects, o)
	}
	s.Profiles, err = behavior76Rows(f.ctx, f.pool, "memory_profile")
	if err != nil {
		t.Fatal(err)
	}
	s.Claims, err = behavior76Rows(f.ctx, f.pool, "memory_claims")
	if err != nil {
		t.Fatal(err)
	}
	s.Sources, err = behavior76Rows(f.ctx, f.pool, "memory_sources")
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func (f *behavior76Fixture) export(dir string) {
	copy := *f
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	copy.ctx = ctx
	f = &copy
	f.t.Helper()
	var schema string
	if err := f.pool.QueryRow(f.ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		f.t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		f.t.Fatal(err)
	}
	converterLocalNativeExport(f.t, f.pool, dir, schema, []string{"tasks", "task_results", "task_control_events", "task_control_targets", "run_final_results", "accepted_inputs", "runs", "action_intents", "action_authorizations", "action_mandates", "action_effects", "action_effect_dispatches", "action_effect_evidence", "tool_calls", "execution_model_attempts", "execution_budget_scopes", "execution_budget_reservations", "behavior76_objects", "behavior76_object_events", "behavior76_recipient_receipts", "behavior76_aliases", "behavior76_observations", "behavior76_task_mandates", "memory_profile", "memory_claims", "memory_sources", "memory_source_revisions", "memory_source_dependencies", "memory_run_sources", "memory_scope_grants", "memory_fact_heads", "session_ownership", "sessions"})
	// Actual source-check consumers and empty feature tables are evidence too.
	// No credentials/settings or opaque native continuation payload is exported.
	converterLocalNativeExport(f.t, f.pool, dir, schema, []string{
		"messages", "memory_run_transcripts", "transcript_heads", "memory_restore_state", "memory_deletions",
		"memory_summary_sources", "memory_summary_dependencies", "session_summaries",
		"memory_temporal_run_pins", "memory_temporal_source_pins", "memory_claim_temporal_heads",
		"memory_claim_temporal_revisions", "memory_fact_decision_history", "task_authority_bindings", "owner_goals",
	})
}
func (f *behavior76Fixture) aliases() map[string]any {
	copy := *f
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	copy.ctx = ctx
	f = &copy
	f.t.Helper()
	tasks := map[string]any{}
	sources := map[string]any{}
	rows, err := f.pool.Query(f.ctx, `SELECT key,value FROM behavior76_aliases WHERE kind='task' ORDER BY key`)
	if err != nil {
		f.t.Fatal(err)
	}
	for rows.Next() {
		var key, id string
		if err = rows.Scan(&key, &id); err != nil {
			f.t.Fatal(err)
		}
		tasks[key] = map[string]any{"task_id": id}
	}
	rows.Close()
	for key, v := range tasks {
		r, err := f.tasks.Get(f.ctx, v.(map[string]any)["task_id"].(string))
		if err != nil {
			f.t.Fatal(err)
		}
		tasks[key] = map[string]any{"task_id": r.TaskID, "session_key": r.SessionKey}
	}
	rows, err = f.pool.Query(f.ctx, `SELECT a.key,jsonb_agg(jsonb_build_object('source_id',r.source_id,'revision',r.revision) ORDER BY r.revision) FROM behavior76_aliases a JOIN memory_source_revisions r ON r.source_id=a.value WHERE a.kind='source' GROUP BY a.key ORDER BY a.key`)
	if err != nil {
		f.t.Fatal(err)
	}
	for rows.Next() {
		var key string
		var raw []byte
		if err = rows.Scan(&key, &raw); err != nil {
			f.t.Fatal(err)
		}
		sources[key] = json.RawMessage(raw)
	}
	rows.Close()
	return map[string]any{"tasks": tasks, "sources": sources}
}
