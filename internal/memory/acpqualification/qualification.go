// Package acpqualification contains opt-in test support, never a product service.
// Each case is a once-only, retained window against an isolated *_test database.
package acpqualification

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravityacp"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Case struct {
	Pool        *pgxpool.Pool
	Manifest    *catalog.FileManifest
	Provider    transport.ModelProvider
	Policy      budget.Policy
	ID          string
	EvidenceDir string
	MaxCalls    int
}

func Open(t *testing.T, id string, maxCalls int) *Case {
	t.Helper()
	proof := os.Getenv("BUTLER_ACP_AUX_EVIDENCE")
	if proof == "" {
		t.Skip("explicit persistent ACP auxiliary qualification required")
	}
	recovery := id == "summary-recovery-v1" && maxCalls == 1
	if (!recovery && id != "summary-v1" && id != "consolidation-v1" && id != "cancel-v1") || maxCalls < 1 || maxCalls > 2 {
		t.Fatal("undeclared auxiliary case")
	}
	var caps struct {
		SourceSHA256 string
		ExpiresAt    time.Time
		MaxCalls     int
	}
	raw, err := os.ReadFile(filepath.Join(proof, "aux-caps.json"))
	expectedCalls := 5
	if recovery {
		expectedCalls = 1
	}
	if err != nil || json.Unmarshal(raw, &caps) != nil || len(caps.SourceSHA256) != 64 || caps.MaxCalls != expectedCalls || !time.Now().Before(caps.ExpiresAt) {
		t.Fatal("frozen auxiliary caps required")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_ACP_MEMORY_POSTGRES_URL"))
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("isolated persistent database required")
	}
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var prior int
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM memory_model_attempts`).Scan(&prior); err != nil {
		t.Fatal(err)
	}
	claim, _ := json.Marshal(map[string]any{"source_sha256": caps.SourceSHA256, "max_calls": maxCalls, "expires_at": caps.ExpiresAt, "attempts_before": prior})
	tag, err := pool.Exec(context.Background(), `INSERT INTO acp_qualification_windows(window_id,caps) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, claim)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatal("auxiliary window already consumed", err)
	}
	manifest := catalog.NewFileManifest(filepath.Join(proof, "deletions.log"))
	if _, err = os.Stat(filepath.Join(proof, "deletions.log")); err != nil {
		t.Fatal("existing independent anchor required")
	}
	p := budget.DefaultPolicy()
	p.ModelMillis = 300000
	p.Day.ModelTurns = 40
	p.Models["antigravity-acp:gemini-3.8-flash"] = budget.ModelPrice{Basis: budget.AgentSubscriptionBasis}
	provider, err := antigravityacp.NewProvider(antigravityacp.Config{Enabled: true, SocketPath: os.Getenv("BUTLER_ACP_MEMORY_SOCKET"), Model: "gemini-3.8-flash", Timeout: 300 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	c := &Case{Pool: pool, Manifest: manifest, Provider: provider, Policy: p, ID: id, EvidenceDir: proof, MaxCalls: maxCalls}
	t.Cleanup(func() {
		ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		var count int
		var ledger json.RawMessage
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM memory_model_attempts`).Scan(&count); err != nil {
			t.Error(err)
		}
		if count-prior > maxCalls {
			t.Error("auxiliary call window exceeded")
		}
		if err := pool.QueryRow(ctx, `SELECT jsonb_build_object('attempts',(SELECT jsonb_agg(to_jsonb(a)) FROM memory_model_attempts a),'scopes',(SELECT jsonb_agg(to_jsonb(s)) FROM execution_budget_scopes s))`).Scan(&ledger); err != nil {
			t.Error(err)
		}
		data, _ := json.MarshalIndent(map[string]any{"case": id, "source_sha256": caps.SourceSHA256, "attempts_before": prior, "attempts_after": count, "max_calls": maxCalls, "passed": !t.Failed(), "ledger": ledger}, "", "  ")
		if err := os.WriteFile(filepath.Join(proof, id+"-receipt.json"), data, 0600); err != nil {
			t.Error(err)
		}
	})
	return c
}

func (c *Case) Allowance() budget.Units {
	return budget.Units{ModelTurns: int64(c.MaxCalls), ActiveMillis: int64(c.MaxCalls) * 300000, ProviderInputBytes: int64(c.MaxCalls) * 8388608, EgressBytes: int64(c.MaxCalls) * 65536}
}
