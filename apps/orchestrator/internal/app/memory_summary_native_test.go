package app

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// This independent paired window preserves v1's3 reservations and unknown usage.
// No model call can run twice under the same persistent admission identity.
func TestNativeOrdinaryTaskSummaryContinuityIntegration(t *testing.T) {
	if os.Getenv("BUTLER_SUMMARY_NATIVE_V2") != "1" {
		t.Skip("explicit independent native summary continuity window required")
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
	var prior []byte
	if err = admin.QueryRow(ctx, `SELECT jsonb_agg(jsonb_build_object('state',state,'usage',usage,'quote',quote,'reason',reason)) FROM summary_native_v1.memory_model_attempts`).Scan(&prior); err != nil {
		t.Fatal("retained v1 ledger missing")
	}
	if _, err = admin.Exec(ctx, `INSERT INTO issue84_native_windows(id)VALUES('summary-native-v2');CREATE SCHEMA summary_native_v2`); err != nil {
		t.Fatal("native one-use window denied")
	}
	cfg.MaxConns = 2
	cfg.ConnConfig.RuntimeParams["search_path"] = "summary_native_v2,public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
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
	auth, err := os.ReadFile("/private/auth.value")
	if err != nil {
		t.Fatal("encrypted auth unavailable")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO system_settings(key,value,component,is_secret,updated_by)VALUES('BUTLER_PROVIDER_AUTH_ANTIGRAVITY',$1,'provider-auth',true,'isolated-summary-native-v2')`, strings.TrimSpace(string(auth))); err != nil {
		t.Fatal("encrypted import denied")
	}
	p := &continuityProvider{}
	defer func() {
		var encrypted string
		if pool.QueryRow(context.Background(), `SELECT value FROM system_settings WHERE key='BUTLER_PROVIDER_AUTH_ANTIGRAVITY'`).Scan(&encrypted) == nil {
			_ = os.WriteFile("/private/auth-return-v2.value", []byte(encrypted), 0600)
		}
		var ledger []byte
		if pool.QueryRow(context.Background(), `SELECT jsonb_build_object('background',(SELECT jsonb_agg(to_jsonb(a)) FROM memory_model_attempts a),'foreground',(SELECT jsonb_agg(to_jsonb(a)) FROM execution_model_attempts a),'scopes',(SELECT jsonb_agg(jsonb_build_object('kind',kind,'used',used_json,'limits',limits_json)) FROM execution_budget_scopes),'schema_physical_bytes',(SELECT sum(pg_total_relation_size(c.oid))FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='summary_native_v2' AND c.relkind='r'))`).Scan(&ledger) == nil {
			_ = os.WriteFile("/proof/full-ledger.json", ledger, 0600)
		}
		p.mu.Lock()
		usageRaw, _ := json.Marshal(p.usages)
		probeRaw, _ := json.Marshal(p.probes)
		p.mu.Unlock()
		_ = os.WriteFile("/proof/native-usage.json", usageRaw, 0600)
		_ = os.WriteFile("/proof/native-probes.json", probeRaw, 0600)
		_ = os.WriteFile("/proof/retained-v1-ledger.json", prior, 0600)
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
	begin := time.Now()
	report := runSummaryContinuity(t, pool, p, "/private/native-v2-deletions.log", 3, result.ModelName, b)
	encoded, _ := json.Marshal(map[string]any{"report": report, "elapsed_millis": time.Since(begin).Milliseconds(), "provider": "antigravity", "model": result.ModelName, "route": "server_proxy_18080", "reserved_scope_calls": 16, "actual_planned_calls": 9, "retained_v1_attempts": 3, "retained_v1_unknown_usage": 1, "monetary_basis": "subscription_no_overage", "invoice_cost": "unknown", "comparison": "baseline three full-history foreground calls; candidate three compactions plus three foreground calls; all attempts and complete native token observations separate"})
	_ = os.WriteFile("/proof/comparison.json", encoded, 0600)
}
