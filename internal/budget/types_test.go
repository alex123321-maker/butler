package budget

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestAdmissionDoesNotOverflowOrAcceptUnknownCost(t *testing.T) {
	now := time.Now()
	scope := Scope{ScopeSpec: ScopeSpec{Kind: "task", Limits: Units{Tokens: math.MaxInt64}, Deadline: now.Add(time.Minute)}, Used: Units{Tokens: math.MaxInt64 - 1}}
	_, err := addWithin(scope, Units{Tokens: math.MaxInt64}, now)
	var exhausted *ExhaustedError
	if !errors.As(err, &exhausted) || exhausted.Dimension != "tokens" {
		t.Fatalf("overflow escaped quota: %v", err)
	}
	req := Request{OwnerID: "owner", OperationID: "model-turn:1", ScopeIDs: []string{ScopeID("task", "owner", "one")}, Charge: Units{ModelTurns: 1}}
	if _, err = canonicalRequest(req); !errors.Is(err, ErrUnknownCost) {
		t.Fatalf("unknown cost admitted: %v", err)
	}
	req.CostKnown = true
	req.Charge.Tokens = -1
	if _, err = canonicalRequest(req); !errors.Is(err, ErrInvalid) {
		t.Fatalf("negative quote admitted: %v", err)
	}
}

func TestDeadlineAndZeroLimitsAreHardBounds(t *testing.T) {
	now := time.Now()
	scope := Scope{ScopeSpec: ScopeSpec{Kind: "task", Deadline: now}}
	_, err := addWithin(scope, Units{}, now)
	var exhausted *ExhaustedError
	if !errors.As(err, &exhausted) || exhausted.Dimension != "deadline" {
		t.Fatalf("expired scope admitted: %v", err)
	}
	scope.Deadline = now.Add(time.Minute)
	if _, err = addWithin(scope, Units{Children: 1}, now); !errors.As(err, &exhausted) || exhausted.Dimension != "children" {
		t.Fatalf("zero child limit bypassed: %v", err)
	}
}
