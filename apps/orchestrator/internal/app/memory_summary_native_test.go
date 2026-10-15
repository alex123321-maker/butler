package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

type nativeSummaryCosts struct {
	Attempts, KnownUsage, UnknownUsage     int
	InputTokens, OutputTokens, TotalTokens int
	SerializedInputBytes, TransportMillis  int64
}

// v5 is a new independent prospective budget in the same isolated database.
// v1/v2/v3/v4 reservations, including all four unknown expenses, are immutable history.
// A persistent admission identity prevents repeating any paid window.
func TestNativeOrdinaryTaskSummaryContinuityIntegration(t *testing.T) {
	if os.Getenv("BUTLER_SUMMARY_NATIVE_V5") != "1" {
		t.Skip("explicit independent native summary continuity v5 window required")
	}
	if os.Getenv("HTTPS_PROXY") != "http://127.0.0.1:18080" {
		t.Fatal("server proxy pin required")
	}
	dsn := os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("isolated DB required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var prior json.RawMessage
	if err = admin.QueryRow(ctx, `SELECT jsonb_agg(x ORDER BY x->>'window',x->>'operation_id') FROM (
 SELECT jsonb_build_object('window','v1','operation_id',operation_id,'state',state,'usage',usage,'quote',quote,'reason',reason) x FROM summary_native_v1.memory_model_attempts
 UNION ALL SELECT jsonb_build_object('window','v2','operation_id',operation_id,'state',state,'usage',usage,'quote',quote,'reason',reason) FROM summary_native_v2.memory_model_attempts
 UNION ALL SELECT jsonb_build_object('window','v3','operation_id',operation_id,'state',state,'usage',usage,'quote',quote,'reason',reason) FROM summary_native_v3.memory_model_attempts
) history`).Scan(&prior); err != nil {
		t.Fatal("retained v1/v2/v3 ledger missing")
	}
	var retained []struct{ Usage json.RawMessage }
	if json.Unmarshal(prior, &retained) != nil || len(retained) != 5 {
		t.Fatal("retained history changed")
	}
	unknown := 0
	var historicalTokens int
	for _, a := range retained {
		if string(a.Usage) == "null" {
			unknown++
			continue
		}
		var usage struct{ TotalTokens int }
		if json.Unmarshal(a.Usage, &usage) != nil {
			t.Fatal("invalid retained usage")
		}
		historicalTokens += usage.TotalTokens
	}
	if unknown != 3 || historicalTokens != 12979 {
		t.Fatal("retained expenses changed")
	}
	// Foreground durable ledgers retain input/output observations; native totals
	// additionally include returned reasoning dimensions. Preserve both exactly.
	v4Ledger, err := os.ReadFile("/history/native-v4/full-ledger.json")
	if err != nil || !json.Valid(v4Ledger) {
		t.Fatal("retained v4 ledger unavailable")
	}
	v4Native, err := os.ReadFile("/history/native-v4/native-usage.json")
	var v4Usage map[string]transport.Usage
	if err != nil || json.Unmarshal(v4Native, &v4Usage) != nil || len(v4Usage) != 3 {
		t.Fatal("retained complete v4 native usage unavailable")
	}
	v4Tokens := 0
	for _, u := range v4Usage {
		if u.Validate() != nil {
			t.Fatal("invalid retained native usage")
		}
		v4Tokens += u.TotalTokens
	}
	var v4Attempts, v4Unknown int
	if err := admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM summary_native_v4.memory_model_attempts)+(SELECT count(*) FROM summary_native_v4.execution_model_attempts),(SELECT count(*) FROM summary_native_v4.memory_model_attempts WHERE usage IS NULL)+(SELECT count(*) FROM summary_native_v4.execution_model_attempts WHERE observed_at IS NULL)`).Scan(&v4Attempts, &v4Unknown); err != nil || v4Attempts != 4 || v4Unknown != 1 || v4Tokens != 23449 {
		t.Fatal("retained v4 expenses changed")
	}
	historicalAttempts := len(retained) + v4Attempts
	historicalTokens += v4Tokens
	unknown += v4Unknown
	if _, err = admin.Exec(ctx, `INSERT INTO issue84_native_windows(id)VALUES('summary-native-v5');CREATE SCHEMA summary_native_v5`); err != nil {
		t.Fatal("native one-use window denied")
	}
	cfg.MaxConns = 2
	trace := &summaryPGTrace{}
	cfg.ConnConfig.Tracer = trace
	cfg.ConnConfig.RuntimeParams["search_path"] = "summary_native_v5,public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	trace.pool = pool
	paths, err := filepath.Glob("../../../../migrations/*.up.sql")
	if err != nil || len(paths) == 0 {
		t.Fatal("migrations unavailable")
	}
	sort.Strings(paths)
	for _, path := range paths {
		raw, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(raw)); e != nil {
			t.Fatal("migration failed", filepath.Base(path))
		}
	}
	auth, err := os.ReadFile("/private/auth-v5.value")
	if err != nil {
		t.Fatal("encrypted auth unavailable")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO system_settings(key,value,component,is_secret,updated_by)VALUES('BUTLER_PROVIDER_AUTH_ANTIGRAVITY',$1,'provider-auth',true,'isolated-summary-native-v5')`, strings.TrimSpace(string(auth))); err != nil {
		t.Fatal("encrypted import denied")
	}
	p := &continuityProvider{}
	begin := time.Now()
	var report continuityReport
	defer func() {
		writeProof := func(path string, raw []byte) {
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Error("native proof export failed", filepath.Base(path))
			}
		}
		pgRaw, _ := json.Marshal(trace.snapshot())
		writeProof("/proof/native-pgtrace.json", pgRaw)
		var encrypted string
		if pool.QueryRow(context.Background(), `SELECT value FROM system_settings WHERE key='BUTLER_PROVIDER_AUTH_ANTIGRAVITY'`).Scan(&encrypted) == nil {
			writeProof("/private/auth-return-v5.value", []byte(encrypted))
		} else {
			t.Error("encrypted native auth export unavailable")
		}
		_, _ = pool.Exec(context.Background(), `UPDATE memory_model_control SET state='disabled' WHERE singleton`)
		var ledger json.RawMessage
		if pool.QueryRow(context.Background(), `SELECT jsonb_build_object('background',(SELECT jsonb_agg(to_jsonb(a)) FROM memory_model_attempts a),'foreground',(SELECT jsonb_agg(to_jsonb(a)) FROM execution_model_attempts a),'scopes',(SELECT jsonb_agg(jsonb_build_object('kind',kind,'used',used_json,'limits',limits_json)) FROM execution_budget_scopes),'schema_physical_bytes',(SELECT sum(pg_total_relation_size(c.oid))FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='summary_native_v5' AND c.relkind='r'))`).Scan(&ledger) == nil {
			writeProof("/proof/full-ledger.json", ledger)
		} else {
			t.Error("native ledger export unavailable")
		}
		var reserved int
		if err := pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM memory_model_attempts)+(SELECT count(*) FROM execution_model_attempts)`).Scan(&reserved); err != nil {
			t.Error("native attempt accounting unavailable")
		}
		p.mu.Lock()
		usageRaw, _ := json.Marshal(p.usages)
		probeRaw, _ := json.Marshal(p.probes)
		costs := map[string]*nativeSummaryCosts{"baseline": {}, "compacted_foreground": {}, "summary_background": {}}
		for _, r := range p.requests {
			arm := "compacted_foreground"
			if strings.HasPrefix(r.Context.SessionKey, "memory:") {
				arm = "summary_background"
			} else if r.Context.SessionKey == "continuity-baseline" {
				arm = "baseline"
			}
			c := costs[arm]
			c.Attempts++
			raw, _ := json.Marshal(r.InputItems)
			c.SerializedInputBytes += int64(len(raw))
			c.TransportMillis += p.elapsed[r.Context.RunID]
			if u, ok := p.usages[r.Context.RunID]; ok {
				c.KnownUsage++
				c.InputTokens += u.InputTokens
				c.OutputTokens += u.OutputTokens
				c.TotalTokens += u.TotalTokens
			} else {
				c.UnknownUsage++
			}
		}
		p.mu.Unlock()
		attempts, knownTokens, knownCount := historicalAttempts+reserved, historicalTokens, 0
		for _, c := range costs {
			knownTokens += c.TotalTokens
			knownCount += c.KnownUsage
		}
		newUnknown := reserved - knownCount
		if newUnknown < 0 {
			t.Error("native observation exceeds admitted attempts")
		}
		encoded, _ := json.Marshal(map[string]any{"window": "summary-native-v5", "new_independent_budget": true, "report": report, "quality_pass": !t.Failed(), "elapsed_millis": time.Since(begin).Milliseconds(), "costs": costs, "provider": "antigravity", "model": "gemini-3.8-flash-high", "route": "server_proxy_18080", "reserved_scope_calls": 16, "actual_planned_calls": 9, "retained_v1_attempts": 3, "retained_v1_unknown_usage": 1, "retained_v2_attempts": 1, "retained_v2_unknown_usage": 1, "retained_v3_attempts": 1, "retained_v3_unknown_usage": 1, "retained_v4_attempts": v4Attempts, "retained_v4_unknown_usage": v4Unknown, "cumulative_attempts": attempts, "cumulative_known_native_total_tokens": knownTokens, "cumulative_unknown_usage_attempts": unknown + newUnknown, "monetary_basis": "subscription_no_overage", "invoice_cost": "unknown", "comparison": "independent sessions with identical primary inputs/current typed state; baseline3 full-history foreground; candidate3 primary compactions+3 ordinary foreground; no answer sharing or automatic replay; all background costs included"})
		writeProof("/proof/comparison.json", encoded)
		writeProof("/proof/native-usage.json", usageRaw)
		writeProof("/proof/native-probes.json", probeRaw)
		writeProof("/proof/retained-v1-v2-v3-ledger.json", prior)
		writeProof("/proof/retained-v4-ledger.json", v4Ledger)
		writeProof("/proof/retained-v4-native-usage.json", v4Native)
	}()
	manager := providerauth.NewManager(config.NewPostgresSettingsStore(pool))
	result, err := providerfactory.New(manager, slog.New(slog.NewTextHandler(io.Discard, nil))).Build(ctx, providerfactory.BuildConfig{ActiveProvider: "antigravity", AntigravityModel: "gemini-3.8-flash-high", Timeout: 90 * time.Second})
	if err != nil {
		t.Fatal("native provider denied")
	}
	p.base = result.Provider
	b := budget.NewController(pool, budget.DefaultPolicy())
	b.Policy.Models["antigravity:gemini-3.8-flash-high"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	b.Policy.ModelMillis = 90000
	b.Policy.Day = budget.Units{ModelTurns: 16, Tokens: 16 * 1056768, ActiveMillis: 16 * 90000, ProviderInputBytes: 16 * 8388608}
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	report = runSummaryContinuity(t, pool, p, "/private/native-v5-deletions.log", 3, result.ModelName, b, &report)
	p.mu.Lock()
	known := len(p.usages)
	p.mu.Unlock()
	if len(p.requests) != 9 || known != 9 {
		t.Fatal("incomplete native attempt accounting")
	}
}
