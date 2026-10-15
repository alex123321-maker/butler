package app

import (
	"encoding/json"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/readcontrol"
)

func prepareMemoryReadCheckpoint(t *testing.T, f *dispatchAdmissionFixture) {
	t.Helper()
	checkpoint, _ := json.Marshal(map[string]string{"tool_call_id": f.call.ToolCallId, "action_id": f.prepared.Binding.ActionID, "effect_id": f.prepared.EffectID, "authorization_id": f.grant.ID})
	if err := durableinput.NewStore(f.f.pool).SaveCheckpoint(f.owned, f.call.RunId, "tool_dispatching", string(checkpoint)); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryReadDispatchedBrokerIntegration(t *testing.T) {
	for _, tool := range []string{readcontrol.Search, readcontrol.Open} {
		t.Run(tool, func(t *testing.T) {
			f := newNamedToolControlDispatchFixture(t, nil, false, false, tool)
			prepareMemoryReadCheckpoint(t, f)
			service, err := NewMemoryReads(f.f.pool, f.f.sources)
			if err != nil {
				t.Fatal(err)
			}
			token := "guarded-request-private-process-fixture-token"
			h, err := readcontrol.NewHandler(token, service)
			if err != nil {
				t.Fatal(err)
			}
			process := controlBrokerProcess(t, f, h, readcontrol.Path, "BUTLER_MEMORY_READ_HARNESS="+tool)
			result, err := process.ExecuteAuthorizedToolCall(f.f.ctx, f.call, f.grant.ID)
			if err != nil || result.GetStatus() != "completed" {
				t.Fatal("real Broker read failed", err, result)
			}
			read, err := readcontrol.DecodeResult([]byte(result.GetResultJson()))
			if err != nil || read.Status != "ok" || len(read.Items) == 0 || !strings.Contains(read.Items[0].Text, "проверить") {
				t.Fatal("exact DATA evidence missing", read, err)
			}
			var reservations, pins int
			if err = f.f.pool.QueryRow(f.f.ctx, `SELECT (SELECT count(*) FROM execution_budget_reservations),(SELECT count(*) FROM memory_run_sources WHERE run_id=$1)`, f.call.RunId).Scan(&reservations, &pins); err != nil || reservations != 1 || pins == 0 {
				t.Fatal("read bypassed shared budget or current source pins", reservations, pins, err)
			}
		})
	}
}
func TestMemoryReadOwnerProofCannotReplaceDispatchIntegration(t *testing.T) {
	f := newNamedToolControlDispatchFixture(t, nil, false, false, readcontrol.Open)
	prepareMemoryReadCheckpoint(t, f)
	service, err := NewMemoryReads(f.f.pool, f.f.sources)
	if err != nil {
		t.Fatal(err)
	}
	f.claim(t)
	e := readcontrol.Envelope{RunID: f.call.RunId, ToolCallID: f.call.ToolCallId, EffectID: f.envelope.EffectID, Tool: readcontrol.Open, ArgsJSON: f.call.ArgsJson, ContractHash: f.envelope.ContractHash}
	if _, err = service.ReadDispatched(f.f.owner, e); err == nil {
		t.Fatal("owner actor minted run authority")
	}
	h, err := readcontrol.NewHandler(strings.Repeat("private-test-", 4), service)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	client, err := readcontrol.NewClient(server.URL+readcontrol.Path, strings.Repeat("private-test-", 4))
	if err != nil {
		t.Fatal(err)
	}
	// An exact reference can be invalidated after discovery, before opening.
	var args readcontrol.Args
	if err = json.Unmarshal([]byte(e.ArgsJSON), &args); err != nil {
		t.Fatal(err)
	}
	if _, err = f.f.sources.Delete(f.f.owner, args.SourceID, "delete", args.Revision); err != nil {
		t.Fatal(err)
	}
	r, err := client.ReadDispatched(f.f.ctx, e)
	if err == nil && len(r.Items) > 0 {
		t.Fatal("deleted evidence returned")
	}
}
