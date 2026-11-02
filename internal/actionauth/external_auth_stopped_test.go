package actionauth

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/externalauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/jackc/pgx/v5"
)

func TestExternalStoppedMetadataContractPin(t *testing.T) {
	raw, err := os.ReadFile("testdata/stopped_external_login_6bee.json")
	if err != nil {
		t.Fatal(err)
	}
	var c toolbrokerv1.ToolContract
	if json.Unmarshal(raw, &c) != nil {
		t.Fatal("contract fixture")
	}
	hash, err := ContractHash(&c)
	if err != nil || hash != stoppedExternalLoginContract || c.ToolName != externalauth.RequestTool || c.SupportsCredentialRefs {
		t.Fatal("historical contract pin drift", hash, err)
	}
}

func TestExternalStoppedMetadataAbsenceIntegration(t *testing.T) {
	for _, mode := range []string{"absent", "rollback", "existing_request", "foreign_request", "live_lease", "live_claim", "claim_expiry", "live_run", "wrong_event", "wrong_owner", "wrong_task", "wrong_revision", "contract", "credential", "wrong_page", "commit", "provider_policy", "unfinished_dispatch", "live_dispatch", "live_grant", "wrong_grant", "wrong_fingerprint", "wrong_hash", "wrong_intent"} {
		t.Run(mode, func(t *testing.T) {
			pool, b := stoppedLocalFixture(t, false, func(b *ActionBinding) {
				b.ActionID = StableActionID("run", "old-call")
				b.TaskID, b.TaskRevision, b.ConstraintsRevision = "task", 7, 2
				b.ToolName, b.ContractRevision, b.ContractHash = externalauth.RequestTool, "1", stoppedExternalLoginContract
				b.Resource = "tool:" + externalauth.RequestTool
				b.CommunicationDestinationsHash = ""
				ref := browserpage.Ref{ID: strings.Repeat("a", 32), DocumentID: strings.Repeat("b", 32), Version: 1}
				b.BrowserPage = &browserpage.Binding{Ref: ref, URL: "https://rp.example/", Epoch: 1, RuntimeVersion: 1}
				b.ExternalAuth = &externalauth.Scope{Browser: strings.Repeat("c", 32), RPPage: ref, RPURL: "https://rp.example/", VerifyURL: "https://rp.example/auth/google/account", Provider: "google"}
				switch mode {
				case "contract":
					b.ContractHash = strings.Repeat("d", 64)
				case "credential":
					b.CredentialRefs = []CredentialRef{{Alias: "private", Field: "password"}}
				case "wrong_page":
					b.BrowserPage.URL = "https://other.example/"
				case "commit":
					b.Effect = EffectCommit
				}
			})
			ctx := t.Context()
			effect := "effect_" + strings.Repeat("e", 64)
			// Distinct permanent effect/execution IDs are essential to reproduce the
			// native failure; no collapsed-ID happy-path fixture is used here.
			_, err := pool.Exec(ctx, `CREATE TABLE browser_external_auth_requests(tool_call_id TEXT,run_id TEXT);
    UPDATE runs SET task_id='task'; UPDATE accepted_inputs SET task_id='task';
    INSERT INTO action_effects SELECT $2,fingerprint,initial_binding,policy,state,idempotency_key,authorization_id,dispatch_outcome,created_at,updated_at FROM action_effects WHERE action_id=$1;
    UPDATE action_intents SET effect_id=$2 WHERE action_id=$1;
    UPDATE action_effect_dispatches SET action_id=$2 WHERE execution_action_id=$1;
    DELETE FROM action_effects WHERE action_id=$1;
    UPDATE action_authorizations SET grant_data=jsonb_build_object('binding',$3::jsonb)`, pgx.QueryExecModeSimpleProtocol, b.ActionID, effect, mustJSONExternalStopped(t, b))
			if err != nil {
				t.Fatal(err)
			}
			changes := map[string]string{
				"existing_request":    `INSERT INTO browser_external_auth_requests VALUES('old-call','run')`,
				"foreign_request":     `INSERT INTO browser_external_auth_requests VALUES('old-call','other')`,
				"live_lease":          `UPDATE session_ownership SET lease_id='live',expires_at=clock_timestamp()+interval '1 hour'`,
				"live_claim":          `UPDATE accepted_inputs SET claim_id='live'`,
				"claim_expiry":        `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()+interval '1 hour'`,
				"live_run":            `UPDATE runs SET current_state='tool_running'`,
				"wrong_event":         `UPDATE runs SET input_event_id='other'`,
				"wrong_owner":         `UPDATE sessions SET user_id='other'`,
				"wrong_task":          `UPDATE runs SET task_id='other';UPDATE accepted_inputs SET task_id='other'`,
				"wrong_revision":      `UPDATE runs SET task_revision=8`,
				"provider_policy":     `UPDATE action_effects SET policy='{"retry_safety":"unsafe","verification_method":"external_google_rp_v1"}'`,
				"unfinished_dispatch": `UPDATE action_effect_dispatches SET finished_at=NULL`,
				"live_dispatch":       `UPDATE action_effect_dispatches SET outcome='executing'`,
				"live_grant":          `UPDATE action_authorizations SET expires_at=clock_timestamp()+interval '1 hour'`,
				"wrong_grant":         `UPDATE action_authorizations SET grant_data='{}'`,
				"wrong_fingerprint":   `UPDATE action_intents SET effect_fingerprint='other'`,
				"wrong_hash":          `UPDATE action_intents SET action_hash='other'`,
				"wrong_intent":        `UPDATE action_intents SET tool_call_id='other'`,
			}
			if change := changes[mode]; change != "" {
				if _, err = pool.Exec(ctx, change); err != nil {
					t.Fatal(err)
				}
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if err = ReviewStoppedExternalRequestTx(ctx, tx, "input"); err != nil {
				t.Fatal(err)
			}
			var state string
			var evidence int
			if err = tx.QueryRow(ctx, `SELECT state,(SELECT count(*) FROM action_effect_evidence) FROM action_effects WHERE action_id=$1`, effect).Scan(&state, &evidence); err != nil {
				t.Fatal(err)
			}
			allowed := mode == "absent" || mode == "rollback"
			if allowed && (state != string(StateAbsent) || evidence != 1) || !allowed && (state != string(StateUnknown) || evidence != 0) {
				t.Fatalf("unsafe result: state=%s evidence=%d", state, evidence)
			}
			if mode == "rollback" {
				err = tx.Rollback(ctx)
			} else {
				err = tx.Commit(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "rollback" {
				if pool.QueryRow(ctx, `SELECT state FROM action_effects WHERE action_id=$1`, effect).Scan(&state) != nil || state != string(StateUnknown) {
					t.Fatal("preview changed durable history")
				}
			}
			var grant, dispatch, input, run string
			if err = pool.QueryRow(ctx, `SELECT g.state,d.outcome,i.status,r.current_state FROM action_authorizations g CROSS JOIN action_effect_dispatches d CROSS JOIN accepted_inputs i CROSS JOIN runs r`).Scan(&grant, &dispatch, &input, &run); err != nil {
				t.Fatal(err)
			}
			wantDispatch := "unknown"
			if mode == "live_dispatch" {
				wantDispatch = "executing"
			}
			wantRun := "failed"
			if mode == "live_run" {
				wantRun = "tool_running"
			}
			if grant != "unknown" || dispatch != wantDispatch || input != "failed" || run != wantRun {
				t.Fatal("review rewrote consumed dispatch or failed work")
			}
		})
	}
}

func mustJSONExternalStopped(t *testing.T, b ActionBinding) string {
	t.Helper()
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
