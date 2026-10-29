package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/acceptance/utilitytrace"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/butler/butler/internal/transport"
)

// Scripted transport events exercise actual Core/Broker persistence. They are
// neither native model calls nor evidence of model quality/retrieval usefulness.
type utilityTraceProvider struct {
	experienceOriginalControlProvider
	mode  string
	wrong bool
	pin   readcontrol.SourceRef
}

func (p *utilityTraceProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	if p.mode == "prepared" || p.mode == "rejected" {
		if p.mode == "rejected" {
			p.ready = "maybe"
		}
		return p.experienceOriginalControlProvider.StartRun(ctx, q)
	}
	if transport.CheckInferenceAuthority(ctx) != nil || transport.CheckInferenceDispatch(ctx) != nil {
		return nil, fmt.Errorf("controlled authority unavailable")
	}
	p.run = q.Context.RunID
	p.phase = 1
	if p.mode == "memory" {
		return p.emit(readcontrol.Search, map[string]any{"query": "идентификатор комплекта", "limit": 1}), nil
	}
	if p.mode == "negative" {
		return p.emit("extension.project.inspect", map[string]string{"url": egresstest.Origin + "/project/alpha?validate_ready=maybe"}), nil
	}
	return p.emit("extension.project.inspect", map[string]string{"url": egresstest.Origin + "/project/alpha"}), nil

}
func (p *utilityTraceProvider) final(f acceptance.UtilityFinal) transport.EventStream {
	if p.wrong {
		f.Value = "wrong unsupported claim"
	}
	raw, _ := json.Marshal(f)
	ch := make(chan transport.TransportEvent, 1)
	ch <- transport.NewAssistantFinalEvent(p.run, p.Name(), transport.AssistantFinal{Content: string(raw), Usage: &transport.Usage{InputTokens: 30, OutputTokens: 20, TotalTokens: 60}})
	close(ch)
	return ch
}
func (p *utilityTraceProvider) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if transport.CheckInferenceAuthority(ctx) != nil || transport.CheckInferenceDispatch(ctx) != nil {
		return nil, fmt.Errorf("controlled continuation authority unavailable")
	}
	if (p.mode == "prepared" || p.mode == "rejected") && p.phase < 3 {
		return p.experienceOriginalControlProvider.SubmitToolResult(ctx, q)
	}
	var env struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal([]byte(q.ToolResultJSON), &env) != nil || env.Status != "completed" {
		return nil, fmt.Errorf("actual tool did not complete")
	}
	f := acceptance.UtilityFinal{Outcome: "answered", Sources: []acceptance.UtilityRef{}, Effect: "none", Check: "not_checked"}
	if p.mode == "negative" {
		f.Value = "unsupported_ready_value"
		f.Effect = "absent"
		return p.final(f), nil
	}
	if p.mode == "memory" {
		data, err := readcontrol.DecodeResult(env.Result)
		if err != nil || len(data.Items) != 1 {
			return nil, fmt.Errorf("one actual source required: %v", err)
		}
		if p.phase == 1 {
			p.phase++
			p.pin = data.Items[0].Source
			return p.emit(readcontrol.Open, map[string]any{"source_id": p.pin.ID, "source_revision": p.pin.Revision, "access_revision": p.pin.AccessRevision, "content_hash": p.pin.Hash, "max_chars": 8192}), nil
		}
		words := strings.Fields(data.Items[0].Text)
		if len(words) == 0 {
			return nil, fmt.Errorf("actual source body absent")
		}
		f.Value = words[len(words)-1]
		f.Sources = []acceptance.UtilityRef{{ID: p.pin.ID, Revision: p.pin.Revision, SHA256: p.pin.Hash}}
		return p.final(f), nil
	}
	var envelope struct {
		Body string `json:"body"`
	}
	if json.Unmarshal(env.Result, &envelope) != nil {
		return nil, fmt.Errorf("recipient envelope absent")
	}
	var body struct {
		Artifact string `json:"artifact_id"`
		Passed   *bool  `json:"checks_passed"`
	}
	if json.Unmarshal([]byte(envelope.Body), &body) != nil || body.Passed == nil {
		return nil, fmt.Errorf("actual check absent")
	}
	if p.mode == "prepared" {
		if !*body.Passed || body.Artifact == "" {
			return nil, fmt.Errorf("actual artifact failed")
		}
		f.Value = "ready"
		f.Effect = "confirmed"
		f.Artifact = body.Artifact
		f.Check = "passed"
	} else {
		if *body.Passed {
			return nil, fmt.Errorf("expected failed current inspection")
		}
		f.Value = "failed"
		f.Check = "failed"
	}
	return p.final(f), nil
}

func TestMemoryUtilityActualCoreBrokerTraceIntegration(t *testing.T) {
	if os.Getenv("BUTLER_UTILITY_TRACE_OFFLINE") != "1" {
		t.Skip("explicit isolated Core/Broker utility proof required")
	}
	cases := []struct {
		name, mode     string
		index          int
		wrong, success bool
	}{
		{"memory_read", "memory", 0, false, true}, {"memory_wrong_answer", "memory", 0, true, false},
		{"inspect", "inspect", 20, false, true}, {"inspect_wrong_answer", "inspect", 20, true, false},
		{"prepared", "prepared", 23, false, true}, {"prepared_wrong_answer", "prepared", 23, true, false},
		{"generic_rejection_remains_unknown", "rejected", 22, false, false},
		{"typed_negative", "negative", 22, false, true}, {"typed_negative_wrong_answer", "negative", 22, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			pool := memorytest.Database(t)
			dir := t.TempDir()
			anchor := filepath.Join(dir, "anchor")
			manifest := catalog.NewFileManifest(anchor)
			if err := manifest.Initialize(); err != nil {
				t.Fatal(err)
			}
			cat := catalog.New(pool, manifest)
			suite, err := acceptance.NewMemoryUtilitySuite()
			if err != nil {
				t.Fatal(err)
			}
			c := suite.Cases[tc.index]
			arm, runtimeArm := "A", experienceNativeArms[0]
			var subset []catalog.SourceRef
			var mappings []utilitytrace.SourceBinding
			if tc.mode == "memory" {
				arm, runtimeArm = "B", experienceNativeArms[1]
				// Persist original DATA before constructing the current Core/reader.
				source := c.Preparation[0]
				stored, e := cat.Put(experienceNativeActor(ctx, "utility-prior-source"), catalog.PutSource{ExternalKey: source.Source.ID, Kind: "document", ScopeType: "project", ScopeID: "utility-prior-alpha", Title: "идентификатор комплекта alpha", Body: source.Source.Body})
				if e != nil {
					t.Fatal(e)
				}
				pin := catalog.SourceRef{ID: stored.ID, Revision: stored.Revision, AccessRevision: stored.AccessRevision, Hash: stored.Hash}
				subset = []catalog.SourceRef{pin}
				mappings = []utilitytrace.SourceBinding{{UtilityID: source.Source.ID, Pin: pin}}
				cat = catalog.New(pool, catalog.NewFileManifest(anchor)) // new reader, durable source
			}
			recipient, err := newExperienceRecipient(dir, experienceRecipientConfig{Version: "v1", Ready: "true", Files: map[string]string{"alpha": "Utility current project\nready=false\n", "beta": "Other project\nready=false\n"}})
			if err != nil {
				t.Fatal(err)
			}
			defer recipient.close()
			before := acceptance.MemoryDigest(recipient.snapshot().Config.Files)
			r := newExperienceNativeRuntime(t, pool, cat, anchor, dir, recipient, newExperienceNativeAddresses(t), experienceNativeSourcePolicy(), acceptance.MemoryUtilityInstructions, runtimeArm, subset, time.Now().Add(time.Hour))
			defer r.close()
			id := experienceDigest(t.Name())
			permissions := "inspect_and_check_only"
			if tc.mode == "prepared" || tc.mode == "rejected" {
				permissions = "prepare_and_verify_exact_resource"
			}
			record := r.createTask(t, ctx, id, c.Question, "alpha", permissions)
			r.acceptCurrentProfile(t, ctx, record)
			for _, m := range mappings {
				if err = cat.GrantSource(experienceNativeActor(ctx, "utility-grant"), record.TaskID, m.Pin.ID, "memory_read", record.Revision); err != nil {
					t.Fatal(err)
				}
			}
			binding := utilitytrace.Binding{SuiteSHA256: acceptance.MemoryDigest(suite), CaseID: c.ID, Arm: arm, TaskID: record.TaskID, SessionKey: record.SessionKey, OwnerID: record.OwnerID, TaskRevision: record.Revision, ConstraintsRevision: record.ConstraintsRevision, OriginalFilesSHA256: before, Sources: mappings}
			raw, _ := json.Marshal(c.Input())
			out := r.execute(t, ctx, record, id, string(raw), &utilityTraceProvider{mode: tc.mode, wrong: tc.wrong})
			var run string
			if out.Result != nil {
				run = out.Result.RunID
			} else {
				if err = pool.QueryRow(ctx, `SELECT run_id FROM accepted_inputs WHERE task_id=$1`, record.TaskID).Scan(&run); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := utilitytrace.Capture(ctx, pool, cat, binding, run, recipient.snapshot().Config.Files)
			if err != nil {
				t.Fatal(err)
			}
			seal := acceptance.MemoryDigest(snapshot)
			observation, projectErr := utilitytrace.Project(suite, binding, snapshot, seal)
			result := acceptance.EvaluateMemoryUtility(suite, observation)
			attacks := 0
			defer func() {
				if root := os.Getenv("BUTLER_UTILITY_TRACE_OUTPUT"); root != "" {
					dst := filepath.Join(root, tc.name)
					if err = os.Mkdir(dst, 0700); err != nil {
						t.Fatal(err)
					}
					for name, value := range map[string]any{"suite.private.json": suite, "binding.json": binding, "snapshot.private.json": snapshot, "observation.json": observation, "result.json": result, "summary.json": map[string]any{"controlled_transport": true, "native_calls": 0, "trace_sha256": seal, "expected_task_success": tc.success, "actual_task_success": result.TaskSuccess, "mutation_controls_rejected": attacks, "projection_error": fmt.Sprint(projectErr)}} {
						data, _ := json.MarshalIndent(value, "", "  ")
						if err = os.WriteFile(filepath.Join(dst, name), data, 0600); err != nil {
							t.Fatal(err)
						}
					}
				}
			}()

			if projectErr != nil {
				t.Fatal("trace projection", projectErr)
			}
			if result.TaskSuccess != tc.success {
				t.Fatalf("score=%+v project=%v run=%v", result, projectErr, out.Err)
			}
			if tc.success && out.Err != nil {
				t.Fatal(out.Err)
			}
			if tc.mode == "rejected" {
				if out.Err != nil || out.Result == nil || out.Result.ReceiptStatus != "blocked" || out.Result.ErrorCode != "effect_unknown" || observation.Completed || observation.Receipt.Effect != "unknown" {
					t.Fatal("generic rejection lost its explicit blocked/unknown state", out.Err, out.Result, observation)
				}
				var n int
				if err = pool.QueryRow(ctx, `SELECT count(*) FROM action_effects WHERE state='effect_unknown'`).Scan(&n); err != nil || n != 1 {
					t.Fatal("original unknown lost", err, n)
				}
			}
			if tc.success {
				for _, table := range []string{"calls", "intents", "effects", "dispatches", "evidence", "finals", "inputs"} {
					mutated := cloneUtilityTrace(t, snapshot)
					mutated.Tables[table] = json.RawMessage(`[]`)
					obs, e := utilitytrace.Project(suite, binding, mutated, acceptance.MemoryDigest(mutated))
					if e == nil && acceptance.EvaluateMemoryUtility(suite, obs).TaskSuccess {
						t.Fatal("omitted evidence passed", table)
					}
					attacks++
				}
				mutated := cloneUtilityTrace(t, snapshot)
				var calls []map[string]any
				_ = json.Unmarshal(mutated.Tables["calls"], &calls)
				calls[0]["run_id"] = "foreign-run"
				mutated.Tables["calls"], _ = json.Marshal(calls)
				if _, e := utilitytrace.Project(suite, binding, mutated, acceptance.MemoryDigest(mutated)); e == nil {
					t.Fatal("foreign run accepted")
				}
				attacks++
				mutated = cloneUtilityTrace(t, snapshot)
				_ = json.Unmarshal(mutated.Tables["calls"], &calls)
				calls[0]["result"] = map[string]any{"status": "completed", "invented": true}
				mutated.Tables["calls"], _ = json.Marshal(calls)
				if _, e := utilitytrace.Project(suite, binding, mutated, acceptance.MemoryDigest(mutated)); e == nil {
					t.Fatal("altered result accepted")
				}
				attacks++
				mutated = cloneUtilityTrace(t, snapshot)
				mutated.RecipientFiles["alpha"] = "changed"
				obs, e := utilitytrace.Project(suite, binding, mutated, acceptance.MemoryDigest(mutated))
				if e == nil && acceptance.EvaluateMemoryUtility(suite, obs).TaskSuccess {
					t.Fatal("mutated original passed")
				}
				attacks++
				for _, field := range []string{"args", "result"} {
					mutated := cloneUtilityTrace(t, snapshot)
					var values []map[string]any
					_ = json.Unmarshal(mutated.Tables["calls"], &values)
					values[0][field] = map[string]any{"substituted": true}
					mutated.Tables["calls"], _ = json.Marshal(values)
					if _, e := utilitytrace.Project(suite, binding, mutated, acceptance.MemoryDigest(mutated)); e == nil {
						t.Fatal("substituted call accepted", field)
					}
					attacks++
				}
				if tc.mode == "memory" {
					for _, table := range []string{"pins", "source_heads"} {
						mutated := cloneUtilityTrace(t, snapshot)
						mutated.Tables[table] = json.RawMessage(`[]`)
						if _, e := utilitytrace.Project(suite, binding, mutated, acceptance.MemoryDigest(mutated)); e == nil {
							t.Fatal("unproven source accepted", table)
						}
						attacks++
					}
					bad := binding
					bad.Sources = append(append([]utilitytrace.SourceBinding{}, binding.Sources...), binding.Sources[0])
					bad.Sources[1].Pin.ID = "other-native-source"
					if _, e := utilitytrace.Project(suite, bad, snapshot, seal); e == nil {
						t.Fatal("ambiguous source map accepted")
					}
					attacks++
				}
			}

			if tc.name == "memory_read" {
				owner := experienceNativeActor(ctx, "utility-real-source-currentness")
				source := c.Preparation[0]
				updated, err := cat.Put(owner, catalog.PutSource{ExternalKey: source.Source.ID, Kind: "document", ScopeType: "project", ScopeID: "utility-prior-alpha", Title: "идентификатор комплекта alpha", Body: "Superseded: prior fact no longer current"})
				if err != nil {
					t.Fatal(err)
				}
				for _, kind := range []string{"stale", "revoked"} {
					if kind == "revoked" {
						if _, err := cat.Delete(owner, updated.ID, "revoke", updated.Revision); err != nil {
							t.Fatal(err)
						}
					}
					fresh, err := utilitytrace.Capture(ctx, pool, cat, binding, run, recipient.snapshot().Config.Files)
					if err != nil {
						t.Fatal(err)
					}
					obs, e := utilitytrace.Project(suite, binding, fresh, acceptance.MemoryDigest(fresh))
					if e == nil && acceptance.EvaluateMemoryUtility(suite, obs).TaskSuccess {
						t.Fatal("actual currentness mutation passed", kind)
					}
					attacks++
				}
			}
		})
	}
}
func cloneUtilityTrace(t *testing.T, s utilitytrace.Snapshot) utilitytrace.Snapshot {
	t.Helper()
	b, e := json.Marshal(s)
	if e != nil {
		t.Fatal(e)
	}
	var out utilitytrace.Snapshot
	if json.Unmarshal(b, &out) != nil {
		t.Fatal("copy")
	}
	return out
}
