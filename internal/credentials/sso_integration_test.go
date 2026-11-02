package credentials

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/browsersso"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ssoFixture(t *testing.T) (*Vault, Principal, Entry, browsersso.Scope) {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_VAULT_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires disposable *_test PostgreSQL")
	}
	ctx := context.Background()
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("disposable database required")
	}
	admin, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	schema := randomID("sso_test_")
	if _, e = admin.Exec(ctx, `CREATE SCHEMA `+schema); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { admin.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`); admin.Close() })
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	base := `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT);CREATE TABLE runs(run_id TEXT PRIMARY KEY,session_key TEXT,task_id TEXT,current_state TEXT);CREATE TABLE tasks(task_id TEXT PRIMARY KEY,owner_id TEXT,revision BIGINT,constraints_revision BIGINT,status TEXT);CREATE TABLE server_browser_sessions(browser_session_id TEXT PRIMARY KEY,run_id TEXT,owner_id TEXT,task_id TEXT,mode TEXT,epoch BIGINT NOT NULL DEFAULT 0,expires_at TIMESTAMPTZ);INSERT INTO sessions VALUES('session','owner');INSERT INTO runs VALUES('run','session','task','tool_running');INSERT INTO tasks VALUES('task','owner',1,1,'active');`
	if _, e = pool.Exec(ctx, base); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"096_credential_vault", "106_browser_login_consent", "107_browser_pages", "108_browser_page_execution", "109_browser_sso_consent"} {
		raw, e := os.ReadFile(filepath.Join("..", "..", "migrations", name+".up.sql"))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(raw)); e != nil {
			t.Fatalf("%s: %v", name, e)
		}
	}
	v, e := NewVault(pool, strings.Repeat("synthetic-sso-key", 3))
	if e != nil {
		t.Fatal(e)
	}
	entry, e := v.Save(ctx, "owner", EntryInput{Alias: "test.idp", Title: "Dedicated test IdP", Username: "synthetic-private-user", Password: "synthetic-private-password"})
	if e != nil {
		t.Fatal(e)
	}
	s := browsersso.Scope{Browser: strings.Repeat("c", 32), RPPage: browserpage.Ref{ID: strings.Repeat("a", 32), DocumentID: strings.Repeat("b", 32), Version: 1}, RPURL: "https://rp.example/", Trigger: "#sso", IdPURL: "https://idp.example/authorize", TargetURL: "https://idp.example/login", Form: "form", Username: "#u", Password: "#p", VerifyURL: "https://rp.example/account"}
	if _, e = pool.Exec(ctx, `INSERT INTO server_browser_sessions(browser_session_id,run_id,owner_id,task_id,mode,expires_at,page_mode,page_origin) VALUES($1,'run','owner','task','agent',now()+interval '15 minutes',true,'https://rp.example')`, s.Browser); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `INSERT INTO server_browser_pages(browser_session_id,page_id,document_id,observation_version,observed_epoch,observed_url,state,runtime_version) VALUES($1,$2,$3,1,0,'https://rp.example/','ready',1)`, s.Browser, s.RPPage.ID, s.RPPage.DocumentID); e != nil {
		t.Fatal(e)
	}
	return v, Principal{OwnerID: "owner", SessionKey: "session", RunID: "run", TaskID: "task", TaskRevision: 1, ConstraintsRevision: 1}, entry, s
}

func TestVaultSSOPostgresIntegration(t *testing.T) {
	ctx := context.Background()
	t.Run("protected_scope_and_no_legacy_release", func(t *testing.T) {
		v, p, entry, s := ssoFixture(t)
		r, e := v.CreateSSORequest(ctx, p, "sso-once", entry.ID, "One test SSO login", s)
		if e != nil {
			t.Fatal(e)
		}
		again, e := v.CreateSSORequest(ctx, p, "sso-once", entry.ID, r.Purpose, s)
		if e != nil || again.ID != r.ID || !again.ExpiresAt.Equal(r.ExpiresAt) {
			t.Fatal("request repeat changed identity or expiry")
		}
		separate, e := v.CreateSSORequest(ctx, p, "another-sso", entry.ID, r.Purpose, s)
		if e != nil || separate.ScopeDigest == r.ScopeDigest {
			t.Fatal("card digest transferred across attempts")
		}
		if v.RespondSSO(ctx, p.OwnerID, separate.ID, entry.ID, true, browsersso.Consent, r.ScopeDigest) == nil {
			t.Fatal("old card approved a different request")
		}
		if v.RespondSSO(ctx, p.OwnerID, separate.ID, "", false, "", "") != nil || v.RespondSSO(ctx, p.OwnerID, separate.ID, "", false, "", "") != nil {
			t.Fatal("marker-free denial unavailable or not idempotent")
		}
		changed := s
		changed.Trigger = "#different"
		if _, e = v.CreateSSORequest(ctx, p, "sso-once", entry.ID, r.Purpose, changed); e == nil {
			t.Fatal("same call substituted consent")
		}
		listed, e := v.OwnerRequests(ctx, p.OwnerID)
		if e != nil || len(listed) != 1 || listed[0].SSOScope == nil || listed[0].ScopeDigest != r.ScopeDigest {
			t.Fatal("protected projection unavailable")
		}
		raw, _ := json.Marshal(listed)
		if strings.Contains(string(raw), "synthetic-private") {
			t.Fatal("secret in owner metadata")
		}
		foreign, e := v.OwnerRequests(ctx, "other")
		if e != nil || len(foreign) != 0 {
			t.Fatal("cross-owner metadata")
		}
		for _, answer := range [][3]string{{"", r.ScopeDigest, entry.ID}, {"browser_login_once_v1", r.ScopeDigest, entry.ID}, {browsersso.Consent, strings.Repeat("0", 64), entry.ID}, {browsersso.Consent, r.ScopeDigest, "other-account"}} {
			if v.RespondSSO(ctx, p.OwnerID, r.ID, answer[2], true, answer[0], answer[1]) == nil {
				t.Fatal("generic or substituted approval accepted")
			}
		}
		if v.RespondSSO(ctx, "other", r.ID, entry.ID, true, browsersso.Consent, r.ScopeDigest) == nil {
			t.Fatal("foreign owner approved")
		}
		var wg sync.WaitGroup
		failures := make(chan error, 8)
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				failures <- v.RespondSSO(ctx, p.OwnerID, r.ID, entry.ID, true, browsersso.Consent, r.ScopeDigest)
			}()
		}
		wg.Wait()
		close(failures)
		for e := range failures {
			if e != nil {
				t.Fatal("idempotent concurrent answer failed", e)
			}
		}
		var count int
		if v.Pool.QueryRow(ctx, `SELECT count(*) FROM credential_vault_audit WHERE request_id=$1 AND operation='sso_approved'`, r.ID).Scan(&count) != nil || count != 1 {
			t.Fatal("decision repeated")
		}
		if _, e = v.GetByAlias(ctx, r.ID); e == nil {
			t.Fatal("SSO consent became legacy alias")
		}
		for _, tool := range []string{"browser.prepare_fill", "browser.login", "browser.sso_login"} {
			call := &toolbrokerv1.ToolCall{RunId: p.RunID, ToolCallId: "attempt", ToolName: tool, ArgsJson: `{"browser_session_id":"` + s.Browser + `","expected_url":"https://idp.example/authorize"}`, CredentialRefs: []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: r.ID, Field: "password"}}}
			if v.CheckGrantedCall(ctx, call) {
				t.Fatal("unimplemented protocol gained release")
			}
			if _, e = v.ResolveField(ctx, call, Record{Alias: r.ID}, "password"); e == nil {
				t.Fatal("SSO consent released a field")
			}
		}
		if v.RespondSSO(ctx, p.OwnerID, r.ID, entry.ID, false, "", "") == nil {
			t.Fatal("approved decision replaced")
		}
		if e = v.Remove(ctx, p.OwnerID, entry.ID, entry.Revision, true); e != nil {
			t.Fatal("retained consent blocked account deletion", e)
		}
		for _, sql := range []string{`DELETE FROM credential_sso_requests`, `TRUNCATE credential_sso_requests`, `UPDATE credential_sso_requests SET purpose='replacement'`} {
			if _, e = v.Pool.Exec(ctx, sql); e == nil {
				t.Fatal("consent history altered")
			}
		}
		down, e := os.ReadFile(filepath.Join("..", "..", "migrations", "109_browser_sso_consent.down.sql"))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = v.Pool.Exec(ctx, string(down)); e == nil {
			t.Fatal("downgrade deleted consent")
		}
	})
	t.Run("expiry_is_clipped_and_cannot_be_renewed", func(t *testing.T) {
		v, p, entry, s := ssoFixture(t)
		if _, e := v.Pool.Exec(ctx, `UPDATE server_browser_sessions SET expires_at=clock_timestamp()+interval '1 second'`); e != nil {
			t.Fatal(e)
		}
		r, e := v.CreateSSORequest(ctx, p, "short-sso", entry.ID, "Short consent", s)
		if e != nil {
			t.Fatal(e)
		}
		if delay := time.Until(r.ExpiresAt); delay > time.Second {
			t.Fatal("request outlived context")
		}
		time.Sleep(time.Until(r.ExpiresAt) + 20*time.Millisecond)
		if v.RespondSSO(ctx, p.OwnerID, r.ID, entry.ID, true, browsersso.Consent, r.ScopeDigest) == nil {
			t.Fatal("expired consent approved")
		}
		if _, e = v.CreateSSORequest(ctx, p, "short-sso", entry.ID, r.Purpose, s); e == nil {
			t.Fatal("expired request renewed")
		}
		pending, e := v.OwnerRequests(ctx, p.OwnerID)
		if e != nil || len(pending) != 0 {
			t.Fatal("expired card remained pending")
		}
	})
	t.Run("request_lock_cannot_extend_consent_ttl", func(t *testing.T) {
		v, p, entry, s := ssoFixture(t)
		if _, e := v.Pool.Exec(ctx, `UPDATE server_browser_sessions SET expires_at=clock_timestamp()+interval '3 seconds'`); e != nil {
			t.Fatal(e)
		}
		r, e := v.CreateSSORequest(ctx, p, "locked-sso", entry.ID, "Bounded answer", s)
		if e != nil {
			t.Fatal(e)
		}
		tx, e := v.Pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		var id string
		if tx.QueryRow(ctx, `SELECT id FROM credential_sso_requests WHERE id=$1 FOR UPDATE`, r.ID).Scan(&id) != nil {
			t.Fatal("hold request")
		}
		answer := make(chan error, 1)
		go func() {
			answer <- v.RespondSSO(ctx, p.OwnerID, r.ID, entry.ID, true, browsersso.Consent, r.ScopeDigest)
		}()
		blocked := false
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
			if v.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)) AND query LIKE 'SELECT state FROM credential_sso_requests%')`, int64(tx.Conn().PgConn().PID())).Scan(&blocked) != nil {
				t.Fatal("observe lock wait")
			}
			if blocked {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !blocked {
			t.Fatal("answer never reached the held request lock")
		}
		time.Sleep(time.Until(r.ExpiresAt) + 20*time.Millisecond)
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		select {
		case e := <-answer:
			if e == nil {
				t.Fatal("lock wait extended consent")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("answer failed to settle")
		}
		var state string
		if v.Pool.QueryRow(ctx, `SELECT state FROM credential_sso_requests WHERE id=$1`, r.ID).Scan(&state) != nil || state != "pending" {
			t.Fatal("expired decision persisted")
		}
	})
	for _, c := range []struct{ name, sql string }{
		{"owner_takeover", `UPDATE server_browser_sessions SET mode='owner'`},
		{"runtime_epoch", `UPDATE server_browser_sessions SET epoch=epoch+1`},
		{"expired_context", `UPDATE server_browser_sessions SET expires_at=now()-interval '1 second'`},
		{"foreign_run", `UPDATE server_browser_sessions SET run_id='other'`},
		{"new_document", `UPDATE server_browser_pages SET document_id='dddddddddddddddddddddddddddddddd',observation_version=2`},
		{"new_observation", `UPDATE server_browser_pages SET observation_version=2`},
		{"page_lost", `UPDATE server_browser_pages SET state='lost',observation_version=2`},
		{"task_revision", `UPDATE tasks SET revision=2`},
		{"constraint_revision", `UPDATE tasks SET constraints_revision=2`},
		{"account_revision", `UPDATE credential_vault SET revision=revision+1`},
		{"account_revoked", `UPDATE credential_vault SET status='revoked'`},
		{"run_terminal", `UPDATE runs SET current_state='completed'`},
	} {
		t.Run(c.name, func(t *testing.T) {
			v, p, entry, s := ssoFixture(t)
			r, e := v.CreateSSORequest(ctx, p, "scope-fence", entry.ID, "One login", s)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = v.Pool.Exec(ctx, c.sql); e != nil {
				t.Fatal(e)
			}
			if v.RespondSSO(ctx, p.OwnerID, r.ID, entry.ID, true, browsersso.Consent, r.ScopeDigest) == nil {
				t.Fatal("stale authority approved")
			}
			if _, e = v.CreateSSORequest(ctx, p, "new-call", entry.ID, "One login", s); e == nil && c.name != "account_revision" {
				t.Fatal("stale creation accepted")
			}
		})
	}
}
