package browsercontrol

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/browsersso"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/grpc/metadata"
)

// Real PG page ledger, mandate/goal checks and Broker's private dispatch/release
// mediation. Only the runtime's value-free report is mechanical; actual TLS
// Chromium discovery is a separate mandatory CI profile.
func TestSSOInspectionPageDispatchAndReleaseIntegration(t *testing.T) {
	for _, mode := range []string{"allowed", "stale_page", "owner_epoch", "revoked_before_dispatch", "revoked_before_release", "discovered_path_denied", "foreign_provider"} {
		t.Run(mode, func(t *testing.T) {
			s := pageDB(t)
			ctx := t.Context()
			if _, err := s.Pool.Exec(ctx, `CREATE TABLE tasks(task_id TEXT PRIMARY KEY,owner_id TEXT,status TEXT,revision BIGINT,constraints_revision BIGINT);INSERT INTO tasks VALUES('task','owner','active',1,1);ALTER TABLE runs ADD COLUMN task_id TEXT DEFAULT 'task',ADD COLUMN task_revision BIGINT DEFAULT 1,ADD COLUMN current_state TEXT DEFAULT 'tool_pending';ALTER TABLE server_browser_sessions ADD COLUMN observed_url TEXT DEFAULT '';`); err != nil {
				t.Fatal(err)
			}
			for _, migration := range []string{"031_action_authorization.up.sql", "035_external_effect_journal.up.sql", "038_owner_goals_mandates.up.sql", "038_owner_purpose_policies.up.sql", "108_browser_page_execution.up.sql"} {
				if pageTestMigration(t, s.Pool, migration) != nil {
					t.Fatal(migration)
				}
			}
			p := pageSession(t, s)
			if _, err := s.Pool.Exec(ctx, `UPDATE server_browser_sessions SET page_mode=true,page_origin='https://rp.example',observed_url='https://rp.example/' WHERE browser_session_id=$1`, p.SessionID); err != nil {
				t.Fatal(err)
			}
			root := readyPage(t, s, p, nil, "https://rp.example/")
			if _, err := s.Pool.Exec(ctx, `UPDATE server_browser_pages SET runtime_version=3,observation_version=observation_version+1 WHERE browser_session_id=$1`, p.SessionID); err != nil {
				t.Fatal(err)
			}
			root.Version++
			a := browsersso.Inspection{Browser: p.SessionID, RPPage: root.PageRef, RPURL: root.URL, Trigger: "button >> nth=2", IdPOrigin: "https://idp.example", VerifyURL: "https://rp.example/account"}
			assignment := actionauth.Assignment{TaskID: p.TaskID, OwnerID: p.OwnerID, GoalID: "goal", GoalRevision: 1, MandateID: "mandate", MandateRevision: 1, Purpose: "inspection"}
			m := actionauth.Mandate{ID: "mandate", OwnerID: p.OwnerID, ScopeKind: "owner_goal", GoalID: "goal", GoalRevision: 1, Revision: 1, Status: "active", Purpose: "inspection", Tools: []string{browsersso.InspectTool}, Effects: []actionauth.Effect{actionauth.EffectRead}, ExpiresAt: time.Now().Add(time.Hour), ResourceScopes: []actionauth.ResourceScope{{Origin: "https://rp.example", PathPrefix: "/"}, {Origin: a.IdPOrigin, PathPrefix: "/"}}}
			g := actionauth.GoalPolicy{ID: "goal", OwnerID: p.OwnerID, Kind: "objective", Status: "active", Revision: 1, Purpose: m.Purpose, ExpiresAt: m.ExpiresAt}
			mr, _ := json.Marshal(m)
			gr, _ := json.Marshal(g)
			if _, err := s.Pool.Exec(ctx, `INSERT INTO owner_goals(goal_id,owner_id,kind,status,revision,expires_at,goal_data,source_data) VALUES('goal','owner','objective','active',1,$1,$2,'{}')`, g.ExpiresAt, gr); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Pool.Exec(ctx, `INSERT INTO action_mandates(mandate_id,owner_id,task_id,revision,mandate_data,expires_at,scope_kind,goal_id,goal_revision,status) VALUES('mandate','owner','',1,$1,$2,'owner_goal','goal',1,'active')`, mr, m.ExpiresAt); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Pool.Exec(ctx, `INSERT INTO owner_purpose_policies VALUES('owner','inspection',0);INSERT INTO task_authority_bindings(task_id,owner_id,goal_id,goal_revision,mandate_id,mandate_revision,purpose,initiative_reason,observation_ref) VALUES('task','owner','goal',1,'mandate',1,'inspection','owner-request','fixture')`); err != nil {
				t.Fatal(err)
			}
			current := ssoContext{actionauth.ContextSnapshot{RunID: p.RunID, OwnerID: p.OwnerID, TaskID: p.TaskID, Authority: &assignment, LeaseExpiresAt: m.ExpiresAt}}
			s.Contexts = current
			raw, _ := json.Marshal(a)
			call := &toolbrokerv1.ToolCall{RunId: p.RunID, ToolCallId: "inspect", ToolName: browsersso.InspectTool, ArgsJson: string(raw)}
			b := actionauth.ActionBinding{ActionID: actionauth.StableActionID(p.RunID, call.ToolCallId), RunID: p.RunID, ToolCallID: call.ToolCallId, OwnerID: p.OwnerID, TaskID: p.TaskID, TaskRevision: 1, ConstraintsRevision: 1, ToolName: call.ToolName, Effect: actionauth.EffectRead, GoalID: "goal", GoalRevision: 1, MandateID: "mandate", MandateRevision: 1, Purpose: m.Purpose, BrowserSSOInspection: &a, BrowserPage: &browserpage.Binding{Ref: a.RPPage, URL: a.RPURL, RuntimeVersion: 3}}
			b.ArgsHash, _ = actionauth.ArgumentsHash(call.ArgsJson)
			br, _ := json.Marshal(b)
			if _, err := s.Pool.Exec(ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('effect','fingerprint',$1,'{}','prepared')`, br); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Pool.Exec(ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at,effect_id,effect_fingerprint,effect_policy) VALUES($1,$2,'inspect','fixture',$3,false,clock_timestamp()+interval '1 minute','effect','fingerprint','{}')`, b.ActionID, p.RunID, br); err != nil {
				t.Fatal(err)
			}
			exec := func(sql string) {
				t.Helper()
				if _, err := s.Pool.Exec(ctx, sql); err != nil {
					t.Fatal(err)
				}
			}
			switch mode {
			case "stale_page":
				exec(`UPDATE server_browser_pages SET observation_version=observation_version+1`)
			case "owner_epoch":
				exec(`UPDATE server_browser_sessions SET epoch=epoch+1`)
			case "revoked_before_dispatch":
				exec(`UPDATE action_mandates SET status='revoked'`)
			}
			dispatched, err := s.beforeSSOInspection(ctx, call, 0)
			if mode == "stale_page" || mode == "owner_epoch" || mode == "revoked_before_dispatch" {
				if err == nil {
					t.Fatal("stale dispatch released")
				}
				return
			}
			if err != nil {
				t.Fatal("inspection dispatch", err)
			}
			md, _ := metadata.FromOutgoingContext(dispatched)
			d, ok := IncomingSSOInspection(metadata.NewIncomingContext(ctx, md), a)
			if !ok || d.Page.Ref != root.PageRef || !d.Allows(a.IdPOrigin+"/") {
				t.Fatal("private dispatch lost exact page/caps")
			}
			f := browsersso.InspectedForm{IdPURL: a.IdPOrigin + "/authorize-new", TargetURL: a.IdPOrigin + "/login-new", Form: "form >> nth=1", Username: "input >> nth=0", Password: "input >> nth=1"}
			if mode == "foreign_provider" {
				f.TargetURL = "https://other.example/login"
			}
			if mode == "revoked_before_release" {
				exec(`UPDATE action_mandates SET status='revoked'`)
			}
			if mode == "discovered_path_denied" {
				current.value.DenyResources = []actionauth.ResourceScope{{Origin: a.IdPOrigin, PathPrefix: "/login-new"}}
				s.Contexts = current
			}
			output, _ := json.Marshal(map[string]any{"ok": true, "browser_session_id": a.Browser, "final_url": a.RPURL, "selected_page_id": a.RPPage.ID, "network_state": "frozen", "inspection_forms": []browsersso.InspectedForm{f}, "private_pages": []browserpage.Report{{ID: a.RPPage.ID, DocumentID: a.RPPage.DocumentID, RuntimeVersion: 4, URL: a.RPURL, State: "ready"}}})
			result := &toolbrokerv1.ToolResult{RunId: p.RunID, ToolCallId: call.ToolCallId, ToolName: call.ToolName, Status: "completed", ResultJson: string(output)}
			err = s.Record(ctx, call, actionauth.Grant{Binding: b}, result)
			if mode != "allowed" {
				if err == nil || result.ResultJson != "{}" {
					t.Fatal("unsafe inspection bytes released")
				}
				return
			}
			var public struct {
				Scopes []browsersso.Scope `json:"sso_scopes"`
			}
			if err != nil || json.Unmarshal([]byte(result.ResultJson), &public) != nil || len(public.Scopes) != 1 || public.Scopes[0].RPPage.Version <= a.RPPage.Version || public.Scopes[0].TargetURL != f.TargetURL || strings.Contains(result.ResultJson, "private_pages") || strings.Contains(result.ResultJson, "inspection_forms") {
				t.Fatal("safe fresh scope release failed", err)
			}
		})
	}
}
