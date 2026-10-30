package credentials

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/credentialcontrol"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
)

func TestVaultLocalPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_VAULT_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires disposable local credential_vault_test database")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("disposable test database required")
	}
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("connect")
	}
	defer admin.Close()
	schema := randomID("vault_test_")
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create fixture schema: %v", err)
	}
	defer admin.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("pool")
	}
	defer pool.Close()
	base := `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT);CREATE TABLE runs(run_id TEXT PRIMARY KEY,session_key TEXT,task_id TEXT,current_state TEXT);CREATE TABLE tasks(task_id TEXT PRIMARY KEY,owner_id TEXT,revision BIGINT,constraints_revision BIGINT,status TEXT);CREATE TABLE server_browser_sessions(browser_session_id TEXT);INSERT INTO sessions VALUES('session','owner');INSERT INTO runs VALUES('run','session','task','tool_running');INSERT INTO tasks VALUES('task','owner',1,1,'active');`
	if _, err = pool.Exec(ctx, base); err != nil {
		t.Fatal("base fixture")
	}
	if _, err = pool.Exec(ctx, `CREATE TABLE mobile_devices(device_id TEXT PRIMARY KEY,owner_id TEXT,status TEXT,device_name TEXT)`); err != nil {
		t.Fatal("mobile fixture")
	}
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "096_credential_vault.up.sql"))
	if err != nil {
		t.Fatal("migration source")
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("migration: %v", err)
	}
	v, err := NewVault(pool, strings.Repeat("synthetic-key-", 4))
	if err != nil {
		t.Fatal("vault")
	}
	token, err := v.Unlock(ctx, "owner", "654321", "654321")
	if err != nil || v.CheckUnlock(ctx, "owner", token) != nil {
		t.Fatal("first unlock")
	}
	if _, err = v.Unlock(ctx, "owner", "000000", ""); err == nil {
		t.Fatal("wrong PIN accepted")
	}
	if _, err = v.Unlock(ctx, "owner", "654321", ""); err == nil {
		t.Fatal("rate limit bypassed")
	}
	if v.CheckUnlock(ctx, "other", token) == nil {
		t.Fatal("cross owner unlock")
	}
	first, err := v.Save(ctx, "owner", EntryInput{Alias: "shop.personal", Title: "Shop", SiteHint: "https://shop.example", Username: "same@example.com", Password: "synthetic-first-password"})
	if err != nil {
		t.Fatal("save first")
	}
	second, err := v.Save(ctx, "owner", EntryInput{Alias: "forum.personal", Title: "Forum", SiteHint: "https://forum.example", Username: "same@example.com", Password: "synthetic-second-password"})
	if err != nil {
		t.Fatal("save second")
	}
	found, err := v.List(ctx, "owner", "same@example.com")
	if err != nil || len(found) != 2 {
		t.Fatal("same login search")
	}
	raw, _ := json.Marshal(found)
	if strings.Contains(string(raw), "same@example.com") || strings.Contains(string(raw), "synthetic-first-password") {
		t.Fatal("metadata contains plaintext")
	}
	var cipher string
	if pool.QueryRow(ctx, `SELECT username_cipher||password_cipher FROM credential_vault WHERE id=$1`, first.ID).Scan(&cipher) != nil || strings.Contains(cipher, "same@example.com") {
		t.Fatal("plaintext DB")
	}
	p := Principal{OwnerID: "owner", SessionKey: "session", RunID: "run", TaskID: "task", TaskRevision: 1, ConstraintsRevision: 1}
	// A site hint is never a permanent allowlist: the owner can approve another site.
	req, err := v.CreateRequest(ctx, p, "call", "access", first.ID, "https://different.example/login", "browser", "Sign in", []string{"username", "password"})
	if err != nil {
		t.Fatal("request")
	}
	if _, err = v.GetByAlias(ctx, req.ID); err == nil {
		t.Fatal("pending request grants access")
	}
	if v.Respond(ctx, "owner", req.ID, first.ID, true, "") != nil {
		t.Fatal("approve")
	}
	if v.Respond(ctx, "owner", req.ID, first.ID, true, "") != nil || v.Respond(ctx, "owner", req.ID, second.ID, true, "") == nil || v.Respond(ctx, "owner", req.ID, "", false, "") == nil {
		t.Fatal("response replay changed accepted decision")
	}
	metadata, err := v.GetByAlias(ctx, req.ID)
	if err != nil {
		t.Fatal("approved metadata")
	}
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "fill-call", ToolName: "browser.prepare_fill", ArgsJson: `{"browser_session_id":"browser","expected_url":"https://different.example/login"}`}
	for field, expected := range map[string]string{"username": "same@example.com", "password": "synthetic-first-password"} {
		value, e := v.ResolveField(ctx, call, metadata, field)
		if e != nil || value != expected {
			t.Fatal("separate field resolution")
		}
	}
	changed := proto.Clone(call).(*toolbrokerv1.ToolCall)
	changed.ArgsJson = `{"browser_session_id":"browser","expected_url":"https://evil.example/login"}`
	if _, err = v.ResolveField(ctx, changed, metadata, "password"); err == nil {
		t.Fatal("destination changed")
	}
	changed = proto.Clone(call).(*toolbrokerv1.ToolCall)
	changed.RunId = "foreign"
	if _, err = v.ResolveField(ctx, changed, metadata, "password"); err == nil {
		t.Fatal("cross run grant")
	}
	if _, err = v.Save(ctx, "owner", EntryInput{ID: first.ID, Alias: first.Alias, Title: first.Title, ExpectedRevision: first.Revision, Password: "rotated-synthetic-password"}); err != nil {
		t.Fatal("rotate")
	}
	if _, err = v.ResolveField(ctx, call, metadata, "password"); err == nil {
		t.Fatal("old grant survived rotation")
	}
	otp, err := v.CreateRequest(ctx, p, "otp-call", "otp", "", "https://different.example/otp", "browser", "Complete sign in", []string{"otp"})
	if err != nil {
		t.Fatal("OTP request")
	}
	if v.Respond(ctx, "owner", otp.ID, "", true, "739284") != nil {
		t.Fatal("OTP response")
	}
	if v.Respond(ctx, "owner", otp.ID, "", true, "739284") != nil || v.Respond(ctx, "owner", otp.ID, "", true, "000000") == nil {
		t.Fatal("OTP replay changed accepted code")
	}
	otpMeta, err := v.GetByAlias(ctx, otp.ID)
	if err != nil {
		t.Fatal("OTP metadata")
	}
	if code, e := v.ResolveField(ctx, call, otpMeta, "otp"); e != nil || code != "739284" {
		t.Fatal("OTP release")
	}
	otherCall := proto.Clone(call).(*toolbrokerv1.ToolCall)
	otherCall.ToolCallId = "different-call"
	if _, err = v.ResolveField(ctx, otherCall, otpMeta, "otp"); err == nil {
		t.Fatal("OTP reused")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO mobile_devices VALUES('phone','owner','ACTIVE','Test phone')`); err != nil {
		t.Fatal("paired phone")
	}
	route := SMSRoute{RecordID: second.ID, DeviceID: "phone", Package: "sms.app", Sender: "Example", Enabled: true}
	if v.SaveSMSRoute(ctx, "owner", route) != nil {
		t.Fatal("SMS opt-in")
	}
	parent, err := v.CreateRequest(ctx, p, "sms-parent", "access", second.ID, "https://different.example/login", "browser", "Sign in", []string{"password"})
	if err != nil {
		t.Fatal("SMS parent")
	}
	if v.Respond(ctx, "owner", parent.ID, second.ID, true, "") != nil {
		t.Fatal("SMS parent approval")
	}
	t.Run("parent_consent_pins_task_versions", func(t *testing.T) {
		if _, e := pool.Exec(ctx, `UPDATE tasks SET constraints_revision=2 WHERE task_id='task'`); e != nil {
			t.Fatal(e)
		}
		defer pool.Exec(ctx, `UPDATE tasks SET constraints_revision=1 WHERE task_id='task'`)
		changed := p
		changed.ConstraintsRevision = 2
		if _, e := v.CreateRequest(ctx, changed, "stale-parent", "otp", "", "https://different.example/otp", "browser", "Finish sign in", []string{"otp"}, parent.ID); e == nil {
			t.Fatal("old password consent authorized an OTP under changed constraints")
		}
	})
	sms, err := v.CreateRequest(ctx, p, "sms-challenge", "otp", "", "https://different.example/otp", "browser", "Finish sign in", []string{"otp"}, parent.ID)
	if err != nil {
		t.Fatal("SMS challenge")
	}
	if _, err = pool.Exec(ctx, `UPDATE runs SET current_state='awaiting_approval';UPDATE tasks SET status='waiting_for_user'`); err != nil {
		t.Fatal("SMS waiting")
	}
	relay := func(candidate SMSCandidate) bool {
		t.Helper()
		tx, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal("relay transaction")
		}
		defer tx.Rollback(ctx)
		accepted, e := v.RelaySMS(ctx, tx, "owner", "phone", candidate)
		if e != nil {
			t.Fatal("relay unavailable")
		}
		if tx.Commit(ctx) != nil {
			t.Fatal("relay commit")
		}
		return accepted
	}
	candidate := SMSCandidate{EventID: "synthetic-sms-event-1", Package: "sms.app", Sender: "Example", Code: "284739", PostedAt: time.Now().Add(time.Second).UnixMilli()}
	wrong := candidate
	wrong.Sender = "Foreign"
	if relay(wrong) {
		t.Fatal("foreign sender accepted")
	}
	if !relay(candidate) || !relay(candidate) {
		t.Fatal("SMS acceptance or idempotent receipt")
	}
	if _, err = pool.Exec(ctx, `UPDATE runs SET current_state='tool_running';UPDATE tasks SET status='active'`); err != nil {
		t.Fatal("SMS resume")
	}
	smsMeta, err := v.GetByAlias(ctx, sms.ID)
	if err != nil {
		t.Fatal("SMS metadata")
	}
	if code, e := v.ResolveField(ctx, call, smsMeta, "otp"); e != nil || code != "284739" {
		t.Fatal("SMS private release")
	}
	route.Enabled = false
	if v.SaveSMSRoute(ctx, "owner", route) != nil {
		t.Fatal("disable SMS route")
	}
	if _, err = v.ResolveField(ctx, call, smsMeta, "otp"); err == nil {
		t.Fatal("disabled source released SMS code")
	}
	t.Run("revoked_sms_source_invalidates_metadata", func(t *testing.T) {
		if _, e := v.GetByAlias(ctx, sms.ID); e == nil {
			t.Fatal("final dispatch metadata survived SMS route revocation")
		}
	})
	route.Enabled = true
	if v.SaveSMSRoute(ctx, "owner", route) != nil {
		t.Fatal("enable source")
	}
	for _, id := range []string{"ambiguous-one", "ambiguous-two"} {
		if _, err = v.CreateRequest(ctx, p, id, "otp", "", "https://different.example/otp", "browser", "Finish sign in", []string{"otp"}, parent.ID); err != nil {
			t.Fatal("ambiguous fixture")
		}
	}
	if _, err = pool.Exec(ctx, `UPDATE runs SET current_state='awaiting_approval';UPDATE tasks SET status='waiting_for_user'`); err != nil {
		t.Fatal("ambiguous waiting")
	}
	candidate.EventID = "synthetic-sms-event-2"
	candidate.PostedAt = time.Now().Add(time.Second).UnixMilli()
	if relay(candidate) {
		t.Fatal("ambiguous OTP selected automatically")
	}
	if v.Remove(ctx, "owner", second.ID, second.Revision, true) != nil {
		t.Fatal("delete")
	}
	t.Run("stopped_expired_access_receipt_grants_no_secret", func(t *testing.T) {
		if _, e := pool.Exec(ctx, `ALTER TABLE server_browser_sessions ADD COLUMN owner_id TEXT,ADD COLUMN run_id TEXT,ADD COLUMN mode TEXT,ADD COLUMN expires_at timestamptz;
 INSERT INTO runs VALUES('stopped-access','session',NULL,'tool_running');
 INSERT INTO server_browser_sessions(browser_session_id,observed_url,owner_id,run_id,mode,expires_at) VALUES('stopped-browser','https://different.example/login','owner','stopped-access','agent',now()-interval '1 hour')`); e != nil {
			t.Fatal("receipt fixture", e)
		}
		entry, e := v.Save(ctx, "owner", EntryInput{Alias: "stopped-receipt", Title: "Receipt", Username: "synthetic-user", Password: "synthetic-secret"})
		if e != nil {
			t.Fatal("save receipt fixture")
		}
		r, e := v.CreateRequest(ctx, Principal{OwnerID: "owner", SessionKey: "session", RunID: "stopped-access"}, "lost-ack", "access", entry.ID, "https://different.example/login", "stopped-browser", "Sign in", []string{"username", "password"})
		if e != nil {
			t.Fatal("create real owning request")
		}
		if _, e = pool.Exec(ctx, `UPDATE credential_vault_requests SET state='expired',expires_at=now()-interval '1 hour' WHERE id=$1;`, r.ID); e != nil {
			t.Fatal("expire request")
		}
		if _, e = pool.Exec(ctx, `UPDATE runs SET current_state='failed' WHERE run_id='stopped-access'`); e != nil {
			t.Fatal("stop run")
		}
		tx, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		if _, e = tx.Exec(ctx, `SELECT 1 FROM runs WHERE run_id='stopped-access' FOR UPDATE;SELECT 1 FROM sessions WHERE session_key='session' FOR UPDATE`); e != nil {
			t.Fatal("fence")
		}
		args, receipt, e := credentialcontrol.StoppedAccessReceiptTx(ctx, tx, credentialcontrol.StoppedAccessIdentity{OwnerID: "owner", SessionKey: "session", RunID: "stopped-access"}, "lost-ack")
		if e != nil || len(receipt) != 64 || strings.Contains(args, "synthetic-secret") || !strings.Contains(args, entry.ID) {
			t.Fatal("owning creation receipt", e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		if _, e = v.GetByAlias(ctx, r.ID); e == nil {
			t.Fatal("receipt authorized credential metadata")
		}
		if e = v.Respond(ctx, "owner", r.ID, entry.ID, true, ""); e == nil {
			t.Fatal("receipt reopened expired consent")
		}
	})
	if v.Lock(ctx, "owner", token) != nil || v.CheckUnlock(ctx, "owner", token) == nil {
		t.Fatal("lock")
	}
	t.Run("migration_rollback_and_reapply", func(t *testing.T) {
		down, e := os.ReadFile(filepath.Join("..", "..", "migrations", "096_credential_vault.down.sql"))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(down)); e != nil {
			t.Fatal("vault rollback", e)
		}
		if _, e = pool.Exec(ctx, string(migration)); e != nil {
			t.Fatal("vault reapply", e)
		}
	})
}
