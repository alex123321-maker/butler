package actionauth

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/budget"
	"os"
	"strings"
	"testing"
	"time"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestOwnerOutcomePinnedAccessRequestContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/stopped_request_access_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var c toolbrokerv1.ToolContract
	if json.Unmarshal(raw, &c) != nil {
		t.Fatal("contract fixture")
	}
	hash, err := ContractHash(&c)
	if err != nil || hash != stoppedAccessRequestContract || c.SupportsCredentialRefs {
		t.Fatal("contract drift", hash, err)
	}
}

func TestOwnerOutcomePinnedCurrentMessageContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/stopped_message_00cb.json")
	if err != nil {
		t.Fatal(err)
	}
	var c toolbrokerv1.ToolContract
	if json.Unmarshal(raw, &c) != nil {
		t.Fatal("contract fixture")
	}
	h, err := ContractHash(&c)
	if err != nil || h != stoppedMessage00cbContract || c.ContractRevision != "2" || c.SupportsCredentialRefs {
		t.Fatal("publication pin drift", h, err)
	}
}

func TestOwnerOutcomeStoppedExpiredAccessRequestIntegration(t *testing.T) {
	for _, mode := range []string{"valid", "contract", "revision", "tool", "credential", "args", "live_lease", "live_input", "live_run", "grant", "dispatch", "missing", "owner", "session", "run", "call", "versions", "pending", "approved", "unexpired", "notice", "notice_ack", "notice_started", "parent", "browser_owner", "browser_run", "browser_live", "browser_url", "fields", "duplicate_fields", "missing_audit", "foreign_audit", "released", "pruned_valid", "pruned_digest", "pruned_foreign", "pruned_live", "pruned_no_receipt"} {
		t.Run(mode, func(t *testing.T) {
			args := `{"record_id":"record","browser_session_id":"browser","expected_url":"https://example.com/login","purpose":"Sign in","fields":["username","password"]}`
			hash, err := ArgumentsHash(args)
			if err != nil {
				t.Fatal(err)
			}
			pool, b := stoppedLocalFixture(t, false, func(b *ActionBinding) {
				b.ToolName, b.ContractRevision, b.ContractHash = "credentials.request_access", "1", stoppedAccessRequestContract
				b.Resource, b.ArgsHash, b.CommunicationDestinationsHash = "tool:credentials.request_access", hash, ""
				switch mode {
				case "contract":
					b.ContractHash = strings.Repeat("f", 64)
				case "revision":
					b.ContractRevision = "2"
				case "tool":
					b.ToolName = "credentials.request_otp"
				case "credential":
					b.CredentialRefs = []CredentialRef{{Alias: "record", Field: "password"}}
				case "args":
					b.ArgsHash = strings.Repeat("f", 64)
				}
			})
			ctx := context.Background()
			_, err = pool.Exec(ctx, `CREATE TABLE server_browser_sessions(browser_session_id TEXT PRIMARY KEY,owner_id TEXT,run_id TEXT,mode TEXT,expires_at timestamptz,observed_url TEXT);
 INSERT INTO server_browser_sessions VALUES('browser','owner','run','agent',now()-interval '1 hour','https://example.com/login');
 CREATE TABLE credential_vault_requests(id TEXT PRIMARY KEY,owner_id TEXT,session_key TEXT,run_id TEXT,task_id TEXT,task_revision BIGINT,constraints_revision BIGINT,tool_call_id TEXT,kind TEXT,record_id TEXT,record_revision BIGINT,target_origin TEXT,browser_session_id TEXT,purpose TEXT,fields JSONB,state TEXT,expires_at timestamptz,notice_state TEXT,notice_ack TEXT,notice_started_at timestamptz,parent_request_id TEXT);
 INSERT INTO credential_vault_requests VALUES('request','owner','web:owner','run','',0,0,'old-call','access','record',1,'https://example.com','browser','Sign in','["username","password"]','expired',now()-interval '1 hour','pending','',NULL,'');
 CREATE TABLE credential_vault_audit(request_id TEXT,owner_id TEXT,record_id TEXT,operation TEXT);
 INSERT INTO credential_vault_audit VALUES('request','owner','record','requested');
 UPDATE action_authorizations g SET grant_data=jsonb_build_object('binding',e.initial_binding) FROM action_effects e WHERE g.action_id=e.action_id;`)
			if err != nil {
				t.Fatal(err)
			}
			changes := map[string]string{
				"live_lease":       `UPDATE session_ownership SET lease_id='live',expires_at=now()+interval '1 hour'`,
				"live_input":       `UPDATE accepted_inputs SET status='running'`,
				"live_run":         `UPDATE runs SET current_state='tool_running'`,
				"grant":            `UPDATE action_authorizations SET grant_data='{}'`,
				"dispatch":         `UPDATE action_effect_dispatches SET outcome='executing',finished_at=NULL`,
				"missing":          `DELETE FROM credential_vault_requests`,
				"owner":            `UPDATE credential_vault_requests SET owner_id='other'`,
				"session":          `UPDATE credential_vault_requests SET session_key='other'`,
				"run":              `UPDATE credential_vault_requests SET run_id='other'`,
				"call":             `UPDATE credential_vault_requests SET tool_call_id='other'`,
				"versions":         `UPDATE credential_vault_requests SET constraints_revision=99`,
				"pending":          `UPDATE credential_vault_requests SET state='pending'`,
				"approved":         `UPDATE credential_vault_requests SET state='approved'`,
				"unexpired":        `UPDATE credential_vault_requests SET expires_at=now()+interval '1 hour'`,
				"notice":           `UPDATE credential_vault_requests SET notice_state='unknown'`,
				"notice_ack":       `UPDATE credential_vault_requests SET notice_ack='ack'`,
				"notice_started":   `UPDATE credential_vault_requests SET notice_started_at=now()`,
				"parent":           `UPDATE credential_vault_requests SET parent_request_id='parent'`,
				"browser_owner":    `UPDATE server_browser_sessions SET owner_id='other'`,
				"browser_run":      `UPDATE server_browser_sessions SET run_id='other'`,
				"browser_live":     `UPDATE server_browser_sessions SET expires_at=now()+interval '1 hour'`,
				"browser_url":      `UPDATE server_browser_sessions SET observed_url='https://other.example/login'`,
				"fields":           `UPDATE credential_vault_requests SET fields='["otp"]'`,
				"duplicate_fields": `UPDATE credential_vault_requests SET fields='["username","username"]'`,
				"missing_audit":    `DELETE FROM credential_vault_audit`,
				"foreign_audit":    `UPDATE credential_vault_audit SET owner_id='other'`,
				"released":         `INSERT INTO credential_vault_audit VALUES('request','owner','record','released_password')`,
			}
			if sql := changes[mode]; sql != "" {
				if _, err = pool.Exec(ctx, sql); err != nil {
					t.Fatal(err)
				}
			}
			if strings.HasPrefix(mode, "pruned_") {
				if _, err = pool.Exec(ctx, `DELETE FROM server_browser_sessions;CREATE TABLE tool_calls(tool_call_id TEXT PRIMARY KEY,run_id TEXT,tool_name TEXT,args JSONB,result JSONB,status TEXT,finished_at timestamptz)`); err != nil {
					t.Fatal(err)
				}
				expiry := time.Now().Add(-time.Hour)
				if mode == "pruned_live" {
					expiry = time.Now().Add(time.Hour)
				}
				raw, _ := json.Marshal(map[string]any{"browser_session_id": "browser", "final_url": "https://example.com/login", "expires_at": expiry, "ok": true})
				normalized, _ := budget.NormalizeResult(string(raw))
				if mode == "pruned_digest" {
					normalized = strings.Repeat("f", 64)
				}
				read := ActionBinding{ActionID: "open", RunID: "run", ToolCallID: "open-call", ToolName: "browser.open", ContractRevision: "3", OwnerID: "owner", CredentialRefs: []CredentialRef{}, Resource: "https://example.com/login", Effect: EffectRead}
				if mode == "pruned_foreign" {
					read.OwnerID = "other"
				}
				binding, _ := json.Marshal(read)
				policy := `{"retry_safety":"read_only","verification_method":"read_result"}`
				if _, err = pool.Exec(ctx, `INSERT INTO tool_calls VALUES('open-call','run','browser.open','{"url":"https://example.com/login"}',$1::jsonb,'completed',now())`, raw); err != nil {
					t.Fatal(err)
				}
				if _, err = pool.Exec(ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('open','open-fingerprint',$1::jsonb,$2::jsonb,'effect_confirmed')`, binding, policy); err != nil {
					t.Fatal(err)
				}
				if _, err = pool.Exec(ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at,effect_id,effect_fingerprint,effect_policy) VALUES('open','run','open-call','open-hash',$1::jsonb,false,now(),'open','open-fingerprint',$2::jsonb)`, binding, policy); err != nil {
					t.Fatal(err)
				}
				if mode != "pruned_no_receipt" {
					ev := EffectEvidence{EvidenceID: "open-evidence", ActionID: "open", SourceActionID: "open", SourceRunID: "run", SourceToolCallID: "open-call", Source: "runtime_response", Verdict: StateConfirmed, NormalizedResultDigest: normalized, ObservedAt: time.Now(), RecordedAt: time.Now()}
					tx, e := pool.Begin(ctx)
					if e != nil {
						t.Fatal(e)
					}
					if e = insertEvidence(ctx, tx, ev); e != nil {
						tx.Rollback(ctx)
						t.Fatal(e)
					}
					if e = tx.Commit(ctx); e != nil {
						t.Fatal(e)
					}
				}
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err = ReviewStoppedLocalTx(ctx, tx, "input"); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			e, err := NewPostgresStore(pool).GetEffect(ctx, b.ActionID)
			want := StateUnknown
			if mode == "missing" || mode == "call" {
				want = StateAbsent
			}
			if mode == "valid" || mode == "pruned_valid" {
				want = StateConfirmed
			}
			if err != nil || e.State != want || e.DispatchOutcome != "unknown" {
				t.Fatal("unexpected request verdict", e.State, err)
			}
			var grant, input string
			if pool.QueryRow(ctx, `SELECT state FROM action_authorizations WHERE authorization_id='old-grant'`).Scan(&grant) != nil || grant != "unknown" {
				t.Fatal("grant replay permitted")
			}
			if pool.QueryRow(ctx, `SELECT status FROM accepted_inputs WHERE input_id='input'`).Scan(&input) != nil || input != map[bool]string{true: "running", false: "failed"}[mode == "live_input"] {
				t.Fatal("input state changed")
			}
			if mode == "valid" || mode == "pruned_valid" {
				var raw []byte
				if pool.QueryRow(ctx, `SELECT evidence FROM action_effect_evidence WHERE action_id=$1 AND evidence->>'method'='stopped_expired_access_request_v1'`, b.ActionID).Scan(&raw) != nil {
					t.Fatal("owning receipt missing")
				}
				var ev EffectEvidence
				if json.Unmarshal(raw, &ev) != nil || ev.Source != "owning_credential_request_store" || ev.Verdict != StateConfirmed || len(ev.ResultDigest) != 64 || ev.SourceRunID != b.RunID || ev.SourceToolCallID != b.ToolCallID {
					t.Fatal("receipt provenance")
				}
			}
		})
	}
}
