package effort

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBoundedReviewModes(t *testing.T) {
	c := CasesV1()[0]
	for _, m := range []Mode{Baseline, Repeat, CheckSame, Checked} {
		t.Run(string(m), func(t *testing.T) {
			calls := 0
			o := Run(context.Background(), c, Config{m, 6, time.Now().Add(time.Minute)}, func(_ context.Context, s State) (Reply, error) {
				calls++
				return Reply{GoalVersion: s.GoalVersion, Decision: "answer", Value: "35", Answer: "35 стульев", Sources: []string{"inventory"}}, nil
			}, func(context.Context, State) error { return nil })
			want := 2
			if m == Baseline {
				want = 1
			}
			if !o.Correct || o.Status != "completed" || calls != want || o.State.Calls != want {
				t.Fatalf("%+v calls=%d", o, calls)
			}
		})
	}
}

func TestNoProgressAndFiniteBudget(t *testing.T) {
	c := Case{ID: "loop", Available: []Source{{"a", "one fact"}}}
	for _, budget := range []int{1, 8} {
		calls := 0
		o := Run(context.Background(), c, Config{Checked, budget, time.Now().Add(time.Minute)}, func(_ context.Context, s State) (Reply, error) {
			calls++
			return Reply{GoalVersion: 1, Decision: "read", ReadRefs: []string{"a", "a"}}, nil
		}, func(context.Context, State) error { return nil })
		want := "no_progress"
		if budget == 1 {
			want = "call_budget"
		}
		if o.Reason != want || calls > 3 || len(o.State.Actions) != 1 {
			t.Fatalf("%+v calls=%d", o, calls)
		}
	}
}

func TestStopAndRetentionPreventNextInference(t *testing.T) {
	c := CasesV1()[0]
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name   string
		ctx    context.Context
		save   Save
		reason string
	}{
		{"cancel", ctx, func(context.Context, State) error { return nil }, "stopped"},
		{"retention", context.Background(), func(context.Context, State) error { return errors.New("db down") }, "state_retention_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := Run(tc.ctx, c, Config{Checked, 6, time.Now().Add(time.Minute)}, func(context.Context, State) (Reply, error) { t.Fatal("unexpected inference"); return Reply{}, nil }, tc.save)
			if o.Reason != tc.reason {
				t.Fatal(o)
			}
		})
	}
	ctx, cancel = context.WithCancel(context.Background())
	calls := 0
	o := Run(ctx, c, Config{Checked, 6, time.Now().Add(time.Minute)}, func(_ context.Context, s State) (Reply, error) {
		calls++
		cancel()
		return Reply{GoalVersion: 1, Decision: "answer", Value: "35", Answer: "35"}, nil
	}, func(context.Context, State) error { return nil })
	if o.Reason != "stopped" || calls != 1 {
		t.Fatal(o)
	}
}

func TestHypothesisCannotBecomeSourceFact(t *testing.T) {
	c := CasesV1()[0]
	o := Run(context.Background(), c, Config{Checked, 6, time.Now().Add(time.Minute)}, func(context.Context, State) (Reply, error) {
		return Reply{GoalVersion: 1, Decision: "answer", Answer: "Возможно 40", Hypotheses: []string{"В комнате 40 стульев"}, Claims: []Claim{{"40 стульев", "inventory", "40 стульев"}}}, nil
	}, func(context.Context, State) error { return nil })
	if o.Reason != "unsupported_claim_quote" || len(o.State.Sources) != 1 || strings.Contains(o.State.Sources[0].Text, "40") {
		t.Fatal(o)
	}
}

func TestUnresolvedContradictionCannotComplete(t *testing.T) {
	c := CasesV1()[11]
	o := Run(context.Background(), c, Config{Baseline, 6, time.Now().Add(time.Minute)}, func(context.Context, State) (Reply, error) {
		return Reply{GoalVersion: 1, Decision: "answer", Answer: "В 10", Contradictions: []string{"10 и 15 равноправны"}}, nil
	}, func(context.Context, State) error { return nil })
	if o.Reason != "unresolved_answer" || o.Correct {
		t.Fatal(o)
	}
}

func TestOwnerAmendmentInvalidatesCandidate(t *testing.T) {
	c := CasesV1()[10]
	calls := 0
	o := Run(context.Background(), c, Config{Baseline, 6, time.Now().Add(time.Minute)}, func(_ context.Context, s State) (Reply, error) {
		calls++
		if calls == 1 {
			return Reply{GoalVersion: 1, Decision: "read", ReadRefs: []string{"mail.lee"}}, nil
		}
		if s.GoalVersion != 2 || s.Candidate != nil || !strings.Contains(s.Goal, "Смит") {
			t.Fatal(s)
		}
		return Reply{GoalVersion: 1, Decision: "answer", Answer: "семинар"}, nil
	}, func(context.Context, State) error { return nil })
	if o.Reason != "stale_goal" || len(o.State.Actions) != 0 {
		t.Fatal(o)
	}
}

func TestFreshEvidenceAblation(t *testing.T) {
	c := Case{ID: "evidence", Initial: []Source{{"old", "завтра"}}, Available: []Source{{"new", "отменено"}}, Expected: Expected{"answer", "cancelled", []string{"new"}}}
	for _, m := range []Mode{Repeat, CheckSame, Checked} {
		calls := 0
		o := Run(context.Background(), c, Config{m, 6, time.Now().Add(time.Minute)}, func(_ context.Context, s State) (Reply, error) {
			calls++
			if calls == 1 {
				return Reply{GoalVersion: 1, Decision: "answer", Value: "tomorrow", Answer: "Завтра", Sources: []string{"old"}}, nil
			}
			if calls == 2 {
				return Reply{GoalVersion: 1, Decision: "read", ReadRefs: []string{"new"}}, nil
			}
			return Reply{GoalVersion: 1, Decision: "answer", Value: "cancelled", Answer: "Отменено", Sources: []string{"new"}}, nil
		}, func(context.Context, State) error { return nil })
		if m == Checked {
			if !o.Correct || len(o.State.Actions) != 1 {
				t.Fatal(o)
			}
		} else if o.Reason != "review_has_no_new_evidence" || len(o.State.Actions) != 0 {
			t.Fatal(o)
		}
	}
}

func TestExpectedAnswerNeverEntersModelInput(t *testing.T) {
	c := CasesV1()[0]
	c.Expected.Value = "secret-evaluator-answer"
	b, err := ModelInput(c, State{CaseID: c.ID, Goal: c.Goal, GoalVersion: 1})
	if err != nil || strings.Contains(string(b), "secret-evaluator-answer") || strings.Contains(string(b), "expected") {
		t.Fatal(string(b), err)
	}
}

func TestStrictReplyRejectsTrailingAndUnknown(t *testing.T) {
	for _, s := range []string{`{"extra":true}`, `{} {}`, `{} garbage`, `{"goal_version":"one"}`, `{"goal_version":1,"goal_version":2}`, `{"Goal_Version":1}`, `null`, strings.Repeat(" ", 65537)} {
		if _, err := StrictReply([]byte(s)); err == nil {
			t.Fatal("accepted", s[:min(len(s), 50)])
		}
	}
}
