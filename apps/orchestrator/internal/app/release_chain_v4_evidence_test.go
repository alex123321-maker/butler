package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/ownership"
)

// The Task fixtures delete their private schemas at cleanup. Snapshot these
// allowlisted original rows before cleanup into the separate shared PG ledger.
// No prompt, body, tool arguments, authority token or provider credential enters
// this durable witness. The separately exported PG rows can be read by the final
// reviewer without trusting the model's semantic verdict or only a test PASS.
func recordReleaseChainV4ProductEvidence(parent context.Context, w *releaseChainV4Window, f *developerFixture, scenario string, failed bool, fault *releaseChainV4Fault) error {
	if w == nil || f == nil || f.f == nil || f.f.pool == nil || f.channel == nil {
		return errNativeFixtureDenied
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	var raw json.RawMessage
	const query = `SELECT json_build_object(
 'root_task_id',$1::text,
 'tasks',COALESCE((SELECT json_agg(row_to_json(t) ORDER BY task_id) FROM (SELECT task_id,parent_task_id,status,revision,constraints_revision,result_delivery,completion_result_id,created_at,finished_at FROM tasks WHERE task_id=$1 OR parent_task_id=$1) t),'[]'::json),
 'runs',COALESCE((SELECT json_agg(row_to_json(r) ORDER BY started_at,run_id) FROM (SELECT run_id,task_id,task_revision,status,current_state,model_provider,started_at,finished_at,error_type FROM runs WHERE task_id=$1 OR task_id IN(SELECT task_id FROM tasks WHERE parent_task_id=$1)) r),'[]'::json),
 'delegations',COALESCE((SELECT json_agg(row_to_json(d) ORDER BY task_id) FROM (SELECT task_id,root_task_id,parent_task_id,parent_revision,parent_constraints_revision,contract_revision,contract_source_id,depth,state,input_id,result_id,deadline FROM task_delegations WHERE parent_task_id=$1) d),'[]'::json),
 'developer_admissions',COALESCE((SELECT json_agg(row_to_json(d) ORDER BY task_id) FROM (SELECT task_id,parent_task_id,parent_revision,parent_constraints_revision,execution_action_id,effect_id,authorization_id,action_hash,args_hash,contract_hash,snapshot_hash FROM converter_developer_snapshots WHERE parent_task_id=$1) d),'[]'::json),
 'converter_versions',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY digest) FROM (SELECT digest,converter_id,version,need_id,need_hash,source_ref_json,profile_pin,created_by_run FROM converter_versions) v),'[]'::json),
 'converter_states',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY digest) FROM (SELECT digest,status FROM converter_states) v),'[]'::json),
 'converter_catalog',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY converter_id) FROM (SELECT converter_id,generation,active_digest FROM converter_catalog) v),'[]'::json),
 'converter_validations',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY validation_id) FROM (SELECT validation_id,digest,run_id,action_id,report_json->>'package_hash' AS package_hash,report_json->>'need_hash' AS need_hash,report_json->>'profile_hash' AS profile_hash,report_json->>'checker' AS checker,report_json->>'passed' AS passed FROM converter_validations) v),'[]'::json),
 'converter_trials',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY action_id) FROM (SELECT action_id,digest,run_id,task_id,task_revision,status,input_ref_json,output_ref_json,output_hash FROM converter_trials) v),'[]'::json),
 'converter_usages',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY action_id) FROM (SELECT action_id,digest,run_id,task_id,task_revision,constraints_revision,generation,output_ref_json,output_hash FROM converter_usages) v),'[]'::json),
 'converter_events',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY event_id) FROM (SELECT event_id,converter_id,digest,generation,kind,run_id,action_id FROM converter_events) v),'[]'::json),
 'converter_returns',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY child_task_id) FROM (SELECT child_task_id,parent_task_id,result_id,child_run_id,digest,projection_hash FROM converter_developer_returns) v),'[]'::json),
 'continuations',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY continuation_id) FROM (SELECT continuation_id,child_task_id,parent_task_id,parent_revision,constraints_revision,purpose,intent_hash,state,input_id,ready_hash FROM converter_parent_continuations) v),'[]'::json),
 'actions',COALESCE((SELECT json_agg(row_to_json(a) ORDER BY created_at,action_id) FROM (SELECT i.action_id,i.run_id,i.tool_call_id,c.tool_name,i.action_hash,i.effect_id,e.fingerprint,e.state,e.dispatch_outcome,i.created_at FROM action_intents i JOIN runs r ON r.run_id=i.run_id JOIN action_effects e ON e.action_id=i.effect_id LEFT JOIN tool_calls c ON c.run_id=i.run_id AND c.tool_call_id=i.tool_call_id WHERE r.task_id=$1 OR r.task_id IN(SELECT task_id FROM tasks WHERE parent_task_id=$1)) a),'[]'::json),
 'final_results',COALESCE((SELECT json_agg(row_to_json(r) ORDER BY result_id) FROM (SELECT result_id,run_id,task_id,task_revision,constraints_revision,artifact_id,message_id,content_hash,answer_state,verdict,privacy_redacted FROM run_final_results WHERE task_id=$1 OR task_id IN(SELECT task_id FROM tasks WHERE parent_task_id=$1)) r),'[]'::json),
 'task_results',COALESCE((SELECT json_agg(row_to_json(r) ORDER BY result_id) FROM (SELECT result_id,task_id,task_revision,run_id,verdict,verifier,created_at FROM task_results WHERE task_id=$1 OR task_id IN(SELECT task_id FROM tasks WHERE parent_task_id=$1)) r),'[]'::json),
 'catalog_sources',COALESCE((SELECT json_agg(row_to_json(s) ORDER BY source_id) FROM (SELECT s.source_id,s.head_revision AS revision,s.access_revision,s.status,r.content_hash FROM memory_sources s JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision) s),'[]'::json),
 'goals',COALESCE((SELECT json_agg(row_to_json(g) ORDER BY goal_id) FROM (SELECT goal_id,kind,status,revision,expires_at FROM owner_goals) g),'[]'::json),
 'mandates',COALESCE((SELECT json_agg(row_to_json(m) ORDER BY mandate_id) FROM (SELECT mandate_id,revision,status,scope_kind,goal_id,goal_revision,expires_at,revoked_at FROM action_mandates) m),'[]'::json),
 'task_authority',COALESCE((SELECT json_agg(row_to_json(a) ORDER BY task_id) FROM (SELECT task_id,goal_id,goal_revision,mandate_id,mandate_revision,observation_ref FROM task_authority_bindings WHERE task_id=$1 OR task_id IN(SELECT task_id FROM tasks WHERE parent_task_id=$1)) a),'[]'::json),
 'notices',COALESCE((SELECT json_agg(row_to_json(n) ORDER BY notification_id) FROM (SELECT notification_id,task_id,result_id,task_revision,constraints_revision,run_id,channel,status,attempt_count,channel_ack,error_code,created_at FROM task_verified_notifications WHERE task_id=$1) n),'[]'::json),
 'live_tree_leases',(SELECT count(*) FROM session_ownership o JOIN runs r ON r.run_id=o.run_id WHERE (r.task_id=$1 OR r.task_id IN(SELECT task_id FROM tasks WHERE parent_task_id=$1)) AND o.expires_at>clock_timestamp()),
 'run_outbox_rows',(SELECT count(*) FROM delivery_outbox),
 'digest_outbox_rows',(SELECT count(*) FROM initiative_digest_outbox),
 'technical_sent_rows',(SELECT count(*) FROM channel_delivery_events WHERE state='sent'),
 'initiatives',COALESCE((SELECT json_agg(row_to_json(i) ORDER BY initiative_id) FROM (SELECT initiative_id,task_id,status,reason FROM initiatives WHERE task_id=$1) i),'[]'::json))`
	if f.f.pool.QueryRow(ctx, query, f.f.parent.TaskID).Scan(&raw) != nil {
		return errNativeFixtureDenied
	}
	var faultEvidence map[string]any
	if fault != nil {
		var leaseHash *string
		if fault.oldChild.LeaseID != "" {
			sum := sha256.Sum256([]byte(fault.oldChild.LeaseID))
			value := hex.EncodeToString(sum[:])
			leaseHash = &value
		}
		faultEvidence = map[string]any{"triggered": false, "old_child_lease_sha256": leaseHash, "old_child_run_id": fault.oldChild.RunID, "stopped_child_source_denied": fault.oldSourceDenied, "stopped_child_broker_prepare_denied": fault.oldBrokerDenied}
		select {
		case <-fault.triggered:
			faultEvidence["triggered"] = fault.err == nil
		default:
		}
	}
	var patchHash *string
	if patches, err := os.ReadFile(filepath.Join(f.projectDirectory, "patches.json")); err == nil {
		sum := sha256.Sum256(patches)
		value := hex.EncodeToString(sum[:])
		patchHash = &value
	}
	body, err := json.Marshal(map[string]any{
		"version": "butler-release-chain-v4-product-witness-v1", "scenario": scenario, "test_failed": failed, "postgres": raw,
		"original_document": f.document, "original_need": f.needRef, "original_spec": f.specRef,
		"patch_artifact_file_sha256": patchHash,
		"recipient_requests":         f.channel.requests.Load(), "recipient_acceptances": f.channel.accepted.Load(),
		"all_channel_requests": f.channel.total.Load(),
		"fault":                faultEvidence,
	})
	if err != nil {
		return errNativeFixtureDenied
	}
	// PostgreSQL jsonb changes whitespace/key order. Hash a documented recursive
	// canonical projection, preserving numeric lexemes and array order.
	var canonical any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&canonical) != nil {
		return errNativeFixtureDenied
	}
	body, err = json.Marshal(canonical)
	if err != nil {
		return errNativeFixtureDenied
	}
	sum := sha256.Sum256(body)
	if _, err = w.pool.Exec(ctx, `INSERT INTO `+w.table("fixture_chain_v4_high_case_evidence")+`(scenario,payload_sha256,payload_json) VALUES($1,$2,$3)`, scenario, hex.EncodeToString(sum[:]), body); err != nil {
		return errNativeFixtureDenied
	}
	return nil
}

func TestReleaseChainV4ProductEvidenceRetainedBeforeCleanupIntegration(t *testing.T) {
	f := newDeveloperFixture(t)
	f.channel = &releaseChainChannel{}
	f.projectDirectory = t.TempDir()
	w, err := createReleaseChainV4Window(f.f.ctx, f.f.pool, strings.Repeat("e", 64), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	const privateLease = "synthetic-private-lease-proof"
	fault := &releaseChainV4Fault{oldChild: ownership.Token{LeaseID: privateLease}}
	if err = recordReleaseChainV4ProductEvidence(f.f.ctx, w, f, "explicit_absence", true, fault); err != nil {
		t.Fatal("synthetic owning SQL product snapshot failed")
	}
	var raw []byte
	var expected string
	if f.f.pool.QueryRow(f.f.ctx, `SELECT payload_json,payload_sha256 FROM `+w.table("fixture_chain_v4_high_case_evidence")).Scan(&raw, &expected) != nil {
		t.Fatal("stored product snapshot missing")
	}
	var canonical any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&canonical) != nil {
		t.Fatal("stored canonical metadata malformed")
	}
	canon, err := json.Marshal(canonical)
	sum := sha256.Sum256(canon)
	if err != nil || hex.EncodeToString(sum[:]) != expected {
		t.Fatal("jsonb representation changed immutable product digest")
	}
	if bytes.Contains(canon, []byte(privateLease)) || bytes.Contains(canon, []byte(`"old_child_lease_id"`)) || !bytes.Contains(canon, []byte(`"old_child_lease_sha256"`)) {
		t.Fatal("private lease proof escaped the content-free snapshot")
	}
	for _, token := range []string{`"body":`, `"summary":`, `"content":`, `"accepted_json":`, `"report_json":`, `"grant_data":`, `"binding":`} {
		if bytes.Contains(canon, []byte(token)) {
			t.Fatal("private payload column escaped allowlist")
		}
	}
	for _, query := range []string{`UPDATE ` + w.table("fixture_chain_v4_high_case_evidence") + ` SET payload_sha256=repeat('a',64)`, `DELETE FROM ` + w.table("fixture_chain_v4_high_case_evidence")} {
		if _, err = f.f.pool.Exec(f.f.ctx, query); err == nil {
			t.Fatal("product witness was mutable")
		}
	}
}
