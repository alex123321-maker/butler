package communication_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/butler/butler/internal/actionauth"
)

// Exercise the real Input/Communication composition, including dry-run rollback.
// The historical hashes are compatibility pins, never grants to execute them.
func TestStoppedLocalKernelReviewIntegration(t *testing.T) {
	for _, mode := range []string{"publication_absent", "read_result_unknown", "unsupported_contract"} {
		t.Run(mode, func(t *testing.T) {
			f := stoppedKernelFixture(t, false, false)
			ctx := context.Background()
			b := actionauth.ActionBinding{ActionID: "legacy-effect", RunID: f.p.RunID, OwnerID: f.p.OwnerID, ToolCallID: "legacy-call", ToolName: "communication.message", ContractRevision: "1", ContractHash: "f86dbe12464322551ff3d386412b54644fd32227fe0011f1bbac13b7db27b6fc", ArgsHash: strings.Repeat("a", 64), Resource: "tool:communication.message", Effect: actionauth.EffectPrepare, CredentialRefs: []actionauth.CredentialRef{}, CommunicationDestinationsHash: strings.Repeat("b", 64)}
			policy := actionauth.EffectPolicy{RetrySafety: "unsafe", VerificationMethod: "local_runtime_ack"}
			if mode == "read_result_unknown" {
				b.ToolName, b.Resource, b.Effect = "web.fetch", "https://example.com/", actionauth.EffectRead
				b.ContractHash, b.CommunicationDestinationsHash = "18a7d901908ac523b3ab1539d3368a9ea9a4b33ce8190d4d37aa58a4a11ff3d2", ""
				policy = actionauth.EffectPolicy{RetrySafety: "read_only", VerificationMethod: "read_result"}
			}
			if mode == "unsupported_contract" {
				b.ContractHash = strings.Repeat("d", 64)
			}
			raw, _ := json.Marshal(b)
			h := sha256.Sum256(raw)
			b.ActionHash = hex.EncodeToString(h[:])
			raw, _ = json.Marshal(b)
			_, fp, err := actionauth.EffectIdentity(b, "")
			if err != nil {
				t.Fatal(err)
			}
			p, _ := json.Marshal(policy)
			if _, err = f.pool.Exec(ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state,authorization_id,dispatch_outcome) VALUES($1,$2,$3,$4,'effect_unknown','legacy-grant','unknown')`, b.ActionID, fp, string(raw), string(p)); err != nil {
				t.Fatal(err)
			}
			if _, err = f.pool.Exec(ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at,effect_id,effect_fingerprint,effect_policy) VALUES($1,$2,$3,$4,$5,FALSE,clock_timestamp()-interval '1 hour',$1,$6,$7)`, b.ActionID, b.RunID, b.ToolCallID, b.ActionHash, string(raw), fp, string(p)); err != nil {
				t.Fatal(err)
			}
			if _, err = f.pool.Exec(ctx, `INSERT INTO action_authorizations(authorization_id,action_id,action_hash,grant_data,expires_at,state) VALUES('legacy-grant',$1,$2,'{}',clock_timestamp()-interval '1 hour','unknown')`, b.ActionID, b.ActionHash); err != nil {
				t.Fatal(err)
			}
			if _, err = f.pool.Exec(ctx, `INSERT INTO action_effect_dispatches(authorization_id,action_id,execution_action_id,outcome,finished_at) VALUES('legacy-grant',$1,$1,'unknown',clock_timestamp())`, b.ActionID); err != nil {
				t.Fatal(err)
			}
			if err = f.s.Inputs.SweepStopped(ctx); err != nil {
				t.Fatal(err)
			}
			snapshot := func() string {
				t.Helper()
				var out string
				if err := f.pool.QueryRow(ctx, `SELECT jsonb_build_object('input',(SELECT to_jsonb(i)-'updated_at' FROM accepted_inputs i WHERE input_id=$1),'usage',(SELECT jsonb_agg(to_jsonb(x)) FROM execution_model_attempts x),'grants',(SELECT jsonb_agg(to_jsonb(g)) FROM action_authorizations g),'dispatches',(SELECT jsonb_agg(to_jsonb(d)) FROM action_effect_dispatches d),'publications',(SELECT jsonb_agg(to_jsonb(p)) FROM communication_publications p))::text`, f.claim.InputID).Scan(&out); err != nil {
					t.Fatal(err)
				}
				return out
			}
			before := snapshot()
			want := "known"
			if mode == "unsupported_contract" {
				want = "uncertain"
			}
			preview, err := f.s.Inputs.ReviewStoppedInput(ctx, f.claim.InputID, false)
			if err != nil || preview.Classification != want {
				t.Fatal("dry-run proof", preview.Classification, err)
			}
			var classification string
			if err = f.pool.QueryRow(ctx, `SELECT classification FROM input_stopped_attempts WHERE input_id=$1`, f.claim.InputID).Scan(&classification); err != nil || classification != "uncertain" {
				t.Fatal("dry run committed", err)
			}
			result, err := f.s.Inputs.ReviewStoppedInput(ctx, f.claim.InputID, true)
			if err != nil || result.Classification != want {
				t.Fatal("applied proof", result.Classification, err)
			}
			if before != snapshot() {
				t.Fatal("review replayed work, reset usage or changed failure history")
			}
			var state string
			if err = f.pool.QueryRow(ctx, `SELECT state FROM action_effects WHERE action_id=$1`, b.ActionID).Scan(&state); err != nil {
				t.Fatal(err)
			}
			wantState := "effect_unknown"
			if mode == "publication_absent" {
				wantState = "effect_absent"
			}
			if state != wantState {
				t.Fatal("read success or write outcome manufactured", state)
			}
		})
	}
}
