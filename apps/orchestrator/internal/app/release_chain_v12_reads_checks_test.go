package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/internal/extensions/projectfixture"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/butler/butler/internal/transport/antigravity"
)

func v12RequiredActions() []releaseChainV12Action {
	actions := []releaseChainV12Action{}
	for _, group := range []struct {
		task  string
		tools []string
	}{{"root", []string{"converters.discover", "task.delegate_converter", "converters.collect", "converters.test", "converters.trial", "converters.activate", "converter.inventory.convert", "extension.project.inspect", "extension.project.prepare-patch", "extension.project.check"}}, {"child", []string{"converters.discover", "converters.develop", "converters.test"}}} {
		for _, tool := range group.tools {
			n := len(actions)
			actions = append(actions, releaseChainV12Action{ID: fmt.Sprintf("action-%d", n), Run: group.task + "-run", Call: fmt.Sprintf("call-%d", n), Task: group.task, Tool: tool, State: "effect_confirmed", Outcome: "dispatch_completed"})
		}
	}
	return actions
}

func TestReleaseChainV12ReadInventoryBudgetedRepeatsAndLegacy(t *testing.T) {
	base := v12RequiredActions()
	if releaseChainV12ValidateInventory(base, "root", "child", "inventory") != nil {
		t.Fatal("mandatory logical inventory rejected")
	}
	for i := 0; i < 8; i++ {
		base = append(base, releaseChainV12Action{ID: fmt.Sprintf("extra-action-%d", i), Run: "root-next-run", Call: fmt.Sprintf("extra-call-%d", i), Task: "root", Tool: []string{"extension.project.inspect", "extension.project.check"}[i%2], State: "effect_confirmed", Outcome: "dispatch_completed"})
	}
	if releaseChainV12ValidateInventory(base, "root", "child", "inventory") != nil {
		t.Fatal("fresh GET reads acquired an artificial per-tool limit")
	}
	base[10].Call = base[0].Call
	if releaseChainV12ValidateInventory(base, "root", "child", "inventory") != nil {
		t.Fatal("different runs acquired artificial global call-ID uniqueness")
	}
	raw, _ := json.Marshal(&releaseChainV5OriginalResult{})
	if strings.Contains(string(raw), "check_receipts\"") || strings.Contains(string(raw), "inspect_receipts") || (releaseChainHighEntry{}).AllowRepeatedPatchChecks || (releaseChainModelConfig{}).AllowRepeatedPatchChecks {
		t.Fatal("legacy default/witness shape changed")
	}
}

func TestReleaseChainV12ReadInventoryRejectsEffectsAndUnknowns(t *testing.T) {
	for _, kind := range []string{"double_prepare", "double_activation", "child_read", "foreign_task", "unknown_effect", "missing_check", "duplicate_id", "duplicate_run_call", "missing_run"} {
		t.Run(kind, func(t *testing.T) {
			a := v12RequiredActions()
			switch kind {
			case "double_prepare", "double_activation", "child_read", "foreign_task":
				extra := releaseChainV12Action{ID: "extra-action", Run: "root-run", Call: "extra-call", Task: "root", Tool: "extension.project.prepare-patch", State: "effect_confirmed", Outcome: "dispatch_completed"}
				if kind == "double_activation" {
					extra.Tool = "converters.activate"
				}
				if kind == "child_read" {
					extra.Task = "child"
					extra.Tool = "extension.project.check"
				}
				if kind == "foreign_task" {
					extra.Task = "foreign"
					extra.Tool = "extension.project.inspect"
				}
				a = append(a, extra)
			case "unknown_effect":
				a[0].State = "effect_unknown"
			case "missing_check":
				a = append(a[:9], a[10:]...)
			case "duplicate_id":
				a[1].ID = a[0].ID
			case "duplicate_run_call":
				a[1].Call = a[0].Call
			case "missing_run":
				a[0].Run = ""
			}
			if releaseChainV12ValidateInventory(a, "root", "child", "inventory") == nil {
				t.Fatal("invalid effect/read inventory accepted")
			}
		})
	}
}

func v12RealRecipient(t *testing.T) (func(string) string, string, string, string) {
	t.Helper()
	directory := t.TempDir()
	proposal := projectfixture.Proposal{ProjectRevision: projectfixture.Revision(), Changes: []projectfixture.Change{{Path: "README.md", Before: projectfixture.Original, After: projectfixture.Prepared}}}
	patch := projectfixture.Patch{Key: "v12-synthetic-stored-patch", Fingerprint: strings.Repeat("a", 64), Proposal: proposal}
	patch.ArtifactID = generatedtools.HashBytes([]byte(patch.Key + "\x00" + patch.Fingerprint))
	stored, _ := json.Marshal(map[string]projectfixture.Patch{patch.Key: patch})
	if err := os.WriteFile(filepath.Join(directory, "patches.json"), stored, 0600); err != nil {
		t.Fatal("synthetic recipient state unavailable")
	}
	// Setup is synthetic; the recipient validates the stored immutable patch.
	// Repeated observations below use real HTTP GETs without a filesystem write.
	s, err := projectfixture.New(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	server := httptest.NewServer(s)
	t.Cleanup(server.Close)
	raw, _ := json.Marshal(proposal)
	req, _ := http.NewRequest("POST", server.URL+"/project/patches", bytes.NewReader(raw))
	req.Header.Set("Idempotency-Key", patch.Key)
	req.Header.Set("Butler-Action-Fingerprint", patch.Fingerprint)
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	var body map[string]any
	if err != nil || json.Unmarshal(responseBody, &body) != nil || response.StatusCode != 200 {
		t.Fatal("actual stored-patch receipt unavailable")
	}
	artifact, ok := body["artifact_id"].(string)
	if !ok {
		t.Fatal("actual artifact missing")
	}
	get := func(path string) string {
		t.Helper()
		response, err := server.Client().Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(map[string]any{"status_code": response.StatusCode, "body": string(data)})
		return string(raw)
	}
	prepare, _ := json.Marshal(map[string]any{"status_code": response.StatusCode, "body": string(responseBody)})
	return get, artifact, directory, string(prepare)
}

func TestReleaseChainV12ActualGETReceiptsAgreeWithoutMutation(t *testing.T) {
	get, artifact, _, _ := v12RealRecipient(t)
	for _, tool := range []string{"extension.project.inspect", "extension.project.check"} {
		path := "/project"
		if tool == "extension.project.check" {
			path += "/patches/" + artifact + "/checks"
		}
		args, _ := json.Marshal(map[string]string{"url": egresstest.Origin + path})
		first := ""
		for i := 0; i < 6; i++ {
			receipt, err := releaseChainV12ReadReceiptFrom("synthetic-root-run", fmt.Sprintf("read-%d", i), tool, string(args), get(path), artifact)
			if err != nil || receipt.Revision != projectfixture.Revision() || (first != "" && first != receipt.BodySHA) {
				t.Fatal("actual repeated GET evidence disagrees")
			}
			first = receipt.BodySHA
			if tool == "extension.project.check" && (!receipt.ChecksPassed || receipt.Applied == nil || *receipt.Applied) {
				t.Fatal("read check altered actual unapplied result")
			}
		}
	}
	manifest, err := projectfixture.Manifest(strings.Repeat("b", 64), "fixture-current")
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range manifest.Manifest.Operations {
		if operation.Name == "extension.project.check" || operation.Name == "extension.project.inspect" {
			if operation.Method != "GET" || operation.Effect != "read" {
				t.Fatal("read permission semantics changed")
			}
		}
	}
}

func TestReleaseChainV12ReadReceiptRejectsConflictsAndForeignArtifact(t *testing.T) {
	get, artifact, _, _ := v12RealRecipient(t)
	path := "/project/patches/" + artifact + "/checks"
	args, _ := json.Marshal(map[string]string{"url": egresstest.Origin + path})
	for _, kind := range []string{"foreign_url", "foreign_artifact", "checks_false", "applied_true", "wrong_revision", "wrong_check_array", "duplicate_body_key", "failed_http", "missing_bool"} {
		t.Run(kind, func(t *testing.T) {
			input := string(args)
			raw := get(path)
			var outer map[string]any
			_ = json.Unmarshal([]byte(raw), &outer)
			var body map[string]any
			_ = json.Unmarshal([]byte(outer["body"].(string)), &body)
			switch kind {
			case "foreign_url":
				input = `{"url":"https://foreign.invalid/project"}`
			case "foreign_artifact":
				body["artifact_id"] = strings.Repeat("c", 64)
			case "checks_false":
				body["checks_passed"] = false
			case "applied_true":
				body["applied"] = true
			case "wrong_revision":
				body["project_revision"] = strings.Repeat("d", 64)
			case "wrong_check_array":
				body["checks"] = []any{map[string]string{"name": "ready_flag", "status": "failed"}}
			case "failed_http":
				outer["status_code"] = 503
			case "missing_bool":
				delete(body, "applied")
			}
			bodyRaw, _ := json.Marshal(body)
			if kind == "duplicate_body_key" {
				bodyRaw = []byte(strings.TrimSuffix(string(bodyRaw), "}") + `,"checks_passed":true}`)
			}
			outer["body"] = string(bodyRaw)
			encoded, _ := json.Marshal(outer)
			if _, err := releaseChainV12ReadReceiptFrom("synthetic-root-run", "read-call", "extension.project.check", input, string(encoded), artifact); err == nil {
				t.Fatal("conflicting or foreign read receipt accepted")
			}
		})
	}
}

// The recipient HTTP responses are actual loopback observations. PG journal
// rows here are explicit synthetic fixture setup, not a Broker dispatch proof.
func TestReleaseChainV12OriginalResultActualRepeatedReceiptsIntegration(t *testing.T) {
	for _, kind := range []string{"actual_repeated_reads", "conflicting_check", "foreign_artifact", "unconfirmed_read", "duplicate_prepare", "owner_stop"} {
		t.Run(kind, func(t *testing.T) {
			f := newDeveloperFixture(t)
			get, artifact, directory, prepare := v12RealRecipient(t)
			f.projectDirectory = directory
			f.modelConfig = &releaseChainModelConfig{Provider: antigravity.Name, Model: antigravity.FlashHighModel, AllowRepeatedPatchChecks: true}
			proposal := projectfixture.Proposal{ProjectRevision: projectfixture.Revision(), Changes: []projectfixture.Change{{Path: "README.md", Before: projectfixture.Original, After: projectfixture.Prepared}}}
			prepareArgs, _ := json.Marshal(map[string]any{"url": egresstest.Origin + "/project/patches", "body": proposal})
			store := func(call, tool string, args []byte, result string, confirmed bool) {
				t.Helper()
				if _, err := f.f.pool.Exec(f.f.ctx, `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,finished_at,result) VALUES($1,$2,$3,$4,'completed','synthetic-loopback-project',clock_timestamp(),clock_timestamp(),$5)`, call, f.parentRun, tool, args, []byte(result)); err != nil {
					t.Fatal("synthetic owning tool row unavailable", err)
				}
				effect, action := "v12-effect-"+call, "v12-action-"+call
				binding, _ := json.Marshal(map[string]string{"tool_name": tool})
				state := "effect_confirmed"
				if !confirmed {
					state = "effect_unknown"
				}
				fingerprint := generatedtools.HashBytes([]byte(f.parentRun + "\x00" + call))
				if _, err := f.f.pool.Exec(f.f.ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state,dispatch_outcome) VALUES($1,$2,$3,'{}'::jsonb,$4,'dispatch_completed')`, effect, fingerprint, binding, state); err != nil {
					t.Fatal("synthetic effect row unavailable", err)
				}
				if _, err := f.f.pool.Exec(f.f.ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at,effect_id) VALUES($1,$2,$3,$4,$5,false,clock_timestamp()+interval '1 minute',$6)`, action, f.parentRun, call, fingerprint, binding, effect); err != nil {
					t.Fatal("synthetic intent row unavailable", err)
				}
			}
			store("prepared", "extension.project.prepare-patch", prepareArgs, prepare, true)
			for i := 0; i < 2; i++ {
				inspectArgs, _ := json.Marshal(map[string]string{"url": egresstest.Origin + "/project"})
				store(fmt.Sprintf("inspect-%d", i), "extension.project.inspect", inspectArgs, get("/project"), true)
				path := "/project/patches/" + artifact + "/checks"
				checkArgs, _ := json.Marshal(map[string]string{"url": egresstest.Origin + path})
				result := get(path)
				if i == 1 && (kind == "conflicting_check" || kind == "foreign_artifact") {
					var envelope, body map[string]any
					_ = json.Unmarshal([]byte(result), &envelope)
					_ = json.Unmarshal([]byte(envelope["body"].(string)), &body)
					if kind == "conflicting_check" {
						body["checks_passed"] = false
					} else {
						body["artifact_id"] = strings.Repeat("c", 64)
					}
					raw, _ := json.Marshal(body)
					envelope["body"] = string(raw)
					raw, _ = json.Marshal(envelope)
					result = string(raw)
				}
				store(fmt.Sprintf("check-%d", i), "extension.project.check", checkArgs, result, !(i == 1 && kind == "unconfirmed_read"))
			}
			if kind == "duplicate_prepare" {
				store("prepared-again", "extension.project.prepare-patch", prepareArgs, prepare, true)
			}
			scenario := "normal_sent"
			if kind == "owner_stop" {
				scenario = kind
			}
			fileSHA, r, err := releaseChainV5CaptureOriginalResult(f.f.ctx, f, scenario)
			if kind != "actual_repeated_reads" {
				if err == nil {
					t.Fatal("conflicting, foreign, unknown or repeated write evidence accepted")
				}
				return
			}
			stored, readErr := os.ReadFile(filepath.Join(directory, "patches.json"))
			if err != nil || readErr != nil || fileSHA == nil || *fileSHA != generatedtools.HashBytes(stored) || r == nil || r.PatchCount != 1 || r.PrepareReceipts != 1 || r.CheckReceipts != 2 || len(r.CheckInventory) != 2 || len(r.InspectInventory) != 2 || r.ChecksPassed == nil || !*r.ChecksPassed || r.Applied == nil || *r.Applied {
				t.Fatal("actual PG/recipient repeated receipt projection differs", err)
			}
			for _, receipt := range append(append([]releaseChainV12ReadReceipt{}, r.CheckInventory...), r.InspectInventory...) {
				if receipt.RunID != f.parentRun || receipt.ToolCall == "" || !nativeAuditHash.MatchString(receipt.ResultSHA) || !nativeAuditHash.MatchString(receipt.BodySHA) {
					t.Fatal("qualified actual receipt binding missing")
				}
			}
			safe, _ := json.Marshal(r)
			for _, private := range []string{projectfixture.Original, projectfixture.Prepared, "status_code", "\"body\"", "\"args\""} {
				if bytes.Contains(safe, []byte(private)) {
					t.Fatal("raw project observations escaped safe witness")
				}
			}
		})
	}
}
