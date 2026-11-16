package budget

import (
	"context"
	"errors"
	"testing"
)

func TestAuxiliaryOpaqueAdmissionRejectsUndeclaredOrChangedQuote(t *testing.T) {
	p := DefaultPolicy()
	p.Models["antigravity-acp:gemini-fixture"] = ModelPrice{Basis: AgentSubscriptionBasis}
	quote, err := p.AgentQuote("antigravity-acp", "gemini-fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"implicit", "native_basis", "fake_tokens", "changed_quote", "changed_input", "changed_time", "fake_native_limits"} {
		t.Run(mode, func(t *testing.T) {
			q := quote
			a := ModelAllowance{Provider: "antigravity-acp", Model: "gemini-fixture", Agent: true}
			i := AuxiliaryIdentity{OwnerID: "owner", JobID: "job", OperationID: "operation", InputBytes: q.ProviderInputBytes, ActiveMillis: q.ActiveMillis, Quote: &q, Agent: &a}
			c := &Controller{Policy: p}
			switch mode {
			case "implicit":
				i.Agent = nil
			case "native_basis":
				a.Provider = "antigravity"
			case "fake_tokens":
				i.Tokens = 1
			case "changed_quote":
				q.EgressBytes--
			case "changed_input":
				i.InputBytes--
			case "changed_time":
				i.ActiveMillis--
			case "fake_native_limits":
				a.InputTokens = 1
			}
			if _, err := c.ReserveAuxiliaryTx(context.Background(), nil, i); !errors.Is(err, ErrInvalid) {
				t.Fatal("invalid opaque reservation reached SQL", mode, err)
			}
		})
	}
}

func TestAuxiliaryCrossContextRejectsAmbiguousTopMandate(t *testing.T) {
	c := NewController(nil, DefaultPolicy())
	_, err := c.ReserveAuxiliaryTx(context.Background(), nil, AuxiliaryIdentity{OwnerID: "owner", JobID: "job", OperationID: "operation", InputBytes: 1, Tokens: 1, ActiveMillis: 1, MandateID: "ambiguous-top-level", AdditionalTasks: []AuxiliaryTaskIdentity{{TaskID: "actual-root", MandateID: "actual-assignment"}}})
	if !errors.Is(err, ErrInvalid) {
		t.Fatal("ambiguous top-level scope accepted", err)
	}
}
