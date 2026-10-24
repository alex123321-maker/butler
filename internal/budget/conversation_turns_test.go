package budget

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestConversationModelTurnPolicyRequiresExplicitOperatorOptOut(t *testing.T) {
	for _, raw := range []string{"", `{}`, `{"disable_conversation_model_turns":false}`} {
		p, err := ParsePolicy(raw)
		if err != nil || p.DisableConversationModelTurns {
			t.Fatal(raw, err)
		}
	}
	p, err := ParsePolicy(`{"disable_conversation_model_turns":true}`)
	if err != nil || !p.DisableConversationModelTurns || p.DisableOwnerDay || p.Conversation != DefaultPolicy().Conversation || p.Task != DefaultPolicy().Task {
		t.Fatal("opt-out changed other policy", err)
	}
	if _, err = ParsePolicy(`{"disable_conversation_model_turns":"true"}`); err == nil {
		t.Fatal("nonboolean accepted")
	}
}

func TestConversationModelTurnOptOutRetainsOtherBounds(t *testing.T) {
	now := time.Now()
	s := Scope{ScopeSpec: ScopeSpec{Kind: "conversation", Limits: Units{ModelTurns: 1}, Deadline: now.Add(time.Minute)}, Used: Units{ModelTurns: 48}}
	got, err := addWithinPolicy(s, Units{ModelTurns: 1}, now, true)
	if err != nil || got.ModelTurns != 49 || s.Limits.ModelTurns != 1 {
		t.Fatal(got, err)
	}
	for _, kind := range []string{"task", "mandate", "owner_day", "delegation", "auxiliary"} {
		scope := s
		scope.Kind = kind
		_, err = addWithinPolicy(scope, Units{ModelTurns: 1}, now, true)
		assertBudgetDimension(t, err, kind, "model_turns")
	}
	for dimension := 1; dimension < 8; dimension++ {
		v := [8]int64{}
		v[dimension] = 1
		_, err = addWithinPolicy(s, fromValues(v), now, true)
		var exhausted *ExhaustedError
		if !errors.As(err, &exhausted) || exhausted.Dimension == "model_turns" {
			t.Fatal("other zero cap bypassed", dimension, err)
		}
	}
	_, err = addWithinPolicy(s, Units{ModelTurns: 1}, now, false)
	assertBudgetDimension(t, err, "conversation", "model_turns")
	s.Deadline = now
	_, err = addWithinPolicy(s, Units{ModelTurns: 1}, now, true)
	assertBudgetDimension(t, err, "conversation", "deadline")
	s.Deadline = now.Add(time.Minute)
	s.Used.ModelTurns = math.MaxInt64
	if _, err = addWithinPolicy(s, Units{ModelTurns: 1}, now, true); !errors.Is(err, ErrInvalid) {
		t.Fatal("overflow", err)
	}
}
