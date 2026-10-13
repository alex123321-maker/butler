package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/storage/postgres"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Candidate bytes for offline checks only. Native preparation additionally
// requires separately frozen external V4 golden identities and reviewer approval.
const shadowV4RawCorpusSHA = "a4bf81093ec00be4c424fc511c80f446e96908580b3e4cb5abb0398f94274949"

func readShadowV4CorpusBytes(path string) ([]byte, error) {
	const limit = 512 * 1024
	stat, err := os.Lstat(path)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > limit {
		return nil, errNativeFixtureDenied
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(stat, opened) {
		return nil, errNativeFixtureDenied
	}
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	hash := sha256.Sum256(raw)
	if err != nil || len(raw) > limit || hex.EncodeToString(hash[:]) != shadowV4RawCorpusSHA {
		return nil, errNativeFixtureDenied
	}
	return raw, nil
}

func loadShadowV4Corpus() (acceptance.CloudV4Corpus, error) {
	path := filepath.Join(shadowRepoRoot(), "testdata", "acceptance", "ru-cloud-fresh-v4.json")
	raw, err := readShadowV4CorpusBytes(path)
	if err != nil {
		return acceptance.CloudV4Corpus{}, errNativeFixtureDenied
	}
	corpus, err := acceptance.LoadCloudV4Corpus(strings.NewReader(string(raw)))
	if err != nil {
		return acceptance.CloudV4Corpus{}, errNativeFixtureDenied
	}
	return corpus, nil
}

type shadowV4Resources = acceptance.CloudV4Resources

func fixedShadowV4Resources() shadowV4Resources {
	return acceptance.FixedCloudV4Resources()
}

type shadowV4Observations struct {
	Status           string                           `json:"status"`
	Model            string                           `json:"model"`
	ReleaseRef       string                           `json:"release_ref"`
	Historical       nativeHistoricalEvidence         `json:"historical"`
	NativeTotal      int                              `json:"native_attempts"`
	NewAttempts      int                              `json:"new_native_attempts"`
	MaximumTotal     int                              `json:"maximum_total_attempts"`
	MaximumNew       int                              `json:"maximum_new_attempts"`
	Resources        shadowV4Resources                `json:"declared_resources"`
	Pin              acceptance.CloudPin              `json:"pin"`
	QualityAttempts  []acceptance.CloudAttempt        `json:"quality_attempts"`
	Quality          acceptance.CloudV4QualityReport  `json:"quality"`
	Single           acceptance.CloudComparisonRun    `json:"single"`
	Multi            acceptance.CloudComparisonRun    `json:"multi"`
	Comparison       acceptance.CloudComparisonReport `json:"comparison"`
	Ordinals         []shadowV4ChargedAttempt         `json:"charged_attempts"`
	StartedAt        time.Time                        `json:"started_at_utc"`
	FinishedAt       time.Time                        `json:"finished_at_utc"`
	WindowExpiresAt  time.Time                        `json:"window_expires_at_utc"`
	ExecutionAllowed bool                             `json:"execution_authorized"`
}

type shadowV4ChargedAttempt struct {
	nativeAttemptEvidence
	ParentContextID *string `json:"parent_context_id"`
	Phase           string  `json:"phase"`
	CaseID          string  `json:"case_id"`
	Role            string  `json:"role"`
	Outcome         string  `json:"outcome"`
	InputTokens     *int64  `json:"input_tokens"`
	OutputTokens    *int64  `json:"output_tokens"`
	TotalTokens     *int64  `json:"total_tokens"`
	OutputSHA256    *string `json:"output_sha256"`
}

func shadowV4ContextID(phase, caseID, role string) string {
	return "shadow-" + shadowV4Hash([]string{shadowV4NativeWindow, phase, caseID, role})[:32]
}

type shadowV4Sampler struct {
	provider        transport.ModelProvider
	providerFactory func(json.RawMessage) (transport.ModelProvider, error)
	window          *shadowV4Window
	pin             *acceptance.CloudPin
	private         string
	observedVersion *string
	expectedVersion string // Frozen native identity; fixture samplers leave unset.
}

// Import once into the private control plane. Every observation constructs an
// independent run owner with the exact all-original-ID case schema; no model output
// can choose credentials, model, schema, effort or generated-token allowance.
func newShadowV4NativeFactory(ctx context.Context, pool *pgxpool.Pool, importPath, keyPath string) (func(json.RawMessage) (transport.ModelProvider, error), error) {
	key, err := privateNativeFixtureKey(keyPath)
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	store := config.NewPostgresSettingsStore(pool, config.WithSettingsCodec(config.StaticSecretValueCodec{Key: key}))
	manager := providerauth.NewManager(store)
	state, err := manager.State(ctx, modelprovider.ProviderAntigravity)
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	if !state.Connected {
		file, err := privateNativeFixtureFile(importPath, 64*1024)
		if err != nil {
			return nil, errNativeFixtureDenied
		}
		err = manager.ImportAntigravity(ctx, file, "isolated-issue33-prospective-v4")
		_ = file.Close()
		if err != nil {
			return nil, errNativeFixtureDenied
		}
	}
	return func(schema json.RawMessage) (transport.ModelProvider, error) {
		return antigravity.NewProvider(antigravity.Config{Model: antigravity.FlashHighModel, Timeout: 120 * time.Second,
			AuthSource: manager, MaximumOutputTokens: 32768, JSONResponseSchema: schema})
	}, nil
}

func (s *shadowV4Sampler) call(parent context.Context, phase string, c acceptance.CloudInputCase, role string, parentID *string, children []string) (acceptance.CloudAttempt, error) {
	started := time.Now().UTC()
	ctx, cancel := context.WithTimeout(parent, 120*time.Second)
	defer cancel()
	input, err := acceptance.MakeCloudV4ModelInput(c, role, children)
	if err != nil {
		return acceptance.CloudAttempt{}, errNativeFixtureDenied
	}
	expectedParent := shadowV4ContextID("comparison", c.ID, acceptance.CloudRoleSynthesis)
	child := role == acceptance.CloudRoleChildA || role == acceptance.CloudRoleChildB
	if child && (phase != "comparison" || parentID == nil || *parentID != expectedParent) || !child && parentID != nil {
		return acceptance.CloudAttempt{}, errNativeFixtureDenied
	}
	contextID := shadowV4ContextID(phase, c.ID, role)
	provider := s.provider
	if s.providerFactory != nil {
		provider, err = s.providerFactory(input.Schema)
	}
	if err != nil || provider == nil {
		return acceptance.CloudAttempt{}, errNativeFixtureDenied
	}
	ordinal, err := s.window.reserve(ctx, phase, c, role, contextID, input)
	if err != nil {
		return acceptance.CloudAttempt{}, errNativeFixtureDenied
	}
	a := acceptance.CloudAttempt{AttemptID: fmt.Sprintf("native-%03d", ordinal), CaseID: c.ID,
		ContextID: contextID, ParentContextID: parentID, Role: role, StartUTC: started,
		InputSHA256: acceptance.CloudV4ModelInputDigest(input), Output: json.RawMessage("null"), Outcome: "transport_error"}
	// There is no tool declaration, Tool Broker dispatcher or provider continuation
	// path here. Child contexts are shadow observations, never fabricated Tasks.
	user, _ := json.Marshal(struct {
		CaseID       string            `json:"case_id"`
		Input        string            `json:"input"`
		Documents    map[string]string `json:"documents"`
		ChildReports []string          `json:"child_reports_data"`
	}{input.CaseID, input.Input, input.Documents, input.ChildReports})
	req := transport.StartRunRequest{Context: transport.TransportRunContext{
		RunID: contextID, SessionKey: "scenario-shadow:" + c.ID, ProviderName: antigravity.Name,
		ModelName: antigravity.FlashHighModel, SupportsStreaming: true}, StreamingEnabled: true,
		InputItems: []transport.InputItem{{Role: "system", Content: input.SystemInstructions}, {Role: "user", Content: string(user)}}}
	ctx = transport.WithInputByteLimit(ctx, 65536)
	var stageMu sync.Mutex
	var lastStage *transport.ProviderStage
	ctx = transport.WithProviderStageObserver(ctx, func(stage transport.ProviderStage) {
		stageMu.Lock()
		defer stageMu.Unlock()
		copy := stage
		lastStage = &copy
	})
	// Observe filtered finals without altering the original stream. Owning SQL
	// spend is saved before any raw filesystem persistence below.
	capture := &privateFinalCaptureProvider{ModelProvider: provider, private: s.private, ordinal: ordinal}
	stream, startErr := capture.StartRun(ctx, req)
	diagnostic := transport.DiagnosticCode(startErr)
	failureStage := "provider_start"
	var finishReason *string
	finals, terminals := 0, 0
	lifecycleViolation := startErr != nil
	if startErr == nil {
		failureStage = "stream_lifecycle"
		var known *transport.Usage
		usageUncertain := false
		versionUncertain := false
	readStream:
		for {
			var event transport.TransportEvent
			select {
			case <-ctx.Done():
				break readStream
			case next, open := <-stream:
				if !open {
					break readStream
				}
				event = next
			}
			if event.RunID != contextID {
				lifecycleViolation = true
				failureStage = "foreign_run"
				startErr = errNativeFixtureDenied
				a.Outcome = "transport_error"
				a.Output = json.RawMessage("null")
				continue
			}
			if terminals > 0 || event.AssistantFinal != nil && event.EventType != transport.EventTypeAssistantFinal {
				lifecycleViolation = true
				startErr = errNativeFixtureDenied
			}
			if event.EventType == transport.EventTypeRunCompleted {
				terminals++
				if terminals != 1 || finals != 1 {
					lifecycleViolation = true
					startErr = errNativeFixtureDenied
				}
			}
			if event.EventType == transport.EventTypeRunCancelled || event.EventType == transport.EventTypeRunTimedOut {
				lifecycleViolation = true
				startErr = errNativeFixtureDenied
			}
			if event.ProviderModelVersion != nil {
				complete := event.EventType == transport.EventTypeAssistantFinal || event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested || event.EventType == transport.EventTypeTransportError || event.EventType == transport.EventTypeRunCompleted
				if !complete {
					versionUncertain = true
					failureStage = "version_contract"
					startErr = errNativeFixtureDenied
				}
				version := *event.ProviderModelVersion
				if a.ProviderModelVersion != nil && *a.ProviderModelVersion != version {
					versionUncertain = true
				}
				if versionUncertain {
					a.ProviderModelVersion = nil
				} else {
					a.ProviderModelVersion = &version
				}
				if versionUncertain || !acceptance.SafeCloudFamilyVersion(version) || s.expectedVersion != "" && version != s.expectedVersion || s.observedVersion != nil && *s.observedVersion != version {
					failureStage = "version_contract"
					startErr = errNativeFixtureDenied
				} else if s.observedVersion == nil {
					s.observedVersion = &version
				}
			}
			usage := event.Usage
			if usage != nil && event.AssistantFinal != nil && event.AssistantFinal.Usage != nil && *usage != *event.AssistantFinal.Usage {
				failureStage = "usage_contract"
				startErr = errNativeFixtureDenied
				usageUncertain = true
			}
			if usage == nil && event.AssistantFinal != nil {
				usage = event.AssistantFinal.Usage
			}
			if usage != nil {
				complete := event.EventType == transport.EventTypeAssistantFinal || event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested || event.EventType == transport.EventTypeTransportError || event.EventType == transport.EventTypeRunCompleted
				if usageUncertain || !complete || usage.Validate() != nil || known != nil && *known != *usage {
					failureStage = "usage_contract"
					startErr = errNativeFixtureDenied
					usageUncertain = true
					a.InputTokens, a.OutputTokens, a.TotalTokens = nil, nil, nil
				} else {
					copy := *usage
					known = &copy
					in, out, total := int64(usage.InputTokens), int64(usage.OutputTokens), int64(usage.TotalTokens)
					a.InputTokens, a.OutputTokens, a.TotalTokens = &in, &out, &total
					if total == 0 && in+out > 0 {
						a.TotalTokens = nil // legacy unspecified total, not measured zero.
					}
					if in > 65536 || out > 32768 || total > in+32768 {
						failureStage = "usage_allowance"
						startErr = errNativeFixtureDenied
					}
				}
			}
			if event.ToolCall != nil || event.ToolCallBatch != nil {
				lifecycleViolation = true
				failureStage = "forbidden_tool_proposal"
				startErr = errNativeFixtureDenied
				a.UnsafeToolProposal = true
				a.Outcome = "invalid_tool_proposal"
				a.Output = json.RawMessage("null")
				continue
			}
			if event.TransportError != nil {
				lifecycleViolation = true
				failureStage = "provider_stream"
				diagnostic = transport.DiagnosticCode(event.TransportError)
				startErr = errNativeFixtureDenied
				a.Outcome = "transport_error"
				a.Output = json.RawMessage("null")
			}
			if event.AssistantFinal != nil {
				if event.AssistantFinal.FinishReason == "STOP" || event.AssistantFinal.FinishReason == "MAX_TOKENS" {
					value := event.AssistantFinal.FinishReason
					finishReason = &value
				}
				finals++
				if finals != 1 || event.EventType != transport.EventTypeAssistantFinal || event.AssistantFinal.FinishReason != "STOP" {
					lifecycleViolation = true
				}
				if finals != 1 || len(event.AssistantFinal.Content) > 8192 || event.AssistantFinal.FinishReason != "STOP" {
					failureStage = "final_contract"
					startErr = errNativeFixtureDenied
					continue
				}
				// Native structured observations must arrive as JSON objects.
				// Wrapping malformed text as a JSON string would preserve bytes,
				// but falsely let the driver continue a completed observation.
				if classifyPrivateFinal([]byte(event.AssistantFinal.Content)) != "json_object" {
					failureStage = "output_encoding_contract"
					startErr = errNativeFixtureDenied
					continue
				}
				a.Output = json.RawMessage(event.AssistantFinal.Content)
				a.Outcome = "completed"
			}
		}
		if a.Outcome == "completed" && (finals != 1 || terminals != 1) {
			failureStage = "incomplete_lifecycle"
			startErr = errNativeFixtureDenied
		}
		// The prospectively fixed native gate requires complete measured tokens
		// and identity. More requests cannot repair a missing observation.
		if startErr == nil && a.Outcome == "completed" && (a.ProviderModelVersion == nil || a.InputTokens == nil || a.OutputTokens == nil || a.TotalTokens == nil) {
			failureStage = "metadata_contract"
			startErr = errNativeFixtureDenied
		}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		failureStage = "attempt_deadline"
		a.Outcome = "timeout"
		a.Output = json.RawMessage("null")
	} else if ctx.Err() != nil {
		failureStage = "context_cancelled"
		a.Outcome = "cancelled"
		a.Output = json.RawMessage("null")
	} else if startErr != nil {
		a.Outcome = "transport_error"
		a.Output = json.RawMessage("null")
	}
	if a.UnsafeToolProposal {
		a.Outcome = "invalid_tool_proposal"
	}
	lifecycleClean := !lifecycleViolation && ctx.Err() == nil && finals == 1 && terminals == 1
	// Cancellation is transport cleanup, never another inference or a refund.
	cleanup, finish := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	_, _ = provider.CancelRun(cleanup, transport.CancelRunRequest{RunID: contextID, Reason: "shadow observation finished"})
	finish()
	a.EndUTC = time.Now().UTC()
	a.Milliseconds = a.EndUTC.Sub(a.StartUTC).Milliseconds()
	a.LocalPeakRSSBytes = shadowClientRSS()
	var outputSHA *string
	if a.Outcome == "completed" {
		value, hashErr := acceptance.CloudV4OutputDigest(a.Output)
		if hashErr != nil {
			a.Outcome, a.Output = "transport_error", json.RawMessage("null")
			failureStage = "output_encoding_contract"
		} else {
			outputSHA = &value
			failureStage = "completed"
		}
	}
	// Persist measured spend before auxiliary diagnostics. Each independent
	// write is attempted even when another fails; the owning charge is never lost.
	writeCtx, stop := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer stop()
	tag, persistErr := s.window.pool.Exec(writeCtx, `UPDATE fixture_scenario_shadow_v4_attempts SET outcome=$2,input_tokens=$3,output_tokens=$4,total_tokens=$5,output_hash=$6 WHERE ordinal=$1 AND outcome='unavailable'`, ordinal, a.Outcome, a.InputTokens, a.OutputTokens, a.TotalTokens, outputSHA)
	var retentionErr error
	if capture.finalMetadata() != nil && persistErr == nil && tag.RowsAffected() == 1 {
		retentionErr = capture.persistFinal()
	}
	finalMetadata := capture.finalMetadata()
	if finalMetadata != nil && (!finalMetadata.RawRetained || capture.captureFailure()) {
		retentionErr = errNativeFixtureDenied
	}
	stageMu.Lock()
	observedStage := lastStage
	stageMu.Unlock()
	reason := "no_visible_final"
	var finalBytes *int
	var finalSHA *string
	retained := false
	if finalMetadata != nil {
		reason = finalMetadata.Reason
		finalBytes, finalSHA, retained = &finalMetadata.Bytes, &finalMetadata.SHA256, finalMetadata.RawRetained
	}
	// Safe immutable facts are separated from quality outputs and raw finals.
	// Even an auxiliary storage failure leaves the owning spend unchanged.
	if diagnostic == "" && a.Outcome != "completed" {
		diagnostic = string(transport.ErrorTypeInternalTransportError)
	}
	_, diagnosticSQLerr := s.window.pool.Exec(writeCtx, `INSERT INTO fixture_scenario_shadow_v4_diagnostics(ordinal,collector_stage,diagnostic_code,encoding_reason,final_bytes,final_hash,raw_retained,retention_ok,model_version,finish_reason,provider_stage,final_count,terminal_count,lifecycle_clean) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, ordinal, failureStage, diagnostic, reason, finalBytes, finalSHA, retained, retentionErr == nil, a.ProviderModelVersion, finishReason, observedStage, finals, terminals, lifecycleClean)
	attemptErr := writeShadowJSON(filepath.Join(s.private, fmt.Sprintf("attempt-%03d.json", ordinal)), a)
	var diagnosticErr error
	// Sidecar diagnostics use the existing bounded vocabulary only. Never
	// persist an upstream message/details, and never retrofit an old observation.
	{
		diagnosticErr = writeShadowJSON(filepath.Join(s.private, fmt.Sprintf("attempt-%03d-diagnostic.json", ordinal)), struct {
			Attempt        string                   `json:"attempt_id"`
			Outcome        string                   `json:"outcome"`
			Code           string                   `json:"diagnostic_code"`
			Stage          string                   `json:"collector_stage"`
			FinishReason   *string                  `json:"observed_finish_reason"`
			OperationStage *transport.ProviderStage `json:"provider_operation_stage"`
			Final          *privateFinalMetadata    `json:"visible_final_metadata"`
			RetentionOK    bool                     `json:"retention_ok"`
			FinalCount     int                      `json:"final_count"`
			TerminalCount  int                      `json:"terminal_count"`
			LifecycleClean bool                     `json:"lifecycle_clean"`
		}{a.AttemptID, a.Outcome, diagnostic, failureStage, finishReason, observedStage, finalMetadata, retentionErr == nil, finals, terminals, lifecycleClean})
	}
	if persistErr != nil || tag.RowsAffected() != 1 || attemptErr != nil || diagnosticErr != nil || diagnosticSQLerr != nil || retentionErr != nil {
		return a, errNativeFixtureDenied
	}
	if a.Outcome != "completed" {
		return a, errNativeFixtureDenied
	}
	return a, nil
}

func TestScenarioShadowV4ActualHighObservationE2E(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_SHADOW_V4_ACCEPTANCE") != "1" {
		t.Skip("separately sealed High shadow window")
	}
	root := shadowRepoRoot()
	corpus, err := loadShadowV4Corpus()
	history, historyErr := readScenarioHighV4History()
	private := os.Getenv("BUTLER_SCENARIO_SHADOW_V4_PRIVATE")
	if err != nil || historyErr != nil || !filepath.IsAbs(private) || os.Getenv("BUTLER_SCENARIO_SHADOW_V4_RELEASE") == "" {
		t.Fatal("frozen source/history/private shadow prerequisites denied")
	}
	keyFile, keyErr := acceptance.ReadPrivateFile("/metadata/public-key.hex", 65536)
	approvalFile, approvalErr := acceptance.ReadPrivateFile("/metadata/approval.json", 65536)
	approval, key, err := acceptance.DecodeCloudV4GoldenArtifacts(corpus, keyFile, approvalFile)
	if keyErr != nil || approvalErr != nil || err != nil {
		t.Fatal("independent golden proof denied")
	}
	var pin acceptance.CloudPin
	metadata, err := acceptance.ReadPrivateFile("/metadata/pin.json", 65536)
	if err != nil || json.Unmarshal(metadata, &pin) != nil || pin.CodeCommit != os.Getenv("BUTLER_SCENARIO_SHADOW_V4_RELEASE") || pin.PolicySHA256 != acceptance.CloudV4PolicyDigest(corpus) || pin.ResourcesSHA256 != acceptance.CloudV4ResourcesDigest() || pin.WindowID != shadowV4NativeWindow {
		t.Fatal("frozen observation pins denied")
	}
	actualSource, err := acceptance.TreeHash(root, nil)
	if err != nil || actualSource != pin.SourceSHA256 {
		t.Fatal("actual immutable source differs from prepared source")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Minute)
	defer cancel()
	// A stable isolated database retains charged rows and expiry across process
	// loss. Unlike unit fixtures, this actual window never drops its schema.
	dsn := os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL")
	cfg, parseErr := pgxpool.ParseConfig(dsn)
	if parseErr != nil || cfg.ConnConfig.Database != "butler_shadow_v4_test" {
		t.Fatal("dedicated shadow database required")
	}
	store, err := postgres.Open(ctx, postgres.Config{URL: dsn, MaxConns: 2}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal("durable shadow database denied")
	}
	t.Cleanup(store.Close)
	if err = store.RunMigrations(ctx, filepath.Join(root, "migrations")); err != nil {
		t.Fatal("durable shadow migrations denied")
	}
	pool := store.Pool()
	w, err := createShadowV4Window(ctx, pool, 75*time.Minute)
	if err != nil {
		t.Fatal("new finite shadow scope denied")
	}
	report := shadowV4Observations{Status: "failed", Model: antigravity.FlashHighModel,
		ReleaseRef: pin.CodeCommit, Historical: history, NativeTotal: 268, MaximumTotal: 520, MaximumNew: 252,
		Resources: fixedShadowV4Resources(), Pin: pin, StartedAt: time.Now().UTC(),
		QualityAttempts: []acceptance.CloudAttempt{}, Ordinals: []shadowV4ChargedAttempt{},
		Single: acceptance.CloudComparisonRun{Mode: "single", Tasks: []acceptance.CloudComparisonTask{}},
		Multi:  acceptance.CloudComparisonRun{Mode: "multi", Tasks: []acceptance.CloudComparisonTask{}}}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if pool.QueryRow(cleanup, `SELECT attempts,expires_at FROM fixture_scenario_shadow_v4_limit`).Scan(&report.NativeTotal, &report.WindowExpiresAt) != nil {
			t.Error("actual shadow counter unavailable")
		}
		report.NewAttempts = report.NativeTotal - 268
		rows, err := pool.Query(cleanup, `SELECT ordinal,context_id,input_hash,created_at,phase,case_id,role,outcome,input_tokens,output_tokens,total_tokens,output_hash,parent_context_id FROM fixture_scenario_shadow_v4_attempts ORDER BY ordinal`)
		if err != nil {
			t.Error("actual shadow charged ledger unavailable")
		} else {
			for rows.Next() {
				var a shadowV4ChargedAttempt
				a.Kind = "start"
				if rows.Scan(&a.Attempt, &a.RunID, &a.InputHash, &a.CreatedAt, &a.Phase, &a.CaseID, &a.Role, &a.Outcome, &a.InputTokens, &a.OutputTokens, &a.TotalTokens, &a.OutputSHA256, &a.ParentContextID) != nil {
					t.Error("shadow charged projection denied")
					break
				}
				report.Ordinals = append(report.Ordinals, a)
			}
			if rows.Err() != nil {
				t.Error("shadow charged projection incomplete")
			}
			rows.Close()
		}
		report.Pin = pin
		report.Single.Pin, report.Multi.Pin = pin, pin
		report.Quality, err = acceptance.ScoreCloudV4(corpus, approval, key, pin, report.QualityAttempts)
		if err != nil {
			t.Error("independent quality schema/correlation denied")
		}
		report.Comparison, err = acceptance.CompareCloudV4(corpus, approval, key, report.Single, report.Multi)
		if err != nil {
			t.Error("independent comparison schema/correlation denied")
		}
		if !t.Failed() && report.NewAttempts == 252 && shadowV4QualityPassed(report.Quality) && shadowV4ComparisonPassed(report.Comparison) {
			report.Status = "passed"
		}
		report.FinishedAt = time.Now().UTC()
		if writeShadowJSON(filepath.Join(private, "observations.json"), report) != nil {
			t.Error("bounded private shadow report persistence denied")
		}
	})
	factory, err := newShadowV4NativeFactory(ctx, pool, os.Getenv("BUTLER_SCENARIO_SHADOW_V4_IMPORT_FILE"), filepath.Join(private, "encrypted-settings-key"))
	if err != nil {
		t.Fatal("backend native auth initialization denied")
	}
	sampler := &shadowV4Sampler{providerFactory: factory, window: w, pin: &pin, private: private, expectedVersion: acceptance.CloudV4ExpectedModelVersion}
	inputs, _ := acceptance.ExportCloudV4Inputs(corpus, "")
	for index, c := range inputs.Cases {
		a, err := sampler.call(ctx, "quality", c, acceptance.CloudRoleSingle, nil, nil)
		if a.AttemptID != "" {
			report.QualityAttempts = append(report.QualityAttempts, a)
		}
		if err != nil {
			t.Fatal("charged shadow observation failed; no retry")
		}
		if (index+1)%16 == 0 {
			t.Logf("completed_quality_observations=%d model_actions=0", index+1)
		}
	}
	pairs, _ := acceptance.CloudV4ComparisonInputs(corpus)
	for _, c := range pairs {
		for _, multi := range []bool{false, true} {
			start := time.Now().UTC()
			armCtx, stop := context.WithTimeout(ctx, 480*time.Second)
			task := acceptance.CloudComparisonTask{CaseID: c.ID, OriginalInputSHA256: c.OriginalInputSHA256, StartUTC: start, Attempts: []acceptance.CloudAttempt{}}
			if !multi {
				a, callErr := sampler.call(armCtx, "comparison", c, acceptance.CloudRoleSingle, nil, nil)
				task.Attempts = append(task.Attempts, a)
				err = callErr
			} else {
				parentID := shadowV4ContextID("comparison", c.ID, acceptance.CloudRoleSynthesis)
				var children []string
				for _, role := range []string{acceptance.CloudRoleChildA, acceptance.CloudRoleChildB} {
					a, callErr := sampler.call(armCtx, "comparison", c, role, &parentID, nil)
					task.Attempts = append(task.Attempts, a)
					if callErr != nil {
						err = callErr
						break
					}
					children = append(children, string(a.Output))
				}
				if err == nil {
					a, callErr := sampler.call(armCtx, "comparison", c, acceptance.CloudRoleSynthesis, nil, children)
					task.Attempts = append(task.Attempts, a)
					err = callErr
				}
			}
			stop()
			task.EndUTC = time.Now().UTC()
			task.Milliseconds = task.EndUTC.Sub(task.StartUTC).Milliseconds()
			if multi {
				report.Multi.Tasks = append(report.Multi.Tasks, task)
			} else {
				report.Single.Tasks = append(report.Single.Tasks, task)
			}
			if err != nil {
				t.Fatal("charged paired observation failed; all prior attempts retained")
			}
		}
	}
	quality, err := acceptance.ScoreCloudV4(corpus, approval, key, pin, report.QualityAttempts)
	if err != nil || !shadowV4QualityPassed(quality) {
		t.Fatal("frozen independent quality thresholds failed")
	}
	report.Single.Pin, report.Multi.Pin = pin, pin
	comparison, err := acceptance.CompareCloudV4(corpus, approval, key, report.Single, report.Multi)
	if err != nil || !shadowV4ComparisonPassed(comparison) {
		t.Fatal("frozen paired-context quality or complete-cost thresholds failed")
	}
}

func shadowV4QualityPassed(r acceptance.CloudV4QualityReport) bool {
	return r.Status == "CLOUD_V4_SHADOW_THRESHOLDS_PASS" && r.Complete && r.ThresholdsPassed && r.Cost.FullTokenCostKnown
}

func shadowV4ComparisonPassed(r acceptance.CloudComparisonReport) bool {
	return r.Status == "CLOUD_V4_SHADOW_COMPARISON_PASS" && r.PairedTasks == 4 && r.SingleCorrect == 4 && r.MultiCorrect == 4 && r.ThresholdsPassed && r.FullTokenCostKnown
}
