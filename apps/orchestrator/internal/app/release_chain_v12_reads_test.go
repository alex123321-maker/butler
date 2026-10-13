package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/extensions/projectfixture"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/butler/butler/internal/transport/antigravity"
)

type releaseChainV12ReadReceipt struct {
	RunID        string `json:"run_id"`
	ToolCall     string `json:"tool_call_id"`
	Artifact     string `json:"artifact_id,omitempty"`
	Revision     string `json:"project_revision"`
	ChecksPassed bool   `json:"checks_passed"`
	Applied      *bool  `json:"applied"`
	ResultSHA    string `json:"result_sha256"`
	BodySHA      string `json:"body_sha256"`
}

func releaseChainV12RepeatedReads(f *developerFixture) bool {
	return f != nil && f.modelConfig != nil && f.modelConfig.AllowRepeatedPatchChecks && f.modelConfig.Provider == antigravity.Name && f.modelConfig.Model == antigravity.FlashHighModel && f.projectDirectory != ""
}

type releaseChainV12Action struct{ ID, Run, Call, Task, Tool, State, Outcome string }

func releaseChainV12ValidateInventory(actions []releaseChainV12Action, root, child, converter string) error {
	if root == "" || child == "" || root == child || converter == "" {
		return errNativeFixtureDenied
	}
	rootCounts, childCounts := map[string]int{}, map[string]int{}
	ids, calls := map[string]bool{}, map[string]bool{}
	for _, a := range actions {
		key := a.Run + "\x00" + a.Call
		if a.ID == "" || a.Run == "" || a.Call == "" || ids[a.ID] || calls[key] || a.State != "effect_confirmed" || a.Outcome != "dispatch_completed" {
			return errNativeFixtureDenied
		}
		ids[a.ID], calls[key] = true, true
		switch a.Task {
		case root:
			rootCounts[a.Tool]++
		case child:
			childCounts[a.Tool]++
		default:
			return errNativeFixtureDenied
		}
	}
	for _, name := range []string{"converters.discover", "task.delegate_converter", "converters.collect", "converters.test", "converters.trial", "converters.activate", "converter." + converter + ".convert", "extension.project.prepare-patch"} {
		if rootCounts[name] != 1 {
			return errNativeFixtureDenied
		}
		delete(rootCounts, name)
	}
	for _, name := range []string{"extension.project.inspect", "extension.project.check"} {
		if rootCounts[name] < 1 {
			return errNativeFixtureDenied
		}
		delete(rootCounts, name)
	}
	for _, name := range []string{"converters.discover", "converters.develop", "converters.test"} {
		if childCounts[name] != 1 {
			return errNativeFixtureDenied
		}
		delete(childCounts, name)
	}
	if len(rootCounts) != 0 || len(childCounts) != 0 {
		return errNativeFixtureDenied
	}
	return nil
}

func releaseChainV12CheckInventory(ctx context.Context, f *developerFixture, child string) error {
	rows, err := f.f.pool.Query(ctx, `SELECT i.action_id,i.run_id,i.tool_call_id,r.task_id,COALESCE(c.tool_name,''),COALESCE(e.state,''),COALESCE(e.dispatch_outcome,'') FROM action_intents i JOIN runs r ON r.run_id=i.run_id LEFT JOIN action_effects e ON e.action_id=i.effect_id LEFT JOIN tool_calls c ON c.run_id=i.run_id AND c.tool_call_id=i.tool_call_id WHERE r.task_id IN($1,$2) ORDER BY i.created_at,i.action_id`, f.f.parent.TaskID, child)
	if err != nil {
		return errNativeFixtureDenied
	}
	defer rows.Close()
	actions := []releaseChainV12Action{}
	for rows.Next() {
		var a releaseChainV12Action
		if rows.Scan(&a.ID, &a.Run, &a.Call, &a.Task, &a.Tool, &a.State, &a.Outcome) != nil {
			return errNativeFixtureDenied
		}
		actions = append(actions, a)
	}
	if rows.Err() != nil {
		return errNativeFixtureDenied
	}
	return releaseChainV12ValidateInventory(actions, f.f.parent.TaskID, child, f.need.ConverterID)
}

func releaseChainV12ReadReceiptFrom(run, call, tool, args, result, artifact string) (releaseChainV12ReadReceipt, error) {
	r := releaseChainV12ReadReceipt{RunID: run, ToolCall: call, ResultSHA: generatedtools.HashBytes([]byte(result))}
	if run == "" || call == "" {
		return r, errNativeFixtureDenied
	}
	if _, err := actionauth.ArgumentsHash(args); err != nil {
		return r, errNativeFixtureDenied
	}
	var input map[string]any
	if json.Unmarshal([]byte(args), &input) != nil || len(input) != 1 {
		return r, errNativeFixtureDenied
	}
	want := egresstest.Origin + "/project"
	if tool == "extension.project.check" {
		want += "/patches/" + artifact + "/checks"
	} else if tool != "extension.project.inspect" {
		return r, errNativeFixtureDenied
	}
	if input["url"] != want {
		return r, errNativeFixtureDenied
	}
	var envelope map[string]any
	if _, err := actionauth.ArgumentsHash(result); err != nil {
		return r, errNativeFixtureDenied
	}
	if json.Unmarshal([]byte(result), &envelope) != nil {
		return r, errNativeFixtureDenied
	}
	if nested, ok := envelope["result"].(map[string]any); ok {
		envelope = nested
	}
	if envelope["status_code"] != float64(200) {
		return r, errNativeFixtureDenied
	}
	bodyText, ok := envelope["body"].(string)
	if !ok {
		return r, errNativeFixtureDenied
	}
	if _, err := actionauth.ArgumentsHash(bodyText); err != nil {
		return r, errNativeFixtureDenied
	}
	body, err := initiativeProjectToolBody(result)
	if err != nil || body["project_ref"] != projectfixture.ProjectRef || body["project_revision"] != projectfixture.Revision() || body["revision_sha256"] != projectfixture.Revision() || body["observation_format"] != "project_check_v1" {
		return r, errNativeFixtureDenied
	}
	r.Revision, r.BodySHA = projectfixture.Revision(), generatedtools.Hash(body)
	if tool == "extension.project.check" {
		if len(body) != 12 || body["artifact_id"] != artifact || body["checks_passed"] != true || body["applied"] != false || body["status"] != "passed" || body["produced_by_butler"] != true || body["error_kind"] != "" || body["check_id"] != generatedtools.HashBytes([]byte(projectfixture.ProjectRef+"\x00"+projectfixture.Revision()+"\x00ready_flag\x00passed")) {
			return r, errNativeFixtureDenied
		}
		checkRaw, _ := json.Marshal(body["checks"])
		var checks []map[string]string
		if json.Unmarshal(checkRaw, &checks) != nil || len(checks) != 1 || len(checks[0]) != 2 || checks[0]["name"] != "ready_flag" || checks[0]["status"] != "passed" {
			return r, errNativeFixtureDenied
		}
		value := false
		r.Artifact, r.ChecksPassed, r.Applied = artifact, true, &value
	} else {
		if len(body) != 10 || body["checks_passed"] != false || body["produced_by_butler"] != false || body["status"] != "failed" || body["error_kind"] != "ready_flag_false" || body["check_id"] != generatedtools.HashBytes([]byte(projectfixture.ProjectRef+"\x00"+projectfixture.Revision()+"\x00ready_flag\x00failed")) {
			return r, errNativeFixtureDenied
		}
		raw, _ := json.Marshal(body["files"])
		var files []map[string]string
		if json.Unmarshal(raw, &files) != nil || len(files) != 1 || len(files[0]) != 2 || files[0]["path"] != "README.md" || files[0]["content"] != projectfixture.Original {
			return r, errNativeFixtureDenied
		}
	}
	return r, nil
}

func releaseChainV12PrepareReceipt(args, result string, patch projectfixture.Patch) error {
	if len(patch.Proposal.Changes) != 1 {
		return errNativeFixtureDenied
	}
	if _, err := actionauth.ArgumentsHash(args); err != nil {
		return errNativeFixtureDenied
	}
	var input map[string]json.RawMessage
	if json.Unmarshal([]byte(args), &input) != nil || len(input) != 2 {
		return errNativeFixtureDenied
	}
	var url string
	if json.Unmarshal(input["url"], &url) != nil || url != egresstest.Origin+"/project/patches" {
		return errNativeFixtureDenied
	}
	decoder := json.NewDecoder(bytes.NewReader(input["body"]))
	decoder.DisallowUnknownFields()
	var proposal projectfixture.Proposal
	if decoder.Decode(&proposal) != nil || decoder.Decode(new(any)) != io.EOF || proposal.ProjectRevision != patch.Proposal.ProjectRevision || len(proposal.Changes) != 1 || proposal.Changes[0] != patch.Proposal.Changes[0] {
		return errNativeFixtureDenied
	}
	var envelope map[string]any
	if _, err := actionauth.ArgumentsHash(result); err != nil {
		return errNativeFixtureDenied
	}
	if json.Unmarshal([]byte(result), &envelope) != nil {
		return errNativeFixtureDenied
	}
	if nested, ok := envelope["result"].(map[string]any); ok {
		envelope = nested
	}
	if envelope["status_code"] != float64(200) && envelope["status_code"] != float64(201) {
		return errNativeFixtureDenied
	}
	text, ok := envelope["body"].(string)
	if !ok {
		return errNativeFixtureDenied
	}
	if _, err := actionauth.ArgumentsHash(text); err != nil {
		return errNativeFixtureDenied
	}
	body, err := initiativeProjectToolBody(result)
	if err != nil || len(body) != 2 || body["artifact_id"] != patch.ArtifactID {
		return errNativeFixtureDenied
	}
	receipt, ok := body["butler_effect_receipt"].(map[string]any)
	if !ok || len(receipt) != 4 || receipt["idempotency_key"] != patch.Key || receipt["action_fingerprint"] != patch.Fingerprint || receipt["effect_state"] != "effect_confirmed" || receipt["artifact_id"] != patch.ArtifactID {
		return errNativeFixtureDenied
	}
	return nil
}

func releaseChainV12CaptureOriginalResult(ctx context.Context, f *developerFixture, scenario string) (*string, *releaseChainV5OriginalResult, error) {
	raw, err := releaseChainV5ReadPatch(filepath.Join(f.projectDirectory, "patches.json"))
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil || scenario == "owner_stop" {
		return nil, nil, errNativeFixtureDenied
	}
	fileSHA := generatedtools.HashBytes(raw)
	var patches map[string]projectfixture.Patch
	if json.Unmarshal(raw, &patches) != nil || len(patches) != 1 {
		return &fileSHA, nil, errNativeFixtureDenied
	}
	r := releaseChainV5PatchProjection(patches)
	if r.ArtifactID == nil || !nativeAuditHash.MatchString(*r.ArtifactID) {
		return &fileSHA, r, errNativeFixtureDenied
	}
	var actualPatch projectfixture.Patch
	for key, patch := range patches {
		if key != patch.Key || len(key) == 0 || len(key) > 160 || !nativeAuditHash.MatchString(patch.Fingerprint) || patch.ArtifactID != generatedtools.HashBytes([]byte(key+"\x00"+patch.Fingerprint)) || patch.Proposal.ProjectRevision != projectfixture.Revision() || len(patch.Proposal.Changes) != 1 || patch.Proposal.Changes[0] != (projectfixture.Change{Path: "README.md", Before: projectfixture.Original, After: projectfixture.Prepared}) {
			return &fileSHA, r, errNativeFixtureDenied
		}
		actualPatch = patch
	}
	rows, err := f.f.pool.Query(ctx, `SELECT c.run_id,c.tool_name,c.tool_call_id,c.args::text,c.result::text,c.status,COALESCE(e.state,''),COALESCE(e.dispatch_outcome,'') FROM tool_calls c JOIN runs rr ON rr.run_id=c.run_id LEFT JOIN action_intents i ON i.run_id=c.run_id AND i.tool_call_id=c.tool_call_id AND i.binding->>'tool_name'=c.tool_name LEFT JOIN action_effects e ON e.action_id=i.effect_id WHERE rr.task_id=$1 AND c.tool_name IN('extension.project.inspect','extension.project.prepare-patch','extension.project.check') ORDER BY c.started_at,c.run_id,c.tool_call_id`, f.f.parent.TaskID)
	if err != nil {
		return &fileSHA, r, errNativeFixtureDenied
	}
	defer rows.Close()
	for rows.Next() {
		var run, tool, call, args, result, status, state, outcome string
		if rows.Scan(&run, &tool, &call, &args, &result, &status, &state, &outcome) != nil || status != "completed" || state != "effect_confirmed" || outcome != "dispatch_completed" {
			return &fileSHA, r, errNativeFixtureDenied
		}
		if tool == "extension.project.prepare-patch" {
			r.PrepareReceipts++
			if releaseChainV12PrepareReceipt(args, result, actualPatch) != nil {
				return &fileSHA, r, errNativeFixtureDenied
			}
			r.PatchCall, r.PrepareArtifact = &call, r.ArtifactID
			continue
		}
		receipt, e := releaseChainV12ReadReceiptFrom(run, call, tool, args, result, *r.ArtifactID)
		if e != nil {
			return &fileSHA, r, errNativeFixtureDenied
		}
		if tool == "extension.project.check" {
			if len(r.CheckInventory) > 0 && receipt.BodySHA != r.CheckInventory[0].BodySHA {
				return &fileSHA, r, errNativeFixtureDenied
			}
			r.CheckInventory = append(r.CheckInventory, receipt)
		} else {
			if len(r.InspectInventory) > 0 && receipt.BodySHA != r.InspectInventory[0].BodySHA {
				return &fileSHA, r, errNativeFixtureDenied
			}
			r.InspectInventory = append(r.InspectInventory, receipt)
		}
	}
	if rows.Err() != nil || r.PrepareReceipts != 1 || len(r.CheckInventory) < 1 || len(r.InspectInventory) < 1 {
		return &fileSHA, r, errNativeFixtureDenied
	}
	r.CheckReceipts = len(r.CheckInventory)
	chosen := r.CheckInventory[0]
	truth := true
	r.CheckCall, r.CheckArtifact, r.ChecksPassed, r.Applied = &chosen.ToolCall, r.ArtifactID, &truth, chosen.Applied
	return &fileSHA, r, nil
}
