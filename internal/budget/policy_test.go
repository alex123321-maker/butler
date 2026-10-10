package budget

import (
	"errors"
	"testing"
)

func TestCostBasisAndFixedNativeReservation(t *testing.T) {
	p := DefaultPolicy()
	if _, err := p.ModelQuote("antigravity", "gemini-3.8-flash-tiered", 1048576); !errors.Is(err, ErrUnknownCost) {
		t.Fatal("unknown cost guessed", err)
	}
	p.Models["antigravity:gemini-3.8-flash-tiered"] = ModelPrice{Basis: "subscription_no_overage"}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	q, err := p.ModelQuote("antigravity", "gemini-3.8-flash-tiered", 1048576)
	if err != nil || q.Tokens != 1048576+8192 || q.CostMicros != 0 || p.Task.Tokens < q.Tokens {
		t.Fatalf("invalid declared subscription allowance %+v %v", q, err)
	}
	p.Models["bad"] = ModelPrice{Basis: "operator_rate"}
	if !errors.Is(p.Validate(), ErrUnknownCost) {
		t.Fatal("zero invented price accepted")
	}
	delete(p.Models, "bad")
	p.Models["priced"] = ModelPrice{Basis: "operator_rate", InputMicrosPerToken: 2, OutputMicrosPerToken: 3}
	q, err = p.ModelQuote("", "", 10)
	if !errors.Is(err, ErrUnknownCost) {
		t.Fatal(q, err)
	}
	p.Models["p:m"] = p.Models["priced"]
	q, err = p.ModelQuote("p", "m", 10)
	if err != nil || q.CostMicros != 20+3*8192 {
		t.Fatal(q, err)
	}
}
func TestNormalizeProgressIgnoresVolatileMetadata(t *testing.T) {
	a, _ := NormalizeResult(`{"timestamp":"one","data":{"x":1,"y":2}}`)
	b, _ := NormalizeResult(`{"data":{"y":2e0,"x":1.00},"timestamp":"two"}`)
	c, _ := NormalizeResult(`{"data":{"x":2,"y":2}}`)
	if a != b || a == c {
		t.Fatal("normalization does not distinguish progress")
	}
	if _, err := NormalizeResult("invalid"); err == nil {
		t.Fatal("invalid result accepted")
	}
}

func TestCanonicalLoopArgumentsAreEquivalentWithoutNumberRounding(t *testing.T) {
	a, _ := NormalizedArgumentsHash(`{"x":1,"y":2.0}`)
	b, _ := NormalizedArgumentsHash(`{"y":2e0,"x":1.00}`)
	if a != b {
		t.Fatal("numeric/object equivalent work bypassed loop identity")
	}
	c, _ := NormalizedArgumentsHash(`{"x":9007199254740992}`)
	d, _ := NormalizedArgumentsHash(`{"x":9007199254740993}`)
	if c == d {
		t.Fatal("large integer inputs rounded to a false match")
	}
	if _, err := NormalizedArgumentsHash(`{} {}`); err == nil {
		t.Fatal("ambiguous JSON accepted")
	}
}
