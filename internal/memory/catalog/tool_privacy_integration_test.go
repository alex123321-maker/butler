package catalog

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/transcript"
)

func TestDeletionErasesPinnedToolAndApprovalPayloadsWithoutChangingOutcomesIntegration(t *testing.T) {
	s, owner := fixture(t)
	live := seedRun(t, s)
	source := put(t, s, owner, "tool-private", "Unique private source payload")
	if _, err := s.Get(live, source.ID); err != nil {
		t.Fatal(err)
	}
	const canary = "PRIVATE_TOOL_PAYLOAD_79"
	var err error
	for _, query := range []string{
		`INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,result,error) VALUES('private-read','run','memory.open',jsonb_build_object('query',$1::text),'completed','memory',clock_timestamp(),jsonb_build_object('items',jsonb_build_array(jsonb_build_object('text',$1::text))),jsonb_build_object('detail',$1::text))`,
		`INSERT INTO approvals(approval_id,run_id,session_key,tool_call_id,status,requested_via,tool_name,args_json,summary,details_json) VALUES('pending-private','run','web:owner','other-call','pending','web','http.request',jsonb_build_object('body',$1::text),$1,jsonb_build_object('detail',$1::text)),('resolved-private','run','web:owner','resolved-call','approved','web','http.request',jsonb_build_object('body',$1::text),$1,'{}')`,
		`INSERT INTO approval_events(approval_id,run_id,session_key,event_type,reason,metadata_json) VALUES('pending-private','run','web:owner','requested',$1,jsonb_build_object('detail',$1::text))`,
	} {
		if _, err = s.pool.Exec(owner, query, canary); err != nil {
			t.Fatal(err)
		}
	}
	_, err = s.pool.Exec(owner, `INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('unrelated-run','web:owner','unrelated','completed','mode_0','completed','fixture',clock_timestamp(),clock_timestamp());INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,result) VALUES('unrelated-tool','unrelated-run','memory.open','{}','completed','memory',clock_timestamp(),'"unrelated surviving payload"')`)
	if err != nil {
		t.Fatal(err)
	}
	var before string
	if err = s.pool.QueryRow(owner, `SELECT (to_jsonb(t)-ARRAY['args','result','error'])::text FROM tool_calls t WHERE tool_call_id='private-read'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Delete(owner, source.ID, "delete", source.Revision); err != nil {
		t.Fatal(err)
	}
	var after string
	if err = s.pool.QueryRow(owner, `SELECT (to_jsonb(t)-ARRAY['args','result','error'])::text FROM tool_calls t WHERE tool_call_id='private-read'`).Scan(&after); err != nil || before != after {
		t.Fatal("tool identity/outcome changed", err)
	}
	var raw []byte
	if err = s.pool.QueryRow(owner, `SELECT jsonb_build_object('tools',(SELECT jsonb_agg(to_jsonb(t))FROM tool_calls t WHERE run_id='run'),'approvals',(SELECT jsonb_agg(to_jsonb(a))FROM approvals a WHERE run_id='run'),'events',(SELECT jsonb_agg(to_jsonb(e))FROM approval_events e WHERE run_id='run'))`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), canary) || !strings.Contains(string(raw), "privacy_redacted") {
		t.Fatal("cached private payload survived")
	}
	var pending, resolved, other string
	if err = s.pool.QueryRow(owner, `SELECT (SELECT status FROM approvals WHERE approval_id='pending-private'),(SELECT status FROM approvals WHERE approval_id='resolved-private'),(SELECT result::text FROM tool_calls WHERE tool_call_id='unrelated-tool')`).Scan(&pending, &resolved, &other); err != nil || pending != "failed" || resolved != "approved" || !strings.Contains(other, "unrelated surviving payload") {
		t.Fatal("unrelated data or actual resolved status changed", err)
	}
	rows, err := transcript.NewStore(s.pool).GetTranscript(owner, "web:owner")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(rows)
	if strings.Contains(string(encoded), canary) {
		t.Fatal("transcript API returned erased tool data")
	}
	for _, query := range []string{
		`UPDATE tool_calls SET result='"PRIVATE_TOOL_PAYLOAD_79"' WHERE tool_call_id='private-read'`,
		`UPDATE approvals SET summary='PRIVATE_TOOL_PAYLOAD_79' WHERE approval_id='pending-private'`,
		`UPDATE approval_events SET reason='PRIVATE_TOOL_PAYLOAD_79' WHERE approval_id='pending-private'`,
		`DELETE FROM memory_run_payload_redactions WHERE run_id='run'`,
		`INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,result) VALUES('late-private','run','memory.open','{}','completed','memory',clock_timestamp(),'"PRIVATE_TOOL_PAYLOAD_79"')`,
	} {
		if _, err = s.pool.Exec(owner, query); err == nil {
			t.Fatal("late privacy resurrection permitted")
		}
	}
	if err = New(s.pool, s.manifest).Reconcile(owner); err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile("../../../migrations/079_memory_tool_payload_redaction.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(owner, string(down)); err == nil {
		t.Fatal("rollback removed an existing irreversible privacy fence")
	}
	if _, err = s.pool.Exec(owner, `UPDATE tool_calls SET result='"late private copy"' WHERE tool_call_id='private-read'`); err == nil {
		t.Fatal("denied downgrade did not preserve the payload fence")
	}
}

func TestToolPayloadMigrationErasesAlreadyWithdrawnRunCacheIntegration(t *testing.T) {
	s, owner := fixture(t)
	live := seedRun(t, s)
	source := put(t, s, owner, "historical-cache", "old original")
	if _, err := s.Get(live, source.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(owner, `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,result) VALUES('historical-read','run','memory.open','{}','completed','memory',clock_timestamp(),'"old cached private payload"')`); err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile("../../../migrations/079_memory_tool_payload_redaction.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(owner, string(down)); err != nil {
		t.Fatal(err)
	}
	// An explicit historical-deletion fixture represents the former cache gap;
	// this is an actual schema upgrade test, not a full backup/restore claim.
	if _, err = s.pool.Exec(owner, `INSERT INTO memory_deletions(deletion_id,source_id,owner_id,action) VALUES('historical-deletion',$1,'owner','delete')`, source.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(owner, `UPDATE memory_sources SET status='deleted' WHERE source_id=$1`, source.ID); err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile("../../../migrations/079_memory_tool_payload_redaction.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(owner, string(up)); err != nil {
		t.Fatal(err)
	}
	var payload string
	if err = s.pool.QueryRow(owner, `SELECT result::text FROM tool_calls WHERE tool_call_id='historical-read'`).Scan(&payload); err != nil || strings.Contains(payload, "old cached private payload") || !strings.Contains(payload, "privacy_redacted") {
		t.Fatal("historical tool copy survived upgrade", err)
	}
}
