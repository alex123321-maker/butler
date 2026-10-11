package actionauth

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/delegation"
)

func TestDelegationReadinessPrecedesIntentAndDoesNotGrantChildPermission(t *testing.T) {
	s, store, reader, _, call, _ := fixture(t)
	contract := delegation.AdmissionContract()
	call.ToolName = contract.ToolName
	raw, err := json.Marshal(delegation.AdmissionArgs{
		ContractSource: delegation.SourceRef{ID: "current-contract", Revision: 1, AccessRevision: 1, Hash: strings.Repeat("a", 64)},
		Sources:        []delegation.SourceRef{}, Dependencies: []string{},
		Budget:   delegation.Limits{ModelTurns: 1, Tokens: 1000, ActiveMillis: 1000, ProviderInputBytes: 4096},
		Deadline: s.now().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	call.ArgsJson = string(raw)
	if _, err = s.Prepare(context.Background(), call, contract); err == nil || len(store.intents) != 0 {
		t.Fatal("Broker-only activation persisted an unavailable delegation action", err)
	}
	reader.current.DelegationReady = true
	if _, err = s.Prepare(context.Background(), call, contract); err != nil || len(store.intents) != 1 {
		t.Fatal("composed root admission was refused", err)
	}
	call.ToolCallId = "child-call"
	reader.current.Delegation = &delegation.Contract{Permissions: []delegation.Permission{}}
	if _, err = s.Prepare(context.Background(), call, contract); err == nil || len(store.intents) != 1 {
		t.Fatal("readiness created a recursive child permission", err)
	}
}
