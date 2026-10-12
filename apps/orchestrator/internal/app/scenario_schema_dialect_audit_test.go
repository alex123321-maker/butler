package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/transport"
)

type schemaDiagnosticReport struct {
	Status   string                    `json:"status"`
	Window   string                    `json:"window_id"`
	Baseline int                       `json:"baseline"`
	Ceiling  int                       `json:"ceiling"`
	Attempts []schemaDiagnosticAttempt `json:"attempts"`
	Ready    bool                      `json:"issue33_ready"`
	Quality  bool                      `json:"quality_claim"`
}

type schemaDiagnosticPGAttempt struct {
	Ordinal   int        `json:"ordinal"`
	CaseID    string     `json:"case_id"`
	Dialect   string     `json:"dialect"`
	Context   string     `json:"context_id"`
	InputSHA  string     `json:"input_hash"`
	SystemSHA string     `json:"system_hash"`
	UserSHA   string     `json:"user_hash"`
	SchemaSHA string     `json:"schema_hash"`
	Admitted  time.Time  `json:"admitted_at"`
	Outcome   string     `json:"outcome"`
	Input     *int64     `json:"input_tokens"`
	Output    *int64     `json:"output_tokens"`
	Total     *int64     `json:"total_tokens"`
	Version   *string    `json:"model_version"`
	Clean     bool       `json:"clean_lifecycle"`
	Known     bool       `json:"metadata_known"`
	Finished  *time.Time `json:"finished_at"`
	Elapsed   *int64     `json:"elapsed_ms"`
	Stage     *string    `json:"provider_stage"`
	Error     *string    `json:"error_category"`
	Finals    int        `json:"final_count"`
	Terminals int        `json:"terminal_count"`
	Finish    *string    `json:"finish_reason"`
}

type schemaDiagnosticPGRetention struct {
	Ordinal      int                           `json:"ordinal"`
	OK           bool                          `json:"retention_ok"`
	Bytes        *int                          `json:"visible_bytes"`
	Hash         *string                       `json:"visible_hash"`
	Reason       *string                       `json:"encoding_reason"`
	Retained     bool                          `json:"raw_retained"`
	Checked      bool                          `json:"verification_checked"`
	Semantic     bool                          `json:"semantic_ok"`
	Verification acceptance.CloudResponseCheck `json:"verification_json"`
	Error        *string                       `json:"error_category"`
}

type schemaDiagnosticPGWitness struct {
	Version string `json:"version"`
	Window  struct {
		Singleton bool      `json:"singleton"`
		Attempts  int       `json:"attempts"`
		History   string    `json:"history_hash"`
		Protocol  string    `json:"protocol_hash"`
		Expires   time.Time `json:"expires_at"`
	} `json:"window"`
	Attempts  []schemaDiagnosticPGAttempt   `json:"attempts"`
	Retention []schemaDiagnosticPGRetention `json:"retention"`
}

type schemaDiagnosticFinalAudit struct {
	Status        string                    `json:"status"`
	ReportSHA     string                    `json:"report_sha256"`
	PGSHA         string                    `json:"pg_witness_sha256"`
	HistorySHA    string                    `json:"previous_history_sha256"`
	ProtocolSHA   string                    `json:"protocol_sha256"`
	Baseline      int                       `json:"baseline"`
	New           int                       `json:"new_attempts"`
	Total         int                       `json:"carry_forward_attempts"`
	Rows          int                       `json:"exact_correlated_rows"`
	Raw           int                       `json:"exact_private_originals"`
	NativeExact   int                       `json:"native_original_witness_exact_count"`
	KnownInput    int64                     `json:"new_input_tokens"`
	KnownOutput   int64                     `json:"new_output_tokens"`
	KnownTotal    int64                     `json:"new_total_tokens"`
	UnknownInput  int                       `json:"unknown_input_attempts"`
	UnknownOutput int                       `json:"unknown_output_attempts"`
	UnknownTotal  int                       `json:"unknown_total_attempts"`
	Attempts      []schemaDiagnosticAttempt `json:"attempts"`
	Compatibility bool                      `json:"native_compatibility_passed"`
	Ready         bool                      `json:"issue33_ready"`
	Quality       bool                      `json:"quality_claim"`
	Calls         int                       `json:"auditor_native_calls"`
}

func schemaDiagnosticVerify(e schemaDiagnosticEntry, raw []byte) acceptance.CloudResponseCheck {
	examples, err := schemaDiagnosticExamples()
	if err == nil {
		for _, original := range examples {
			if e.ID == "case:"+shadowV4Hash(acceptance.CloudV4Protocol + "\x00" + original.ID)[:32] {
				return acceptance.VerifyCloudV4Response(original, raw)
			}
		}
	}
	return acceptance.CloudResponseCheck{SchemaError: true}
}

// Read-only correlation uses literal retained originals, never repaired output.
// The operator independently pins inputs and frozen source before this helper.
func auditSchemaDiagnostic(report schemaDiagnosticReport, pg schemaDiagnosticPGWitness, rawFinal func(int) ([]byte, error)) (schemaDiagnosticFinalAudit, error) {
	a := schemaDiagnosticFinalAudit{Status: "SCHEMA_DIALECT_CORRELATION_MATCH", HistorySHA: highV5HistorySHA, ProtocolSHA: schemaDiagnosticProtocolSHA(), Baseline: 279, New: len(pg.Attempts), Total: pg.Window.Attempts, Attempts: report.Attempts}
	deny := func() (schemaDiagnosticFinalAudit, error) { return a, errNativeFixtureDenied }
	entries, err := schemaDiagnosticInputs()
	if err != nil || report.Window != schemaDiagnosticWindowID || report.Baseline != 279 || report.Ceiling != 283 || report.Ready || report.Quality || report.Status != "diagnostic_completed" && report.Status != "diagnostic_failed" ||
		pg.Version != "butler-schema-dialect-pg-witness-v1" || !pg.Window.Singleton || pg.Window.History != highV5HistorySHA || pg.Window.Protocol != a.ProtocolSHA || pg.Window.Expires.IsZero() || pg.Window.Attempts != 279+len(pg.Attempts) || len(pg.Attempts) > 4 || len(pg.Attempts) != len(report.Attempts) || len(pg.Retention) != len(pg.Attempts) {
		return deny()
	}
	for i, p := range pg.Attempts {
		r := report.Attempts[i]
		ret := pg.Retention[i]
		e := entries[i]
		input, ierr := makeSchemaDiagnosticInput(e)
		user, _ := schemaDiagnosticUser(e)
		errorValue := ""
		if ret.Error != nil {
			errorValue = *ret.Error
		}
		if ierr != nil || p.Ordinal != 280+i || r.Ordinal != p.Ordinal || ret.Ordinal != p.Ordinal || p.CaseID != e.ID || r.CaseID != e.ID || p.Dialect != e.Dialect || r.Dialect != e.Dialect ||
			p.Context != schemaDiagnosticContext(e) || r.Context != p.Context || p.InputSHA != shadowV4Hash(input) || r.InputSHA != p.InputSHA || p.SystemSHA != privateFinalFacts([]byte(input.SystemInstructions)).SHA256 || r.SystemSHA != p.SystemSHA ||
			p.UserSHA != privateFinalFacts(user).SHA256 || r.UserSHA != p.UserSHA || p.SchemaSHA != privateFinalFacts(input.Schema).SHA256 || r.SchemaSHA != p.SchemaSHA || p.Outcome != r.Outcome ||
			!reflect.DeepEqual(p.Input, r.Input) || !reflect.DeepEqual(p.Output, r.Output) || !reflect.DeepEqual(p.Total, r.Total) || !reflect.DeepEqual(p.Version, r.Version) || p.Clean != r.CleanLifecycle || p.Known != r.MetadataKnown ||
			p.Elapsed == nil || *p.Elapsed != r.Elapsed || p.Finished == nil || p.Admitted.IsZero() || !p.Admitted.Before(pg.Window.Expires) || p.Finished.Before(p.Admitted) ||
			p.Finals != r.FinalCount || p.Terminals != r.TerminalCount || !reflect.DeepEqual(p.Finish, r.FinishReason) || !nativeEqualJSON(p.Stage, r.Stage) || errorValue != r.ErrorCategory || p.Error != nil && *p.Error != errorValue ||
			ret.OK != r.RetentionOK || ret.Checked != r.VerificationChecked || ret.Semantic != r.Verification.Exact || !reflect.DeepEqual(ret.Verification, r.Verification) {
			return deny()
		}
		measuredMS := p.Finished.Sub(p.Admitted).Milliseconds()
		if r.Elapsed < measuredMS || r.Elapsed-measuredMS > 1 {
			return deny()
		}
		metadataKnown := p.Input != nil && p.Output != nil && p.Total != nil && p.Version != nil && *p.Version == schemaDiagnosticExpectedVersion &&
			*p.Input >= 0 && *p.Input <= 65536 && *p.Output >= 0 && *p.Output <= 32768 && *p.Total >= *p.Input+*p.Output && *p.Total <= *p.Input+32768
		if p.Known != metadataKnown {
			return deny()
		}
		if p.Stage != nil {
			switch transport.ProviderStage(*p.Stage) {
			case transport.ProviderStageAuthentication, transport.ProviderStageSerialization, transport.ProviderStageHTTP, transport.ProviderStageStream:
			default:
				return deny()
			}
		}
		for _, cost := range []struct {
			v       *int64
			sum     *int64
			unknown *int
		}{{p.Input, &a.KnownInput, &a.UnknownInput}, {p.Output, &a.KnownOutput, &a.UnknownOutput}, {p.Total, &a.KnownTotal, &a.UnknownTotal}} {
			if cost.v == nil {
				*cost.unknown++
			} else if *cost.v < 0 || *cost.v > 1<<40-*cost.sum {
				return deny()
			} else {
				*cost.sum += *cost.v
			}
		}
		if r.Final != nil {
			if ret.Bytes == nil || ret.Hash == nil || ret.Reason == nil || *ret.Bytes != r.Final.Bytes || *ret.Hash != r.Final.SHA256 || *ret.Reason != r.Final.Reason || ret.Retained != r.Final.RawRetained {
				return deny()
			}
			if ret.Retained {
				raw, rerr := rawFinal(r.Ordinal)
				facts := privateFinalFacts(raw)
				if rerr != nil || facts.Bytes != r.Final.Bytes || facts.SHA256 != r.Final.SHA256 || facts.Reason != r.Final.Reason {
					return deny()
				}
				a.Raw++
				checked := facts.Reason == "json_object"
				check := acceptance.CloudResponseCheck{}
				if checked {
					check = schemaDiagnosticVerify(e, raw)
				}
				if checked != ret.Checked || !reflect.DeepEqual(check, ret.Verification) {
					return deny()
				}
				if e.Dialect == schemaDialectGoogle && checked && check.Exact && p.Outcome == "completed" && p.Clean && p.Known && ret.OK && p.Finals == 1 && p.Terminals == 1 && p.Finish != nil && *p.Finish == "STOP" && r.Elapsed <= 120000 && p.Version != nil && *p.Version == schemaDiagnosticExpectedVersion {
					a.NativeExact++
				}
			}
		} else if ret.Bytes != nil || ret.Hash != nil || ret.Reason != nil || ret.Retained || ret.Checked {
			return deny()
		}
		a.Rows++
	}
	a.Compatibility = report.Status == "diagnostic_completed" && a.New == 4 && a.Rows == 4 && a.Raw == 4 && a.NativeExact == 2 && a.UnknownInput == 0 && a.UnknownOutput == 0 && a.UnknownTotal == 0
	for i, p := range pg.Attempts {
		ret := pg.Retention[i]
		if p.Outcome != "completed" || !p.Clean || !p.Known || p.Finals != 1 || p.Terminals != 1 || p.Finish == nil || *p.Finish != "STOP" || p.Elapsed == nil || *p.Elapsed > 120000 || p.Finished == nil || p.Finished.After(p.Admitted.Add(120*time.Second)) || p.Finished.After(pg.Window.Expires) || p.Stage == nil || *p.Stage != string(transport.ProviderStageStream) || !ret.OK || !ret.Retained || ret.Checked && !ret.Semantic {
			a.Compatibility = false
		}
	}
	if a.Compatibility {
		a.Status = "SCHEMA_DIALECT_COMPATIBILITY_PASS"
	}
	return a, nil
}

// This process has only explicitly named report/PG/raw leaves, no import/key,
// no signing directory and no network. Stdout is content-free original evidence.
func TestScenarioSchemaDialectFinalAudit(t *testing.T) {
	if os.Getenv("BUTLER_SCHEMA_DIALECT_AUDIT") != "1" {
		t.Skip("explicit read-only original-final audit only")
	}
	if os.Geteuid() != 10001 || schemaDiagnosticGate() != nil {
		t.Fatal("independently frozen audit source denied")
	}
	read := func(path, env string) ([]byte, error) {
		raw, err := acceptance.ReadPrivateFile(path, 128<<10)
		if err != nil || !nativeAuditHash.MatchString(os.Getenv(env)) || privateFinalFacts(raw).SHA256 != os.Getenv(env) {
			return nil, errNativeFixtureDenied
		}
		return raw, nil
	}
	reportRaw, err := read("/report.json", "BUTLER_SCHEMA_DIALECT_REPORT_SHA")
	if err != nil {
		t.Fatal("exact private report denied")
	}
	pgRaw, err := read("/pg.json", "BUTLER_SCHEMA_DIALECT_PG_SHA")
	if err != nil {
		t.Fatal("exact separate PG witness denied")
	}
	source, err := acceptance.TreeHash(shadowRepoRoot(), nil)
	if err != nil || source != os.Getenv("BUTLER_SCHEMA_DIALECT_SOURCE_SHA") {
		t.Fatal("actual frozen source denied")
	}
	var report schemaDiagnosticReport
	var pg schemaDiagnosticPGWitness
	if acceptance.DecodeStrict(reportRaw, &report, "status", "window_id", "baseline", "ceiling", "attempts", "issue33_ready", "quality_claim") != nil || acceptance.DecodeStrict(pgRaw, &pg, "version", "window", "attempts", "retention") != nil {
		t.Fatal("bounded report/SQL inventory denied")
	}
	a, err := auditSchemaDiagnostic(report, pg, func(ordinal int) ([]byte, error) {
		return acceptance.ReadPrivateFile(filepath.Join("/raw", fmt.Sprintf("final-%03d.raw", ordinal)), 8192)
	})
	if err != nil {
		t.Fatal("original-final/SQL correlation denied")
	}
	a.ReportSHA, a.PGSHA = privateFinalFacts(reportRaw).SHA256, privateFinalFacts(pgRaw).SHA256
	raw, _ := json.Marshal(a)
	fmt.Printf("schema_dialect_final_audit=%s\n", raw)
}

func TestScenarioSchemaDialectAuditOriginalSQLAndTamperDenialsIntegration(t *testing.T) {
	ctx := t.Context()
	pool := memorytest.Database(t)
	w, err := createSchemaDiagnosticWindow(ctx, pool, time.Minute)
	if err != nil {
		t.Fatal("synthetic audit window denied")
	}
	entries, _ := schemaDiagnosticInputs()
	examples, _ := schemaDiagnosticExamples()
	private := t.TempDir()
	report := schemaDiagnosticReport{Status: "diagnostic_completed", Window: schemaDiagnosticWindowID, Baseline: 279, Ceiling: 283, Attempts: []schemaDiagnosticAttempt{}}
	for i, e := range entries {
		text := string(examples[i/2].Golden)
		if i == 0 {
			text = "```json\n" + text + "\n```"
		}
		provider := &privateFinalAdversarialProvider{shadowV3FixtureProvider: &shadowV3FixtureProvider{}, events: func(run string) []transport.TransportEvent {
			u := &transport.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 7}
			v := schemaDiagnosticExpectedVersion
			f := transport.NewAssistantFinalEvent(run, "fixture", transport.AssistantFinal{Content: text, FinishReason: "STOP", Usage: u})
			f.Usage, f.ProviderModelVersion = u, &v
			return []transport.TransportEvent{f, transport.NewTerminalEvent(run, transport.EventTypeRunCompleted, "fixture")}
		}}
		observed := &schemaAuditStageProvider{ModelProvider: provider}
		a, cerr := collectSchemaDiagnostic(ctx, w, observed, private, e)
		if cerr != nil {
			t.Fatal("synthetic collection denied")
		}
		report.Attempts = append(report.Attempts, a)
	}
	var pgRaw []byte
	if pool.QueryRow(ctx, `SELECT json_build_object('version','butler-schema-dialect-pg-witness-v1','window',(SELECT row_to_json(w) FROM fixture_schema_dialect_window w),'attempts',(SELECT json_agg(row_to_json(a) ORDER BY ordinal) FROM fixture_schema_dialect_attempts a),'retention',(SELECT json_agg(row_to_json(r) ORDER BY ordinal) FROM fixture_schema_dialect_retention r))`).Scan(&pgRaw) != nil {
		t.Fatal("independent SQL audit export unavailable")
	}
	var pg schemaDiagnosticPGWitness
	if acceptance.DecodeStrict(pgRaw, &pg, "version", "window", "attempts", "retention") != nil {
		t.Fatal("export shape denied")
	}
	readRaw := func(ord int) ([]byte, error) {
		return os.ReadFile(filepath.Join(private, fmt.Sprintf("final-%03d.raw", ord)))
	}
	a, err := auditSchemaDiagnostic(report, pg, readRaw)
	if err != nil || !a.Compatibility || a.NativeExact != 2 || a.Rows != 4 || a.Raw != 4 || a.KnownInput != 16 || a.KnownOutput != 8 || a.KnownTotal != 28 || a.UnknownTotal != 0 || a.Quality || a.Ready || a.Calls != 0 {
		t.Fatal("independent raw/SQL/encoding-only spend correlation lost")
	}
	for _, mode := range []string{"raw", "order", "cost", "version", "stage", "schema", "counts", "time", "encoding_promoted", "semantic", "refund", "quality"} {
		t.Run(mode, func(t *testing.T) {
			var p schemaDiagnosticPGWitness
			_ = json.Unmarshal(pgRaw, &p)
			raw, _ := json.Marshal(report)
			var r schemaDiagnosticReport
			_ = json.Unmarshal(raw, &r)
			reader := readRaw
			switch mode {
			case "raw":
				reader = func(int) ([]byte, error) { return []byte("private wrong bytes"), nil }
			case "order":
				p.Attempts[0], p.Attempts[1] = p.Attempts[1], p.Attempts[0]
			case "cost":
				*p.Attempts[0].Input++
			case "version":
				v := "gemini-3.8-flash-other"
				p.Attempts[0].Version = &v
			case "stage":
				v := "private unallowlisted stage"
				p.Attempts[0].Stage = &v
			case "schema":
				p.Attempts[0].SchemaSHA = highV5HistorySHA
			case "counts":
				p.Attempts[0].Finals = 2
			case "time":
				*p.Attempts[0].Elapsed += 50
			case "encoding_promoted":
				p.Retention[0].Checked = true
			case "semantic":
				p.Retention[1].Semantic = false
			case "refund":
				p.Window.Attempts--
			case "quality":
				r.Quality = true
			}
			if _, err := auditSchemaDiagnostic(r, p, reader); err == nil {
				t.Fatal("tampered native compatibility evidence accepted")
			}
		})
	}
}

type schemaAuditStageProvider struct{ transport.ModelProvider }

func (p *schemaAuditStageProvider) StartRun(ctx context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
	transport.ObserveProviderStage(ctx, transport.ProviderStageStream)
	return p.ModelProvider.StartRun(ctx, req)
}
