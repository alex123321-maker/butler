package catalog

import (
	"strings"
	"testing"
)

func TestDeleteErasesPinnedPreparationBeforeAnyMessageIntegration(t *testing.T) {
	s, owner := fixture(t)
	live := seedRun(t, s)
	src := put(t, s, owner, "before-message", "PRIVATE ASR SOURCE")
	if _, e := s.Get(live, src.ID); e != nil {
		t.Fatal(e)
	}
	_, e := s.pool.Exec(owner, `UPDATE runs SET metadata_json='{"prepared":{"body":"PRIVATE ASR SOURCE"}}' WHERE run_id='run'; INSERT INTO accepted_inputs(input_id,session_key,event_id,dedup_key,source,owner_id,event_json,run_id,status,checkpoint,checkpoint_json) VALUES('input','web:owner','input','input','web','owner','{"text":"PRIVATE ASR SOURCE"}','run','running','prepared','{"prepared":"PRIVATE ASR SOURCE"}')`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Delete(owner, src.ID, "delete", 1); e != nil {
		t.Fatal(e)
	}
	var metadata, event, checkpoint, state string
	if e = s.pool.QueryRow(owner, `SELECT r.metadata_json::text,i.event_json::text,i.checkpoint_json::text,i.status FROM runs r JOIN accepted_inputs i USING(run_id) WHERE run_id='run'`).Scan(&metadata, &event, &checkpoint, &state); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(metadata+event+checkpoint, "PRIVATE ASR SOURCE") || state != "cancelled" {
		t.Fatal("pre-message copy survived deletion", state)
	}
	var n int
	if e = s.pool.QueryRow(owner, `SELECT count(*) FROM memory_run_sources WHERE run_id='run'`).Scan(&n); e != nil || n != 1 {
		t.Fatal("audit source identity lost", n, e)
	}
	if e = s.CheckRunSources(live, "run"); e == nil {
		t.Fatal("late prepared release allowed")
	}
	// Replaying the independent deletion manifest after an old snapshot was
	// restored must erase copies again, not only prevent a new publication.
	_, e = s.pool.Exec(owner, `UPDATE runs SET metadata_json='{"prepared":{"body":"PRIVATE ASR SOURCE"}}' WHERE run_id='run'; UPDATE accepted_inputs SET event_json='{"text":"PRIVATE ASR SOURCE"}',checkpoint_json='{"prepared":"PRIVATE ASR SOURCE"}',status='queued' WHERE input_id='input'; UPDATE memory_restore_state SET watermark=0 WHERE singleton`)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Reconcile(owner); e != nil {
		t.Fatal(e)
	}
	if e = s.pool.QueryRow(owner, `SELECT r.metadata_json::text,i.event_json::text,i.checkpoint_json::text,i.status FROM runs r JOIN accepted_inputs i USING(run_id) WHERE run_id='run'`).Scan(&metadata, &event, &checkpoint, &state); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(metadata+event+checkpoint, "PRIVATE ASR SOURCE") || state != "cancelled" {
		t.Fatal("restored preparation retained deleted content")
	}
	if e = s.CheckRunSources(live, "run"); e == nil {
		t.Fatal("restored late callback allowed")
	}
}
