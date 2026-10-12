package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type schemaDiagnosticNoAuth struct{ calls int }

func (p *schemaDiagnosticNoAuth) ResolveAntigravity(context.Context) (providerauth.AntigravityAuth, error) {
	p.calls++
	return providerauth.AntigravityAuth{}, errNativeFixtureDenied
}

func TestScenarioSchemaDialectClosedGateAndExactInputs(t *testing.T) {
	gateErr := schemaDiagnosticGate()
	if schemaDiagnosticReviewedSHA == "" && gateErr == nil || schemaDiagnosticReviewedSHA != "" && gateErr != nil {
		t.Fatal("empty review must deny; frozen review must satisfy every actual pin")
	}
	history, herr := readScenarioHighV5History()
	if herr != nil {
		t.Fatal("fixed diagnostic history unavailable")
	}
	t.Logf("diagnostic_pins protocol_sha256=%s input_sha256=%s history_sha256=%s schema_sha256=%s requested_model=%s observed_version=%s baseline=279 ceiling=283 seconds=600 attempt_seconds=120 generated_tokens=32768 visible_bytes=8192 input_bytes=65536 concurrency=1", schemaDiagnosticProtocolSHA(), schemaDiagnosticInputSHA, history.BaselineSHA256, acceptance.CloudV4SchemaDigest(), "gemini-3.8-flash-high", schemaDiagnosticExpectedVersion)
	cases, err := schemaDiagnosticInputs()
	if err != nil || len(cases) != 4 || cases[0].ID != cases[1].ID || cases[0].Dialect == cases[1].Dialect {
		t.Fatal("fixed unseen public inputs unavailable")
	}
	for _, c := range cases {
		input, err := makeSchemaDiagnosticInput(c)
		if err != nil || !strings.HasPrefix(c.ID, "case:") || len(c.Documents) != 2 {
			t.Fatal("trusted diagnostic projection denied")
		}
		body, _ := json.Marshal(input)
		for _, forbidden := range []string{"golden", "synthetic_public", "unseen_structural_protocol", "diag:"} {
			if strings.Contains(string(body), forbidden) {
				t.Fatal("author-only metadata entered model projection")
			}
		}
		for id := range c.Documents {
			if !strings.Contains(string(input.Schema), id) {
				t.Fatal("schema omitted an original source key")
			}
		}
		auth := &schemaDiagnosticNoAuth{}
		cfg := antigravity.Config{Model: antigravity.FlashHighModel, MaximumOutputTokens: 32768, AuthSource: auth}
		if c.Dialect == schemaDialectGoogle {
			cfg.GoogleResponseSchema = input.Schema
		} else {
			cfg.JSONResponseSchema = input.Schema
		}
		if _, err := antigravity.NewProvider(cfg); err != nil || auth.calls != 0 {
			t.Fatal("actual caller schema rejected or reached auth during offline construction")
		}
	}
	for i := 0; i < 4; i += 2 {
		a, _ := makeSchemaDiagnosticInput(cases[i])
		b, _ := makeSchemaDiagnosticInput(cases[i+1])
		ua, _ := schemaDiagnosticUser(cases[i])
		ub, _ := schemaDiagnosticUser(cases[i+1])
		if a.SystemInstructions != b.SystemInstructions || string(ua) != string(ub) || string(a.Schema) == string(b.Schema) {
			t.Fatal("paired dialect arms changed original full instructions or user input")
		}
	}
}

func schemaDiagnosticFixtureProvider(mode string) *privateFinalAdversarialProvider {
	entries, _ := schemaDiagnosticInputs()
	examples, _ := schemaDiagnosticExamples()
	goldens := map[string]string{}
	for i, entry := range entries {
		goldens[schemaDiagnosticContext(entry)] = string(examples[i/2].Golden)
	}
	return &privateFinalAdversarialProvider{shadowV3FixtureProvider: &shadowV3FixtureProvider{}, events: func(run string) []transport.TransportEvent {
		usage := &transport.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 7}
		version := schemaDiagnosticExpectedVersion
		text := goldens[run]
		if mode == "encoding_only" {
			text = "PRIVATE_MALFORMED_VISIBLE_BODY"
		}
		first := transport.NewAssistantFinalEvent(run, "fixture", transport.AssistantFinal{Content: text, FinishReason: "STOP", Usage: usage})
		first.Usage, first.ProviderModelVersion = usage, &version
		terminal := transport.NewTerminalEvent(run, transport.EventTypeRunCompleted, "fixture")
		switch mode {
		case "semantic_error":
			first.AssistantFinal.Content = `{"facts":[]}`
		case "missing_final":
			terminal.Usage, terminal.ProviderModelVersion = usage, &version
			return []transport.TransportEvent{terminal}
		case "transport_error_then_encoding":
			first.AssistantFinal.Content = "PRIVATE_MALFORMED_VISIBLE_BODY"
			return []transport.TransportEvent{transport.NewTransportErrorEvent(run, "fixture", &transport.Error{Type: transport.ErrorTypeProviderProtocolError}), first, terminal}
		case "duplicate_final":
			return []transport.TransportEvent{first, first, terminal}
		case "duplicate_terminal":
			return []transport.TransportEvent{first, terminal, terminal}
		case "cancelled_terminal":
			return []transport.TransportEvent{first, transport.NewTerminalEvent(run, transport.EventTypeRunCancelled, "fixture")}
		case "timed_out_terminal":
			return []transport.TransportEvent{first, transport.NewTerminalEvent(run, transport.EventTypeRunTimedOut, "fixture")}
		case "conflicting_usage":
			terminal.Usage = &transport.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 8}
		case "legacy_conflict":
			first.Usage = &transport.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 8}
		case "conflicting_version":
			other := "gemini-3.8-flash-other"
			terminal.ProviderModelVersion = &other
		case "missing_metadata":
			first.Usage = nil
			first.AssistantFinal.Usage = nil
			first.ProviderModelVersion = nil
		case "missing_terminal":
			return []transport.TransportEvent{first}
		case "unsupported_batch":
			batch := transport.NewEvent(run, transport.EventTypeToolCallBatchRequested, "fixture")
			batch.ToolCallBatch = &transport.ToolCallBatch{}
			batch.Usage = usage
			batch.ProviderModelVersion = &version
			return []transport.TransportEvent{batch}
		case "foreign_final":
			first.RunID = "foreign-run"
		case "wrong_finish":
			first.AssistantFinal.FinishReason = "MAX_TOKENS"
		case "generated_bound":
			usage.TotalTokens = 40000
		case "visible_bound":
			first.AssistantFinal.Content = strings.Repeat("x", privateFinalMaximumBytes+1)
		case "provisional_version":
			first.ProviderModelVersion = nil
			provisional := transport.NewEvent(run, transport.EventTypeAssistantDelta, "fixture")
			provisional.ProviderModelVersion = &version
			return []transport.TransportEvent{provisional, first, terminal}
		}
		return []transport.TransportEvent{first, terminal}
	}}
}

func TestScenarioSchemaDialectEncodingOnlyFourContextsIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	w, err := createSchemaDiagnosticWindow(context.Background(), pool, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	cases, _ := schemaDiagnosticInputs()
	private := t.TempDir()
	for i, c := range cases {
		mode := "valid"
		if i == 0 {
			mode = "encoding_only"
		}
		p := schemaDiagnosticFixtureProvider(mode)
		a, err := collectSchemaDiagnostic(context.Background(), w, p, private, c)
		if err != nil || a.Ordinal != 280+i || !a.CleanLifecycle || !a.MetadataKnown || !a.RetentionOK || a.Total == nil || *a.Total != 7 || p.entries != 1 || a.Final == nil {
			t.Fatal("clean independent diagnostic lost measured spend or retention")
		}
		if a.FinalCount != 1 || a.TerminalCount != 1 || a.FinishReason == nil || *a.FinishReason != "STOP" {
			t.Fatal("actual final/terminal/STOP state absent")
		}
		if i == 0 && a.Final.Reason != "json_syntax" {
			t.Fatal("malformed output was repaired or promoted")
		}
		if i == 0 {
			var before, after time.Time
			if pool.QueryRow(context.Background(), `SELECT expires_at FROM fixture_schema_dialect_window`).Scan(&before) != nil {
				t.Fatal("immutable expiry unavailable")
			}
			other, err := pgxpool.NewWithConfig(context.Background(), pool.Config().Copy())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(other.Close)
			w, err = createSchemaDiagnosticWindow(context.Background(), other, 10*time.Minute)
			if err != nil || other.QueryRow(context.Background(), `SELECT expires_at FROM fixture_schema_dialect_window`).Scan(&after) != nil || !before.Equal(after) {
				t.Fatal("independent store reconstruction changed the original deadline")
			}
		}
		body, _ := json.Marshal(a)
		if strings.Contains(string(body), "PRIVATE_MALFORMED") || strings.Contains(string(body), private) {
			t.Fatal("private body/path exported")
		}
	}
	p := schemaDiagnosticFixtureProvider("valid")
	if _, err := collectSchemaDiagnostic(context.Background(), w, p, private, cases[0]); err == nil || p.entries != 0 {
		t.Fatal("spent context or total ceiling reopened")
	}
	var count, distinct int
	if pool.QueryRow(context.Background(), `SELECT count(*),count(DISTINCT context_id) FROM fixture_schema_dialect_attempts WHERE outcome='completed'`).Scan(&count, &distinct) != nil || count != 4 || distinct != 4 {
		t.Fatal("distinct durable Start contexts missing")
	}
	for _, sql := range []string{`UPDATE fixture_schema_dialect_attempts SET total_tokens=8 WHERE ordinal=280`, `DELETE FROM fixture_schema_dialect_retention`, `UPDATE fixture_schema_dialect_window SET attempts=279`} {
		if _, err := pool.Exec(context.Background(), sql); err == nil {
			t.Fatal("sealed measured history mutable")
		}
	}
}

func TestScenarioSchemaDialectFailuresDenyNextEntryIntegration(t *testing.T) {
	for _, mode := range []string{"transport_error_then_encoding", "duplicate_final", "duplicate_terminal", "cancelled_terminal", "timed_out_terminal", "conflicting_usage", "legacy_conflict", "conflicting_version", "missing_metadata", "missing_terminal", "missing_final", "unsupported_batch", "foreign_final", "wrong_finish", "generated_bound", "retention_collision", "visible_bound", "provisional_version", "semantic_error"} {
		t.Run(mode, func(t *testing.T) {
			pool := memorytest.Database(t)
			w, err := createSchemaDiagnosticWindow(context.Background(), pool, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			cases, _ := schemaDiagnosticInputs()
			private := t.TempDir()
			if mode == "retention_collision" {
				if os.WriteFile(filepath.Join(private, "final-280.raw"), []byte("immutable prior bytes"), 0600) != nil {
					t.Fatal("collision unavailable")
				}
			}
			p := schemaDiagnosticFixtureProvider(mode)
			a, err := collectSchemaDiagnostic(context.Background(), w, p, private, cases[0])
			if err == nil || p.entries != 1 || a.Ordinal != 280 {
				t.Fatal("non-encoding failure admitted another phase or refunded failed start")
			}
			var finals, terminals int
			var finish *string
			if pool.QueryRow(context.Background(), `SELECT final_count,terminal_count,finish_reason FROM fixture_schema_dialect_attempts WHERE ordinal=280`).Scan(&finals, &terminals, &finish) != nil || finals != a.FinalCount || terminals != a.TerminalCount || !nativeEqualJSON(finish, a.FinishReason) {
				t.Fatal("actual lifecycle count/finish state differs from owning SQL")
			}
			if mode == "duplicate_final" && finals != 2 || mode == "missing_final" && finals != 0 || mode == "missing_terminal" && terminals != 0 || mode == "wrong_finish" && (finish == nil || *finish != "MAX_TOKENS") {
				t.Fatal("failure lifecycle evidence was reduced to an inferred bool")
			}
			if (mode == "duplicate_terminal" || mode == "transport_error_then_encoding") && terminals != 2 || (mode == "cancelled_terminal" || mode == "timed_out_terminal") && terminals != 1 {
				t.Fatal("rejected terminal event was omitted from actual counts")
			}
			if mode == "cancelled_terminal" && a.Outcome != "cancelled" || mode == "timed_out_terminal" && a.Outcome != "timed_out" {
				t.Fatal("actual rejected terminal outcome was not preserved")
			}
			if mode == "conflicting_usage" || mode == "legacy_conflict" || mode == "missing_metadata" || mode == "foreign_final" {
				if a.Input != nil || a.Output != nil || a.Total != nil {
					t.Fatal("unknown/conflicting original cost became known")
				}
			} else if a.Total == nil || *a.Total != 7 && mode != "generated_bound" {
				t.Fatal("complete failed packet measurement lost")
			}
			if mode == "conflicting_version" && a.Version != nil {
				t.Fatal("contradictory version exported as known")
			}
			if mode == "provisional_version" && (a.Version != nil || a.MetadataKnown) {
				t.Fatal("provisional identity became complete native evidence")
			}
			if mode == "visible_bound" && (a.RetentionOK || a.Final == nil || a.Final.Reason != "visible_bound" || a.Final.RawRetained) {
				t.Fatal("oversized private output admitted continuation")
			}
			if mode == "retention_collision" && (!a.CleanLifecycle || !a.MetadataKnown || a.RetentionOK) {
				t.Fatal("retention failure changed already measured SQL lifecycle")
			}
			var rows int
			var storedTotal *int64
			if pool.QueryRow(context.Background(), `SELECT total_tokens FROM fixture_schema_dialect_attempts WHERE ordinal=280`).Scan(&storedTotal) != nil || pool.QueryRow(context.Background(), `SELECT count(*) FROM fixture_schema_dialect_attempts`).Scan(&rows) != nil || rows != 1 || !nativeEqualJSON(storedTotal, a.Total) {
				t.Fatal("SQL measurements not durable before private failure")
			}
			other, err := pgxpool.NewWithConfig(context.Background(), pool.Config().Copy())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(other.Close)
			restored, err := createSchemaDiagnosticWindow(context.Background(), other, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			next := schemaDiagnosticFixtureProvider("valid")
			if _, err = collectSchemaDiagnostic(context.Background(), restored, next, private, cases[1]); err == nil || next.entries != 0 {
				t.Fatal("sticky bad lifecycle/metadata/retention did not deny next provider entry")
			}
		})
	}
}

func TestScenarioSchemaDialectPendingReconstructionDeniesIntegration(t *testing.T) {
	ctx := context.Background()
	pool := memorytest.Database(t)
	w, err := createSchemaDiagnosticWindow(ctx, pool, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	cases, _ := schemaDiagnosticInputs()
	input, _ := makeSchemaDiagnosticInput(cases[0])
	if ordinal, err := w.reserve(ctx, cases[0], input); err != nil || ordinal != 280 {
		t.Fatal("initial admitted pending entry missing")
	}
	other, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	w, err = createSchemaDiagnosticWindow(ctx, other, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	p := schemaDiagnosticFixtureProvider("valid")
	if _, err = collectSchemaDiagnostic(ctx, w, p, t.TempDir(), cases[1]); err == nil || p.entries != 0 {
		t.Fatal("reconstructed pending entry reopened inference")
	}
	var counter int
	var total *int64
	if other.QueryRow(ctx, `SELECT attempts FROM fixture_schema_dialect_window`).Scan(&counter) != nil || counter != 280 || other.QueryRow(ctx, `SELECT total_tokens FROM fixture_schema_dialect_attempts WHERE ordinal=280 AND outcome='unavailable'`).Scan(&total) != nil || total != nil {
		t.Fatal("pending unknown spend changed during reconstruction")
	}
}

func TestScenarioSchemaDialectAbsoluteDeadlinePersistedIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	deadline := time.Now().UTC().Add(10 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	if _, err := createSchemaDiagnosticWindow(ctx, pool, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	var actual time.Time
	if pool.QueryRow(ctx, `SELECT expires_at FROM fixture_schema_dialect_window`).Scan(&actual) != nil || actual.After(deadline) || deadline.Sub(actual) > time.Microsecond {
		t.Fatal("SQL scope extended the absolute parent deadline")
	}
}

type schemaDiagnosticDeadlineProvider struct {
	transport.ModelProvider
	observe func(context.Context)
}

func (p *schemaDiagnosticDeadlineProvider) StartRun(ctx context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
	p.observe(ctx) // The whole-entry bound must exist before any provider/auth work.
	return p.ModelProvider.StartRun(ctx, req)
}

func TestScenarioSchemaDialectWholeEntryDeadlineIntegration(t *testing.T) {
	for _, mode := range []string{"entry_120_seconds", "earlier_global", "earlier_parent"} {
		t.Run(mode, func(t *testing.T) {
			pool := memorytest.Database(t)
			duration := 3 * time.Minute
			if mode == "earlier_global" {
				duration = time.Minute
			}
			w, err := createSchemaDiagnosticWindow(context.Background(), pool, duration)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if mode == "earlier_parent" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
			}
			entries, _ := schemaDiagnosticInputs()
			base := schemaDiagnosticFixtureProvider("valid")
			observed := false
			provider := &schemaDiagnosticDeadlineProvider{ModelProvider: base, observe: func(call context.Context) {
				var admitted, global time.Time
				if pool.QueryRow(context.Background(), `SELECT a.admitted_at,w.expires_at FROM fixture_schema_dialect_attempts a CROSS JOIN fixture_schema_dialect_window w WHERE a.ordinal=280 AND w.singleton`).Scan(&admitted, &global) != nil {
					t.Fatal("durable entry was absent before provider work")
				}
				wanted := admitted.Add(120 * time.Second)
				if global.Before(wanted) {
					wanted = global
				}
				if parent, ok := ctx.Deadline(); ok && parent.Before(wanted) {
					wanted = parent
				}
				got, ok := call.Deadline()
				if !ok || !got.Equal(wanted) {
					t.Fatal("whole provider entry lacks the earliest durable deadline")
				}
				observed = true
			}}
			if _, err := collectSchemaDiagnostic(ctx, w, provider, t.TempDir(), entries[0]); err != nil || !observed || base.entries != 1 {
				t.Fatal("one bounded synthetic entry was not observed")
			}
			var exactFinishTime bool
			if pool.QueryRow(context.Background(), `SELECT elapsed_ms=floor(extract(epoch FROM finished_at-admitted_at)*1000)::bigint FROM fixture_schema_dialect_attempts WHERE ordinal=280`).Scan(&exactFinishTime) != nil || !exactFinishTime {
				t.Fatal("elapsed measurement differs from the single durable finish timestamp")
			}
		})
	}
}

func TestScenarioSchemaDialectPostWaitGuardsIntegration(t *testing.T) {
	for _, mode := range []string{"prior_failed_after_wait", "expired_after_wait"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			pool := memorytest.Database(t)
			duration := time.Minute
			if mode == "expired_after_wait" {
				duration = 350 * time.Millisecond
			}
			w, err := createSchemaDiagnosticWindow(ctx, pool, duration)
			if err != nil {
				t.Fatal(err)
			}
			cases, _ := schemaDiagnosticInputs()
			input, _ := makeSchemaDiagnosticInput(cases[1])
			blocker, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback(context.Background())
			if _, err = blocker.Exec(ctx, `SELECT singleton FROM fixture_schema_dialect_window FOR UPDATE`); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() { _, err := w.reserve(ctx, cases[1], input); result <- err }()
			// Independent connection proves the admission actually waits for this row.
			probe, err := pgx.ConnectConfig(ctx, pool.Config().ConnConfig.Copy())
			if err != nil {
				t.Fatal(err)
			}
			defer probe.Close(context.Background())
			for {
				var waiting bool
				if probe.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'UPDATE fixture_schema_dialect_window%')`).Scan(&waiting) != nil {
					t.Fatal("bounded row-wait proof unavailable")
				}
				if waiting {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal("admission did not enter actual row wait")
				case <-time.After(5 * time.Millisecond):
				}
			}
			if mode == "prior_failed_after_wait" {
				if _, err = blocker.Exec(ctx, `UPDATE fixture_schema_dialect_window SET attempts=280`); err != nil {
					t.Fatal(err)
				}
				if _, err = blocker.Exec(ctx, `INSERT INTO fixture_schema_dialect_attempts(ordinal,case_id,dialect,context_id,input_hash,system_hash,user_hash,schema_hash) VALUES(280,$1,$2,$3,repeat('a',64),repeat('b',64),repeat('c',64),repeat('d',64))`, cases[0].ID, cases[0].Dialect, schemaDiagnosticContext(cases[0])); err != nil {
					t.Fatal(err)
				}
				if _, err = blocker.Exec(ctx, `UPDATE fixture_schema_dialect_attempts SET outcome='transport_error',finished_at=clock_timestamp() WHERE ordinal=280`); err != nil {
					t.Fatal(err)
				}
			} else {
				time.Sleep(400 * time.Millisecond)
			}
			if blocker.Commit(ctx) != nil {
				t.Fatal("blocker commit failed")
			}
			if <-result == nil {
				t.Fatal("post-wait failure/expiry admitted new inference")
			}
			var counter int
			if pool.QueryRow(ctx, `SELECT attempts FROM fixture_schema_dialect_window`).Scan(&counter) != nil || counter != 279 && mode == "expired_after_wait" || counter != 280 && mode == "prior_failed_after_wait" {
				t.Fatal("denied row wait spent or reopened counter")
			}
		})
	}
}

func TestScenarioSchemaDialectChangedInputDeniedBeforeEntryIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	w, err := createSchemaDiagnosticWindow(context.Background(), pool, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	cases, _ := schemaDiagnosticInputs()
	c := cases[0]
	c.Input += " modified intent"
	p := schemaDiagnosticFixtureProvider("valid")
	if _, err = collectSchemaDiagnostic(context.Background(), w, p, t.TempDir(), c); err == nil || p.entries != 0 {
		t.Fatal("unreviewed input reached provider")
	}
	var counter int
	if pool.QueryRow(context.Background(), `SELECT attempts FROM fixture_schema_dialect_window`).Scan(&counter) != nil || counter != 279 {
		t.Fatal("invalid projection consumed admission")
	}
}

func TestScenarioSchemaDialectOrderAndUnknownModeDenyBeforeEntryIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	w, err := createSchemaDiagnosticWindow(context.Background(), pool, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := schemaDiagnosticInputs()
	unknown := entries[0]
	unknown.Dialect = "model_selected_mode"
	for _, entry := range []schemaDiagnosticEntry{entries[1], unknown, entries[2]} {
		spy := schemaDiagnosticFixtureProvider("valid")
		if _, err := collectSchemaDiagnostic(context.Background(), w, spy, t.TempDir(), entry); err == nil || spy.entries != 0 {
			t.Fatal("omitted/reordered or unknown mode entered provider")
		}
	}
	var total int
	if pool.QueryRow(context.Background(), `SELECT attempts FROM fixture_schema_dialect_window`).Scan(&total) != nil || total != 279 {
		t.Fatal("invalid order/mode changed the global allowance")
	}
}

func TestScenarioSchemaDialectReadOnlyWitnessProbeIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	w, err := createSchemaDiagnosticWindow(context.Background(), pool, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := schemaDiagnosticInputs()
	a, err := collectSchemaDiagnostic(context.Background(), w, schemaDiagnosticFixtureProvider("valid"), t.TempDir(), entries[0])
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(shadowRepoRoot(), "scripts", "dev", "fixtures", "schema-dialect-pg-witness.sql"))
	if err != nil {
		t.Fatal(err)
	}
	query := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(raw)), "BEGIN READ ONLY;")), "COMMIT;"))
	tx, err := pool.BeginTx(context.Background(), pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var snapshot []byte
	if tx.QueryRow(context.Background(), query).Scan(&snapshot) != nil || tx.Commit(context.Background()) != nil {
		t.Fatal("independent read-only SQL export probe failed")
	}
	var witness struct {
		Version  string `json:"version"`
		Attempts []struct {
			Ordinal   int    `json:"ordinal"`
			InputHash string `json:"input_hash"`
			Total     *int64 `json:"total_tokens"`
		} `json:"attempts"`
		Retention []struct {
			Encoding string `json:"encoding_reason"`
			Semantic bool   `json:"semantic_ok"`
		} `json:"retention"`
	}
	if json.Unmarshal(snapshot, &witness) != nil || witness.Version != "butler-schema-dialect-pg-witness-v1" || len(witness.Attempts) != 1 || witness.Attempts[0].Ordinal != 280 || witness.Attempts[0].InputHash != a.InputSHA || !nativeEqualJSON(witness.Attempts[0].Total, a.Total) || len(witness.Retention) != 1 || witness.Retention[0].Encoding != "json_object" || !witness.Retention[0].Semantic {
		t.Fatal("read-only exporter changed actual owning hashes/usage/verdict")
	}
	if strings.Contains(string(snapshot), "Кобальтовый") || strings.Contains(string(snapshot), "КП-604-Я") || strings.Contains(string(snapshot), `"golden"`) {
		t.Fatal("source/model/golden bytes escaped safe SQL export")
	}
}
