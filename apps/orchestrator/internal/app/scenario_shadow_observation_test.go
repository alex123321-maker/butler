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
	"runtime"
	"strconv"
	"strings"
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

// The review froze mixed CRLF/LF bytes (5941a750...). Replacing exactly its
// 542 CRLF pairs with LF reproduces these repository bytes; labels are unchanged.
const shadowRawCorpusSHA = "058b73d4fbcaa146c59fe476e84da91d2fbc652b3888d0ec84ea2d382861cfbd"

func readShadowCorpusBytes(path string) ([]byte, error) {
	const limit = 128 * 1024
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
	if err != nil || len(raw) > limit || hex.EncodeToString(hash[:]) != shadowRawCorpusSHA {
		return nil, errNativeFixtureDenied
	}
	return raw, nil
}

func shadowRepoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

func shadowCorpusBytesCurrent() bool {
	path := filepath.Join(shadowRepoRoot(), "testdata", "acceptance", "ru-scenarios-v3.json")
	raw, err := readShadowCorpusBytes(path)
	return err == nil && len(raw) > 0
}

func loadShadowCorpus() (acceptance.Corpus, error) {
	path := filepath.Join(shadowRepoRoot(), "testdata", "acceptance", "ru-scenarios-v3.json")
	raw, err := readShadowCorpusBytes(path)
	if err != nil {
		return acceptance.Corpus{}, errNativeFixtureDenied
	}
	corpus, err := acceptance.LoadCorpus(strings.NewReader(string(raw)))
	if err != nil || acceptance.CorpusDigest(corpus) != "4f9d5ba8001940c41d1b36efe192a498624eb2f779a2cbc420145b1f89a6e701" {
		return acceptance.Corpus{}, errNativeFixtureDenied
	}
	return corpus, nil
}

type shadowResources struct {
	MaximumCallsPerArm int    `json:"maximum_calls_per_arm"`
	MaximumInputBytes  int64  `json:"maximum_input_bytes"`
	MaximumOutput      int64  `json:"maximum_output_tokens"`
	AttemptSeconds     int    `json:"maximum_attempt_seconds"`
	ArmSeconds         int    `json:"maximum_arm_seconds"`
	Concurrency        int    `json:"concurrency"`
	ClientMemoryBytes  int64  `json:"client_memory_limit_bytes"`
	Pricing            string `json:"pricing_basis"`
	InvoiceMeasured    bool   `json:"invoice_measured"`
	RemoteRAMMeasured  bool   `json:"remote_ram_measured"`
}

func fixedShadowResources() shadowResources {
	return shadowResources{4, 65536, 8192, 120, 480, 1, 1536 << 20, "subscription_no_overage", false, false}
}

type shadowObservations struct {
	Status           string                           `json:"status"`
	Model            string                           `json:"model"`
	ReleaseRef       string                           `json:"release_ref"`
	Historical       nativeHistoricalEvidence         `json:"historical"`
	NativeTotal      int                              `json:"native_attempts"`
	NewAttempts      int                              `json:"new_native_attempts"`
	MaximumTotal     int                              `json:"maximum_total_attempts"`
	MaximumNew       int                              `json:"maximum_new_attempts"`
	Resources        shadowResources                  `json:"declared_resources"`
	Pin              acceptance.CloudPin              `json:"pin"`
	QualityAttempts  []acceptance.CloudAttempt        `json:"quality_attempts"`
	Quality          acceptance.CloudQualityReport    `json:"quality"`
	Single           acceptance.CloudComparisonRun    `json:"single"`
	Multi            acceptance.CloudComparisonRun    `json:"multi"`
	Comparison       acceptance.CloudComparisonReport `json:"comparison"`
	Ordinals         []nativeAttemptEvidence          `json:"charged_attempts"`
	StartedAt        time.Time                        `json:"started_at_utc"`
	FinishedAt       time.Time                        `json:"finished_at_utc"`
	ExecutionAllowed bool                             `json:"execution_authorized"`
}

type shadowSampler struct {
	provider        transport.ModelProvider
	window          *shadowWindow
	pin             *acceptance.CloudPin
	private         string
	observedVersion *string
}

// The control-plane Manager is the only importer/auth resolver. Its private
// key and encrypted rotating credentials never enter model input or reports.
func newShadowNativeProvider(ctx context.Context, pool *pgxpool.Pool, importPath, keyPath string) (*antigravity.Provider, error) {
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
		err = manager.ImportAntigravity(ctx, file, "isolated-issue33-shadow-observation")
		_ = file.Close()
		if err != nil {
			return nil, errNativeFixtureDenied
		}
	}
	return antigravity.NewProvider(antigravity.Config{Model: antigravity.FlashHighModel, Timeout: 120 * time.Second, AuthSource: manager})
}

func shadowClientRSS() *int64 {
	raw, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "VmHWM:" && fields[2] == "kB" {
			value, err := strconv.ParseInt(fields[1], 10, 64)
			if err == nil && value >= 0 && value <= (1<<63-1)/1024 {
				value *= 1024
				return &value
			}
		}
	}
	return nil
}

// Invalid text is preserved as a JSON string for bounded artifact serialization,
// never stripped, repaired or re-requested. The scorer rejects its object schema.
func shadowOutput(text string) json.RawMessage {
	raw := []byte(text)
	if json.Valid(raw) {
		return append(json.RawMessage(nil), raw...)
	}
	wrapped, _ := json.Marshal(text)
	return wrapped
}

func (s *shadowSampler) call(parent context.Context, phase string, c acceptance.CloudInputCase, role string, parentID *string, children []string) (acceptance.CloudAttempt, error) {
	started := time.Now().UTC()
	ctx, cancel := context.WithTimeout(parent, 120*time.Second)
	defer cancel()
	input, err := acceptance.MakeCloudModelInput(c, role, children)
	if err != nil {
		return acceptance.CloudAttempt{}, errNativeFixtureDenied
	}
	contextID := "shadow-" + shadowHash([]string{shadowNativeWindow, phase, c.ID, role})[:32]
	ordinal, err := s.window.reserve(ctx, phase, c, role, contextID, input)
	if err != nil {
		return acceptance.CloudAttempt{}, errNativeFixtureDenied
	}
	a := acceptance.CloudAttempt{AttemptID: fmt.Sprintf("native-%03d", ordinal), CaseID: c.ID,
		ContextID: contextID, ParentContextID: parentID, Role: role, StartUTC: started,
		InputSHA256: acceptance.CloudModelInputDigest(input), Output: json.RawMessage("null"), Outcome: "transport_error"}
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
	stream, startErr := s.provider.StartRun(ctx, req)
	if startErr == nil {
		finals, terminals := 0, 0
		var known *transport.Usage
		usageUncertain := false
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
				a.Outcome = "transport_error"
				a.Output = json.RawMessage("null")
				break
			}
			if terminals > 0 || event.AssistantFinal != nil && event.EventType != transport.EventTypeAssistantFinal {
				startErr = errNativeFixtureDenied
			}
			if event.EventType == transport.EventTypeRunCompleted {
				terminals++
				if terminals != 1 || finals != 1 {
					startErr = errNativeFixtureDenied
				}
			}
			if event.EventType == transport.EventTypeRunCancelled || event.EventType == transport.EventTypeRunTimedOut {
				startErr = errNativeFixtureDenied
			}
			if event.ProviderModelVersion != nil {
				version := *event.ProviderModelVersion
				if s.observedVersion != nil && *s.observedVersion != version || a.ProviderModelVersion != nil && *a.ProviderModelVersion != version {
					startErr = errNativeFixtureDenied
				}
				a.ProviderModelVersion = &version
				s.observedVersion = &version
			}
			usage := event.Usage
			if usage == nil && event.AssistantFinal != nil {
				usage = event.AssistantFinal.Usage
			}
			if usage != nil {
				complete := event.EventType == transport.EventTypeAssistantFinal || event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested || event.EventType == transport.EventTypeTransportError || event.EventType == transport.EventTypeRunCompleted
				if usageUncertain || !complete || usage.Validate() != nil || known != nil && *known != *usage {
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
					if in > 65536 || out > 8192 || total > 65536+8192 {
						startErr = errNativeFixtureDenied
					}
				}
			}
			if event.ToolCall != nil || event.ToolCallBatch != nil {
				a.UnsafeToolProposal = true
				a.Outcome = "invalid_tool_proposal"
				a.Output = json.RawMessage("null")
				break
			}
			if event.TransportError != nil {
				startErr = errNativeFixtureDenied
				a.Outcome = "transport_error"
				a.Output = json.RawMessage("null")
			}
			if event.AssistantFinal != nil {
				finals++
				if finals != 1 || len(event.AssistantFinal.Content) > 8192 || event.AssistantFinal.FinishReason != "STOP" {
					startErr = errNativeFixtureDenied
					continue
				}
				a.Output = shadowOutput(event.AssistantFinal.Content)
				a.Outcome = "completed"
			}
		}
		if a.Outcome == "completed" && (finals != 1 || terminals != 1) {
			startErr = errNativeFixtureDenied
		}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		a.Outcome = "timeout"
		a.Output = json.RawMessage("null")
	} else if ctx.Err() != nil {
		a.Outcome = "cancelled"
		a.Output = json.RawMessage("null")
	} else if startErr != nil {
		a.Outcome = "transport_error"
		a.Output = json.RawMessage("null")
	}
	// Cancellation is transport cleanup, never another inference or a refund.
	cleanup, finish := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	_, _ = s.provider.CancelRun(cleanup, transport.CancelRunRequest{RunID: contextID, Reason: "shadow observation finished"})
	finish()
	a.EndUTC = time.Now().UTC()
	a.Milliseconds = a.EndUTC.Sub(a.StartUTC).Milliseconds()
	a.LocalPeakRSSBytes = shadowClientRSS()
	writeCtx, stop := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer stop()
	tag, persistErr := s.window.pool.Exec(writeCtx, `UPDATE fixture_scenario_shadow_attempts SET outcome=$2,input_tokens=$3,output_tokens=$4,total_tokens=$5 WHERE ordinal=$1 AND outcome='unavailable'`, ordinal, a.Outcome, a.InputTokens, a.OutputTokens, a.TotalTokens)
	if persistErr != nil || tag.RowsAffected() != 1 || writeShadowJSON(filepath.Join(s.private, fmt.Sprintf("attempt-%03d.json", ordinal)), a) != nil {
		return a, errNativeFixtureDenied
	}
	if a.Outcome != "completed" {
		return a, errNativeFixtureDenied
	}
	return a, nil
}

func writeShadowJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return errNativeFixtureDenied
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errNativeFixtureDenied
	}
	_, writeErr := file.Write(append(raw, '\n'))
	syncErr, closeErr := file.Sync(), file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errNativeFixtureDenied
	}
	return nil
}

func TestScenarioShadowActualHighObservationE2E(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_SHADOW_ACCEPTANCE") != "1" {
		t.Skip("separately sealed High shadow window")
	}
	root := shadowRepoRoot()
	corpus, err := loadShadowCorpus()
	history, historyErr := readScenarioNativeHistory(scenarioNativeHistoryPaths())
	private := os.Getenv("BUTLER_SCENARIO_SHADOW_PRIVATE")
	if err != nil || historyErr != nil || !filepath.IsAbs(private) || os.Getenv("BUTLER_SCENARIO_SHADOW_RELEASE") == "" {
		t.Fatal("frozen source/history/private shadow prerequisites denied")
	}
	keyFile, keyErr := acceptance.ReadPrivateFile("/metadata/public-key.hex", 65536)
	approvalFile, approvalErr := acceptance.ReadPrivateFile("/metadata/approval.json", 65536)
	approval, key, err := acceptance.DecodeCloudGoldenArtifacts(corpus, keyFile, approvalFile)
	if keyErr != nil || approvalErr != nil || err != nil {
		t.Fatal("independent golden proof denied")
	}
	var pin acceptance.CloudPin
	metadata, err := acceptance.ReadPrivateFile("/metadata/pin.json", 65536)
	if err != nil || json.Unmarshal(metadata, &pin) != nil || pin.CodeCommit != os.Getenv("BUTLER_SCENARIO_SHADOW_RELEASE") || pin.PolicySHA256 != acceptance.CloudPolicyDigest() || pin.ResourcesSHA256 != shadowHash(fixedShadowResources()) || pin.WindowID != shadowNativeWindow {
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
	if parseErr != nil || cfg.ConnConfig.Database != "butler_shadow_test" {
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
	w, err := createShadowWindow(ctx, pool, 75*time.Minute)
	if err != nil {
		t.Fatal("new finite shadow scope denied")
	}
	report := shadowObservations{Status: "failed", Model: antigravity.FlashHighModel,
		ReleaseRef: pin.CodeCommit, Historical: history, NativeTotal: 45, MaximumTotal: 297, MaximumNew: 252,
		Resources: fixedShadowResources(), Pin: pin, StartedAt: time.Now().UTC(),
		QualityAttempts: []acceptance.CloudAttempt{}, Ordinals: []nativeAttemptEvidence{},
		Single: acceptance.CloudComparisonRun{Mode: "single", Tasks: []acceptance.CloudComparisonTask{}},
		Multi:  acceptance.CloudComparisonRun{Mode: "multi", Tasks: []acceptance.CloudComparisonTask{}}}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if pool.QueryRow(cleanup, `SELECT attempts FROM fixture_scenario_shadow_limit`).Scan(&report.NativeTotal) != nil {
			t.Error("actual shadow counter unavailable")
		}
		report.NewAttempts = report.NativeTotal - 45
		rows, err := pool.Query(cleanup, `SELECT ordinal,context_id,input_hash,created_at FROM fixture_scenario_shadow_attempts ORDER BY ordinal`)
		if err != nil {
			t.Error("actual shadow charged ledger unavailable")
		} else {
			for rows.Next() {
				var a nativeAttemptEvidence
				a.Kind = "start"
				if rows.Scan(&a.Attempt, &a.RunID, &a.InputHash, &a.CreatedAt) != nil {
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
		report.Quality, err = acceptance.ScoreCloud(corpus, approval, key, pin, report.QualityAttempts)
		if err != nil {
			t.Error("independent quality schema/correlation denied")
		}
		report.Comparison, err = acceptance.CompareCloud(corpus, approval, key, report.Single, report.Multi)
		if err != nil {
			t.Error("independent comparison schema/correlation denied")
		}
		if !t.Failed() && report.NewAttempts == 252 && shadowQualityPassed(report.Quality) && shadowComparisonPassed(report.Comparison) {
			report.Status = "passed"
		}
		report.FinishedAt = time.Now().UTC()
		if writeShadowJSON(filepath.Join(private, "observations.json"), report) != nil {
			t.Error("bounded private shadow report persistence denied")
		}
	})
	provider, err := newShadowNativeProvider(ctx, pool, os.Getenv("BUTLER_SCENARIO_SHADOW_IMPORT_FILE"), filepath.Join(private, "encrypted-settings-key"))
	if err != nil {
		t.Fatal("backend native auth initialization denied")
	}
	sampler := &shadowSampler{provider: provider, window: w, pin: &pin, private: private}
	inputs, _ := acceptance.ExportCloudInputs(corpus, "")
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
	pairs, _ := acceptance.CloudComparisonInputs(corpus)
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
				parentID := "shadow-" + shadowHash([]string{shadowNativeWindow, "comparison", c.ID, acceptance.CloudRoleSynthesis})[:32]
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
	quality, err := acceptance.ScoreCloud(corpus, approval, key, pin, report.QualityAttempts)
	if err != nil || !shadowQualityPassed(quality) {
		t.Fatal("frozen independent quality thresholds failed")
	}
	report.Single.Pin, report.Multi.Pin = pin, pin
	comparison, err := acceptance.CompareCloud(corpus, approval, key, report.Single, report.Multi)
	if err != nil || !shadowComparisonPassed(comparison) {
		t.Fatal("frozen paired-context quality or complete-cost thresholds failed")
	}
}

func shadowQualityPassed(r acceptance.CloudQualityReport) bool {
	return r.Status == "CLOUD_SHADOW_THRESHOLDS_PASS" && r.Complete && r.ThresholdsPassed && r.Cost.FullTokenCostKnown
}

func shadowComparisonPassed(r acceptance.CloudComparisonReport) bool {
	return r.Status == "CLOUD_SHADOW_COMPARISON_PASS" && r.PairedTasks == 4 && r.SingleCorrect == 4 && r.MultiCorrect == 4 && r.ThresholdsPassed && r.FullTokenCostKnown
}
