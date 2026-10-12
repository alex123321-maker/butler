package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

const schemaDiagnosticWindowID = "issue33-high-schema-dialect-20261013-baseline279"
const schemaDiagnosticInputSHA = "f1d1ca8172e7fd1c794b531f9e73ac02895967dca87c3313963121cedfed89fc"
const schemaDiagnosticReviewedSHA = ""                       // Closed until independent protocol/source/data review; never quality admission.
const schemaDiagnosticExpectedVersion = "gemini-3.8-flash-n" // Observed in all18 prior V3 packets.

type schemaDiagnosticEntry struct {
	acceptance.CloudInputCase
	Dialect string `json:"schema_dialect"`
}
type schemaDiagnosticInput struct {
	acceptance.CloudV4ModelInput
	Dialect string `json:"schema_dialect"`
}

func schemaDiagnosticProtocolSHA() string {
	entries, err := schemaDiagnosticInputs()
	if err != nil {
		return ""
	}
	inputs := []schemaDiagnosticInput{}
	for _, e := range entries {
		input, err := makeSchemaDiagnosticInput(e)
		if err != nil {
			return ""
		}
		inputs = append(inputs, input)
	}
	code := map[string]string{}
	for _, path := range []string{"internal/transport/antigravity/generation.go", "internal/transport/antigravity/protocol.go", "internal/transport/antigravity/provider.go", "apps/orchestrator/internal/app/scenario_schema_dialect_provider_test.go", "apps/orchestrator/internal/app/scenario_private_final_capture_test.go"} {
		raw, err := os.ReadFile(filepath.Join(shadowRepoRoot(), path))
		if err != nil {
			return ""
		}
		code[path] = privateFinalFacts(raw).SHA256
	}
	return shadowV4Hash(struct {
		Window, Inputs, Model, ExpectedVersion, SchemaSHA, History                                            string
		Baseline, Ceiling, Seconds, AttemptSeconds, Generated, Visible, InputBytes, Concurrency, Depth, Array int
		StartOnly, FourIndependent, SQLBeforeRaw                                                              bool
		Projection                                                                                            []schemaDiagnosticInput
		Code                                                                                                  map[string]string
	}{schemaDiagnosticWindowID, schemaDiagnosticInputSHA, antigravity.FlashHighModel, schemaDiagnosticExpectedVersion, acceptance.CloudV4SchemaDigest(), highV5HistorySHA, 279, 283, 600, 120, 32768, 8192, 65536, 1, 16, 32, true, true, true, inputs, code})
}

func schemaDiagnosticExamples() ([]acceptance.Example, error) {
	path := filepath.Join(shadowRepoRoot(), "testdata", "acceptance", "cloud-schema-dialect-diagnostic-20261013.json")
	raw, err := readNativeArtifact(path, schemaDiagnosticInputSHA)
	var fixture struct {
		Version    string               `json:"version"`
		Provenance string               `json:"provenance"`
		Purpose    string               `json:"purpose"`
		Cases      []acceptance.Example `json:"cases"`
	}
	if err != nil || acceptance.DecodeStrict(raw, &fixture, "version", "provenance", "purpose", "cases") != nil || len(fixture.Cases) != 2 || fixture.Version != "butler-cloud-schema-dialect-diagnostic-inputs-v1" || fixture.Provenance != "synthetic_public" || fixture.Purpose != "unseen_schema_dialect_observations_not_quality" {
		return nil, errNativeFixtureDenied
	}
	for _, e := range fixture.Cases {
		check := acceptance.VerifyCloudV4Response(e, e.Golden)
		if e.Kind != "extraction" || e.NoModel || len(e.Documents) != 2 || e.Input == "" || !check.Exact || check.SchemaError || check.SourceFabrication || check.UnsafeAuthority {
			return nil, errNativeFixtureDenied
		}
	}
	return fixture.Cases, nil
}

func schemaDiagnosticInputs() ([]schemaDiagnosticEntry, error) {
	examples, err := schemaDiagnosticExamples()
	if err != nil {
		return nil, err
	}
	inputs := []schemaDiagnosticEntry{}
	for _, e := range examples {
		x := acceptance.CloudInputCase{ID: "case:" + shadowV4Hash(acceptance.CloudV4Protocol + "\x00" + e.ID)[:32], Kind: e.Kind, Input: e.Input, Documents: e.Documents}
		x.OriginalInputSHA256 = shadowV4Hash(x)
		for _, dialect := range []string{schemaDialectJSON, schemaDialectGoogle} {
			inputs = append(inputs, schemaDiagnosticEntry{x, dialect}) // Golden/provenance/purpose never enter model input.
		}
	}
	return inputs, nil
}

func makeSchemaDiagnosticInput(e schemaDiagnosticEntry) (schemaDiagnosticInput, error) {
	if e.Dialect != schemaDialectJSON && e.Dialect != schemaDialectGoogle {
		return schemaDiagnosticInput{}, errNativeFixtureDenied
	}
	x, err := acceptance.MakeCloudV4ModelInput(e.CloudInputCase, acceptance.CloudRoleSingle, nil)
	if err != nil || e.Kind != "extraction" || len(e.Documents) != 2 {
		return schemaDiagnosticInput{}, errNativeFixtureDenied
	}
	ids := []string{}
	for id := range e.Documents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	stringType := map[string]any{"type": "string"}
	if e.Dialect == schemaDialectGoogle {
		stringType = map[string]any{"type": "STRING"}
	}
	kind, arr := "object", "array"
	if e.Dialect == schemaDialectGoogle {
		kind, arr = "OBJECT", "ARRAY"
	}
	sourceType := map[string]any{"type": stringType["type"], "enum": ids}
	object := func(p map[string]any, r []string) map[string]any {
		m := map[string]any{"type": kind, "properties": p, "required": r}
		if e.Dialect == schemaDialectJSON {
			m["additionalProperties"] = false
		}
		return m
	}
	facts := map[string]any{"type": arr, "items": object(map[string]any{"key": stringType, "value": stringType, "source_id": sourceType, "quote": stringType}, []string{"key", "value", "source_id", "quote"}), "maxItems": 32}
	if e.Dialect == schemaDialectGoogle {
		facts["maxItems"] = "32"
	}
	x.Schema, err = json.Marshal(object(map[string]any{"facts": facts}, []string{"facts"}))
	return schemaDiagnosticInput{x, e.Dialect}, err
}

func schemaDiagnosticGate() error {
	if !nativeAuditHash.MatchString(schemaDiagnosticReviewedSHA) {
		return errNativeFixtureDenied
	}
	raw, err := readNativeArtifact(filepath.Join(shadowRepoRoot(), "deploy", "acceptance", "schema-dialect-diagnostic-approval-20261013.json"), schemaDiagnosticReviewedSHA)
	var approval struct {
		Version     string `json:"version"`
		Reviewer    string `json:"reviewer"`
		Author      string `json:"author"`
		ProtocolSHA string `json:"protocol_sha256"`
		InputSHA    string `json:"input_sha256"`
		HistorySHA  string `json:"history_sha256"`
	}
	history, herr := readScenarioHighV5History()
	if err != nil || herr != nil || acceptance.DecodeStrict(raw, &approval, "version", "reviewer", "author", "protocol_sha256", "input_sha256", "history_sha256") != nil || approval.Version != "butler-schema-dialect-protocol-review-v1" || approval.Reviewer == "" || approval.Author == "" || approval.Reviewer == approval.Author || approval.ProtocolSHA != schemaDiagnosticProtocolSHA() || approval.InputSHA != schemaDiagnosticInputSHA || approval.HistorySHA != history.BaselineSHA256 {
		return errNativeFixtureDenied
	}
	_, err = schemaDiagnosticInputs()
	return err
}

type schemaDiagnosticWindow struct {
	pool              *pgxpool.Pool
	history, protocol string
	cases             map[string]string
}

const schemaDiagnosticSafePriorSQL = `SELECT NOT EXISTS(SELECT 1 FROM fixture_schema_dialect_attempts a LEFT JOIN fixture_schema_dialect_retention r USING(ordinal) WHERE a.outcome<>'completed' OR NOT a.clean_lifecycle OR NOT a.metadata_known OR a.elapsed_ms IS NULL OR a.elapsed_ms>120000 OR NOT COALESCE(r.retention_ok,false) OR (r.verification_checked AND NOT r.semantic_ok))`

func createSchemaDiagnosticWindow(ctx context.Context, pool *pgxpool.Pool, duration time.Duration) (*schemaDiagnosticWindow, error) {
	deadline := time.Now().UTC().Add(duration)
	if parentDeadline, ok := ctx.Deadline(); ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline
	}
	history, err := readScenarioHighV5History()
	cases, cerr := schemaDiagnosticInputs()
	if pool == nil || err != nil || cerr != nil || history.NativeAttempts != 279 || history.KnownUsagePairs != 253 || history.UnknownUsagePairs != 26 || duration <= 0 || duration > 10*time.Minute || !nativeAuditHash.MatchString(schemaDiagnosticProtocolSHA()) {
		return nil, errNativeFixtureDenied
	}
	const ddl = `CREATE TABLE IF NOT EXISTS fixture_schema_dialect_window(singleton boolean PRIMARY KEY CHECK(singleton),attempts int NOT NULL CHECK(attempts BETWEEN 279 AND 283),history_hash text NOT NULL,protocol_hash text NOT NULL,expires_at timestamptz NOT NULL);
CREATE TABLE IF NOT EXISTS fixture_schema_dialect_attempts(ordinal int PRIMARY KEY CHECK(ordinal BETWEEN 280 AND 283),case_id text NOT NULL,dialect text NOT NULL CHECK(dialect IN('json_schema_response_json_schema_v1','native_openapi_response_schema_v1')),context_id text NOT NULL UNIQUE,input_hash text NOT NULL CHECK(length(input_hash)=64),system_hash text NOT NULL CHECK(length(system_hash)=64),user_hash text NOT NULL CHECK(length(user_hash)=64),schema_hash text NOT NULL CHECK(length(schema_hash)=64),admitted_at timestamptz NOT NULL DEFAULT clock_timestamp(),outcome text NOT NULL DEFAULT 'unavailable' CHECK(outcome IN('unavailable','completed','transport_error','cancelled','timed_out')),input_tokens bigint CHECK(input_tokens>=0),output_tokens bigint CHECK(output_tokens>=0),total_tokens bigint CHECK(total_tokens>=input_tokens+output_tokens),model_version text,clean_lifecycle boolean NOT NULL DEFAULT false,metadata_known boolean NOT NULL DEFAULT false,finished_at timestamptz,elapsed_ms bigint CHECK(elapsed_ms>=0),provider_stage text,error_category text,final_count int NOT NULL DEFAULT 0 CHECK(final_count>=0),terminal_count int NOT NULL DEFAULT 0 CHECK(terminal_count>=0),finish_reason text CHECK(finish_reason IN('STOP','MAX_TOKENS')),UNIQUE(case_id,dialect));
CREATE UNIQUE INDEX IF NOT EXISTS fixture_schema_dialect_one_pending ON fixture_schema_dialect_attempts((TRUE)) WHERE outcome='unavailable';
CREATE TABLE IF NOT EXISTS fixture_schema_dialect_retention(ordinal int PRIMARY KEY REFERENCES fixture_schema_dialect_attempts(ordinal),retention_ok boolean NOT NULL,visible_bytes int,visible_hash text,encoding_reason text,raw_retained boolean NOT NULL,verification_checked boolean NOT NULL,semantic_ok boolean NOT NULL,verification_json jsonb NOT NULL,error_category text);
CREATE OR REPLACE FUNCTION fixture_schema_dialect_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR NEW.singleton IS DISTINCT FROM OLD.singleton OR NEW.history_hash IS DISTINCT FROM OLD.history_hash OR NEW.protocol_hash IS DISTINCT FROM OLD.protocol_hash OR NEW.expires_at IS DISTINCT FROM OLD.expires_at OR NEW.attempts<>OLD.attempts+1 THEN RAISE EXCEPTION 'sealed schema dialect scope'; END IF; RETURN NEW; END $$;
CREATE OR REPLACE FUNCTION fixture_schema_dialect_attempt_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR OLD.outcome<>'unavailable' OR NEW.outcome='unavailable' OR NEW.ordinal IS DISTINCT FROM OLD.ordinal OR NEW.case_id IS DISTINCT FROM OLD.case_id OR NEW.dialect IS DISTINCT FROM OLD.dialect OR NEW.context_id IS DISTINCT FROM OLD.context_id OR NEW.input_hash IS DISTINCT FROM OLD.input_hash OR NEW.system_hash IS DISTINCT FROM OLD.system_hash OR NEW.user_hash IS DISTINCT FROM OLD.user_hash OR NEW.schema_hash IS DISTINCT FROM OLD.schema_hash OR NEW.admitted_at IS DISTINCT FROM OLD.admitted_at THEN RAISE EXCEPTION 'sealed schema dialect attempt'; END IF; RETURN NEW; END $$;
CREATE OR REPLACE FUNCTION fixture_schema_dialect_retention_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'sealed schema dialect retention'; END $$;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='schema_dialect_window_guard' AND tgrelid='fixture_schema_dialect_window'::regclass) THEN CREATE TRIGGER schema_dialect_window_guard BEFORE UPDATE OR DELETE ON fixture_schema_dialect_window FOR EACH ROW EXECUTE FUNCTION fixture_schema_dialect_guard(); END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='schema_dialect_attempt_guard' AND tgrelid='fixture_schema_dialect_attempts'::regclass) THEN CREATE TRIGGER schema_dialect_attempt_guard BEFORE UPDATE OR DELETE ON fixture_schema_dialect_attempts FOR EACH ROW EXECUTE FUNCTION fixture_schema_dialect_attempt_guard(); END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='schema_dialect_retention_guard' AND tgrelid='fixture_schema_dialect_retention'::regclass) THEN CREATE TRIGGER schema_dialect_retention_guard BEFORE UPDATE OR DELETE ON fixture_schema_dialect_retention FOR EACH ROW EXECUTE FUNCTION fixture_schema_dialect_retention_guard(); END IF;
END $$;`
	// Spaces in these fixed SQL keywords are kept explicit below.
	if _, err = pool.Exec(ctx, ddl); err != nil {
		return nil, errNativeFixtureDenied
	}
	protocol := schemaDiagnosticProtocolSHA()
	if _, err = pool.Exec(ctx, `INSERT INTO fixture_schema_dialect_window VALUES(TRUE,279,$1,$2,$3) ON CONFLICT DO NOTHING`, history.BaselineSHA256, protocol, deadline); err != nil {
		return nil, errNativeFixtureDenied
	}
	var compatible bool
	if pool.QueryRow(ctx, `SELECT history_hash=$1 AND protocol_hash=$2 FROM fixture_schema_dialect_window WHERE singleton`, history.BaselineSHA256, protocol).Scan(&compatible) != nil || !compatible {
		return nil, errNativeFixtureDenied
	}
	w := &schemaDiagnosticWindow{pool: pool, history: history.BaselineSHA256, protocol: protocol, cases: map[string]string{}}
	for _, c := range cases {
		w.cases[schemaDiagnosticContext(c)] = shadowV4Hash(c)
	}
	return w, nil
}

func schemaDiagnosticContext(e schemaDiagnosticEntry) string {
	return "schema-dialect-" + shadowV4Hash([]string{schemaDiagnosticWindowID, e.ID, e.Dialect})[:32]
}

func (w *schemaDiagnosticWindow) reserve(ctx context.Context, c schemaDiagnosticEntry, input schemaDiagnosticInput) (int, error) {
	history, err := readScenarioHighV5History()
	entries, ierr := schemaDiagnosticInputs()
	expected, merr := makeSchemaDiagnosticInput(c)
	if w == nil || err != nil || ierr != nil || merr != nil || history.BaselineSHA256 != w.history || w.cases[schemaDiagnosticContext(c)] != shadowV4Hash(c) || !nativeEqualJSON(input, expected) || w.protocol != schemaDiagnosticProtocolSHA() {
		return 0, errNativeFixtureDenied
	}
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return 0, errNativeFixtureDenied
	}
	defer tx.Rollback(ctx)
	var safe bool
	if tx.QueryRow(ctx, schemaDiagnosticSafePriorSQL).Scan(&safe) != nil || !safe {
		return 0, errNativeFixtureDenied
	}
	var ordinal int
	if tx.QueryRow(ctx, `UPDATE fixture_schema_dialect_window SET attempts=attempts+1 WHERE singleton AND attempts<283 AND history_hash=$1 AND protocol_hash=$2 AND expires_at>clock_timestamp() RETURNING attempts`, w.history, w.protocol).Scan(&ordinal) != nil {
		return 0, errNativeFixtureDenied
	}
	if tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() FROM fixture_schema_dialect_window WHERE singleton`).Scan(&safe) != nil || !safe {
		return 0, errNativeFixtureDenied
	}
	if ordinal < 280 || ordinal > 283 || schemaDiagnosticContext(entries[ordinal-280]) != schemaDiagnosticContext(c) {
		return 0, errNativeFixtureDenied
	}
	// The UPDATE may have waited behind another admission. Every prior result,
	// source byte and history pin must still be current after that wait.
	if tx.QueryRow(ctx, schemaDiagnosticSafePriorSQL).Scan(&safe) != nil || !safe {
		return 0, errNativeFixtureDenied
	}
	freshHistory, herr := readScenarioHighV5History()
	_, ferr := schemaDiagnosticInputs()
	if herr != nil || ferr != nil || freshHistory.BaselineSHA256 != w.history {
		return 0, errNativeFixtureDenied
	}
	user, _ := schemaDiagnosticUser(c)
	if _, err = tx.Exec(ctx, `INSERT INTO fixture_schema_dialect_attempts(ordinal,case_id,dialect,context_id,input_hash,system_hash,user_hash,schema_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ordinal, c.ID, c.Dialect, schemaDiagnosticContext(c), shadowV4Hash(input), privateFinalFacts([]byte(input.SystemInstructions)).SHA256, privateFinalFacts(user).SHA256, privateFinalFacts(input.Schema).SHA256); err != nil || tx.Commit(ctx) != nil {
		return 0, errNativeFixtureDenied
	}
	return ordinal, nil
}

type schemaDiagnosticAttempt struct {
	Ordinal             int                           `json:"ordinal"`
	CaseID              string                        `json:"case_id"`
	Dialect             string                        `json:"schema_dialect"`
	Context             string                        `json:"context_id"`
	InputSHA            string                        `json:"input_sha256"`
	SystemSHA           string                        `json:"system_sha256"`
	UserSHA             string                        `json:"user_sha256"`
	SchemaSHA           string                        `json:"schema_sha256"`
	Outcome             string                        `json:"outcome"`
	Input               *int64                        `json:"input_tokens"`
	Output              *int64                        `json:"output_tokens"`
	Total               *int64                        `json:"total_tokens"`
	Version             *string                       `json:"model_version"`
	CleanLifecycle      bool                          `json:"clean_lifecycle"`
	MetadataKnown       bool                          `json:"metadata_known"`
	Final               *privateFinalMetadata         `json:"visible_final_metadata"`
	RetentionOK         bool                          `json:"retention_ok"`
	Stage               *transport.ProviderStage      `json:"provider_operation_stage"`
	Elapsed             int64                         `json:"elapsed_ms"`
	ErrorCategory       string                        `json:"error_category"`
	VerificationChecked bool                          `json:"verification_checked"`
	Verification        acceptance.CloudResponseCheck `json:"original_witness_check"`
	FinalCount          int                           `json:"final_count"`
	TerminalCount       int                           `json:"terminal_count"`
	FinishReason        *string                       `json:"finish_reason"`
}

func schemaDiagnosticUser(c schemaDiagnosticEntry) ([]byte, error) {
	return json.Marshal(struct {
		CaseID    string            `json:"case_id"`
		Input     string            `json:"input"`
		Documents map[string]string `json:"documents"`
	}{c.ID, c.Input, c.Documents})
}

func collectSchemaDiagnostic(ctx context.Context, w *schemaDiagnosticWindow, provider transport.ModelProvider, private string, c schemaDiagnosticEntry) (schemaDiagnosticAttempt, error) {
	input, err := makeSchemaDiagnosticInput(c)
	if err != nil {
		return schemaDiagnosticAttempt{}, errNativeFixtureDenied
	}
	ordinal, err := w.reserve(ctx, c, input)
	if err != nil {
		return schemaDiagnosticAttempt{}, errNativeFixtureDenied
	}
	user, _ := schemaDiagnosticUser(c)
	a := schemaDiagnosticAttempt{Ordinal: ordinal, CaseID: c.ID, Dialect: c.Dialect, Context: schemaDiagnosticContext(c), InputSHA: shadowV4Hash(input), SystemSHA: privateFinalFacts([]byte(input.SystemInstructions)).SHA256, UserSHA: privateFinalFacts(user).SHA256, SchemaSHA: privateFinalFacts(input.Schema).SHA256, Outcome: "transport_error"}
	var deadline time.Time
	if w.pool.QueryRow(ctx, `SELECT LEAST(a.admitted_at+interval '120 seconds',w.expires_at) FROM fixture_schema_dialect_attempts a CROSS JOIN fixture_schema_dialect_window w WHERE a.ordinal=$1 AND w.singleton`, ordinal).Scan(&deadline) != nil {
		return a, errNativeFixtureDenied
	}
	call, cancel := context.WithDeadline(ctx, deadline)
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
	observer := &privateFinalCaptureProvider{ModelProvider: provider, private: private, ordinal: ordinal}
	stream, startErr := observer.StartRun(call, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: a.Context, SessionKey: a.Context, ProviderName: antigravity.Name, ModelName: antigravity.FlashHighModel, SupportsStreaming: true}, StreamingEnabled: true, InputItems: []transport.InputItem{{Role: "system", Content: input.SystemInstructions}, {Role: "user", Content: string(user)}}})
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer stop()
		_, _ = observer.CancelRun(cleanup, transport.CancelRunRequest{RunID: a.Context, Reason: "schema dialect observation finished"})
	}()
	clean := startErr == nil && stream != nil
	if startErr != nil {
		a.ErrorCategory = transport.DiagnosticCode(transport.NormalizeError(startErr, antigravity.Name))
	}
	finals, terminals := 0, 0
	var known *transport.Usage
	usageUncertain, versionUncertain := false, false
	finishUncertain := false
	usageReason, versionReason := "", ""
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
					if event.AssistantFinal == nil || event.AssistantFinal.FinishReason != "STOP" && event.AssistantFinal.FinishReason != "MAX_TOKENS" {
						finishUncertain = true
					} else {
						r := event.AssistantFinal.FinishReason
						if a.FinishReason != nil && *a.FinishReason != r {
							finishUncertain = true
						}
						a.FinishReason = &r
					}
					if finals != 1 || event.AssistantFinal == nil || event.AssistantFinal.FinishReason != "STOP" {
						clean = false
					}
				case transport.EventTypeRunCompleted:
					terminals++
					if finals != 1 || terminals != 1 {
						clean = false
					}
				case transport.EventTypeRunCancelled, transport.EventTypeRunTimedOut, transport.EventTypeTransportError:
					terminals++
					clean = false
					if event.EventType == transport.EventTypeRunCancelled {
						a.Outcome = "cancelled"
					} else if event.EventType == transport.EventTypeRunTimedOut {
						a.Outcome = "timed_out"
					}
				default:
					clean = false
				}
				if event.ToolCall != nil || event.ToolCallBatch != nil || event.TransportError != nil {
					clean = false
					if event.TransportError != nil {
						a.ErrorCategory = transport.DiagnosticCode(event.TransportError)
					}
				}
				usage := event.Usage
				if event.AssistantFinal != nil && event.AssistantFinal.Usage != nil {
					if usage != nil && *usage != *event.AssistantFinal.Usage {
						usageUncertain = true
						usageReason = "conflicting_usage"
					}
					if usage == nil {
						usage = event.AssistantFinal.Usage
					}
				}
				complete := event.EventType == transport.EventTypeAssistantFinal || event.EventType == transport.EventTypeRunCompleted || event.EventType == transport.EventTypeTransportError || event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested
				if usage != nil {
					if !complete || usage.Validate() != nil || known != nil && *known != *usage {
						usageUncertain = true
						usageReason = "invalid_usage"
						if !complete {
							usageReason = "provisional_usage"
						} else if known != nil && *known != *usage {
							usageReason = "conflicting_usage"
						}
					} else if known == nil {
						copy := *usage
						known = &copy
					}
				}
				if event.ProviderModelVersion != nil {
					if !complete || a.Version != nil && *a.Version != *event.ProviderModelVersion {
						versionUncertain = true
						versionReason = "conflicting_version"
						if !complete {
							versionReason = "provisional_version"
						}
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
	a.FinalCount, a.TerminalCount = finals, terminals
	if finishUncertain {
		a.FinishReason = nil
	}
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
	a.MetadataKnown = a.Input != nil && a.Output != nil && a.Total != nil && a.Version != nil && *a.Version == schemaDiagnosticExpectedVersion && *a.Input <= 65536 && *a.Output <= 32768 && *a.Total <= *a.Input+32768
	if a.CleanLifecycle && a.MetadataKnown {
		a.Outcome = "completed"
	}
	if call.Err() != nil {
		a.Outcome = "cancelled"
		if call.Err() == context.DeadlineExceeded {
			a.Outcome = "timed_out"
		}
	}
	if !a.CleanLifecycle && a.ErrorCategory == "" {
		a.ErrorCategory = "lifecycle_error"
	}
	if !a.MetadataKnown && a.ErrorCategory == "" {
		switch {
		case usageReason != "":
			a.ErrorCategory = usageReason
		case versionReason != "":
			a.ErrorCategory = versionReason
		case a.Input == nil || a.Output == nil:
			a.ErrorCategory = "missing_usage"
		case a.Total == nil:
			a.ErrorCategory = "missing_total"
		case a.Version == nil:
			a.ErrorCategory = "missing_version"
		case *a.Version != schemaDiagnosticExpectedVersion:
			a.ErrorCategory = "model_version_mismatch"
		case *a.Input > 65536:
			a.ErrorCategory = "input_bound"
		case *a.Output > 32768 || *a.Total > *a.Input+32768:
			a.ErrorCategory = "generation_bound"
		default:
			a.ErrorCategory = "metadata_unavailable"
		}
	}
	write, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	if err = w.pool.QueryRow(write, `WITH finished AS MATERIALIZED (SELECT clock_timestamp() AS at) UPDATE fixture_schema_dialect_attempts SET outcome=$2,input_tokens=$3,output_tokens=$4,total_tokens=$5,model_version=$6,clean_lifecycle=$7,metadata_known=$8,finished_at=finished.at,elapsed_ms=GREATEST(0,floor(extract(epoch FROM finished.at-admitted_at)*1000)::bigint),provider_stage=$9,error_category=NULLIF($10,''),final_count=$11,terminal_count=$12,finish_reason=$13 FROM finished WHERE ordinal=$1 AND outcome='unavailable' RETURNING elapsed_ms`, ordinal, a.Outcome, a.Input, a.Output, a.Total, a.Version, a.CleanLifecycle, a.MetadataKnown, a.Stage, a.ErrorCategory, a.FinalCount, a.TerminalCount, a.FinishReason).Scan(&a.Elapsed); err != nil {
		return a, errNativeFixtureDenied
	}
	observer.mu.Lock()
	rawFinal := append([]byte(nil), observer.raw...)
	observer.mu.Unlock()
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
	if a.Final != nil && a.Final.Reason == "json_object" {
		examples, e := schemaDiagnosticExamples()
		if e != nil {
			return a, errNativeFixtureDenied
		}
		for _, example := range examples {
			if c.ID == "case:"+shadowV4Hash(acceptance.CloudV4Protocol + "\x00" + example.ID)[:32] {
				a.VerificationChecked = true
				a.Verification = acceptance.VerifyCloudV4Response(example, rawFinal)
			}
		}
	}
	if !a.RetentionOK && a.ErrorCategory == "" {
		a.ErrorCategory = "raw_retention_error"
	}
	if a.Final != nil && a.Final.Reason != "json_object" && a.ErrorCategory == "" {
		a.ErrorCategory = "encoding_" + a.Final.Reason
	}
	if a.VerificationChecked && !a.Verification.Exact && a.ErrorCategory == "" {
		a.ErrorCategory = "original_witness_error"
	}
	if a.Elapsed > 120000 && a.ErrorCategory == "" {
		a.ErrorCategory = "attempt_deadline"
	}
	verification, _ := json.Marshal(a.Verification)
	if _, err = w.pool.Exec(write, `INSERT INTO fixture_schema_dialect_retention VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''))`, ordinal, a.RetentionOK, size, hash, reason, rawRetained, a.VerificationChecked, a.Verification.Exact, verification, a.ErrorCategory); err != nil {
		return a, errNativeFixtureDenied
	}
	if !a.CleanLifecycle || !a.MetadataKnown || !a.RetentionOK || a.Elapsed > 120000 || a.VerificationChecked && !a.Verification.Exact {
		return a, errNativeFixtureDenied
	}
	return a, nil // Encoding-only failure is a recorded diagnostic, never quality PASS.
}

func TestScenarioSchemaDialectOperationalGate(t *testing.T) {
	if os.Getenv("BUTLER_SCHEMA_DIALECT_GATE") != "1" {
		t.Skip("explicit future diagnostic gate only")
	}
	if schemaDiagnosticGate() != nil {
		t.Fatal("schema dialect diagnostic protocol/source has not been independently frozen")
	}
}

func TestScenarioSchemaDialectActualHighE2E(t *testing.T) {
	if os.Getenv("BUTLER_SCHEMA_DIALECT_ACCEPTANCE") != "1" {
		t.Skip("separate future4 schema dialect window only")
	}
	if schemaDiagnosticGate() != nil {
		t.Fatal("reviewed diagnostic source denied before auth")
	}
	private := os.Getenv("BUTLER_SCHEMA_DIALECT_PRIVATE")
	if private != "/private" {
		t.Fatal("fixed private mount required")
	}
	deadline, derr := time.Parse(time.RFC3339, os.Getenv("BUTLER_SCHEMA_DIALECT_DEADLINE"))
	remaining := time.Until(deadline)
	if derr != nil || remaining <= 0 || remaining > 10*time.Minute {
		t.Fatal("fixed global deadline denied")
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
	if err != nil || cfg.ConnConfig.Database != "butler_schema_dialect_test" {
		t.Fatal("isolated diagnostic database required")
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("diagnostic store unavailable")
	}
	defer pool.Close()
	for _, path := range schemaDiagnosticMigrationPaths() {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal("migration bytes missing")
		}
		if _, err = pool.Exec(ctx, string(raw)); err != nil {
			t.Fatal("isolated migration denied")
		}
	}
	w, err := createSchemaDiagnosticWindow(ctx, pool, time.Until(deadline))
	if err != nil {
		t.Fatal("immutable diagnostic window denied")
	}
	cases, err := schemaDiagnosticInputs()
	if err != nil {
		t.Fatal("fixed public inputs denied")
	}
	attempts := []schemaDiagnosticAttempt{}
	status := "diagnostic_failed"
	defer func() {
		if err := writeSchemaDiagnosticReport(filepath.Join(private, "diagnostic.json"), struct {
			Status       string                    `json:"status"`
			Window       string                    `json:"window_id"`
			Baseline     int                       `json:"baseline"`
			Ceiling      int                       `json:"ceiling"`
			Attempts     []schemaDiagnosticAttempt `json:"attempts"`
			IssueReady   bool                      `json:"issue33_ready"`
			QualityClaim bool                      `json:"quality_claim"`
		}{status, schemaDiagnosticWindowID, 279, 283, attempts, false, false}); err != nil {
			t.Error("safe diagnostic report persistence failed; SQL spend retained")
		}
	}()
	for _, c := range cases {
		input, _ := makeSchemaDiagnosticInput(c)
		factory, err := newSchemaDialectNativeFactory(ctx, pool, os.Getenv("BUTLER_SCHEMA_DIALECT_IMPORT_FILE"), filepath.Join(private, "encrypted-settings-key"), c.Dialect)
		if err != nil {
			t.Fatal("private typed High provider unavailable")
		}
		provider, err := factory(input.Schema)
		if err != nil {
			t.Fatal("trusted schema provider denied")
		}
		a, err := collectSchemaDiagnostic(ctx, w, provider, private, c)
		if a.Ordinal > 0 {
			attempts = append(attempts, a)
		}
		if err != nil {
			t.Fatal("diagnostic lifecycle/metadata/retention denied; spent evidence retained")
		}
	}
	status = "diagnostic_completed"
}

func writeSchemaDiagnosticReport(path string, report any) error {
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

func schemaDiagnosticMigrationPaths() []string {
	paths, _ := filepath.Glob(filepath.Join(shadowRepoRoot(), "migrations", "*.up.sql"))
	return paths
}
