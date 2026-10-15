package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/semantic"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

func hashMemoryBytes(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

func (c *memoryEvalCollector) reserveCall(ctx context.Context, units int64) error {
	tag, err := c.pool.Exec(ctx, `UPDATE fixture_memory_eval_window SET calls=calls+1,reserved_units=reserved_units+$1 WHERE singleton AND NOT closed AND calls<max_calls AND reserved_units+$1<=4000000 AND expires_at>clock_timestamp()`, units)
	if err != nil || tag.RowsAffected() != 1 {
		return errors.New("immutable memory window exhausted or closed")
	}
	return nil
}

func (c *memoryEvalCollector) sample(ctx context.Context, cse acceptance.MemoryCase, i acceptance.MemoryInput) (o acceptance.MemoryObservation, resultErr error) {
	defer func() {
		if resultErr != nil && o.Status == "completed" {
			o.Status = "failed"
			o.Reason = "post_inference_retention_or_authority_failed"
		}
	}()
	falseValue := false
	o = acceptance.MemoryObservation{CaseID: i.CaseID, Arm: i.Arm, Model: c.plan.Model, ModelVersion: "", Runtime: c.plan.Runtime, InputSHA256: acceptance.MemoryInputDigest(i), Status: "failed", Reason: "not_admitted", Output: json.RawMessage("null"), Outcome: acceptance.MemoryOutcome{EvidenceSHA256: acceptance.MemoryDigest([]string{i.CaseID, i.Arm, "no_external_tool_definitions"}), PermissionViolation: &falseValue, PrivacyViolation: &falseValue, StopViolation: &falseValue, DeletionViolation: &falseValue}}
	if i.SkipReason != "" {
		o.Status = "skipped"
		o.Reason = i.SkipReason
		return o, nil
	}
	started := time.Now()
	live, run, err := c.ownedRun(ctx, cse, i.Arm)
	if err != nil {
		return o, err
	}
	if err = c.checkSources(live, i); err != nil {
		return o, err
	}
	user, err := acceptance.MemoryModelProjection(i)
	if err != nil {
		return o, err
	}
	zeroOverhead := int64(0)
	if err = c.recordExpense(live, acceptance.MemoryExpense{ID: i.CaseID + "/" + i.Arm + "/kernel-overhead", CaseID: i.CaseID, Arm: i.Arm, Stage: "background", Status: "unknown", Outcome: "success", EvidenceSHA256: acceptance.MemoryDigest([]string{o.InputSHA256, "session_task_guard_and_ledger_overhead_not_separately_measured"}), InputTokens: &zeroOverhead, OutputTokens: &zeroOverhead, TotalTokens: &zeroOverhead, Microunits: &zeroOverhead}); err != nil {
		return o, err
	}
	// Each arm actually reads/checks its allowed context and constructs input.
	readMS := time.Since(started).Milliseconds()
	zeroRead := int64(0)
	if err = c.recordExpense(live, acceptance.MemoryExpense{ID: i.CaseID + "/" + i.Arm + "/model-input-read", CaseID: i.CaseID, Arm: i.Arm, Stage: "read", Status: "measured", Outcome: "success", EvidenceSHA256: hashMemoryBytes(user), Calls: 1, InputTokens: &zeroRead, OutputTokens: &zeroRead, TotalTokens: &zeroRead, Milliseconds: &readMS, Microunits: &zeroRead, StorageBytes: &zeroRead}); err != nil {
		return o, err
	}
	inputWriteStarted := time.Now()
	inputBytesBefore := c.retainedBytes
	if err = c.writeEvidence("input-"+strings.TrimPrefix(i.CaseID, "memory:")+"-"+i.Arm+".json", struct {
		Input        string
		Instructions string
		Schema       json.RawMessage
	}{string(user), i.Instructions, acceptance.MemoryNativeSchema(i)}); err != nil {
		return o, err
	}
	inputWriteMS := time.Since(inputWriteStarted).Milliseconds()
	inputWritten := c.retainedBytes - inputBytesBefore
	if err = c.recordExpense(live, acceptance.MemoryExpense{ID: i.CaseID + "/" + i.Arm + "/input-write", CaseID: i.CaseID, Arm: i.Arm, Stage: "write", Status: "measured", Outcome: "success", EvidenceSHA256: hashMemoryBytes(user), InputTokens: &zeroRead, OutputTokens: &zeroRead, TotalTokens: &zeroRead, Milliseconds: &inputWriteMS, Microunits: &zeroRead, StorageBytes: &inputWritten}); err != nil {
		return o, err
	}
	// Double-encoded user data/schema plus8KiB framing margin, clamped by the
	// same complete HTTP payload cap enforced at the provider boundary.
	inputUnits := acceptance.MemoryNativeReservedUnits(i) - 32768
	if err = c.reserveCall(live, inputUnits+32768); err != nil {
		return o, err
	}
	_, err = c.pool.Exec(live, `INSERT INTO fixture_memory_eval_attempts(case_id,arm,run_id,input_hash) VALUES($1,$2,$3,$4)`, i.CaseID, i.Arm, run, o.InputSHA256)
	if err != nil {
		return o, err
	}
	admission, err := c.budgets.AdmitModel(live, run, "", antigravity.Name, antigravity.FlashHighModel, inputUnits)
	if err != nil {
		return o, err
	}
	defer func() {
		retain, cancel := context.WithTimeout(context.WithoutCancel(live), 10*time.Second)
		defer cancel()
		var count int
		if c.pool.QueryRow(retain, `SELECT count(*) FROM fixture_memory_eval_expenses WHERE id=$1`, admission.OperationID).Scan(&count) == nil && count == 0 {
			zero := int64(0)
			elapsed := time.Since(started).Milliseconds()
			o.Status = "failed"
			o.Reason = "admitted_without_complete_observation"
			o.Milliseconds = &elapsed
			_ = c.recordExpense(retain, acceptance.MemoryExpense{ID: admission.OperationID, CaseID: i.CaseID, Arm: i.Arm, Stage: "generation", Status: "unknown", Outcome: "failed", Calls: 1, EvidenceSHA256: acceptance.MemoryDigest([]string{admission.OperationID, "unknown-retained-prefix"}), Milliseconds: &elapsed, StorageBytes: &zero})
		}
	}()
	if _, err = c.pool.Exec(live, `UPDATE fixture_memory_eval_attempts SET operation_id=$3 WHERE case_id=$1 AND arm=$2`, i.CaseID, i.Arm, admission.OperationID); err != nil {
		return o, err
	}
	provider, err := c.factory(acceptance.MemoryNativeSchema(i))
	if err != nil {
		return o, err
	}
	entry, stop := context.WithDeadline(live, admission.Deadline)
	defer stop()
	entry, stopGuard := context.WithCancel(entry)
	defer stopGuard()
	check := func(call context.Context) error {
		bounded, cancel := context.WithTimeout(call, 200*time.Millisecond)
		defer cancel()
		return c.checkSources(bounded, i)
	}
	entry = budget.WithModelOperation(entry, admission.OperationID)
	entry = transport.WithInferenceAuthority(entry, func(call context.Context) error {
		bounded, cancel := context.WithTimeout(call, 200*time.Millisecond)
		defer cancel()
		return c.checkSourcesPhase(bounded, i, "dispatch")
	})
	entry = transport.WithInputByteLimit(entry, inputUnits)
	if err = check(entry); err != nil {
		return o, err
	}
	var authDiagnostics []providerauth.AntigravityDiagnostic
	entry = providerauth.WithAntigravityDiagnostics(entry, func(d providerauth.AntigravityDiagnostic) { authDiagnostics = append(authDiagnostics, d) })
	done := make(chan struct{})
	defer func() { stopGuard(); <-done }()
	go func() {
		defer close(done)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-entry.Done():
				return
			case <-ticker.C:
				if check(entry) != nil {
					stopGuard()
					return
				}
			}
		}
	}()
	req := transport.StartRunRequest{Context: transport.TransportRunContext{RunID: run, SessionKey: run + ":session", ProviderName: antigravity.Name, ModelName: antigravity.FlashHighModel, SupportsStreaming: true}, StreamingEnabled: true, InputItems: []transport.InputItem{{Role: "system", Content: i.Instructions}, {Role: "user", Content: string(user)}}}
	var usage *transport.Usage
	var original []byte
	var events []memoryEvalEvent
	version := ""
	finals, terminals, tools := 0, 0, 0
	terminal := ""
	clean := true
	stream, startErr := provider.StartRun(entry, req)
	if startErr != nil {
		clean = false
	} else {
		for event := range stream {
			if len(events) >= 4096 {
				clean = false
				stopGuard()
				continue
			}
			events = append(events, memoryEvalTrace(event))
			if event.RunID != run || event.ProviderName != antigravity.Name || event.EventID == "" || event.Timestamp.IsZero() || terminals > 0 {
				clean = false
			}
			if event.Usage != nil {
				if usage != nil && *usage != *event.Usage {
					clean = false
				}
				copy := *event.Usage
				usage = &copy
			}
			if event.ProviderModelVersion != nil {
				if version != "" && version != *event.ProviderModelVersion {
					clean = false
				}
				version = *event.ProviderModelVersion
			}
			if event.AssistantFinal != nil {
				if event.AssistantFinal.FinishReason != "STOP" {
					clean = false
				}
				finals++
				original = []byte(event.AssistantFinal.Content)
				if event.AssistantFinal.Usage != nil {
					if usage != nil && *usage != *event.AssistantFinal.Usage {
						clean = false
					}
					copy := *event.AssistantFinal.Usage
					usage = &copy
				}
			}
			if event.ToolCall != nil || event.ToolCallBatch != nil {
				tools++
				clean = false
			}
			switch event.EventType {
			case transport.EventTypeTransportError:
				clean = false
			case transport.EventTypeRunCompleted, transport.EventTypeRunCancelled, transport.EventTypeRunTimedOut:
				terminals++
				terminal = string(event.EventType)
			}
		}
	}
	elapsed := time.Since(started).Milliseconds()
	o.Output, _ = json.Marshal(string(original))
	o.OutputEncoding = "raw_utf8_json_string"
	o.Milliseconds = &elapsed
	pipelineMS := elapsed
	if i.Arm == "retrieval" {
		pipelineMS += c.captureMilliseconds[i.CaseID]
	}
	o.PipelineMilliseconds = &pipelineMS
	if c.prior != nil && i.Arm == "retrieval" {
		o.PipelineMilliseconds = nil
	}
	o.ModelVersion = version
	var input, output, total *int64
	if usage != nil {
		if usage.Validate() != nil {
			clean = false
		}
		if int64(usage.InputTokens) > inputUnits || int64(usage.OutputTokens) > 32768 || int64(usage.TotalTokens) > inputUnits+32768 {
			clean = false
		}
		in, out, tot := int64(usage.InputTokens), int64(usage.OutputTokens), int64(usage.TotalTokens)
		input = &in
		output = &out
		if tot > 0 || in == 0 && out == 0 {
			total = &tot
		}
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(live), 10*time.Second)
	defer cancel()
	if err = c.budgets.ObserveModel(finishCtx, admission.OperationID, input, output); err != nil {
		return o, err
	}
	status := "transport_error"
	if clean && finals == 1 && terminals == 1 && terminal == string(transport.EventTypeRunCompleted) && usage != nil && version == c.plan.ModelVersion && len(original) <= 8192 {
		status = "completed"
		o.Status = "completed"
		o.Reason = ""
	} else {
		o.Reason = "transport_lifecycle_or_metadata_incomplete"
	}
	outputHash := hashMemoryBytes(original)
	_, err = c.pool.Exec(finishCtx, `UPDATE fixture_memory_eval_attempts SET output_hash=$3,outcome=$4,input_tokens=$5,output_tokens=$6,total_tokens=$7,model_version=$8,milliseconds=$9 WHERE case_id=$1 AND arm=$2 AND outcome='reserved'`, i.CaseID, i.Arm, outputHash, status, input, output, total, version, elapsed)
	if err != nil {
		return o, err
	}
	zero := int64(0)
	expenseStatus := "unknown"
	outcome := "failed"
	if status == "completed" {
		outcome = "success"
	}
	// Subscription incremental invoice is unobserved. A zero admission quote
	// cannot become a measured zero invoice. All actual token dimensions remain.
	if err = c.recordExpense(finishCtx, acceptance.MemoryExpense{ID: admission.OperationID, CaseID: i.CaseID, Arm: i.Arm, Stage: "generation", Status: expenseStatus, Outcome: outcome, EvidenceSHA256: acceptance.MemoryDigest([]string{admission.OperationID, outputHash}), Calls: 1, InputTokens: input, OutputTokens: output, TotalTokens: total, Milliseconds: &elapsed, Microunits: nil, StorageBytes: &zero}); err != nil {
		return o, err
	}
	// Owning durable spend precedes exclusive private original retention.
	saveStarted := time.Now()
	bytesBefore := c.retainedBytes
	originalWriteFailed := true
	defer func() {
		retain, cancel := context.WithTimeout(context.WithoutCancel(live), 10*time.Second)
		defer cancel()
		zero := int64(0)
		ms := time.Since(saveStarted).Milliseconds()
		bytes := c.retainedBytes - bytesBefore
		outcome := "success"
		if originalWriteFailed {
			outcome = "failed"
		}
		if recordErr := c.recordExpense(retain, acceptance.MemoryExpense{ID: i.CaseID + "/" + i.Arm + "/original-write", CaseID: i.CaseID, Arm: i.Arm, Stage: "write", Status: "measured", Outcome: outcome, EvidenceSHA256: acceptance.MemoryDigest([]string{admission.OperationID, outputHash, "private_artifact_bytes"}), InputTokens: &zero, OutputTokens: &zero, TotalTokens: &zero, Milliseconds: &ms, Microunits: &zero, StorageBytes: &bytes}); recordErr != nil {
			resultErr = recordErr
		}
	}()
	if err = c.writeOriginal("final-"+strings.TrimPrefix(i.CaseID, "memory:")+"-"+i.Arm+".txt", original); err != nil {
		return o, err
	}
	if err = c.writeEvidence("events-"+strings.TrimPrefix(i.CaseID, "memory:")+"-"+i.Arm+".json", events); err != nil {
		return o, err
	}
	if c.renewed {
		authName := "auth-" + strings.TrimPrefix(i.CaseID, "memory:") + "-" + i.Arm + ".json"
		trace := memoryNativeAuthTrace{RunID: run, InputSHA: o.InputSHA256, Events: authDiagnostics}
		if err = c.writeEvidence(authName, trace); err != nil {
			return o, err
		}
		sha, hashErr := acceptance.FileHash(filepath.Join(c.private, authName))
		if hashErr != nil {
			return o, hashErr
		}
		if _, err = c.pool.Exec(finishCtx, `INSERT INTO fixture_memory_eval_auth VALUES($1,$2,$3)`, run, o.InputSHA256, sha); err != nil {
			return o, err
		}
	}
	originalWriteFailed = false
	finishCtx = budget.WithModelOperation(finishCtx, admission.OperationID)
	if err = c.checkSourcesPhase(finishCtx, i, "final"); err != nil {
		o.Status = "failed"
		o.Reason = "current_source_task_or_lease_changed"
		return o, errors.New("source authority changed during model run")
	}
	violation := tools > 0
	o.Outcome.PermissionViolation = &violation
	if status == "completed" {
		var answer acceptance.MemoryAnswer
		parseErr := json.Unmarshal(original, &answer)
		assessment := acceptance.AssessMemoryV2(cse, i, answer, parseErr == nil)
		// Narrow synthetic QUESTION result, independently persisted and checked.
		// The enclosing long-lived task remains active; no external effect or
		// completion of that broader goal is claimed from next_action prose.
		result := struct {
			InputSHA, OutputSHA, Scope string
			Assessment                 acceptance.MemoryAssessment
			BroaderGoalCompleted       *bool
		}{o.InputSHA256, outputHash, "synthetic_question_answer_and_current_instruction", assessment, nil}
		if err = c.writeEvidence("result-"+strings.TrimPrefix(i.CaseID, "memory:")+"-"+i.Arm+".json", result); err != nil {
			originalWriteFailed = true
			return o, err
		}
		_, err = c.pool.Exec(finishCtx, `INSERT INTO fixture_memory_eval_results VALUES($1,$2,$3,$4,$5,'synthetic_question_answer_and_current_instruction',$6)`, i.CaseID, i.Arm, o.InputSHA256, outputHash, assessment, i.Task.Revision)
		if err != nil {
			return o, err
		}
		// This is an answer observation; no broader user task effect is fabricated.
		o.Outcome.Success = nil
		o.Outcome.EvidenceSHA256 = acceptance.MemoryDigest(result)
	}
	_, err = c.pool.Exec(finishCtx, `UPDATE runs SET current_state=$2,status=$2,finished_at=clock_timestamp() WHERE run_id=$1`, run, func() string {
		if status == "completed" {
			return "completed"
		}
		return "failed"
	}())
	if err != nil {
		return o, err
	}
	if status != "completed" {
		if c.renewed && !providerauth.UnsafeAntigravityRefresh(authDiagnostics) && memoryCleanAvailability(events, original, version, usage, startErr) {
			return o, errMemoryAvailability
		}
		return o, errors.New("model entry failed; fresh window sealed without retry")
	}
	return o, nil
}

func (c *memoryEvalCollector) checkSources(ctx context.Context, i acceptance.MemoryInput) error {
	return c.checkSourcesPhase(ctx, i, "")
}

func (c *memoryEvalCollector) checkSourcesPhase(ctx context.Context, i acceptance.MemoryInput, phase string) error {
	refs := []catalog.SourceRef{}
	for _, h := range i.Memory {
		ref, ok := c.sourceRefs[i.CaseID][h.ID]
		if !ok {
			return errors.New("source mapping missing")
		}
		refs = append(refs, ref)
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = c.catalog.AccessTx(ctx, tx, refs, memoryEvalPurpose); err != nil {
		return err
	}
	if phase != "" {
		tok, ok := ownership.TokenFromContext(ctx)
		operation, hasOp := budget.ModelOperationFromContext(ctx)
		if !ok || !hasOp {
			return errors.New("owning guard receipt requires admitted operation")
		}
		_, err = tx.Exec(ctx, `INSERT INTO fixture_memory_eval_guards(run_id,case_id,arm,operation_id,phase,checked_at,task_revision,constraints_revision,lease_id,lease_expires,sources) SELECT r.run_id,$2,$3,$4,$5,clock_timestamp(),r.task_revision,t.constraints_revision,o.lease_id,o.expires_at,$6 FROM runs r JOIN tasks t ON t.task_id=r.task_id JOIN session_ownership o ON o.run_id=r.run_id WHERE r.run_id=$1`, tok.RunID, i.CaseID, i.Arm, operation, phase, refs)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func memoryEvalDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return memoryEvalDatabaseURL(t, os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
}

func memoryEvalDatabaseURL(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("fresh memory eval test database required")
	}
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	t.Cleanup(pool.Close)
	var tables int
	if pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_tables WHERE schemaname='public'`).Scan(&tables) != nil || tables != 0 {
		t.Fatal("collector refuses an existing database")
	}
	paths, err := filepath.Glob(filepath.Join(shadowRepoRoot(), "migrations", "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(context.Background(), string(raw)); err != nil {
			t.Fatal("migration failed", filepath.Base(path))
		}
	}
	return pool
}

// Explicit fresh-root entry. Unit tests never authenticate or invoke a model.
func TestMemoryEvalActualCollection(t *testing.T) {
	if os.Getenv("BUTLER_MEMORY_EVAL_ACTUAL") != "1" {
		t.Skip("explicit reviewed fresh model window required")
	}
	private := os.Getenv("BUTLER_MEMORY_EVAL_PRIVATE")
	release := os.Getenv("BUTLER_MEMORY_EVAL_RELEASE")
	if !filepath.IsAbs(private) || len(release) != 40 {
		t.Fatal("frozen private root and release required")
	}
	file, err := os.Open(filepath.Join(shadowRepoRoot(), "testdata/acceptance/ru-memory-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	suite, err := acceptance.LoadMemorySuite(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	keyRaw, err := acceptance.ReadPrivateFile("/review/public-key.hex", 1024)
	if err != nil {
		t.Fatal("external review key unavailable")
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(keyRaw)))
	if err != nil {
		t.Fatal("review key invalid")
	}
	approvalRaw, err := acceptance.ReadPrivateFile("/review/approval.json", 1<<20)
	if err != nil {
		t.Fatal("independent review unavailable")
	}
	var approval acceptance.GoldenApproval
	if acceptance.DecodeStrict(approvalRaw, &approval, "digest", "criteria_sha256", "rules", "reviewer", "candidate_author", "signature") != nil || acceptance.VerifyMemoryGolden(suite, approval, key) != nil {
		t.Fatal("independent golden review denied")
	}
	hardware, err := acceptance.ReadPrivateFile("/metadata/hardware.json", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	sourceHash, err := acceptance.TreeHash(shadowRepoRoot(), nil)
	if err != nil {
		t.Fatal(err)
	}
	pinsRaw, err := acceptance.ReadPrivateFile("/metadata/pins.json", 1<<20)
	var pins acceptance.MemoryRuntimePins
	executable, exeErr := os.Executable()
	binarySHA, binErr := acceptance.FileHash(executable)
	if err != nil || json.Unmarshal(pinsRaw, &pins) != nil || exeErr != nil || binErr != nil || pins.WindowID == "" || pins.Release != release || pins.Source != sourceHash || pins.Binary != binarySHA || pins.Hardware != hashMemoryBytes(hardware) || pins.Suite != acceptance.MemorySuiteDigest(suite) || pins.Criteria != acceptance.MemoryVersionCriteriaDigest(suite) || pins.MaxCalls != 280 || pins.MaxUnits != 4000000 || pins.CPUs != 2 || pins.MemoryBytes != 1610612736 || pins.PIDs != 128 || pins.Seconds != 10800 || pins.MaxEvents != 4096 || pins.MaxReportBytes != 128<<20 {
		t.Fatal("operator-pinned source/binary/resources differ")
	}
	var execution acceptance.MemoryExecutionProof
	executionRaw, err := acceptance.ReadPrivateFile("/metadata/execution.json", 1<<20)
	var hardwareImage struct {
		GoImage string `json:"go_image"`
	}
	_ = json.Unmarshal(hardware, &hardwareImage)
	if err != nil || json.Unmarshal(executionRaw, &execution) != nil || acceptance.ValidateMemoryExecution(execution, pins, hardwareImage.GoImage) != nil {
		t.Fatal("actual container proof denied before credentials")
	}
	plan := acceptance.MemoryPlan{Version: acceptance.MemoryEvalV2, WindowID: pins.WindowID, EvidenceKind: "model_observation", CodeCommit: release, SourceSHA256: sourceHash, HardwareSHA256: hashMemoryBytes(hardware), Model: antigravity.FlashHighModel, ModelVersion: "gemini-3.8-flash-n", Runtime: "go1.27.2/current-budget+catalog+owned-index+hybrid+bundle/native-openapi", MemoryConfigSHA256: memoryEvalConfigDigest(), PricingBasis: "subscription_invoice_unavailable;local_no_billing", SuiteSHA256: acceptance.MemorySuiteDigest(suite), CriteriaSHA256: acceptance.MemoryVersionCriteriaDigest(suite), Split: "all"}
	if acceptance.ValidateMemoryPlan(suite, plan) != nil {
		t.Fatal("frozen collector plan denied")
	}
	var prior *acceptance.MemoryPrior
	started := time.Now().UTC()
	deadline := started.Add(3 * time.Hour)
	if pins.Prior != nil {
		p, priorErr := acceptance.LoadMemoryPrior(suite, "/prior", *pins.Prior)
		if priorErr != nil {
			t.Fatal("retained prior prefix or original deadline denied")
		}
		prior = &p
		if pins.Renewal != nil {
			schedule, scheduleErr := acceptance.MemoryPendingSchedule(p, plan)
			if scheduleErr != nil {
				t.Fatal("renewed schedule denied")
			}
			deadline, err = acceptance.ValidateMemoryRenewal(p, pins, schedule, time.Now())
			if err != nil {
				t.Fatal(err)
			}
		} else {
			if !p.Expires.After(time.Now()) {
				t.Fatal("original deadline denied")
			}
			deadline, started = p.Expires, p.Expires.Add(-3*time.Hour)
		}
	}
	pool := memoryEvalDatabase(t)
	manifest := catalog.NewFileManifest(filepath.Join(private, "deletion-anchor"))
	if err = manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	cat := catalog.New(pool, manifest)
	p := memoryEvalPolicy()
	if p.Validate() != nil {
		t.Fatal("operator admission policy invalid")
	}
	budgets := budget.NewController(pool, p)
	budgets.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	target, err := budget.NewLocalTarget("local-models:9090", "operator-pinned-service")
	if err != nil {
		t.Fatal(err)
	}
	backend, closeBackend, err := semantic.NewPinnedBackend(target, os.Getenv("BUTLER_MEMORY_EVAL_LOCAL_TOKEN_FILE"))
	if err != nil {
		t.Fatal("pinned local backend unavailable")
	}
	defer closeBackend()
	c := &memoryEvalCollector{pool: pool, catalog: cat, budgets: budgets, backend: backend, suite: suite, plan: plan, private: private, sourceRefs: map[string]map[string]catalog.SourceRef{}, started: started, deadline: deadline, renewed: pins.Renewal != nil, prior: prior, priorPins: pins.Prior, priorDirectory: "/prior"}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	if err = c.init(ctx); err != nil {
		t.Fatal(err)
	}
	var collectionErr error
	defer func() {
		finishCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, closeErr := pool.Exec(finishCtx, `UPDATE fixture_memory_eval_window SET closed=TRUE WHERE singleton`); closeErr != nil {
			collectionErr = closeErr
			t.Error("window close failed")
		}
		if retentionErr := c.writeEvidence("captures.json", c.captures); retentionErr != nil {
			collectionErr = retentionErr
			t.Error("capture retention failed")
		}
		if retentionErr := c.writeEvidence("partial-measurements.json", c.measurements); retentionErr != nil {
			collectionErr = retentionErr
			t.Error("partial expense retention failed")
		}
		if exportErr := c.exportLedger(finishCtx); exportErr != nil {
			t.Error("safe ledger export failed")
		}
		_ = c.writeEvidence("collection-status.json", struct {
			Complete bool
			Failure  string
		}{collectionErr == nil, func() string {
			if collectionErr != nil {
				return "collection_failed_no_retry"
			}
			return ""
		}()})
		if c.frozen.SHA256 != "" {
			c.measurements.FrozenSHA256 = c.frozen.SHA256
			if retentionErr := c.writeEvidence("frozen.json", c.frozen); retentionErr != nil {
				collectionErr = retentionErr
				t.Error("freeze retention failed")
			}
			if retentionErr := c.writeEvidence("measurements.json", c.measurements); retentionErr != nil {
				collectionErr = retentionErr
				t.Error("measurement retention failed")
			}
			scoreFrozen, scoreMeasurements := c.frozen, c.measurements
			if prior != nil {
				scoreFrozen = prior.Frozen
				combined, combineErr := acceptance.CombineMemoryMeasurements(*prior, c.measurements)
				if combineErr != nil {
					t.Error("prior failed denominator or expense lost")
					return
				}
				scoreMeasurements = combined
				if c.writeEvidence("comparison-frozen.json", scoreFrozen) != nil || c.writeEvidence("comparison-measurements.json", scoreMeasurements) != nil {
					t.Error("composite originals retention failed")
				}
			}
			report, scoreErr := acceptance.ScoreMemory(suite, scoreFrozen, scoreMeasurements, approval, key)
			if retentionErr := c.writeEvidence("report.json", report); retentionErr != nil {
				collectionErr = retentionErr
				t.Error("report retention failed")
			}
			if scoreErr != nil {
				t.Error("retained report validation failed")
			}
		}
	}()
	if err = c.writeEvidence("plan.json", plan); err != nil {
		t.Fatal(err)
	}
	for _, cse := range suite.Cases {
		if err = c.capture(ctx, cse); err != nil {
			collectionErr = err
			t.Fatal("current retrieval capture failed without native fallback")
		}
	}
	if prior == nil {
		c.frozen, err = acceptance.FreezeMemory(suite, plan, c.captures)
	} else {
		if err = c.verifyRebasedChunks(ctx); err == nil {
			c.frozen, err = acceptance.MemoryPendingSchedule(*prior, plan)
		}
	}
	if err != nil {
		collectionErr = err
		t.Fatal(err)
	}
	if err = c.writeEvidence("frozen-before-native.json", c.frozen); err != nil {
		collectionErr = err
		t.Fatal(err)
	}
	authPool := pool
	if c.renewed {
		authPool = memoryEvalRenewedAuthPool(t)
		if err = c.retainAuthPreflight(ctx); err != nil {
			collectionErr = err
			t.Fatal("auth preflight evidence denied")
		}
	}
	c.factory, err = newSchemaDialectNativeFactory(ctx, authPool, os.Getenv("BUTLER_MEMORY_EVAL_IMPORT_FILE"), filepath.Join(private, "settings.key"), schemaDialectGoogle)
	if err != nil {
		collectionErr = err
		t.Fatal("private provider initialization failed")
	}
	for _, i := range c.frozen.Inputs {
		var cse acceptance.MemoryCase
		for _, candidate := range suite.Cases {
			if acceptance.MemoryOpaqueCaseID(candidate) == i.CaseID {
				cse = candidate
				break
			}
		}
		o, sampleErr := c.sample(ctx, cse, i)
		c.measurements.Observations = append(c.measurements.Observations, o)
		if err = c.fillUnused(context.Background(), i); err != nil {
			collectionErr = err
			t.Fatal("expense retention failed")
		}
		if sampleErr != nil && !(c.renewed && errors.Is(sampleErr, errMemoryAvailability)) {
			collectionErr = sampleErr
			t.Fatal("model comparison stopped; every spent entry retained")
		}
	}
	t.Log("memory collector complete; question results, broader-goal unknowns and all expenses retained; no promotion claimed")
}

var errMemoryAvailability = errors.New("clean retained provider failure; no retry")

func memoryCleanAvailability(events []memoryEvalEvent, raw []byte, version string, usage *transport.Usage, startErr error) bool {
	if startErr != nil || len(raw) != 0 || version != "" || usage != nil || len(events) != 2 || events[0].Type != "run_started" || events[1].Type != "transport_error" {
		return false
	}
	for _, e := range events {
		if e.ToolProposed || e.Final != nil || e.Usage != nil || e.Version != nil {
			return false
		}
	}
	return acceptance.MemoryAvailabilityCode(events[1].ErrorCode)
}
