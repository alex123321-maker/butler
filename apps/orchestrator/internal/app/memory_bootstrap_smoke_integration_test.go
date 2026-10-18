package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	"github.com/butler/butler/internal/memory/candidatecontrol"
	"github.com/butler/butler/internal/memory/candidates"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/compaction"
	"github.com/butler/butler/internal/memory/consolidation"
	memorydoctor "github.com/butler/butler/internal/memory/doctor"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/factcontrol"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/transport/antigravity"
	"google.golang.org/grpc"
)

// This tests the actual composition root and HTTP Doctor from its own objects.
// The existing labeled Doctor runtime is a boundary control, not host health or
// native answer quality. No provider is replaced and no inference is admitted.
func TestMemoryRealBootstrapDoctorIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEST_BOOTSTRAP_SMOKE") != "1" {
		t.Skip("explicit isolated PG/Redis/compiled-Broker bootstrap smoke required")
	}
	binary := os.Getenv("BUTLER_TEST_DOCTOR_BROKER_BINARY")
	redisURL := os.Getenv("BUTLER_TEST_MEMORY_REDIS_URL")
	redisTarget, err := url.Parse(redisURL)
	if binary == "" || err != nil || redisTarget.Scheme != "redis" || redisTarget.Host != "redis:6379" || redisTarget.Path != "/0" || redisTarget.User != nil {
		t.Fatal("owned compiled Broker and separate Redis fixture required")
	}
	pool := memorytest.Database(t)
	dsn, err := url.Parse(os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
	if err != nil {
		t.Fatal("private test DSN invalid")
	}
	query := dsn.Query()
	query.Set("search_path", pool.Config().ConnConfig.RuntimeParams["search_path"])
	dsn.RawQuery = query.Encode()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err = pool.Exec(ctx, `UPDATE memory_model_control SET state='disabled' WHERE singleton`); err != nil {
		t.Fatal("worker control unavailable")
	}
	private := t.TempDir()
	secret := func() string {
		var raw [32]byte
		if _, e := rand.Read(raw[:]); e != nil {
			t.Fatal("local fixture identity unavailable")
		}
		return hex.EncodeToString(raw[:])
	}
	ownerToken, serviceToken := secret(), secret()
	ownerFile, serviceFile := filepath.Join(private, "owner"), filepath.Join(private, "service")
	for path, value := range map[string]string{ownerFile: ownerToken, serviceFile: serviceToken} {
		if err = os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal("private fixture file unavailable")
		}
	}
	anchor := filepath.Join(private, "deletions")
	if err = catalog.NewFileManifest(anchor).Initialize(); err != nil {
		t.Fatal("fresh deletion anchor unavailable")
	}
	policy := budget.DefaultPolicy()
	policy.OutputTokens, policy.ModelMillis = 32768, 300000
	policy.Models["antigravity:"+antigravity.FlashHighModel] = budget.ModelPrice{Basis: "subscription_no_overage"}
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"BUTLER_ENVIRONMENT": "test", "BUTLER_LOG_LEVEL": "error", "BUTLER_POSTGRES_URL": dsn.String(), "BUTLER_POSTGRES_MAX_CONNS": "6", "BUTLER_POSTGRES_MIN_CONNS": "1",
		"BUTLER_REDIS_URL": redisURL, "BUTLER_MODEL_PROVIDER": "antigravity", "BUTLER_ANTIGRAVITY_MODEL": antigravity.FlashHighModel, "BUTLER_OPENAI_TIMEOUT_SECONDS": "300",
		"BUTLER_OPENAI_API_KEY": "", "BUTLER_TELEGRAM_BOT_TOKEN": "", "BUTLER_EXTENSION_API_TOKENS": "", "BUTLER_OWNER_CONTROL_TOKEN": "", "BUTLER_OWNER_CONTROL_TOKEN_FILE": ownerFile, "BUTLER_OWNER_CONTROL_OWNER_ID": "bootstrap-owner",
		"BUTLER_ACTION_AUTHORITY_TOKEN_FILE": serviceFile, "BUTLER_MEMORY_DELETION_MANIFEST_FILE": anchor, config.SettingsEncryptionKeyEnv: secret(), "BUTLER_MEMORY_EXPERIENCE_ENVIRONMENT_HASH": secret(),
		"BUTLER_EXECUTION_BUDGET_POLICY_JSON": string(policyJSON), "BUTLER_MEMORY_PIPELINE_ENABLED": "false", "BUTLER_MEMORY_SEMANTIC_ENABLED": "false", "BUTLER_MEMORY_PDF_PARSER_ENABLED": "false",
		"BUTLER_MEMORY_ADAPTIVE_RETRIEVAL_ENABLED": "false", "BUTLER_MEMORY_READ_ENABLED": "true", "BUTLER_MEMORY_MODEL_QUOTE_PROBE_ENABLED": "false", "BUTLER_AUTONOMY_MODE": "mode_0",
		"BUTLER_DELEGATION_ENABLED": "false", "BUTLER_CONVERTER_DELEGATION_ENABLED": "false", "BUTLER_WORKSPACE_ENABLED": "false", "BUTLER_GUARDED_TAB_OWNER_ENABLED": "false", "BUTLER_GUARDED_TAB_REQUEST_ENABLED": "false", "BUTLER_GUARDED_TAB_RUNTIME_ENABLED": "false",
		"BUTLER_SKILL_CONTROL_URL": "", "BUTLER_EXTENSION_CONTROL_URL": "", "BUTLER_CONVERTER_CONTROL_URL": "",
	} {
		t.Setenv(key, value)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	runtime := &doctorObservedRuntime{}
	rpc := grpc.NewServer()
	runtimev1.RegisterToolRuntimeServiceServer(rpc, runtime)
	go rpc.Serve(listener)
	defer rpc.Stop()
	registryBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "configs", "tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Tools []map[string]any `json:"tools"`
	}
	if err = json.Unmarshal(registryBytes, &registry); err != nil {
		t.Fatal(err)
	}
	var only []map[string]any
	for _, tool := range registry.Tools {
		if tool["tool_name"] == "doctor.check_system" {
			tool["runtime_target"] = listener.Addr().String()
			only = append(only, tool)
		}
	}
	if len(only) != 1 {
		t.Fatal("canonical Doctor contract absent")
	}
	registryBytes, _ = json.Marshal(map[string]any{"tools": only})
	registryFile := filepath.Join(private, "tools.json")
	if err = os.WriteFile(registryFile, registryBytes, 0600); err != nil {
		t.Fatal(err)
	}

	type phase struct {
		name                       string
		enabled, jobs, unsupported bool
	}
	phases := []phase{{name: "default_off"}, {name: "experience_context_without_jobs", enabled: true}, {name: "selected_requested", enabled: true, jobs: true}, {name: "unsupported_profile", enabled: true, jobs: true, unsupported: true}, {name: "rollback_default_off"}}
	type sealedReport struct {
		id   int64
		hash [32]byte
	}
	var sealed []sealedReport
	checkSealed := func(t *testing.T) {
		for _, r := range sealed {
			var body string
			if e := pool.QueryRow(ctx, `SELECT report_json::text FROM doctor_reports WHERE id=$1`, r.id).Scan(&body); e != nil || sha256.Sum256([]byte(body)) != r.hash {
				t.Fatal("previous completed Doctor receipt changed")
			}
		}
	}
	var evidence []map[string]any
	for _, phase := range phases {
		t.Run(phase.name, func(t *testing.T) {
			checkSealed(t)
			t.Setenv("BUTLER_MEMORY_EXPERIENCE_ENABLED", strconv.FormatBool(phase.enabled))
			t.Setenv("BUTLER_MEMORY_MODEL_JOBS_ENABLED", strconv.FormatBool(phase.jobs))
			t.Setenv("BUTLER_MEMORY_MODEL_CANDIDATES_ENABLED", strconv.FormatBool(phase.jobs))
			t.Setenv("BUTLER_MEMORY_SUMMARY_CONSUMER_ENABLED", strconv.FormatBool(phase.jobs))
			t.Setenv("BUTLER_MEMORY_CONSOLIDATION_CONSUMER_ENABLED", strconv.FormatBool(phase.jobs))
			t.Setenv("BUTLER_MEMORY_TEMPORAL_ENABLED", strconv.FormatBool(phase.jobs))
			t.Setenv("BUTLER_MEMORY_CANDIDATE_ENABLED", strconv.FormatBool(phase.jobs))
			output := "32768"
			if phase.unsupported {
				output = "8192"
			}
			t.Setenv("BUTLER_ANTIGRAVITY_MAXIMUM_OUTPUT_TOKENS", output)
			httpAddr, brokerAddr := doctorFreeAddress(t), doctorFreeAddress(t)
			t.Setenv("BUTLER_HTTP_ADDR", httpAddr)
			t.Setenv("BUTLER_GRPC_ADDR", "127.0.0.1:0")
			t.Setenv("BUTLER_TOOL_BROKER_ADDR", brokerAddr)
			application, e := New(ctx)
			row := map[string]any{"name": phase.name, "feature_enabled": phase.enabled, "model_jobs_requested": phase.jobs, "profile_supported": !phase.unsupported, "prior_receipts_unchanged": true}
			defer func() { row["pass"] = !t.Failed(); evidence = append(evidence, row) }()
			if phase.enabled && (!experience.ReleaseQualified || phase.unsupported) {
				if application != nil || !errors.Is(e, experience.ErrEvidence) {
					t.Fatal("explicit closed/unsupported startup did not return typed evidence denial")
				}
				row["outcome"] = "typed_evidence_denial"
				checkSealed(t)
				return
			}
			if e != nil || application == nil {
				t.Fatal("actual application construction failed")
			}
			stopped := false
			defer func() {
				if !stopped {
					_ = application.shutdown(nil)
				}
			}()
			if application.pipelineWorker != nil || application.memoryIndexing != nil || (application.memoryModelJobs != nil) != phase.jobs {
				t.Fatal("actual worker construction differs from requested static profile")
			}
			if phase.jobs {
				for handler, want := range map[string]bool{candidates.Version: memoryCandidateReleaseQualified, compaction.HandlerName: compaction.ReleaseQualified, consolidation.Version: consolidation.ReleaseQualified, experience.Version: experience.ReleaseQualified} {
					if modelHandlerConfigured(application.memoryModelJobs, handler) != want {
						t.Fatal("actual bootstrap handler registry differs")
					}
				}
			}
			cmd := exec.CommandContext(ctx, binary)
			cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
			cmd.Env = append(os.Environ(), "BUTLER_HTTP_ADDR="+doctorFreeAddress(t), "BUTLER_GRPC_ADDR="+brokerAddr, "BUTLER_TOOL_REGISTRY_PATH="+registryFile, "BUTLER_ACTION_CONTEXT_URL=http://"+httpAddr+"/internal/action-context", "BUTLER_MEMORY_READ_CONTROL_URL=http://"+httpAddr+readcontrol.Path, "BUTLER_MEMORY_CANDIDATE_CONTROL_URL=http://"+httpAddr+candidatecontrol.Path, "BUTLER_MEMORY_FACT_CONTROL_URL=http://"+httpAddr+factcontrol.Path)
			if e = cmd.Start(); e != nil {
				t.Fatal("owned Broker process unavailable")
			}
			defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
			waitAddress := func(addr string) {
				for until := time.Now().Add(10 * time.Second); ; {
					conn, e := net.DialTimeout("tcp", addr, 50*time.Millisecond)
					if e == nil {
						_ = conn.Close()
						return
					}
					if time.Now().After(until) {
						t.Fatal("owned service listener unavailable")
					}
					time.Sleep(20 * time.Millisecond)
				}
			}
			waitAddress(brokerAddr)
			runCtx, stop := context.WithCancel(ctx)
			done := make(chan error, 1)
			go func() { done <- application.Run(runCtx) }()
			finish := func() {
				stop()
				select {
				case e := <-done:
					stopped = true
					if e != nil {
						t.Error("actual application did not stop cleanly")
					}
				case <-time.After(10 * time.Second):
					t.Error("actual application shutdown timed out")
				}
			}
			defer func() {
				if !stopped {
					finish()
				}
			}()
			waitAddress(httpAddr)
			before := runtime.calls.Load()
			client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second}
			defer client.CloseIdleConnections()
			var inputID string
			var reportID int64
			var memory json.RawMessage
			for until := time.Now().Add(20 * time.Second); ; {
				body := map[string]string{}
				if inputID != "" {
					body["input_id"] = inputID
				}
				raw, _ := json.Marshal(body)
				request, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+httpAddr+"/api/v1/doctor/check", bytes.NewReader(raw))
				request.Header.Set("Authorization", "Bearer "+ownerToken)
				request.Header.Set("Content-Type", "application/json")
				response, e := client.Do(request)
				if e != nil {
					t.Fatal("actual Doctor HTTP unavailable")
				}
				var dto struct {
					ID     int64 `json:"id"`
					Report struct {
						InputID   string          `json:"input_id"`
						Memory    json.RawMessage `json:"memory"`
						Execution struct {
							Status string `json:"status"`
						} `json:"execution"`
					} `json:"report"`
				}
				e = json.NewDecoder(io.LimitReader(response.Body, 256*1024)).Decode(&dto)
				_ = response.Body.Close()
				if e != nil || (response.StatusCode != 200 && response.StatusCode != 202) || dto.ID == 0 {
					t.Fatal("actual Doctor did not retain its report")
				}
				if inputID == "" {
					inputID = dto.Report.InputID
				}
				if inputID == "" || inputID != dto.Report.InputID {
					t.Fatal("Doctor owning input identity changed")
				}
				if dto.Report.Execution.Status == "completed" {
					reportID, memory = dto.ID, dto.Report.Memory
					break
				}
				if dto.Report.Execution.Status != "queued" && dto.Report.Execution.Status != "running" {
					var code, checkpoint, run string
					var attempted []string
					if e = pool.QueryRow(ctx, `SELECT error_code,checkpoint,run_id,ARRAY(SELECT key FROM jsonb_each(COALESCE(checkpoint_json->'action_slots','{}'::jsonb))) FROM accepted_inputs WHERE input_id=$1`, inputID).Scan(&code, &checkpoint, &run, &attempted); e != nil {
						t.Fatal("diagnostic checkpoint metadata unavailable")
					}
					row["outcome"], row["doctor_terminal_status"], row["doctor_error_code"], row["doctor_checkpoint"], row["attempted_tool_refs"], row["controlled_doctor_calls"] = "unexpected_terminal", dto.Report.Execution.Status, code, checkpoint, attempted, runtime.calls.Load()-before
					var collision bool
					if e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tool_calls WHERE tool_name='doctor.check_system' AND run_id<>$1 AND tool_call_id=ANY($2::text[]))`, run, attempted).Scan(&collision); e != nil {
						t.Fatal("diagnostic reference metadata unavailable")
					}
					row["cross_run_tool_ref_collision"] = collision
					t.Fatal("Doctor command did not complete through existing read-only mode_0 path")
				}
				if time.Now().After(until) {
					t.Fatal("actual Doctor checkpoint deadline exceeded")
				}
				time.Sleep(30 * time.Millisecond)
			}
			if runtime.calls.Load()-before != 1 {
				t.Fatal("controlled Doctor runtime was not called exactly once")
			}
			var observed struct {
				AnswerQuality struct {
					Status string `json:"status"`
				} `json:"answer_quality"`
				Effective struct {
					Features []struct {
						Configuration memorydoctor.FeatureConfiguration `json:"configuration"`
						State         string                            `json:"state"`
					} `json:"features"`
					Policies struct {
						Status      string `json:"status"`
						Observation struct {
							WorkerControl string `json:"worker_control"`
						} `json:"observation"`
					} `json:"owner_policies"`
				} `json:"effective_features"`
			}
			if json.Unmarshal(memory, &observed) != nil || observed.AnswerQuality.Status != "unverified" || observed.Effective.Policies.Status != "observed" || observed.Effective.Policies.Observation.WorkerControl != "disabled" {
				t.Fatal("same-object memory Doctor lost unknown quality or current disabled control")
			}
			states := map[string]memorydoctor.FeatureConfiguration{}
			for _, f := range observed.Effective.Features {
				states[f.Configuration.Name] = f.Configuration
				if !f.Configuration.Requested && f.State != "disabled" {
					t.Fatal("disabled feature appeared active")
				}
			}
			for name, want := range map[string]bool{"source_reads": true, "candidate_proposals": phase.jobs, "adaptive_retrieval": false, "temporal_facts": phase.jobs && temporalReleaseQualified, "candidate_jobs": phase.jobs && memoryCandidateReleaseQualified, "task_compaction": phase.jobs && compaction.ReleaseQualified, "consolidation": phase.jobs && consolidation.ReleaseQualified, "experience": phase.jobs && experience.ReleaseQualified, "experience_context": phase.enabled && experience.ReleaseQualified} {
				f, ok := states[name]
				if !ok || f.Wired != want || !f.ProfileReady {
					t.Fatal("same-object Doctor differs from actual bootstrap service", name)
				}
			}
			for name, qualified := range map[string]bool{"source_reads": true, "candidate_proposals": true, "adaptive_retrieval": adaptiveRetrievalQualified, "temporal_facts": temporalReleaseQualified, "candidate_jobs": memoryCandidateReleaseQualified, "task_compaction": compaction.ReleaseQualified, "consolidation": consolidation.ReleaseQualified, "experience": experience.ReleaseQualified, "experience_context": experience.ReleaseQualified} {
				requested := phase.jobs
				if name == "source_reads" {
					requested = true
				}
				if name == "adaptive_retrieval" {
					requested = false
				}
				if name == "experience_context" {
					requested = phase.enabled
				}
				if states[name].Qualified != qualified || states[name].Requested != requested {
					t.Fatal("same-object Doctor changed qualification or requested flag", name)
				}
			}
			if states["experience_context"].Requested != phase.enabled || states["experience"].Requested != (phase.enabled && phase.jobs) || states["temporal_facts"].Requested != phase.jobs {
				t.Fatal("Doctor merged independent context and generation flags")
			}
			finish()
			if !stopped {
				t.Fatal("application still running")
			}
			checkSealed(t)
			var stored string
			if e = pool.QueryRow(ctx, `SELECT report_json::text FROM doctor_reports WHERE id=$1`, reportID).Scan(&stored); e != nil {
				t.Fatal("completed Doctor receipt absent")
			}
			sealed = append(sealed, sealedReport{reportID, sha256.Sum256([]byte(stored))})
			row["outcome"], row["memory"], row["controlled_doctor_calls"], row["clean_shutdown"] = "constructed_and_served", memory, 1, true
		})
	}
	checkSealed(t)
	var foreground, background, settings int
	var control string
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_model_attempts),(SELECT count(*) FROM memory_model_attempts),(SELECT count(*) FROM system_settings),(SELECT state FROM memory_model_control WHERE singleton)`).Scan(&foreground, &background, &settings, &control); err != nil || foreground != 0 || background != 0 || settings != 0 || control != "disabled" {
		t.Fatal("bootstrap smoke created model entries, auth settings or enabled worker control")
	}
	proof := os.Getenv("BUTLER_MEMORY_BOOTSTRAP_PROOF")
	if !filepath.IsAbs(proof) {
		t.Fatal("fresh absolute metadata proof required")
	}
	raw, err := json.MarshalIndent(map[string]any{"version": "memory-real-bootstrap-smoke-v1", "pass": !t.Failed(), "native_calls": 0, "auth_imported": false, "foreground_model_attempts": foreground, "background_model_attempts": background, "system_settings_rows": settings, "worker_control": control, "controlled_diagnostic_runtime": true, "retained_completed_reports": len(sealed), "experience_qualified": experience.ReleaseQualified, "temporal_qualified": temporalReleaseQualified, "consolidation_qualified": consolidation.ReleaseQualified, "cases": evidence}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(proof, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("fresh metadata proof unavailable")
	}
	defer file.Close()
	if _, err = file.Write(raw); err != nil {
		t.Fatal("metadata proof write failed")
	}
}
