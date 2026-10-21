package app

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	input "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/jackc/pgx/v5"
	"net/http/httptest"
	"testing"
)

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
