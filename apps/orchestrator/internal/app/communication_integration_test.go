package app

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	input "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCommunicationOwningPublicationWithOneBusinessConnectionIntegration(t *testing.T) {
	for _, method := range []string{"communication.message", "communication.widget"} {
		t.Run(method, func(t *testing.T) {
			f := newNamedToolControlDispatchFixture(t, nil, false, false, method, true)
			inputs := input.NewStore(f.f.pool)
			inputs.CommunicationEpoch = 1
			data, _ := json.Marshal(map[string]string{"tool_call_id": f.call.ToolCallId, "action_id": f.prepared.Binding.ActionID, "effect_id": f.prepared.EffectID, "authorization_id": f.grant.ID})
			if err := inputs.SaveCheckpoint(f.owned, f.call.RunId, "tool_dispatching", string(data)); err != nil {
				t.Fatal(err)
			}
			f.claim(t)
			core := &communication.Store{Pool: f.f.pool, Inputs: inputs, SourceGuard: f.f.sources.CheckRunSourcesTx, ChannelGuard: func(context.Context, pgx.Tx, string, communication.Binding, communicationcontrol.Args) error {
				return nil
			}}
			var held []*pgxpool.Conn
			defer func() {
				for _, c := range held {
					c.Release()
				}
			}()
			for range f.f.pool.Config().MaxConns - 1 {
				c, err := f.f.pool.Acquire(f.f.ctx)
				if err != nil {
					t.Fatal(err)
				}
				held = append(held, c)
			}
			// A channel poll and another owning writer can leave only one business
			// connection. Publication must join its fence instead of hoarding a
			// guard connection while waiting for a second business transaction.
			bounded, cancel := context.WithTimeout(f.f.ctx, time.Second)
			defer cancel()
			env := communicationcontrol.Envelope{RunID: f.call.RunId, ToolCallID: f.call.ToolCallId, EffectID: f.prepared.EffectID, Tool: f.call.ToolName, ArgsJSON: f.call.ArgsJson, ContractHash: f.prepared.Binding.ContractHash}
			receipt, err := (&CommunicationControl{Store: core}).ExecuteDispatched(bounded, env)
			if err != nil || receipt.PublicationRef == "" {
				t.Fatal("publication consumed a second business connection", err)
			}
			var n int
			if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM communication_publications WHERE run_id=$1`, f.call.RunId).Scan(&n); err != nil || n != 1 {
				t.Fatal("missing or repeated publication", n, err)
			}
		})
	}
}

func TestCommunicationOwningDispatchAndReceiptRecoveryIntegration(t *testing.T) {
	for _, method := range []string{"communication.message", "communication.widget"} {
		t.Run(method, func(t *testing.T) {
			f := newNamedToolControlDispatchFixture(t, nil, false, false, method, true)
			inputs := input.NewStore(f.f.pool)
			inputs.CommunicationEpoch = 1
			core := &communication.Store{Pool: f.f.pool, Inputs: inputs, SourceGuard: f.f.sources.CheckRunSourcesTx, ChannelGuard: func(context.Context, pgx.Tx, string, communication.Binding, communicationcontrol.Args) error {
				return nil
			}}
			h, e := communicationcontrol.NewHandler("private-communication-dispatch-fixture-token", &CommunicationControl{Store: core})
			if e != nil {
				t.Fatal(e)
			}
			server := httptest.NewServer(h)
			defer server.Close()
			client, e := communicationcontrol.NewClient(server.URL+communicationcontrol.Path, "private-communication-dispatch-fixture-token")
			if e != nil {
				t.Fatal(e)
			}
			env := communicationcontrol.Envelope{RunID: f.call.RunId, ToolCallID: f.call.ToolCallId, EffectID: f.prepared.EffectID, Tool: f.call.ToolName, ArgsJSON: f.call.ArgsJson, ContractHash: f.prepared.Binding.ContractHash}
			if _, e = client.ExecuteDispatched(f.f.ctx, env); e == nil {
				t.Fatal("private token alone became authority")
			}
			data, _ := json.Marshal(map[string]string{"tool_call_id": f.call.ToolCallId, "action_id": f.prepared.Binding.ActionID, "effect_id": f.prepared.EffectID, "authorization_id": f.grant.ID})
			if e = inputs.SaveCheckpoint(f.owned, f.call.RunId, "tool_dispatching", string(data)); e != nil {
				t.Fatal(e)
			}
			f.claim(t)
			receipt, e := client.ExecuteDispatched(f.f.ctx, env)
			if e != nil {
				t.Fatal("actual owning dispatch", e)
			}
			env.Lookup = true
			recovered, e := client.ExecuteDispatched(f.f.ctx, env)
			if e != nil || recovered.PublicationRef != receipt.PublicationRef {
				t.Fatal("lost ACK receipt resolution", e)
			}
			var n int
			if e = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM communication_publications WHERE run_id=$1`, f.call.RunId).Scan(&n); e != nil || n != 1 {
				t.Fatal("lookup republished", n, e)
			}
		})
	}
}

func TestCommunicationRevokedAuthorityAfterBrokerClaimIntegration(t *testing.T) {
	var manager *goal.Manager
	var objective actionauth.GoalPolicy
	setup := func(f *delegationGuardFixture) {
		manager = goal.New(f.pool, f.store, nil)
		objective = actionauth.GoalPolicy{Kind: "objective", Status: "active", Title: "Communicate current owner research", Purpose: "owner_communication", ExpiresAt: time.Now().Add(24 * time.Hour)}
		g, e := manager.PutGoal(f.owner, goal.GoalRequest{Goal: objective})
		if e != nil {
			t.Fatal(e)
		}
		objective.ID, objective.Revision = g.SubjectID, g.Revision
		actor := func(id string) context.Context {
			return owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "web", DeliveryID: id})
		}
		a := f.caps.Budget
		m, e := manager.PutMandate(actor("communication-mandate"), goal.MandateRequest{Mandate: actionauth.Mandate{GoalID: objective.ID, GoalRevision: objective.Revision, Status: "active", Purpose: objective.Purpose, Tools: []string{"communication.message"}, ResourceScopes: []actionauth.ResourceScope{{Origin: "tool:communication.message"}}, Effects: []actionauth.Effect{actionauth.EffectPrepare}, Privacy: "local_only", Budget: actionauth.MandateBudget{ModelTurns: a.ModelTurns, Tokens: a.Tokens, ExternalCalls: a.ExternalCalls, Children: a.Children, ActiveMillis: a.ActiveMillis, EgressBytes: a.EgressBytes, ProviderInputBytes: a.ProviderInputBytes}, ExpiresAt: time.Now().Add(2 * time.Hour), Intervention: []string{"scope_change", "effect_unknown", "budget_exhausted"}, Reporting: "exceptions"}})
		if e != nil {
			t.Fatal(e)
		}
		parent, e := manager.CreateOwnerTask(actor("communication-task"), goal.TaskRequest{SessionKey: f.parent.SessionKey, GoalID: objective.ID, MandateID: m.SubjectID, Purpose: objective.Purpose, Goal: "Communicate scoped research", CompletionCriteria: []string{"current evidence"}, InitiativeReason: "explicit owner work", ObservationRef: "fixture:owner"})
		if e != nil {
			t.Fatal(e)
		}
		f.parent = parent
	}
	f := newNamedToolControlDispatchFixture(t, setup, false, false, "communication.message")
	inputs := input.NewStore(f.f.pool)
	data, _ := json.Marshal(map[string]string{"tool_call_id": f.call.ToolCallId, "action_id": f.prepared.Binding.ActionID, "effect_id": f.prepared.EffectID, "authorization_id": f.grant.ID})
	if e := inputs.SaveCheckpoint(f.owned, f.call.RunId, "tool_dispatching", string(data)); e != nil {
		t.Fatal(e)
	}
	f.claim(t) // Actual Broker authorization/currentness succeeded before revocation.
	objective.Status = "paused"
	owner := owneridentity.WithActor(f.f.ctx, owneridentity.Actor{OwnerID: f.f.parent.OwnerID, Channel: "web", DeliveryID: "pause-communication-goal"})
	if _, e := manager.PutGoal(owner, goal.GoalRequest{Goal: objective, ExpectedRevision: objective.Revision}); e != nil {
		t.Fatal(e)
	}
	current, e := f.f.store.Get(f.f.ctx, f.f.parent.TaskID)
	if e != nil || current.Status != "active" || current.Revision != f.f.parent.Revision {
		t.Fatal("fixture must revoke owner authority without changing Task version", current, e)
	}
	core := &communication.Store{Pool: f.f.pool, Inputs: inputs, SourceGuard: f.f.sources.CheckRunSourcesTx, ChannelGuard: func(context.Context, pgx.Tx, string, communication.Binding, communicationcontrol.Args) error {
		return nil
	}}
	env := communicationcontrol.Envelope{RunID: f.call.RunId, ToolCallID: f.call.ToolCallId, EffectID: f.prepared.EffectID, Tool: f.call.ToolName, ArgsJSON: f.call.ArgsJson, ContractHash: f.prepared.Binding.ContractHash}
	if _, e := (&CommunicationControl{Store: core}).ExecuteDispatched(f.f.ctx, env); e == nil {
		t.Fatal("revoked owner goal published after Broker claim")
	}
	var n int
	if e := f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM communication_publications WHERE run_id=$1`, f.call.RunId).Scan(&n); e != nil || n != 0 {
		t.Fatal("revoked publication partially committed", n, e)
	}
}

func TestCommunicationGrantExpiryDuringExportRollsBackIntegration(t *testing.T) {
	f := newNamedToolControlDispatchFixture(t, nil, false, false, "communication.message", true)
	inputs := input.NewStore(f.f.pool)
	data, _ := json.Marshal(map[string]string{"tool_call_id": f.call.ToolCallId, "action_id": f.prepared.Binding.ActionID, "effect_id": f.prepared.EffectID, "authorization_id": f.grant.ID})
	if e := inputs.SaveCheckpoint(f.owned, f.call.RunId, "tool_dispatching", string(data)); e != nil {
		t.Fatal(e)
	}
	f.claim(t)
	// Shorten the real fixture grant to exercise wall-clock expiry after all
	// dispatch checks. Only the deadline changes; authority is never expanded.
	f.grant.ExpiresAt = time.Now().UTC().Add(2 * time.Second).Truncate(time.Microsecond)
	grant, _ := json.Marshal(f.grant)
	if _, e := f.f.pool.Exec(f.f.ctx, `UPDATE action_authorizations SET expires_at=$2,grant_data=$3::jsonb WHERE authorization_id=$1`, f.grant.ID, f.grant.ExpiresAt, grant); e != nil {
		t.Fatal(e)
	}
	entered := false
	core := &communication.Store{Pool: f.f.pool, Inputs: inputs, SourceGuard: f.f.sources.CheckRunSourcesTx, ChannelGuard: func(ctx context.Context, _ pgx.Tx, _ string, _ communication.Binding, _ communicationcontrol.Args) error {
		entered = true
		timer := time.NewTimer(time.Until(f.grant.ExpiresAt.Add(20 * time.Millisecond)))
		defer timer.Stop()
		select {
		case <-timer.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	env := communicationcontrol.Envelope{RunID: f.call.RunId, ToolCallID: f.call.ToolCallId, EffectID: f.prepared.EffectID, Tool: f.call.ToolName, ArgsJSON: f.call.ArgsJson, ContractHash: f.prepared.Binding.ContractHash}
	if _, e := (&CommunicationControl{Store: core}).ExecuteDispatched(f.f.ctx, env); e == nil {
		t.Fatal("expired grant committed publication after export wait")
	}
	if !entered {
		t.Fatal("fixture grant failed before the export wait")
	}
	var n int
	if e := f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM communication_publications WHERE run_id=$1`, f.call.RunId).Scan(&n); e != nil || n != 0 {
		t.Fatal("expired grant publication partially committed", n, e)
	}
}

func TestCommunicationStandardOwnerReplyDispatchWithoutApprovalIntegration(t *testing.T) {
	for _, scenario := range []string{"mode2", "mode1", "missing_claim", "revoked_destination"} {
		t.Run(scenario, func(t *testing.T) {
			f := newNamedToolControlDispatchFixture(t, nil, false, false, "communication.message", true)
			inputs := input.NewStore(f.f.pool)
			inputs.CommunicationEpoch = 1
			f.grant.ApprovalID = ""
			f.grant.MandateID = ""
			f.grant.AutonomyMode = 3
			mode := "mode_2"
			if scenario == "mode1" {
				f.grant.AutonomyMode = 2
				mode = "mode_1"
			}
			if scenario == "missing_claim" {
				f.grant.InputClaimHash = ""
			}
			raw, _ := json.Marshal(f.grant)
			if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE action_authorizations SET grant_data=$2::jsonb WHERE authorization_id=$1`, f.grant.ID, raw); err != nil {
				t.Fatal(err)
			}
			if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE runs SET autonomy_mode=$2 WHERE run_id=$1`, f.call.RunId, mode); err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(map[string]string{"tool_call_id": f.call.ToolCallId, "action_id": f.prepared.Binding.ActionID, "effect_id": f.prepared.EffectID, "authorization_id": f.grant.ID})
			if err := inputs.SaveCheckpoint(f.owned, f.call.RunId, "tool_dispatching", string(data)); err != nil {
				t.Fatal(err)
			}
			f.claim(t)
			if scenario == "revoked_destination" {
				if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE communication_bindings SET enabled=FALSE`); err != nil {
					t.Fatal(err)
				}
			}
			core := &communication.Store{Pool: f.f.pool, Inputs: inputs, SourceGuard: f.f.sources.CheckRunSourcesTx, ChannelGuard: func(context.Context, pgx.Tx, string, communication.Binding, communicationcontrol.Args) error {
				return nil
			}}
			env := communicationcontrol.Envelope{RunID: f.call.RunId, ToolCallID: f.call.ToolCallId, EffectID: f.prepared.EffectID, Tool: f.call.ToolName, ArgsJSON: f.call.ArgsJson, ContractHash: f.prepared.Binding.ContractHash}
			receipt, err := (&CommunicationControl{Store: core}).ExecuteDispatched(f.f.ctx, env)
			if scenario == "mode2" {
				if err != nil || receipt.PublicationRef == "" {
					t.Fatal("normal owner reply failed", receipt, err)
				}
			} else if err == nil {
				t.Fatal("invalid reply context published", scenario)
			}
		})
	}
}
