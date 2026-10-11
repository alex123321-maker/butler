package actionauth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/delegation"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
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

type developerReadinessCapability struct{ calls int }

func (f *developerReadinessCapability) BindAction(_ context.Context, _ *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract, b ActionBinding, _ ContextSnapshot) (ActionBinding, error) {
	f.calls++
	return b, ErrOwnerAuthority
}

func TestDeveloperReadinessPrecedesCapabilitiesAndDoesNotGrantAuthority(t *testing.T) {
	for _, missing := range []string{"task", "delegation", "converter_lifecycle", "converter_delegation"} {
		t.Run(missing, func(t *testing.T) {
			s, store, reader, inspection, call, _ := fixture(t)
			call.ToolName, call.ArgsJson = "task.delegate_converter", `{}`
			contract := &toolbrokerv1.ToolContract{ToolName: call.ToolName, ToolClass: "task_control", Status: "active", RuntimeTarget: "broker://converter-delegation-v1", InputSchemaJson: `{"type":"object"}`}
			reader.current.DelegationReady = true
			reader.current.ConverterLifecycleReady = true
			reader.current.ConverterDelegationReady = true
			switch missing {
			case "task":
				reader.current.TaskID = ""
			case "delegation":
				reader.current.DelegationReady = false
			case "converter_lifecycle":
				reader.current.ConverterLifecycleReady = false
			case "converter_delegation":
				reader.current.ConverterDelegationReady = false
			}
			capabilities := &developerReadinessCapability{}
			s.SetCapabilities(capabilities)
			if _, err := s.Prepare(context.Background(), call, contract); !errors.Is(err, delegation.ErrDenied) || capabilities.calls != 0 || inspection.calls != 0 || len(store.intents) != 0 {
				t.Fatal("uncomposed developer role reached capabilities or persisted an intent", err, capabilities.calls)
			}
		})
	}
	s, store, reader, inspection, call, _ := fixture(t)
	call.ToolName, call.ArgsJson = "task.delegate_converter", `{}`
	contract := &toolbrokerv1.ToolContract{ToolName: call.ToolName, ToolClass: "task_control", Status: "active", RuntimeTarget: "broker://converter-delegation-v1", InputSchemaJson: `{"type":"object"}`}
	reader.current.DelegationReady = true
	reader.current.ConverterLifecycleReady = true
	reader.current.ConverterDelegationReady = true
	capabilities := &developerReadinessCapability{}
	s.SetCapabilities(capabilities)
	if _, err := s.Prepare(context.Background(), call, contract); !errors.Is(err, ErrOwnerAuthority) || capabilities.calls != 1 || inspection.calls != 0 || len(store.intents) != 0 {
		t.Fatal("readiness bypassed current developer authority", err, capabilities.calls)
	}
	reader.current.Delegation = &delegation.Contract{Permissions: []delegation.Permission{}}
	if _, err := s.Prepare(context.Background(), call, contract); !errors.Is(err, delegation.ErrDenied) || capabilities.calls != 1 || inspection.calls != 0 || len(store.intents) != 0 {
		t.Fatal("readiness created a recursive developer permission", err, capabilities.calls)
	}
}
