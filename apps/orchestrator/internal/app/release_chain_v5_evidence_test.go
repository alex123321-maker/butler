package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/extensions/projectfixture"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport/antigravity"
)

type releaseChainV5PatchChange struct {
	Path      string `json:"path"`
	BeforeSHA string `json:"before_sha256"`
	AfterSHA  string `json:"after_sha256"`
}

type releaseChainV5OriginalResult struct {
	PatchCount      int                         `json:"patch_count"`
	ArtifactID      *string                     `json:"artifact_id"`
	ProjectRevision *string                     `json:"project_revision"`
	Changes         []releaseChainV5PatchChange `json:"changes"`
	PatchCall       *string                     `json:"patch_tool_call_id"`
	CheckCall       *string                     `json:"check_tool_call_id"`
	PrepareReceipts int                         `json:"prepare_receipt_count"`
	CheckReceipts   int                         `json:"check_receipt_count"`
	PrepareArtifact *string                     `json:"prepare_receipt_artifact_id"`
	CheckArtifact   *string                     `json:"check_receipt_artifact_id"`
	ChecksPassed    *bool                       `json:"checks_passed"`
	Applied         *bool                       `json:"applied"`
}

func releaseChainV5PatchProjection(patches map[string]projectfixture.Patch) *releaseChainV5OriginalResult {
	r := &releaseChainV5OriginalResult{PatchCount: len(patches), Changes: []releaseChainV5PatchChange{}}
	if len(patches) == 1 {
		for _, patch := range patches {
			id, revision := patch.ArtifactID, patch.Proposal.ProjectRevision
			r.ArtifactID, r.ProjectRevision = &id, &revision
			for _, change := range patch.Proposal.Changes {
				r.Changes = append(r.Changes, releaseChainV5PatchChange{Path: change.Path, BeforeSHA: generatedtools.HashBytes([]byte(change.Before)), AfterSHA: generatedtools.HashBytes([]byte(change.After))})
			}
		}
	}
	return r
}

func releaseChainV5ReadPatch(path string) ([]byte, error) {
	stat, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > releaseChainV5SafeProductOutputMax {
		return nil, errNativeFixtureDenied
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(stat, opened) {
		return nil, errNativeFixtureDenied
	}
	raw, err := io.ReadAll(io.LimitReader(f, releaseChainV5SafeProductOutputMax+1))
	if err != nil || len(raw) > releaseChainV5SafeProductOutputMax {
		return nil, errNativeFixtureDenied
	}
	return raw, nil
}

// Export only actual stored patch hashes and actual Broker receipt fields.
// Missing/ambiguous fields stay NULL; no success value comes from a fixture seed.
func releaseChainV5CaptureOriginalResult(ctx context.Context, f *developerFixture, scenario string) (*string, *releaseChainV5OriginalResult, error) {
	raw, err := releaseChainV5ReadPatch(filepath.Join(f.projectDirectory, "patches.json"))
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil || scenario == "owner_stop" {
		return nil, nil, errNativeFixtureDenied
	}
	patchHash := generatedtools.HashBytes(raw)
	var patches map[string]projectfixture.Patch
	if json.Unmarshal(raw, &patches) != nil {
		return &patchHash, nil, errNativeFixtureDenied
	}
	r := releaseChainV5PatchProjection(patches)
	rows, err := f.f.pool.Query(ctx, `SELECT c.tool_name,c.tool_call_id,c.result::text FROM tool_calls c JOIN runs r ON r.run_id=c.run_id WHERE r.task_id=$1 AND c.tool_name IN('extension.project.prepare-patch','extension.project.check') ORDER BY c.tool_call_id`, f.f.parent.TaskID)
	if err != nil {
		return &patchHash, r, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, call, bodyRaw string
		if err = rows.Scan(&name, &call, &bodyRaw); err != nil {
			return &patchHash, r, err
		}
		if name == "extension.project.prepare-patch" {
			r.PrepareReceipts++
		} else {
			r.CheckReceipts++
		}
		body, bodyErr := initiativeProjectToolBody(bodyRaw)
		if bodyErr != nil {
			continue
		}
		var artifact *string
		if id, ok := body["artifact_id"].(string); ok && nativeAuditHash.MatchString(id) {
			artifact = &id
		}
		if name == "extension.project.prepare-patch" {
			r.PatchCall, r.PrepareArtifact = &call, artifact
		} else {
			r.CheckCall, r.CheckArtifact = &call, artifact
			if checked, ok := body["checks_passed"].(bool); ok {
				r.ChecksPassed = &checked
			}
			if applied, ok := body["applied"].(bool); ok {
				r.Applied = &applied
			}
		}
	}
	if r.PrepareReceipts != 1 {
		r.PatchCall, r.PrepareArtifact = nil, nil
	}
	if r.CheckReceipts != 1 {
		r.CheckCall, r.CheckArtifact, r.ChecksPassed, r.Applied = nil, nil, nil, nil
	}
	return &patchHash, r, rows.Err()
}

// Capture the real absent prior adapter state and setup provenance before the
// first native entry for this case. The fixture's owner controls and cheap
// recorded checks are not represented as model decisions or authored IR.
func recordReleaseChainV5InitialEvidence(parent context.Context, w *releaseChainV5Window, f *developerFixture, scenario string) error {
	if w == nil || f == nil || f.f == nil || f.f.pool == nil {
		return errNativeFixtureDenied
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	var versions, catalogs, scopedModels, nativeRows int
	if f.f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM converter_versions WHERE converter_id=$1),(SELECT count(*) FROM converter_catalog WHERE converter_id=$1),(SELECT count(*) FROM execution_model_attempts a JOIN runs r ON r.run_id=a.run_id WHERE r.task_id=$2 OR r.task_id IN(SELECT task_id FROM tasks WHERE parent_task_id=$2))`, f.need.ConverterID, f.f.parent.TaskID).Scan(&versions, &catalogs, &scopedModels) != nil || w.pool.QueryRow(ctx, `SELECT count(*) FROM `+w.table("fixture_chain_v5_high_requests")+` WHERE scenario=$1`, scenario).Scan(&nativeRows) != nil {
		return errNativeFixtureDenied
	}
	nativeConfigured := f.modelConfig != nil && f.modelConfig.Provider == antigravity.Name && f.modelConfig.Model == antigravity.FlashHighModel
	if nativeConfigured && (versions != 0 || catalogs != 0 || scopedModels != 0 || nativeRows != 0) {
		return errNativeFixtureDenied
	}
	var setup json.RawMessage
	const query = `SELECT json_build_object(
 'promoted_task',(SELECT json_build_object('task_id',task_id,'status',status,'revision',revision,'constraints_revision',constraints_revision,'result_delivery',result_delivery) FROM tasks WHERE task_id=$1),
 'accepted_input',(SELECT json_build_object('input_id',input_id,'source',source,'status',status,'task_id',task_id,'run_id',run_id,'accepted_at',accepted_at,'expires_at',expires_at) FROM accepted_inputs WHERE input_id=$2),
 'setup_runs',COALESCE((SELECT json_agg(row_to_json(r) ORDER BY started_at,run_id) FROM (SELECT run_id,task_id,task_revision,current_state,model_provider,started_at,finished_at FROM runs WHERE model_provider='fixture') r),'[]'::json),
 'setup_actions',COALESCE((SELECT json_agg(row_to_json(a) ORDER BY created_at,action_id) FROM (SELECT i.action_id,i.run_id,i.tool_call_id,i.action_hash,i.effect_id,i.requires_approval,i.created_at,c.tool_name,e.state AS effect_state,a.authorization_id,a.state AS authorization_state FROM action_intents i JOIN runs r ON r.run_id=i.run_id JOIN action_effects e ON e.action_id=i.effect_id JOIN action_authorizations a ON a.action_id=i.action_id LEFT JOIN tool_calls c ON c.run_id=i.run_id AND c.tool_call_id=i.tool_call_id WHERE r.model_provider='fixture') a),'[]'::json),
 'setup_approved_count',(SELECT count(*) FROM approvals WHERE status='approved'),
 'initial_converter_need_count',(SELECT count(*) FROM converter_needs WHERE converter_id=$3))`
	if f.f.pool.QueryRow(ctx, query, f.f.parent.TaskID, f.initialInputID, f.need.ConverterID).Scan(&setup) != nil {
		return errNativeFixtureDenied
	}
	body, err := json.Marshal(map[string]any{"version": "butler-release-chain-v5-initial-witness-v1", "scenario": scenario, "configured_native_high": nativeConfigured, "ordinary_core_entry": "after_owner_bound_initiative_promoted_input", "setup_execution": "owner_services_and_real_approved_broker_reads_without_model", "adapter_development": "production_deterministic_finite_ir_builder_selected_by_native_child", "model_authored_ir": false, "human_recipient": false, "native_batch_support": false, "native_multi_function_contract": "capability_mismatch_before_tool_release", "generic_wrapper_batch_handoff_evidence": "Go_spy_fixture_only", "core_batch_capability": false, "target_converter_id": f.need.ConverterID, "target_need_sha256": generatedtools.Hash(f.need), "original_refs": map[string]any{"document": f.document, "need": f.needRef, "spec": f.specRef}, "existing_target_versions": versions, "existing_target_catalogs": catalogs, "preexisting_root_model_reservations": scopedModels, "preexisting_case_native_requests": nativeRows, "postgres": setup})
	if err != nil {
		return errNativeFixtureDenied
	}
	// jsonb changes nested object ordering, including the owning SQL projection.
	// Use the same recursive numeric-preserving canonical hash as final witnesses.
	var canonical any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&canonical) != nil {
		return errNativeFixtureDenied
	}
	body, err = json.Marshal(canonical)
	if err != nil || len(body) > releaseChainV5SafeProductOutputMax {
		return errNativeFixtureDenied
	}
	sum := sha256.Sum256(body)
	_, err = w.pool.Exec(ctx, `INSERT INTO `+w.table("fixture_chain_v5_high_case_initial")+`(scenario,payload_sha256,payload_json) VALUES($1,$2,$3)`, scenario, hex.EncodeToString(sum[:]), body)
	return err
}

// The Task fixtures delete their private schemas at cleanup. Snapshot these
// allowlisted original rows before cleanup into the separate shared PG ledger.
// No prompt, body, tool arguments, authority token or provider credential enters
// this durable witness. The separately exported PG rows can be read by the final
// reviewer without trusting the model's semantic verdict or only a test PASS.
func recordReleaseChainV5ProductEvidence(parent context.Context, w *releaseChainV5Window, f *developerFixture, scenario string, failed bool, fault *releaseChainV5Fault) error {
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
 'accepted_inputs',COALESCE((SELECT json_agg(row_to_json(i) ORDER BY accepted_at,input_id) FROM (SELECT input_id,event_id,session_sequence,source,task_id,run_id,status,checkpoint,attempts,accepted_at,updated_at,expires_at,error_code FROM accepted_inputs WHERE task_id=$1 OR task_id IN(SELECT task_id FROM tasks WHERE parent_task_id=$1)) i),'[]'::json),
 'model_budget_attempts',COALESCE((SELECT json_agg(row_to_json(a) ORDER BY created_at,operation_id) FROM (SELECT a.operation_id,a.run_id,a.reservation_id,a.provider,a.model,a.input_allowance,a.output_allowance,a.actual_input_tokens,a.actual_output_tokens,a.created_at,a.observed_at FROM execution_model_attempts a JOIN runs r ON r.run_id=a.run_id WHERE r.task_id=$1 OR r.task_id IN(SELECT task_id FROM tasks WHERE parent_task_id=$1)) a),'[]'::json),
 'budget_reservations',COALESCE((SELECT json_agg(row_to_json(b) ORDER BY created_at,reservation_id) FROM (SELECT reservation_id,operation_id,fingerprint,created_at,request_json->'charge' AS charge,request_json->'cost_known' AS quoted_cost_known,request_json->'scope_ids' AS scope_ids FROM execution_budget_reservations) b),'[]'::json),
 'budget_scopes',COALESCE((SELECT json_agg(row_to_json(b) ORDER BY scope_id) FROM (SELECT scope_id,kind,subject_id,limits_json AS limits,used_json AS reserved,deadline,created_at,updated_at FROM execution_budget_scopes) b),'[]'::json),
 'delegations',COALESCE((SELECT json_agg(row_to_json(d) ORDER BY task_id) FROM (SELECT task_id,root_task_id,parent_task_id,parent_revision,parent_constraints_revision,contract_revision,contract_source_id,depth,state,input_id,result_id,deadline FROM task_delegations WHERE parent_task_id=$1) d),'[]'::json),
 'developer_admissions',COALESCE((SELECT json_agg(row_to_json(d) ORDER BY task_id) FROM (SELECT task_id,parent_task_id,parent_revision,parent_constraints_revision,execution_action_id,effect_id,authorization_id,action_hash,args_hash,contract_hash,snapshot_hash FROM converter_developer_snapshots WHERE parent_task_id=$1) d),'[]'::json),
 'converter_versions',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY digest) FROM (SELECT digest,converter_id,version,need_id,need_hash,source_ref_json,profile_pin,created_by_run FROM converter_versions) v),'[]'::json),
 'converter_needs',COALESCE((SELECT json_agg(row_to_json(n) ORDER BY need_id) FROM (SELECT need_id,converter_id,need_hash,source_ref_json,task_id,task_revision,constraints_revision,run_id,discovery_json->'compatible' AS compatible FROM converter_needs) n),'[]'::json),
 'converter_states',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY digest) FROM (SELECT digest,status FROM converter_states) v),'[]'::json),
 'converter_catalog',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY converter_id) FROM (SELECT converter_id,generation,active_digest FROM converter_catalog) v),'[]'::json),
 'converter_validations',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY validation_id) FROM (SELECT validation_id,digest,run_id,action_id,report_json->>'package_hash' AS package_hash,report_json->>'need_hash' AS need_hash,report_json->>'profile_hash' AS profile_hash,report_json->>'checker' AS checker,report_json->>'passed' AS passed FROM converter_validations) v),'[]'::json),
 'converter_trials',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY action_id) FROM (SELECT action_id,digest,run_id,task_id,task_revision,status,input_ref_json,output_ref_json,output_hash FROM converter_trials) v),'[]'::json),
 'converter_usages',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY action_id) FROM (SELECT action_id,digest,run_id,task_id,task_revision,constraints_revision,generation,output_ref_json,output_hash FROM converter_usages) v),'[]'::json),
 'converter_events',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY event_id) FROM (SELECT event_id,converter_id,digest,generation,kind,run_id,action_id FROM converter_events) v),'[]'::json),
 'converter_returns',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY child_task_id) FROM (SELECT child_task_id,parent_task_id,result_id,child_run_id,digest,projection_hash FROM converter_developer_returns) v),'[]'::json),
 'continuations',COALESCE((SELECT json_agg(row_to_json(v) ORDER BY continuation_id) FROM (SELECT continuation_id,child_task_id,parent_task_id,parent_revision,constraints_revision,purpose,intent_hash,state,input_id,ready_hash FROM converter_parent_continuations) v),'[]'::json),
 'actions',COALESCE((SELECT json_agg(row_to_json(a) ORDER BY created_at,action_id) FROM (SELECT i.action_id,i.run_id,i.tool_call_id,c.tool_name,i.action_hash,i.effect_id,e.fingerprint,e.state,e.dispatch_outcome,i.created_at FROM action_intents i JOIN runs r ON r.run_id=i.run_id JOIN action_effects e ON e.action_id=i.effect_id LEFT JOIN tool_calls c ON c.run_id=i.run_id AND c.tool_call_id=i.tool_call_id WHERE r.task_id=$1 OR r.task_id IN(SELECT task_id FROM tasks WHERE parent_task_id=$1)) a),'[]'::json),
 'authorizations',COALESCE((SELECT json_agg(row_to_json(a) ORDER BY created_at,authorization_id) FROM (SELECT a.authorization_id,a.action_id,a.action_hash,a.mandate_id,a.mandate_revision,a.state,a.expires_at,a.created_at,a.dispatched_at,a.finished_at FROM action_authorizations a JOIN action_intents i ON i.action_id=a.action_id JOIN runs r ON r.run_id=i.run_id WHERE r.task_id=$1 OR r.task_id IN(SELECT task_id FROM tasks WHERE parent_task_id=$1)) a),'[]'::json),
 'dispatches',COALESCE((SELECT json_agg(row_to_json(d) ORDER BY dispatched_at,authorization_id) FROM (SELECT d.authorization_id,d.action_id,d.execution_action_id,d.outcome,d.dispatched_at,d.finished_at FROM action_effect_dispatches d JOIN action_intents i ON i.action_id=d.execution_action_id JOIN runs r ON r.run_id=i.run_id WHERE r.task_id=$1 OR r.task_id IN(SELECT task_id FROM tasks WHERE parent_task_id=$1)) d),'[]'::json),
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
	patchHash, originalResult, err := releaseChainV5CaptureOriginalResult(ctx, f, scenario)
	if err != nil {
		return errNativeFixtureDenied
	}
	body, err := json.Marshal(map[string]any{
		"version": "butler-release-chain-v5-product-witness-v1", "scenario": scenario, "test_failed": failed, "postgres": raw,
		"native_batch_support": false, "native_multi_function_contract": "capability_mismatch_before_tool_release", "generic_wrapper_batch_handoff_evidence": "Go_spy_fixture_only", "core_batch_capability": false,
		"original_document": f.document, "original_need": f.needRef, "original_spec": f.specRef,
		"patch_artifact_file_sha256": patchHash,
		"original_result":            originalResult,
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
	if err != nil || len(body) > releaseChainV5SafeProductOutputMax {
		return errNativeFixtureDenied
	}
	sum := sha256.Sum256(body)
	if _, err = w.pool.Exec(ctx, `INSERT INTO `+w.table("fixture_chain_v5_high_case_evidence")+`(scenario,payload_sha256,payload_json) VALUES($1,$2,$3)`, scenario, hex.EncodeToString(sum[:]), body); err != nil {
		return errNativeFixtureDenied
	}
	return nil
}

func TestReleaseChainV5ProductEvidenceRetainedBeforeCleanupIntegration(t *testing.T) {
	f := newDeveloperFixture(t)
	f.channel = &releaseChainChannel{}
	f.projectDirectory = t.TempDir()
	w, err := createReleaseChainV5Window(f.f.ctx, f.f.pool, strings.Repeat("e", 64), strings.Repeat("f", 64), 7, time.Minute) // Synthetic fixture counter, never a native baseline.
	if err != nil {
		t.Fatal(err)
	}
	const privateLease = "synthetic-private-lease-proof"
	fault := &releaseChainV5Fault{oldChild: ownership.Token{LeaseID: privateLease}}
	if err = recordReleaseChainV5ProductEvidence(f.f.ctx, w, f, "explicit_absence", true, fault); err != nil {
		t.Fatal("synthetic owning SQL product snapshot failed")
	}
	var raw []byte
	var expected string
	if f.f.pool.QueryRow(f.f.ctx, `SELECT payload_json,payload_sha256 FROM `+w.table("fixture_chain_v5_high_case_evidence")).Scan(&raw, &expected) != nil {
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
	for _, query := range []string{`UPDATE ` + w.table("fixture_chain_v5_high_case_evidence") + ` SET payload_sha256=repeat('a',64)`, `DELETE FROM ` + w.table("fixture_chain_v5_high_case_evidence")} {
		if _, err = f.f.pool.Exec(f.f.ctx, query); err == nil {
			t.Fatal("product witness was mutable")
		}
	}
}

func TestReleaseChainV5InitialWitnessDoesNotInventNativeOrAdapterProvenanceIntegration(t *testing.T) {
	f := newDeveloperFixture(t)
	w, err := createReleaseChainV5Window(f.f.ctx, f.f.pool, strings.Repeat("e", 64), strings.Repeat("f", 64), 7, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = recordReleaseChainV5InitialEvidence(f.f.ctx, w, f, "owner_stop"); err != nil {
		t.Fatal("initial witness unavailable", err)
	}
	var raw []byte
	var expected string
	if f.f.pool.QueryRow(f.f.ctx, `SELECT payload_json,payload_sha256 FROM `+w.table("fixture_chain_v5_high_case_initial")).Scan(&raw, &expected) != nil {
		t.Fatal("initial witness missing")
	}
	var evidence map[string]any
	if json.Unmarshal(raw, &evidence) != nil || evidence["configured_native_high"] != false || evidence["model_authored_ir"] != false || evidence["ordinary_core_entry"] != "after_owner_bound_initiative_promoted_input" || evidence["setup_execution"] != "owner_services_and_real_approved_broker_reads_without_model" {
		t.Fatal("fixture claimed native or authored-code provenance")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var canonical any
	if decoder.Decode(&canonical) != nil {
		t.Fatal("canonical initial witness denied")
	}
	body, _ := json.Marshal(canonical)
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != expected {
		t.Fatal("initial witness hash differs after jsonb")
	}
	for _, token := range []string{`"body":`, `"content":`, `"event_json":`, `"binding":`, `"grant_data":`, `"lease_id":`} {
		if bytes.Contains(body, []byte(token)) {
			t.Fatal("initial witness exposed private payload")
		}
	}
	for _, query := range []string{`UPDATE ` + w.table("fixture_chain_v5_high_case_initial") + ` SET payload_sha256=repeat('a',64)`, `DELETE FROM ` + w.table("fixture_chain_v5_high_case_initial")} {
		if _, err = f.f.pool.Exec(f.f.ctx, query); err == nil {
			t.Fatal("initial witness mutable")
		}
	}
}

func TestReleaseChainV5OriginalResultContentFreeProjection(t *testing.T) {
	before, after := "PRIVATE_BEFORE_BYTES", "PRIVATE_AFTER_BYTES"
	patches := map[string]projectfixture.Patch{"key": {ArtifactID: strings.Repeat("a", 64), Proposal: projectfixture.Proposal{ProjectRevision: projectfixture.Revision(), Changes: []projectfixture.Change{{Path: "README.md", Before: before, After: after}}}}}
	r := releaseChainV5PatchProjection(patches)
	if r.PatchCount != 1 || r.ArtifactID == nil || *r.ArtifactID != strings.Repeat("a", 64) || r.ProjectRevision == nil || *r.ProjectRevision != projectfixture.Revision() || len(r.Changes) != 1 || r.Changes[0].BeforeSHA != generatedtools.HashBytes([]byte(before)) || r.Changes[0].AfterSHA != generatedtools.HashBytes([]byte(after)) || r.ChecksPassed != nil || r.Applied != nil {
		t.Fatal("actual bytes were replaced by seeded receipt claims")
	}
	body, _ := json.Marshal(r)
	if bytes.Contains(body, []byte(before)) || bytes.Contains(body, []byte(after)) || bytes.Contains(body, []byte(`"before":`)) || bytes.Contains(body, []byte(`"after":`)) {
		t.Fatal("raw stored patch escaped safe projection")
	}
	patches["other"] = patches["key"]
	ambiguous := releaseChainV5PatchProjection(patches)
	if ambiguous.PatchCount != 2 || ambiguous.ArtifactID != nil || ambiguous.ProjectRevision != nil || len(ambiguous.Changes) != 0 || ambiguous.ChecksPassed != nil {
		t.Fatal("ambiguous actual stored patches became one known artifact")
	}
}

func TestReleaseChainV5OriginalResultFileBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "patches.json")
	if os.WriteFile(path, []byte(`{"real":"stored"}`), 0600) != nil {
		t.Fatal("fixture unavailable")
	}
	if raw, err := releaseChainV5ReadPatch(path); err != nil || string(raw) != `{"real":"stored"}` {
		t.Fatal("original stored bytes changed")
	}
	if os.WriteFile(path, []byte{}, 0600) != nil {
		t.Fatal("empty fixture unavailable")
	}
	if _, err := releaseChainV5ReadPatch(path); err == nil {
		t.Fatal("empty original passed")
	}
	if os.WriteFile(path, bytes.Repeat([]byte("x"), releaseChainV5SafeProductOutputMax+1), 0600) != nil {
		t.Fatal("bounded fixture unavailable")
	}
	if _, err := releaseChainV5ReadPatch(path); err == nil {
		t.Fatal("oversized original exceeded declared capture bound")
	}
}

func TestReleaseChainV5OriginalResultActualStoredReceiptsIntegration(t *testing.T) {
	for _, name := range []string{"stored_known_receipts", "failed_check_preserved", "duplicate_check_uncertain", "owner_stop_effect_denied"} {
		t.Run(name, func(t *testing.T) {
			f := newDeveloperFixture(t)
			f.projectDirectory = t.TempDir()
			artifact := strings.Repeat("a", 64)
			patches := map[string]projectfixture.Patch{"synthetic-receipt-key": {ArtifactID: artifact, Proposal: projectfixture.Proposal{ProjectRevision: projectfixture.Revision(), Changes: []projectfixture.Change{{Path: "README.md", Before: projectfixture.Original, After: projectfixture.Prepared}}}}}
			stored, _ := json.Marshal(patches)
			if os.WriteFile(filepath.Join(f.projectDirectory, "patches.json"), stored, 0600) != nil {
				t.Fatal("actual bounded stored file unavailable")
			}
			checks := name != "failed_check_preserved"
			for _, row := range []struct {
				call, tool string
				body       map[string]any
			}{
				{"actual-prepare", "extension.project.prepare-patch", map[string]any{"artifact_id": artifact}},
				{"actual-check", "extension.project.check", map[string]any{"artifact_id": artifact, "checks_passed": checks, "applied": false}},
			} {
				body, _ := json.Marshal(row.body)
				result, _ := json.Marshal(map[string]any{"body": string(body)})
				if _, err := f.f.pool.Exec(f.f.ctx, `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,status,runtime_target,started_at,result) VALUES($1,$2,$3,'completed','synthetic-project-receipt',clock_timestamp(),$4)`, row.call, f.parentRun, row.tool, result); err != nil {
					t.Fatal("owning synthetic receipt row unavailable", err)
				}
			}
			if name == "duplicate_check_uncertain" {
				if _, err := f.f.pool.Exec(f.f.ctx, `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,status,runtime_target,started_at,result) SELECT 'other-check',run_id,tool_name,status,runtime_target,started_at,result FROM tool_calls WHERE tool_call_id='actual-check'`); err != nil {
					t.Fatal("second owning receipt unavailable")
				}
			}
			scenario := "normal_sent"
			if name == "owner_stop_effect_denied" {
				scenario = "owner_stop"
			}
			hash, r, err := releaseChainV5CaptureOriginalResult(f.f.ctx, f, scenario)
			if scenario == "owner_stop" {
				if err == nil {
					t.Fatal("stopped branch acquired a prepared original result")
				}
				return
			}
			if err != nil || hash == nil || *hash != generatedtools.HashBytes(stored) || r == nil || r.PatchCount != 1 || r.ArtifactID == nil || *r.ArtifactID != artifact || r.PrepareReceipts != 1 || r.PatchCall == nil || *r.PatchCall != "actual-prepare" || r.PrepareArtifact == nil || *r.PrepareArtifact != artifact {
				t.Fatal("actual stored file/prepare receipt projection differs")
			}
			if name == "duplicate_check_uncertain" {
				if r.CheckReceipts != 2 || r.CheckCall != nil || r.ChecksPassed != nil || r.Applied != nil {
					t.Fatal("duplicate receipt became a unique success")
				}
				return
			}
			if r.CheckReceipts != 1 || r.CheckCall == nil || *r.CheckCall != "actual-check" || r.CheckArtifact == nil || *r.CheckArtifact != artifact || r.ChecksPassed == nil || *r.ChecksPassed != checks || r.Applied == nil || *r.Applied {
				t.Fatal("actual failed/success check values changed")
			}
		})
	}
}
