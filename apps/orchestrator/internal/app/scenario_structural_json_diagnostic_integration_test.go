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
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestScenarioStructuralJSONClosedGateAndExactInputs(t *testing.T) {
	gateErr := structuralDiagnosticGate()
	if structuralDiagnosticReviewedSHA == "" && gateErr == nil || structuralDiagnosticReviewedSHA != "" && gateErr != nil {
		t.Fatal("empty review must deny; frozen review must satisfy every actual pin")
	}
	history, herr := readScenarioHighV3History()
	if herr != nil {
		t.Fatal("fixed diagnostic history unavailable")
	}
	t.Logf("diagnostic_pins protocol_sha256=%s input_sha256=%s history_sha256=%s schema_sha256=%s requested_model=%s observed_version=%s baseline=266 ceiling=268 seconds=480 attempt_seconds=120 generated_tokens=32768 visible_bytes=8192 input_bytes=65536 concurrency=1", structuralDiagnosticProtocolSHA(), structuralDiagnosticInputSHA, history.BaselineSHA256, acceptance.CloudV3SchemaDigest(), "gemini-3.8-flash-high", structuralDiagnosticExpectedVersion)
	cases, err := structuralDiagnosticInputs()
	if err != nil || len(cases) != 2 || cases[0].ID == cases[1].ID {
		t.Fatal("fixed unseen public inputs unavailable")
	}
	for _, c := range cases {
		input, err := acceptance.MakeCloudV3ModelInput(c, acceptance.CloudRoleSingle, nil)
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
	}
}

func structuralDiagnosticFixtureProvider(mode string) *privateFinalAdversarialProvider {
	return &privateFinalAdversarialProvider{shadowV3FixtureProvider: &shadowV3FixtureProvider{}, events: func(run string) []transport.TransportEvent {
		usage := &transport.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 7}
		version := structuralDiagnosticExpectedVersion
		text := `{"refs":[]}`
		if mode == "encoding_only" {
			text = "PRIVATE_MALFORMED_VISIBLE_BODY"
		}
		first := transport.NewAssistantFinalEvent(run, "fixture", transport.AssistantFinal{Content: text, FinishReason: "STOP", Usage: usage})
		first.Usage, first.ProviderModelVersion = usage, &version
		terminal := transport.NewTerminalEvent(run, transport.EventTypeRunCompleted, "fixture")
		switch mode {
		case "transport_error_then_encoding":
			first.AssistantFinal.Content = "PRIVATE_MALFORMED_VISIBLE_BODY"
			return []transport.TransportEvent{transport.NewTransportErrorEvent(run, "fixture", &transport.Error{Type: transport.ErrorTypeProviderProtocolError}), first, terminal}
		case "duplicate_final":
			return []transport.TransportEvent{first, first, terminal}
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

func TestScenarioStructuralJSONEncodingOnlyTwoContextsIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	w, err := createStructuralDiagnosticWindow(context.Background(), pool, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	cases, _ := structuralDiagnosticInputs()
	private := t.TempDir()
	for i, c := range cases {
		mode := "valid"
		if i == 0 {
			mode = "encoding_only"
		}
		p := structuralDiagnosticFixtureProvider(mode)
		a, err := collectStructuralDiagnostic(context.Background(), w, p, private, c)
		if err != nil || a.Ordinal != 267+i || !a.CleanLifecycle || !a.MetadataKnown || !a.RetentionOK || a.Total == nil || *a.Total != 7 || p.entries != 1 || a.Final == nil {
			t.Fatal("clean independent diagnostic lost measured spend or retention")
		}
		if i == 0 && a.Final.Reason != "json_syntax" {
			t.Fatal("malformed output was repaired or promoted")
		}
		if i == 0 {
			var before, after time.Time
			if pool.QueryRow(context.Background(), `SELECT expires_at FROM fixture_structural_json_window`).Scan(&before) != nil {
				t.Fatal("immutable expiry unavailable")
			}
			other, err := pgxpool.NewWithConfig(context.Background(), pool.Config().Copy())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(other.Close)
			w, err = createStructuralDiagnosticWindow(context.Background(), other, 8*time.Minute)
			if err != nil || other.QueryRow(context.Background(), `SELECT expires_at FROM fixture_structural_json_window`).Scan(&after) != nil || !before.Equal(after) {
				t.Fatal("independent store reconstruction changed the original deadline")
			}
		}
		body, _ := json.Marshal(a)
		if strings.Contains(string(body), "PRIVATE_MALFORMED") || strings.Contains(string(body), private) {
			t.Fatal("private body/path exported")
		}
	}
	p := structuralDiagnosticFixtureProvider("valid")
	if _, err := collectStructuralDiagnostic(context.Background(), w, p, private, cases[0]); err == nil || p.entries != 0 {
		t.Fatal("spent context or total ceiling reopened")
	}
	var count, distinct int
	if pool.QueryRow(context.Background(), `SELECT count(*),count(DISTINCT context_id) FROM fixture_structural_json_attempts WHERE outcome='completed'`).Scan(&count, &distinct) != nil || count != 2 || distinct != 2 {
		t.Fatal("distinct durable Start contexts missing")
	}
	for _, sql := range []string{`UPDATE fixture_structural_json_attempts SET total_tokens=8 WHERE ordinal=267`, `DELETE FROM fixture_structural_json_retention`, `UPDATE fixture_structural_json_window SET attempts=266`} {
		if _, err := pool.Exec(context.Background(), sql); err == nil {
			t.Fatal("sealed measured history mutable")
		}
	}
}

func TestScenarioStructuralJSONFailuresDenyNextEntryIntegration(t *testing.T) {
	for _, mode := range []string{"transport_error_then_encoding", "duplicate_final", "conflicting_usage", "legacy_conflict", "conflicting_version", "missing_metadata", "missing_terminal", "unsupported_batch", "foreign_final", "wrong_finish", "generated_bound", "retention_collision", "visible_bound", "provisional_version"} {
		t.Run(mode, func(t *testing.T) {
			pool := memorytest.Database(t)
			w, err := createStructuralDiagnosticWindow(context.Background(), pool, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			cases, _ := structuralDiagnosticInputs()
			private := t.TempDir()
			if mode == "retention_collision" {
				if os.WriteFile(filepath.Join(private, "final-267.raw"), []byte("immutable prior bytes"), 0600) != nil {
					t.Fatal("collision unavailable")
				}
			}
			p := structuralDiagnosticFixtureProvider(mode)
			a, err := collectStructuralDiagnostic(context.Background(), w, p, private, cases[0])
			if err == nil || p.entries != 1 || a.Ordinal != 267 {
				t.Fatal("non-encoding failure admitted another phase or refunded failed start")
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
			if pool.QueryRow(context.Background(), `SELECT total_tokens FROM fixture_structural_json_attempts WHERE ordinal=267`).Scan(&storedTotal) != nil || pool.QueryRow(context.Background(), `SELECT count(*) FROM fixture_structural_json_attempts`).Scan(&rows) != nil || rows != 1 || !nativeEqualJSON(storedTotal, a.Total) {
				t.Fatal("SQL measurements not durable before private failure")
			}
			other, err := pgxpool.NewWithConfig(context.Background(), pool.Config().Copy())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(other.Close)
			restored, err := createStructuralDiagnosticWindow(context.Background(), other, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			next := structuralDiagnosticFixtureProvider("valid")
			if _, err = collectStructuralDiagnostic(context.Background(), restored, next, private, cases[1]); err == nil || next.entries != 0 {
				t.Fatal("sticky bad lifecycle/metadata/retention did not deny next provider entry")
			}
		})
	}
}

func TestScenarioStructuralJSONPendingReconstructionDeniesIntegration(t *testing.T) {
	ctx := context.Background()
	pool := memorytest.Database(t)
	w, err := createStructuralDiagnosticWindow(ctx, pool, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	cases, _ := structuralDiagnosticInputs()
	input, _ := acceptance.MakeCloudV3ModelInput(cases[0], acceptance.CloudRoleSingle, nil)
	if ordinal, err := w.reserve(ctx, cases[0], input); err != nil || ordinal != 267 {
		t.Fatal("initial admitted pending entry missing")
	}
	other, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	w, err = createStructuralDiagnosticWindow(ctx, other, 8*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	p := structuralDiagnosticFixtureProvider("valid")
	if _, err = collectStructuralDiagnostic(ctx, w, p, t.TempDir(), cases[1]); err == nil || p.entries != 0 {
		t.Fatal("reconstructed pending entry reopened inference")
	}
	var counter int
	var total *int64
	if other.QueryRow(ctx, `SELECT attempts FROM fixture_structural_json_window`).Scan(&counter) != nil || counter != 267 || other.QueryRow(ctx, `SELECT total_tokens FROM fixture_structural_json_attempts WHERE ordinal=267 AND outcome='unavailable'`).Scan(&total) != nil || total != nil {
		t.Fatal("pending unknown spend changed during reconstruction")
	}
}

func TestScenarioStructuralJSONAbsoluteDeadlinePersistedIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	deadline := time.Now().UTC().Add(10 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	if _, err := createStructuralDiagnosticWindow(ctx, pool, 8*time.Minute); err != nil {
		t.Fatal(err)
	}
	var actual time.Time
	if pool.QueryRow(ctx, `SELECT expires_at FROM fixture_structural_json_window`).Scan(&actual) != nil || actual.After(deadline) || deadline.Sub(actual) > time.Microsecond {
		t.Fatal("SQL scope extended the absolute parent deadline")
	}
}

func TestScenarioStructuralJSONPostWaitGuardsIntegration(t *testing.T) {
	for _, mode := range []string{"prior_failed_after_wait", "expired_after_wait"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			pool := memorytest.Database(t)
			duration := time.Minute
			if mode == "expired_after_wait" {
				duration = 350 * time.Millisecond
			}
			w, err := createStructuralDiagnosticWindow(ctx, pool, duration)
			if err != nil {
				t.Fatal(err)
			}
			cases, _ := structuralDiagnosticInputs()
			input, _ := acceptance.MakeCloudV3ModelInput(cases[1], acceptance.CloudRoleSingle, nil)
			blocker, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback(context.Background())
			if _, err = blocker.Exec(ctx, `SELECT singleton FROM fixture_structural_json_window FOR UPDATE`); err != nil {
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
				if probe.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'UPDATE fixture_structural_json_window%')`).Scan(&waiting) != nil {
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
				if _, err = blocker.Exec(ctx, `UPDATE fixture_structural_json_window SET attempts=267`); err != nil {
					t.Fatal(err)
				}
				if _, err = blocker.Exec(ctx, `INSERT INTO fixture_structural_json_attempts(ordinal,case_id,context_id,input_hash) VALUES(267,$1,$2,$3)`, cases[0].ID, structuralDiagnosticContext(cases[0].ID), "fixed-denied-input"); err != nil {
					t.Fatal(err)
				}
				if _, err = blocker.Exec(ctx, `UPDATE fixture_structural_json_attempts SET outcome='transport_error',finished_at=clock_timestamp() WHERE ordinal=267`); err != nil {
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
			if pool.QueryRow(ctx, `SELECT attempts FROM fixture_structural_json_window`).Scan(&counter) != nil || counter != 266 && mode == "expired_after_wait" || counter != 267 && mode == "prior_failed_after_wait" {
				t.Fatal("denied row wait spent or reopened counter")
			}
		})
	}
}

func TestScenarioStructuralJSONChangedInputDeniedBeforeEntryIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	w, err := createStructuralDiagnosticWindow(context.Background(), pool, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	cases, _ := structuralDiagnosticInputs()
	c := cases[0]
	c.Input += " modified intent"
	p := structuralDiagnosticFixtureProvider("valid")
	if _, err = collectStructuralDiagnostic(context.Background(), w, p, t.TempDir(), c); err == nil || p.entries != 0 {
		t.Fatal("unreviewed input reached provider")
	}
	var counter int
	if pool.QueryRow(context.Background(), `SELECT attempts FROM fixture_structural_json_window`).Scan(&counter) != nil || counter != 266 {
		t.Fatal("invalid projection consumed admission")
	}
}
