package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/transport"
)

func TestConverterContinuationEquivalentEffectGuard(t *testing.T) {
	for _, name := range []string{"new_call_id", "numeric_equivalent", "intentional_owner_repeat", "different_intent", "current_read", "unknown_read_arguments", "untrusted_read", "unknown_arguments", "revoked_private_proof", "foreign_prior", "wrong_current_run", "duplicate_keys"} {
		t.Run(name, func(t *testing.T) {
			r, s, ctx, event, p := activatedContextFixture()
			prior, next := `{"method":"POST","amount":1,"body":{"a":1,"b":2}}`, `{"body":{"b":2,"a":1},"amount":1,"method":"POST"}`
			tool := "http.request"
			read, want := false, false
			switch name {
			case "numeric_equivalent":
				next = `{"method":"POST","amount":1e0,"body":{"b":2,"a":1}}`
			case "different_intent":
				next = `{"method":"POST","amount":2}`
				want = true
			case "intentional_owner_repeat":
				want = true
			case "current_read", "unknown_read_arguments":
				tool = "workspace.read"
				prior = `{"path":"owned"}`
				next = prior
				read = true
				want = true
			case "untrusted_read":
				tool = "unregistered.read"
				prior = `{}`
				next = prior
				read = true
			case "duplicate_keys":
				next = `{"method":"POST","body":{"a":1,"a":2}}`
			}
			exact, _ := actionauth.ArgumentsHash(prior)
			canonical, _ := budget.NormalizedArgumentsHash(prior)
			effect := actionauth.EffectCommit
			if read {
				effect = actionauth.EffectRead
			}
			r.value.PriorRunID = "prior"
			r.value.ConfirmedPriorActions = []ConverterContinuationAction{{RunID: "prior", ToolCallID: "OLD-ID", ToolName: tool, ActionID: "confirmed-action", EffectID: "confirmed-effect", ArgsHash: exact, EquivalentArgsHash: canonical, Effect: effect, Result: json.RawMessage(`{"receipt":"existing"}`)}}
			status, safety := "active", "unsafe"
			if read {
				safety = "read_only"
			}
			catalog := &recoveryCatalog{contracts: []*toolbrokerv1.ToolContract{{ToolName: tool, Status: status, EffectContract: &toolbrokerv1.EffectContract{RetrySafety: safety}}}}
			s.config.ToolCatalog = catalog
			p.SourceInputEvent = &event
			if name != "intentional_owner_repeat" {
				ctx = converterContinuationProposalContext(ctx, durableSourcePreparation(p))
			}
			if name == "unknown_arguments" || name == "unknown_read_arguments" {
				r.value.ConfirmedPriorActions[0].EquivalentArgsHash = ""
				r.value.ConfirmedPriorActions[0].ArgsHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			}
			if name == "revoked_private_proof" {
				r.deny = true
			}
			if name == "foreign_prior" {
				r.value.ConfirmedPriorActions[0].RunID = "foreign"
			}
			current := &sessionv1.RunRecord{RunId: "run", SessionKey: "session"}
			if name == "wrong_current_run" {
				current.RunId = "foreign"
			}
			requested := &transport.ToolCallRequest{ToolCallRef: "NEW-ID", ToolName: tool, ArgsJSON: next}
			err := s.guardConverterContinuationProposal(ctx, current, requested)
			if (err == nil) != want {
				t.Fatal("private continuation changed manual intent/read policy or replayed a confirmed mutation", err)
			}
			if !want {
				// Missing run manager/authorizer/executor would panic if the guard
				// moved behind transition, slot admission or Broker preparation.
				if _, _, e := s.handleActionToolCall(ctx, slog.Default(), current, "lease", requested, ""); e == nil {
					t.Fatal("duplicate reached Broker preparation")
				}
				if name == "new_call_id" || name == "numeric_equivalent" {
					var stop *budget.LoopError
					if !errors.As(err, &stop) || stop.Reason != "converter_continuation_equivalent_effect_denied" {
						t.Fatal("equivalent mutation did not use typed stop")
					}
				}
			}
			if name == "intentional_owner_repeat" && r.calls != 0 {
				t.Fatal("ordinary owner input acquired an automatic continuation restriction")
			}
		})
	}
}

func TestConverterContinuationProposalRoutingSurvivesDurableReload(t *testing.T) {
	_, _, ctx, event, p := activatedContextFixture()
	p.SourceInputEvent = &event
	ctx = converterContinuationProposalContext(ctx, durableSourcePreparation(p))
	if got, ok := ctx.Value(converterContinuationProposalKey{}).(interface{}); !ok || got == nil {
		t.Fatal("durable private routing identity lost")
	}
	normal := converterContinuationProposalContext(context.Background(), preparedRun{})
	if normal.Value(converterContinuationProposalKey{}) != nil {
		t.Fatal("normal intent acquired continuation marker")
	}
}
