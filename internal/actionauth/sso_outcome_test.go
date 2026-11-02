package actionauth

import (
	"context"
	"testing"
)

func TestOwnerOutcomeHistoricalLostSSORequiresPinnedExpiredContextIntegration(t *testing.T) {
	for _, mode := range []string{"absent", "unknown", "unchecked", "live_birth", "settling_birth", "wrong_birth_tool", "live_mapping", "foreign_mapping", "wrong_origin", "legacy_mapping", "live_lease", "nonterminal", "foreign_owner", "unfinished_dispatch", "changed_args", "changed_policy"} {
		t.Run(mode, func(t *testing.T) {
			f := newOwnerOutcomeFixtureMode(t, true, true, false, false, true)
			ctx := outcomeActor("owner", "historical-sso")
			sql := ""
			switch mode {
			case "live_birth":
				sql = `UPDATE tool_calls SET result=jsonb_set(result,'{expires_at}',to_jsonb(clock_timestamp()+interval '1 hour')) WHERE tool_call_id='open'`
			case "settling_birth":
				sql = `UPDATE tool_calls SET result=jsonb_set(result,'{expires_at}',to_jsonb(clock_timestamp()-interval '1 second')) WHERE tool_call_id='open'`
			case "wrong_birth_tool":
				sql = `UPDATE tool_calls SET tool_name='browser.open' WHERE tool_call_id='open'`
			case "live_mapping":
				sql = `UPDATE server_browser_sessions SET expires_at=clock_timestamp()+interval '1 hour'`
			case "foreign_mapping":
				sql = `UPDATE server_browser_sessions SET owner_id='other'`
			case "wrong_origin":
				sql = `UPDATE server_browser_sessions SET page_origin='https://foreign.example'`
			case "legacy_mapping":
				sql = `UPDATE server_browser_sessions SET page_mode=false`
			case "live_lease":
				sql = `UPDATE session_ownership SET lease_id='live',expires_at=clock_timestamp()+interval '1 hour'`
			case "nonterminal":
				sql = `UPDATE runs SET current_state='tool_running',status='running'`
			case "foreign_owner":
				ctx = outcomeActor("other", "historical-sso")
			case "unfinished_dispatch":
				sql = `UPDATE action_effect_dispatches SET finished_at=NULL`
			case "changed_args":
				sql = `UPDATE tool_calls SET args=jsonb_set(args,'{target_url}','"https://idp.example/other"') WHERE tool_name='browser.sso_login'`
			case "changed_policy":
				sql = `UPDATE action_effects SET policy=jsonb_set(policy,'{browser_sso,verify_url}','"https://example.com/other"')`
			}
			if sql != "" {
				if _, e := f.pool.Exec(context.Background(), sql); e != nil {
					t.Fatal(e)
				}
			}
			r, e := f.reviews.Historical(ctx, f.effect.ActionID, f.effect.Fingerprint)
			if mode != "absent" && mode != "unknown" && mode != "unchecked" {
				if e == nil {
					t.Fatal("unsettled or unpinned historical SSO accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			q := outcomeDecision(r)
			q.Outcome = string(StateAbsent)
			if mode == "unknown" {
				q.Outcome = "unknown"
			}
			if mode == "unchecked" {
				q.RecipientChecked = false
			}
			_, e = f.reviews.Decide(outcomeActor("owner", "historical-sso-report"), q)
			if mode == "unchecked" {
				if e == nil {
					t.Fatal("unchecked recipient accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			saved, e := f.store.GetEffect(context.Background(), f.effect.ActionID)
			want := StateAbsent
			if mode == "unknown" {
				want = StateUnknown
			}
			if e != nil || saved.State != want {
				t.Fatal("wrong reviewed state", e, saved.State)
			}
			if e = f.store.ClaimGrant(context.Background(), f.grant.ID); e == nil {
				t.Fatal("historical review replayed SSO grant")
			}
			var state string
			if f.pool.QueryRow(context.Background(), `SELECT current_state FROM runs WHERE run_id='run'`).Scan(&state) != nil || state != "failed" {
				t.Fatal("historical review resumed terminal run")
			}
		})
	}
}
