package actionauth

import (
	"context"
	"github.com/butler/butler/internal/browserauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"testing"
	"time"
)

func TestCredentialLoginPostgresEvidenceAndNoReplayIntegration(t *testing.T) {
	for _, mode := range []string{"verified", "missing_proof", "runtime_lost", "input_replaced"} {
		t.Run(mode, func(t *testing.T) {
			pool := actionDatabase(t)
			ctx := context.Background()
			_, err := pool.Exec(ctx, `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT);INSERT INTO sessions VALUES('web:owner','owner');ALTER TABLE runs ADD COLUMN session_key TEXT DEFAULT 'web:owner';CREATE TABLE accepted_inputs(input_id TEXT,claim_id TEXT,run_id TEXT,session_key TEXT,owner_id TEXT,task_id TEXT,status TEXT,claim_expires_at timestamptz,expires_at timestamptz);INSERT INTO accepted_inputs VALUES('login-input','claim','run','web:owner','owner','task','running',now()+interval '1 hour',now()+interval '1 hour')`)
			if err != nil {
				t.Fatal(err)
			}
			s, _, reader, _, call, c := loginAuthorityFixture(t)
			reader.current.Authority = nil
			reader.current.InputClaimHash = InputClaimHash("login-input", "claim")
			store := NewPostgresStore(pool)
			s.store = store
			p, err := s.Prepare(ctx, call, c)
			if err != nil || p.RequiresApproval {
				t.Fatal("prepare", err)
			}
			g, err := s.Authorize(ctx, call, c, p.Binding.ActionID, "", "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Verify(ctx, call, c, g.ID); err != nil {
				t.Fatal(err)
			}
			if mode == "input_replaced" {
				if _, err = pool.Exec(ctx, `UPDATE accepted_inputs SET claim_id='other'`); err != nil {
					t.Fatal(err)
				}
			}
			err = store.ClaimGrant(ctx, g.ID)
			if mode == "input_replaced" {
				if err == nil {
					t.Fatal("stale Input dispatched")
				}
				return
			}
			if err != nil {
				t.Fatal("claim", err)
			}
			if err = store.ClaimGrant(ctx, g.ID); err == nil {
				t.Fatal("replayed grant")
			}
			effect, err := s.EffectForGrant(ctx, g)
			if err != nil {
				t.Fatal(err)
			}
			invalid := EffectEvidence{EvidenceID: "forged", ActionID: effect.ActionID, SourceActionID: g.Binding.ActionID, SourceRunID: g.Binding.RunID, SourceToolCallID: g.Binding.ToolCallID, Source: "runtime_response", Method: browserauth.Verification, Verdict: StateConfirmed, ResultDigest: digest("body"), ObservedAt: time.Now().UTC(), RecordedAt: time.Now().UTC()}
			if _, err = store.RecordDispatchOutcome(ctx, g.ID, "dispatch_completed", invalid); err == nil {
				t.Fatal("PG accepted missing typed evidence")
			}
			raw := `{"ok":true,"browser_session_id":"context","observation_version":8,"final_url":"https://example.com/account","network_state":"frozen","authentication":{"protocol":"browser_login_v1","submitted_once":true,"authenticated_read":true,"anonymous_challenge":true}}`
			if mode == "missing_proof" {
				raw = `{"ok":true,"browser_session_id":"context","observation_version":8,"final_url":"https://example.com/account","network_state":"frozen"}`
			}
			result := &toolbrokerv1.ToolResult{ToolName: call.ToolName, ToolCallId: call.ToolCallId, RunId: call.RunId, Status: "completed", ResultJson: raw}
			effect, err = s.Outcome(ctx, g, result, mode == "runtime_lost")
			if err != nil {
				t.Fatal("recording", err)
			}
			want := StateUnknown
			if mode == "verified" {
				want = StateConfirmed
			}
			if effect.State != want || len(effect.EvidenceRefs) != 1 {
				t.Fatal("wrong durable login verdict", effect.State)
			}
			restarted := NewPostgresStore(pool)
			persisted, err := restarted.GetEffect(ctx, effect.ActionID)
			if err != nil || persisted.State != want {
				t.Fatal("restart changed evidence")
			}
			if err = restarted.ClaimGrant(ctx, g.ID); err == nil {
				t.Fatal("restart permitted POST replay")
			}
		})
	}
}
