package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

const structuralDiagnosticWindowID = "issue33-high-structural-json-20261012-baseline266"
const structuralDiagnosticInputSHA = "529be4819f407e4028dfbf8b44b5e84d2b0ee42aad1edf62be0e0f182bfc6e45"
const structuralDiagnosticReviewedSHA = ""                       // Independent protocol/source review pending.
const structuralDiagnosticExpectedVersion = "gemini-3.8-flash-n" // Observed in all18 prior V3 packets.

func structuralDiagnosticProtocolSHA() string {
	return shadowV3Hash(struct {
		Window, Inputs, Model, ExpectedVersion, SchemaSHA                                                     string
		Baseline, Ceiling, Seconds, AttemptSeconds, Generated, Visible, InputBytes, Concurrency, Depth, Array int
		StartOnly, TwoIndependent, SQLBeforeRaw                                                               bool
	}{
		structuralDiagnosticWindowID, structuralDiagnosticInputSHA, antigravity.FlashHighModel, structuralDiagnosticExpectedVersion, acceptance.CloudV3SchemaDigest(), 266, 268, 480, 120, 32768, 8192, 65536, 1, 16, 256, true, true, true})
}

func structuralDiagnosticInputs() ([]acceptance.CloudInputCase, error) {
	path := filepath.Join(shadowRepoRoot(), "testdata", "acceptance", "cloud-structural-json-diagnostic-20261012.json")
	raw, err := readNativeArtifact(path, structuralDiagnosticInputSHA)
	var fixture struct {
		Version    string               `json:"version"`
		Provenance string               `json:"provenance"`
		Purpose    string               `json:"purpose"`
		Cases      []acceptance.Example `json:"cases"`
	}
	if err != nil || acceptance.DecodeStrict(raw, &fixture, "version", "provenance", "purpose", "cases") != nil || len(fixture.Cases) != 2 || fixture.Version != "butler-cloud-structural-json-diagnostic-inputs-v1" || fixture.Provenance != "synthetic_public" || fixture.Purpose != "unseen_structural_protocol_observations_not_quality" {
		return nil, errNativeFixtureDenied
	}
	inputs := []acceptance.CloudInputCase{}
	for _, e := range fixture.Cases {
		if e.Kind != "retrieval" || e.NoModel || len(e.Documents) != 2 || e.Input == "" {
			return nil, errNativeFixtureDenied
		}
		x := acceptance.CloudInputCase{ID: "case:" + shadowV3Hash(acceptance.CloudV3Protocol + "\x00" + e.ID)[:32], Kind: e.Kind, Input: e.Input, Documents: e.Documents}
		x.OriginalInputSHA256 = shadowV3Hash(x)
		inputs = append(inputs, x) // Golden/provenance/purpose never enter the model input.
	}
	return inputs, nil
}

func structuralDiagnosticGate() error {
	if !nativeAuditHash.MatchString(structuralDiagnosticReviewedSHA) {
		return errNativeFixtureDenied
	}
	raw, err := readNativeArtifact(filepath.Join(shadowRepoRoot(), "deploy", "acceptance", "structural-json-diagnostic-approval-20261012.json"), structuralDiagnosticReviewedSHA)
	var approval struct {
		Version     string `json:"version"`
		Reviewer    string `json:"reviewer"`
		Author      string `json:"author"`
		ProtocolSHA string `json:"protocol_sha256"`
		InputSHA    string `json:"input_sha256"`
		HistorySHA  string `json:"history_sha256"`
	}
	history, herr := readScenarioHighV3History()
	if err != nil || herr != nil || acceptance.DecodeStrict(raw, &approval, "version", "reviewer", "author", "protocol_sha256", "input_sha256", "history_sha256") != nil || approval.Version != "butler-structural-json-protocol-review-v1" || approval.Reviewer == "" || approval.Author == "" || approval.Reviewer == approval.Author || approval.ProtocolSHA != structuralDiagnosticProtocolSHA() || approval.InputSHA != structuralDiagnosticInputSHA || approval.HistorySHA != history.BaselineSHA256 {
		return errNativeFixtureDenied
	}
	_, err = structuralDiagnosticInputs()
	return err
}

type structuralDiagnosticWindow struct {
	pool              *pgxpool.Pool
	history, protocol string
	cases             map[string]string
}

const structuralDiagnosticSafePriorSQL = `SELECT NOT EXISTS(SELECT 1 FROM fixture_structural_json_attempts a LEFT JOIN fixture_structural_json_retention r USING(ordinal) WHERE a.outcome<>'completed' OR NOT a.clean_lifecycle OR NOT a.metadata_known OR NOT COALESCE(r.retention_ok,false))`

func createStructuralDiagnosticWindow(ctx context.Context, pool *pgxpool.Pool, duration time.Duration) (*structuralDiagnosticWindow, error) {
	history, err := readScenarioHighV3History()
	cases, cerr := structuralDiagnosticInputs()
	if pool == nil || err != nil || cerr != nil || history.NativeAttempts != 266 || history.KnownUsagePairs != 240 || history.UnknownUsagePairs != 26 || duration <= 0 || duration > 8*time.Minute {
		return nil, errNativeFixtureDenied
	}
	const ddl = `CREATE TABLE IF NOT EXISTS fixture_structural_json_window(singleton boolean PRIMARY KEY CHECK(singleton),attempts int NOT NULL CHECK(attempts BETWEEN 266 AND 268),history_hash text NOT NULL,protocol_hash text NOT NULL,expires_at timestamptz NOT NULL);
CREATE TABLE IF NOT EXISTS fixture_structural_json_attempts(ordinal int PRIMARY KEY CHECK(ordinal BETWEEN 267 AND 268),case_id text NOT NULL UNIQUE,context_id text NOT NULL UNIQUE,input_hash text NOT NULL,admitted_at timestamptz NOT NULL DEFAULT clock_timestamp(),outcome text NOT NULL DEFAULT 'unavailable' CHECK(outcome IN('unavailable','completed','transport_error','cancelled','timed_out')),input_tokens bigint,output_tokens bigint,total_tokens bigint,model_version text,clean_lifecycle boolean NOT NULL DEFAULT false,metadata_known boolean NOT NULL DEFAULT false,finished_at timestamptz);
CREATE UNIQUE INDEX IF NOT EXISTS fixture_structural_json_one_pending ON fixture_structural_json_attempts((TRUE)) WHERE outcome='unavailable';
CREATE TABLE IF NOT EXISTS fixture_structural_json_retention(ordinal int PRIMARY KEY REFERENCES fixture_structural_json_attempts(ordinal),retention_ok boolean NOT NULL,visible_bytes int,visible_hash text,encoding_reason text,raw_retained boolean NOT NULL);
CREATE OR REPLACE FUNCTION fixture_structural_json_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR NEW.singleton IS DISTINCT FROM OLD.singleton OR NEW.history_hash IS DISTINCT FROM OLD.history_hash OR NEW.protocol_hash IS DISTINCT FROM OLD.protocol_hash OR NEW.expires_at IS DISTINCT FROM OLD.expires_at OR NEW.attempts<>OLD.attempts+1 THEN RAISE EXCEPTION 'sealed structural scope'; END IF; RETURN NEW; END $$;
CREATE OR REPLACE FUNCTION fixture_structural_json_attempt_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR OLD.outcome<>'unavailable' OR NEW.outcome='unavailable' OR NEW.ordinal IS DISTINCT FROM OLD.ordinal OR NEW.case_id IS DISTINCT FROM OLD.case_id OR NEW.context_id IS DISTINCT FROM OLD.context_id OR NEW.input_hash IS DISTINCT FROM OLD.input_hash OR NEW.admitted_at IS DISTINCT FROM OLD.admitted_at THEN RAISE EXCEPTION 'sealed structural attempt'; END IF; RETURN NEW; END $$;
CREATE OR REPLACE FUNCTION fixture_structural_json_retention_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'sealed structural retention'; END $$;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='structural_window_guard' AND tgrelid='fixture_structural_json_window'::regclass) THEN CREATE TRIGGER structural_window_guard BEFORE UPDATE OR DELETE ON fixture_structural_json_window FOR EACH ROW EXECUTE FUNCTION fixture_structural_json_guard(); END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='structural_attempt_guard' AND tgrelid='fixture_structural_json_attempts'::regclass) THEN CREATE TRIGGER structural_attempt_guard BEFORE UPDATE OR DELETE ON fixture_structural_json_attempts FOR EACH ROW EXECUTE FUNCTION fixture_structural_json_attempt_guard(); END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='structural_retention_guard' AND tgrelid='fixture_structural_json_retention'::regclass) THEN CREATE TRIGGER structural_retention_guard BEFORE UPDATE OR DELETE ON fixture_structural_json_retention FOR EACH ROW EXECUTE FUNCTION fixture_structural_json_retention_guard(); END IF;
END $$;`
	// Spaces in these fixed SQL keywords are kept explicit below.
	if _, err = pool.Exec(ctx, ddl); err != nil {
		return nil, errNativeFixtureDenied
	}
	protocol := structuralDiagnosticProtocolSHA()
	if _, err = pool.Exec(ctx, `INSERT INTO fixture_structural_json_window VALUES(TRUE,266,$1,$2,clock_timestamp()+$3*interval '1 millisecond') ON CONFLICT DO NOTHING`, history.BaselineSHA256, protocol, duration.Milliseconds()); err != nil {
		return nil, errNativeFixtureDenied
	}
	var compatible bool
	if pool.QueryRow(ctx, `SELECT history_hash=$1 AND protocol_hash=$2 FROM fixture_structural_json_window WHERE singleton`, history.BaselineSHA256, protocol).Scan(&compatible) != nil || !compatible {
		return nil, errNativeFixtureDenied
	}
	w := &structuralDiagnosticWindow{pool: pool, history: history.BaselineSHA256, protocol: protocol, cases: map[string]string{}}
	for _, c := range cases {
		w.cases[c.ID] = shadowV3Hash(c)
	}
	return w, nil
}

func structuralDiagnosticContext(id string) string {
	return "structural-" + shadowV3Hash([]string{structuralDiagnosticWindowID, id})[:32]
}

func (w *structuralDiagnosticWindow) reserve(ctx context.Context, c acceptance.CloudInputCase, input acceptance.CloudV3ModelInput) (int, error) {
	history, err := readScenarioHighV3History()
	_, ierr := structuralDiagnosticInputs()
	expected, merr := acceptance.MakeCloudV3ModelInput(c, acceptance.CloudRoleSingle, nil)
	if w == nil || err != nil || ierr != nil || merr != nil || history.BaselineSHA256 != w.history || w.cases[c.ID] != shadowV3Hash(c) || !nativeEqualJSON(input, expected) {
		return 0, errNativeFixtureDenied
	}
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return 0, errNativeFixtureDenied
	}
	defer tx.Rollback(ctx)
	var safe bool
	if tx.QueryRow(ctx, structuralDiagnosticSafePriorSQL).Scan(&safe) != nil || !safe {
		return 0, errNativeFixtureDenied
	}
	var ordinal int
	if tx.QueryRow(ctx, `UPDATE fixture_structural_json_window SET attempts=attempts+1 WHERE singleton AND attempts<268 AND history_hash=$1 AND protocol_hash=$2 AND expires_at>clock_timestamp() RETURNING attempts`, w.history, w.protocol).Scan(&ordinal) != nil {
		return 0, errNativeFixtureDenied
	}
	if tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() FROM fixture_structural_json_window WHERE singleton`).Scan(&safe) != nil || !safe {
		return 0, errNativeFixtureDenied
	}
	// The UPDATE may have waited behind another admission. Every prior result,
	// source byte and history pin must still be current after that wait.
	if tx.QueryRow(ctx, structuralDiagnosticSafePriorSQL).Scan(&safe) != nil || !safe {
		return 0, errNativeFixtureDenied
	}
	freshHistory, herr := readScenarioHighV3History()
	_, ferr := structuralDiagnosticInputs()
	if herr != nil || ferr != nil || freshHistory.BaselineSHA256 != w.history {
		return 0, errNativeFixtureDenied
	}
	if _, err = tx.Exec(ctx, `INSERT INTO fixture_structural_json_attempts(ordinal,case_id,context_id,input_hash) VALUES($1,$2,$3,$4)`, ordinal, c.ID, structuralDiagnosticContext(c.ID), acceptance.CloudV3ModelInputDigest(input)); err != nil || tx.Commit(ctx) != nil {
		return 0, errNativeFixtureDenied
	}
	return ordinal, nil
}

type structuralDiagnosticAttempt struct {
	Ordinal        int                      `json:"ordinal"`
	Context        string                   `json:"context_id"`
	InputSHA       string                   `json:"input_sha256"`
	Outcome        string                   `json:"outcome"`
	Input          *int64                   `json:"input_tokens"`
	Output         *int64                   `json:"output_tokens"`
	Total          *int64                   `json:"total_tokens"`
	Version        *string                  `json:"model_version"`
	CleanLifecycle bool                     `json:"clean_lifecycle"`
	MetadataKnown  bool                     `json:"metadata_known"`
	Final          *privateFinalMetadata    `json:"visible_final_metadata"`
	RetentionOK    bool                     `json:"retention_ok"`
	Stage          *transport.ProviderStage `json:"provider_operation_stage"`
}

func collectStructuralDiagnostic(ctx context.Context, w *structuralDiagnosticWindow, provider transport.ModelProvider, private string, c acceptance.CloudInputCase) (structuralDiagnosticAttempt, error) {
	input, err := acceptance.MakeCloudV3ModelInput(c, acceptance.CloudRoleSingle, nil)
	if err != nil {
		return structuralDiagnosticAttempt{}, errNativeFixtureDenied
	}
	ordinal, err := w.reserve(ctx, c, input)
	if err != nil {
		return structuralDiagnosticAttempt{}, errNativeFixtureDenied
	}
	a := structuralDiagnosticAttempt{Ordinal: ordinal, Context: structuralDiagnosticContext(c.ID), InputSHA: acceptance.CloudV3ModelInputDigest(input), Outcome: "transport_error"}
	call, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	call = transport.WithInputByteLimit(call, 65536)
	var stageMu sync.Mutex
	var lastStage *transport.ProviderStage
	call = transport.WithProviderStageObserver(call, func(stage transport.ProviderStage) {
		stageMu.Lock()
		copy := stage
		lastStage = &copy
		stageMu.Unlock()
	})
	user, _ := json.Marshal(struct {
		CaseID    string            `json:"case_id"`
		Input     string            `json:"input"`
		Documents map[string]string `json:"documents"`
	}{c.ID, c.Input, c.Documents})
	observer := &privateFinalCaptureProvider{ModelProvider: provider, private: private, ordinal: ordinal}
	stream, startErr := observer.StartRun(call, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: a.Context, SessionKey: "structural:" + c.ID, ProviderName: antigravity.Name, ModelName: antigravity.FlashHighModel, SupportsStreaming: true}, StreamingEnabled: true, InputItems: []transport.InputItem{{Role: "system", Content: input.SystemInstructions}, {Role: "user", Content: string(user)}}})
	clean := startErr == nil && stream != nil
	finals, terminals := 0, 0
	var known *transport.Usage
	usageUncertain, versionUncertain := false, false
	if startErr == nil && stream != nil {
	read:
		for {
			select {
			case <-call.Done():
				clean = false
				break read
			case event, open := <-stream:
				if !open {
					break read
				}
				if event.RunID != a.Context {
					clean = false
					continue
				}
				if terminals > 0 || event.AssistantFinal != nil && event.EventType != transport.EventTypeAssistantFinal {
					clean = false
				}
				switch event.EventType {
				case transport.EventTypeRunStarted, transport.EventTypeProviderSessionBound, transport.EventTypeAssistantDelta, transport.EventTypeTransportWarning:
				case transport.EventTypeAssistantFinal:
					finals++
					if finals != 1 || event.AssistantFinal == nil || event.AssistantFinal.FinishReason != "STOP" {
						clean = false
					}
				case transport.EventTypeRunCompleted:
					terminals++
					if finals != 1 || terminals != 1 {
						clean = false
					}
				default:
					clean = false
				}
				if event.ToolCall != nil || event.ToolCallBatch != nil || event.TransportError != nil {
					clean = false
				}
				usage := event.Usage
				if event.AssistantFinal != nil && event.AssistantFinal.Usage != nil {
					if usage != nil && *usage != *event.AssistantFinal.Usage {
						usageUncertain = true
					}
					if usage == nil {
						usage = event.AssistantFinal.Usage
					}
				}
				complete := event.EventType == transport.EventTypeAssistantFinal || event.EventType == transport.EventTypeRunCompleted || event.EventType == transport.EventTypeTransportError || event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested
				if usage != nil {
					if !complete || usage.Validate() != nil || known != nil && *known != *usage {
						usageUncertain = true
					} else if known == nil {
						copy := *usage
						known = &copy
					}
				}
				if event.ProviderModelVersion != nil {
					if !complete || a.Version != nil && *a.Version != *event.ProviderModelVersion {
						versionUncertain = true
					}
					if !versionUncertain {
						copy := *event.ProviderModelVersion
						a.Version = &copy
					}
				}
			}
		}
	}
	stageMu.Lock()
	if lastStage != nil {
		copy := *lastStage
		a.Stage = &copy
	}
	stageMu.Unlock()
	if known != nil && !usageUncertain {
		in, out, total := int64(known.InputTokens), int64(known.OutputTokens), int64(known.TotalTokens)
		a.Input, a.Output = &in, &out
		if total > 0 || in+out == 0 {
			a.Total = &total
		}
	}
	if versionUncertain {
		a.Version = nil
	}
	a.CleanLifecycle = clean && finals == 1 && terminals == 1 && call.Err() == nil
	a.MetadataKnown = a.Input != nil && a.Output != nil && a.Total != nil && a.Version != nil && *a.Version == structuralDiagnosticExpectedVersion && *a.Input <= 65536 && *a.Output <= 32768 && *a.Total <= *a.Input+32768
	if a.CleanLifecycle && a.MetadataKnown {
		a.Outcome = "completed"
	}
	if call.Err() != nil {
		a.Outcome = "cancelled"
		if call.Err() == context.DeadlineExceeded {
			a.Outcome = "timed_out"
		}
	}
	write, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	tag, err := w.pool.Exec(write, `UPDATE fixture_structural_json_attempts SET outcome=$2,input_tokens=$3,output_tokens=$4,total_tokens=$5,model_version=$6,clean_lifecycle=$7,metadata_known=$8,finished_at=clock_timestamp() WHERE ordinal=$1 AND outcome='unavailable'`, ordinal, a.Outcome, a.Input, a.Output, a.Total, a.Version, a.CleanLifecycle, a.MetadataKnown)
	if err != nil || tag.RowsAffected() != 1 {
		return a, errNativeFixtureDenied
	}
	// Only now may raw filesystem I/O occur; its failure never hides SQL spend.
	retentionErr := observer.persistFinal()
	a.Final = observer.finalMetadata()
	a.RetentionOK = retentionErr == nil && !observer.captureFailure() && a.Final != nil && a.Final.RawRetained && a.Final.Bytes <= privateFinalMaximumBytes
	var size *int
	var hash, reason *string
	rawRetained := false
	if a.Final != nil {
		size = &a.Final.Bytes
		hash = &a.Final.SHA256
		reason = &a.Final.Reason
		rawRetained = a.Final.RawRetained
	}
	if _, err = w.pool.Exec(write, `INSERT INTO fixture_structural_json_retention VALUES($1,$2,$3,$4,$5,$6)`, ordinal, a.RetentionOK, size, hash, reason, rawRetained); err != nil {
		return a, errNativeFixtureDenied
	}
	if !a.CleanLifecycle || !a.MetadataKnown || !a.RetentionOK {
		return a, errNativeFixtureDenied
	}
	return a, nil // Encoding-only failure is a recorded diagnostic, never quality PASS.
}

func TestScenarioStructuralJSONOperationalGate(t *testing.T) {
	if os.Getenv("BUTLER_STRUCTURAL_JSON_GATE") != "1" {
		t.Skip("explicit future diagnostic gate only")
	}
	if structuralDiagnosticGate() != nil {
		t.Fatal("structural diagnostic protocol/source has not been independently frozen")
	}
}

func TestScenarioStructuralJSONActualHighE2E(t *testing.T) {
	if os.Getenv("BUTLER_STRUCTURAL_JSON_ACCEPTANCE") != "1" {
		t.Skip("separate future2 structural window only")
	}
	if structuralDiagnosticGate() != nil {
		t.Fatal("reviewed diagnostic source denied before auth")
	}
	private := os.Getenv("BUTLER_STRUCTURAL_JSON_PRIVATE")
	if private != "/private" {
		t.Fatal("fixed private mount required")
	}
	deadline, derr := time.Parse(time.RFC3339, os.Getenv("BUTLER_STRUCTURAL_JSON_DEADLINE"))
	remaining := time.Until(deadline)
	if derr != nil || remaining <= 0 || remaining > 8*time.Minute {
		t.Fatal("fixed global deadline denied")
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
	if err != nil || cfg.ConnConfig.Database != "butler_structural_json_test" {
		t.Fatal("isolated diagnostic database required")
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("diagnostic store unavailable")
	}
	defer pool.Close()
	for _, path := range structuralDiagnosticMigrationPaths() {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal("migration bytes missing")
		}
		if _, err = pool.Exec(ctx, string(raw)); err != nil {
			t.Fatal("isolated migration denied")
		}
	}
	w, err := createStructuralDiagnosticWindow(ctx, pool, time.Until(deadline))
	if err != nil {
		t.Fatal("immutable diagnostic window denied")
	}
	cases, err := structuralDiagnosticInputs()
	if err != nil {
		t.Fatal("fixed public inputs denied")
	}
	factory, err := newShadowV3NativeFactory(ctx, pool, os.Getenv("BUTLER_STRUCTURAL_JSON_IMPORT_FILE"), filepath.Join(private, "encrypted-settings-key"))
	if err != nil {
		t.Fatal("private typed High provider unavailable")
	}
	attempts := []structuralDiagnosticAttempt{}
	status := "diagnostic_failed"
	defer func() {
		if err := writeStructuralDiagnosticReport(filepath.Join(private, "diagnostic.json"), struct {
			Status       string                        `json:"status"`
			Window       string                        `json:"window_id"`
			Baseline     int                           `json:"baseline"`
			Ceiling      int                           `json:"ceiling"`
			Attempts     []structuralDiagnosticAttempt `json:"attempts"`
			IssueReady   bool                          `json:"issue33_ready"`
			QualityClaim bool                          `json:"quality_claim"`
		}{status, structuralDiagnosticWindowID, 266, 268, attempts, false, false}); err != nil {
			t.Error("safe diagnostic report persistence failed; SQL spend retained")
		}
	}()
	for _, c := range cases {
		input, _ := acceptance.MakeCloudV3ModelInput(c, acceptance.CloudRoleSingle, nil)
		provider, err := factory(input.Schema)
		if err != nil {
			t.Fatal("trusted schema provider denied")
		}
		a, err := collectStructuralDiagnostic(ctx, w, provider, private, c)
		if a.Ordinal > 0 {
			attempts = append(attempts, a)
		}
		if err != nil {
			t.Fatal("diagnostic lifecycle/metadata/retention denied; spent evidence retained")
		}
	}
	status = "diagnostic_completed"
}

func writeStructuralDiagnosticReport(path string, report any) error {
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return errNativeFixtureDenied
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errNativeFixtureDenied
	}
	_, werr := f.Write(raw)
	serr := f.Sync()
	cerr := f.Close()
	if werr != nil || serr != nil || cerr != nil {
		return errNativeFixtureDenied
	}
	return nil
}

func structuralDiagnosticMigrationPaths() []string {
	paths, _ := filepath.Glob(filepath.Join(shadowRepoRoot(), "migrations", "*.up.sql"))
	return paths
}
