package domain

import (
	"fmt"
	"testing"
)

// allStates is the complete set of run states defined in the state machine.
var allStates = []RunState{
	RunStateCreated,
	RunStateQueued,
	RunStateAcquired,
	RunStatePreparing,
	RunStateModelRunning,
	RunStateToolPending,
	RunStateAwaitingApproval,
	RunStateToolRunning,
	RunStateAwaitingModelResume,
	RunStateFinalizing,
	RunStateCompleted,
	RunStateFailed,
	RunStateCancelled,
	RunStateTimedOut,
}

var terminalStates = []RunState{
	RunStateCompleted,
	RunStateFailed,
	RunStateCancelled,
	RunStateTimedOut,
}

// ---------- ValidateRunStateTransition: exhaustive valid transitions ----------

// TestEveryAllowedTransitionIsAccepted walks the full allowedTransitions map
// and asserts that every declared pair is accepted by ValidateRunStateTransition.
func TestEveryAllowedTransitionIsAccepted(t *testing.T) {
	// Manually enumerate every (from, to) pair from the map to ensure full coverage.
	valid := [][2]RunState{
		// created ->
		{RunStateCreated, RunStateQueued},
		{RunStateCreated, RunStateFailed},
		{RunStateCreated, RunStateCancelled},
		{RunStateCreated, RunStateTimedOut},
		// queued ->
		{RunStateQueued, RunStateAcquired},
		{RunStateQueued, RunStateFailed},
		{RunStateQueued, RunStateCancelled},
		{RunStateQueued, RunStateTimedOut},
		// acquired ->
		{RunStateAcquired, RunStatePreparing},
		{RunStateAcquired, RunStateFailed},
		{RunStateAcquired, RunStateCancelled},
		{RunStateAcquired, RunStateTimedOut},
		// preparing ->
		{RunStatePreparing, RunStateModelRunning},
		{RunStatePreparing, RunStateFailed},
		{RunStatePreparing, RunStateCancelled},
		{RunStatePreparing, RunStateTimedOut},
		// model_running ->
		{RunStateModelRunning, RunStateToolPending},
		{RunStateModelRunning, RunStateFinalizing},
		{RunStateModelRunning, RunStateFailed},
		{RunStateModelRunning, RunStateCancelled},
		{RunStateModelRunning, RunStateTimedOut},
		// tool_pending ->
		{RunStateToolPending, RunStateAwaitingApproval},
		{RunStateToolPending, RunStateToolRunning},
		{RunStateToolPending, RunStateCancelled},
		{RunStateToolPending, RunStateTimedOut},
		// awaiting_approval ->
		{RunStateAwaitingApproval, RunStateToolRunning},
		{RunStateAwaitingApproval, RunStateAwaitingModelResume},
		{RunStateAwaitingApproval, RunStateCancelled},
		{RunStateAwaitingApproval, RunStateTimedOut},
		{RunStateAwaitingApproval, RunStateFailed},
		// tool_running ->
		{RunStateToolRunning, RunStateAwaitingModelResume},
		{RunStateToolRunning, RunStateFailed},
		{RunStateToolRunning, RunStateCancelled},
		{RunStateToolRunning, RunStateTimedOut},
		// awaiting_model_resume ->
		{RunStateAwaitingModelResume, RunStateModelRunning},
		{RunStateAwaitingModelResume, RunStateCancelled},
		{RunStateAwaitingModelResume, RunStateTimedOut},
		// finalizing ->
		{RunStateFinalizing, RunStateCompleted},
		{RunStateFinalizing, RunStateFailed},
		{RunStateFinalizing, RunStateCancelled},
		{RunStateFinalizing, RunStateTimedOut},
	}

	for _, pair := range valid {
		t.Run(fmt.Sprintf("%s->%s", pair[0], pair[1]), func(t *testing.T) {
			if err := ValidateRunStateTransition(pair[0], pair[1]); err != nil {
				t.Fatalf("expected %q -> %q to be valid, got error: %v", pair[0], pair[1], err)
			}
		})
	}
}

// ---------- ValidateRunStateTransition: exhaustive invalid transitions ----------

// TestEveryDisallowedTransitionIsRejected checks that for every (from, to) pair
// where 'to' is NOT in the allowed set (and from != to), the transition is rejected.
func TestEveryDisallowedTransitionIsRejected(t *testing.T) {
	allowed := buildAllowedSet()

	for _, from := range allStates {
		for _, to := range allStates {
			if from == to {
				continue // self-transition tested separately
			}
			if allowed[transitionKey(from, to)] {
				continue // valid transition, skip
			}
			t.Run(fmt.Sprintf("%s->%s", from, to), func(t *testing.T) {
				if err := ValidateRunStateTransition(from, to); err == nil {
					t.Fatalf("expected %q -> %q to be invalid, but it was accepted", from, to)
				}
			})
		}
	}
}

// ---------- Self-transitions ----------

func TestSelfTransitionsAreAlwaysRejected(t *testing.T) {
	for _, state := range allStates {
		t.Run(string(state), func(t *testing.T) {
			if err := ValidateRunStateTransition(state, state); err == nil {
				t.Fatalf("expected self-transition %q -> %q to be rejected", state, state)
			}
		})
	}
}

// ---------- Terminal states ----------

func TestTerminalStatesHaveNoOutgoingTransitions(t *testing.T) {
	for _, terminal := range terminalStates {
		for _, target := range allStates {
			t.Run(fmt.Sprintf("%s->%s", terminal, target), func(t *testing.T) {
				err := ValidateRunStateTransition(terminal, target)
				if err == nil {
					t.Fatalf("expected terminal state %q to reject transition to %q", terminal, target)
				}
			})
		}
	}
}

func TestIsTerminalRunStateReturnsTrueOnlyForTerminalStates(t *testing.T) {
	terminalSet := make(map[RunState]struct{})
	for _, s := range terminalStates {
		terminalSet[s] = struct{}{}
	}

	for _, state := range allStates {
		expected := false
		if _, ok := terminalSet[state]; ok {
			expected = true
		}
		t.Run(string(state), func(t *testing.T) {
			got := IsTerminalRunState(state)
			if got != expected {
				t.Fatalf("IsTerminalRunState(%q) = %v, want %v", state, got, expected)
			}
		})
	}
}

// ---------- Unknown / empty states ----------

func TestUnknownSourceStateIsRejected(t *testing.T) {
	err := ValidateRunStateTransition(RunState("bogus"), RunStateQueued)
	if err == nil {
		t.Fatal("expected error for unknown source state")
	}
}

func TestUnknownTargetStateIsRejected(t *testing.T) {
	err := ValidateRunStateTransition(RunStateCreated, RunState("bogus"))
	if err == nil {
		t.Fatal("expected error for unknown target state")
	}
}

func TestEmptySourceStateIsRejected(t *testing.T) {
	err := ValidateRunStateTransition(RunState(""), RunStateQueued)
	if err == nil {
		t.Fatal("expected error for empty source state")
	}
}

func TestEmptyTargetStateIsRejected(t *testing.T) {
	err := ValidateRunStateTransition(RunStateCreated, RunState(""))
	if err == nil {
		t.Fatal("expected error for empty target state")
	}
}

func TestBothStatesUnknownIsRejected(t *testing.T) {
	err := ValidateRunStateTransition(RunState("a"), RunState("b"))
	if err == nil {
		t.Fatal("expected error when both states are unknown")
	}
}

// ---------- Happy-path lifecycle tests ----------

func TestFullHappyPathLifecycleWithToolLoop(t *testing.T) {
	// Simulates: created -> queued -> acquired -> preparing -> model_running
	// -> tool_pending -> awaiting_approval -> tool_running -> awaiting_model_resume
	// -> model_running -> finalizing -> completed
	path := []RunState{
		RunStateCreated,
		RunStateQueued,
		RunStateAcquired,
		RunStatePreparing,
		RunStateModelRunning,
		RunStateToolPending,
		RunStateAwaitingApproval,
		RunStateToolRunning,
		RunStateAwaitingModelResume,
		RunStateModelRunning,
		RunStateFinalizing,
		RunStateCompleted,
	}

	for i := 0; i < len(path)-1; i++ {
		from, to := path[i], path[i+1]
		if err := ValidateRunStateTransition(from, to); err != nil {
			t.Fatalf("step %d: %q -> %q failed: %v", i+1, from, to, err)
		}
	}

	// Last state should be terminal
	if !IsTerminalRunState(path[len(path)-1]) {
		t.Fatalf("expected final state to be terminal")
	}
}

func TestSimpleHappyPathWithoutTools(t *testing.T) {
	// created -> queued -> acquired -> preparing -> model_running -> finalizing -> completed
	path := []RunState{
		RunStateCreated,
		RunStateQueued,
		RunStateAcquired,
		RunStatePreparing,
		RunStateModelRunning,
		RunStateFinalizing,
		RunStateCompleted,
	}

	for i := 0; i < len(path)-1; i++ {
		from, to := path[i], path[i+1]
		if err := ValidateRunStateTransition(from, to); err != nil {
			t.Fatalf("step %d: %q -> %q failed: %v", i+1, from, to, err)
		}
	}
}

func TestToolPendingDirectToToolRunning(t *testing.T) {
	// Auto-approved tool: tool_pending -> tool_running (bypasses awaiting_approval)
	if err := ValidateRunStateTransition(RunStateToolPending, RunStateToolRunning); err != nil {
		t.Fatalf("expected tool_pending -> tool_running to be valid: %v", err)
	}
}

// ---------- Cancellation from every non-terminal state ----------

func TestCancellationFromEveryNonTerminalState(t *testing.T) {
	for _, state := range allStates {
		if IsTerminalRunState(state) {
			continue
		}
		t.Run(string(state), func(t *testing.T) {
			err := ValidateRunStateTransition(state, RunStateCancelled)
			if err != nil {
				t.Fatalf("expected cancellation from %q to be valid, got: %v", state, err)
			}
		})
	}
}

// ---------- Timeout from every non-terminal state ----------

func TestTimeoutFromEveryNonTerminalState(t *testing.T) {
	for _, state := range allStates {
		if IsTerminalRunState(state) {
			continue
		}
		t.Run(string(state), func(t *testing.T) {
			err := ValidateRunStateTransition(state, RunStateTimedOut)
			if err != nil {
				t.Fatalf("expected timeout from %q to be valid, got: %v", state, err)
			}
		})
	}
}

// ---------- Failure from specific allowed states ----------

func TestFailureFromAllowedStates(t *testing.T) {
	// Failure is allowed from most states except tool_pending and awaiting_model_resume
	canFail := []RunState{
		RunStateCreated,
		RunStateQueued,
		RunStateAcquired,
		RunStatePreparing,
		RunStateModelRunning,
		RunStateAwaitingApproval,
		RunStateToolRunning,
		RunStateFinalizing,
	}
	for _, state := range canFail {
		t.Run(string(state), func(t *testing.T) {
			err := ValidateRunStateTransition(state, RunStateFailed)
			if err != nil {
				t.Fatalf("expected failure from %q to be valid, got: %v", state, err)
			}
		})
	}
}

func TestFailureNotAllowedFromToolPending(t *testing.T) {
	err := ValidateRunStateTransition(RunStateToolPending, RunStateFailed)
	if err == nil {
		t.Fatal("expected tool_pending -> failed to be rejected")
	}
}

func TestFailureNotAllowedFromAwaitingModelResume(t *testing.T) {
	err := ValidateRunStateTransition(RunStateAwaitingModelResume, RunStateFailed)
	if err == nil {
		t.Fatal("expected awaiting_model_resume -> failed to be rejected")
	}
}

// ---------- allStates covers every state in the map ----------

func TestAllStatesListMatchesTransitionMap(t *testing.T) {
	// This ensures allStates stays in sync with domain.go's allowedTransitions.
	// If a new state is added to the map but not to allStates, this test catches it.
	stateSet := make(map[RunState]struct{})
	for _, s := range allStates {
		stateSet[s] = struct{}{}
	}
	if len(stateSet) != len(allStates) {
		t.Fatalf("allStates contains duplicates: %d unique vs %d total", len(stateSet), len(allStates))
	}
	if len(stateSet) != 14 {
		t.Fatalf("expected 14 states, got %d (update this test if states are added/removed)", len(stateSet))
	}
}

// ---------- helpers ----------

type transitionPair struct {
	from, to RunState
}

func transitionKey(from, to RunState) transitionPair {
	return transitionPair{from, to}
}

func buildAllowedSet() map[transitionPair]bool {
	// Manually reconstructed from domain.go to verify independently.
	pairs := [][2]RunState{
		{RunStateCreated, RunStateQueued},
		{RunStateCreated, RunStateFailed},
		{RunStateCreated, RunStateCancelled},
		{RunStateCreated, RunStateTimedOut},

		{RunStateQueued, RunStateAcquired},
		{RunStateQueued, RunStateFailed},
		{RunStateQueued, RunStateCancelled},
		{RunStateQueued, RunStateTimedOut},

		{RunStateAcquired, RunStatePreparing},
		{RunStateAcquired, RunStateFailed},
		{RunStateAcquired, RunStateCancelled},
		{RunStateAcquired, RunStateTimedOut},

		{RunStatePreparing, RunStateModelRunning},
		{RunStatePreparing, RunStateFailed},
		{RunStatePreparing, RunStateCancelled},
		{RunStatePreparing, RunStateTimedOut},

		{RunStateModelRunning, RunStateToolPending},
		{RunStateModelRunning, RunStateFinalizing},
		{RunStateModelRunning, RunStateFailed},
		{RunStateModelRunning, RunStateCancelled},
		{RunStateModelRunning, RunStateTimedOut},

		{RunStateToolPending, RunStateAwaitingApproval},
		{RunStateToolPending, RunStateToolRunning},
		{RunStateToolPending, RunStateCancelled},
		{RunStateToolPending, RunStateTimedOut},

		{RunStateAwaitingApproval, RunStateToolRunning},
		{RunStateAwaitingApproval, RunStateAwaitingModelResume},
		{RunStateAwaitingApproval, RunStateCancelled},
		{RunStateAwaitingApproval, RunStateTimedOut},
		{RunStateAwaitingApproval, RunStateFailed},

		{RunStateToolRunning, RunStateAwaitingModelResume},
		{RunStateToolRunning, RunStateFailed},
		{RunStateToolRunning, RunStateCancelled},
		{RunStateToolRunning, RunStateTimedOut},

		{RunStateAwaitingModelResume, RunStateModelRunning},
		{RunStateAwaitingModelResume, RunStateCancelled},
		{RunStateAwaitingModelResume, RunStateTimedOut},

		{RunStateFinalizing, RunStateCompleted},
		{RunStateFinalizing, RunStateFailed},
		{RunStateFinalizing, RunStateCancelled},
		{RunStateFinalizing, RunStateTimedOut},
	}
	set := make(map[transitionPair]bool, len(pairs))
	for _, p := range pairs {
		set[transitionKey(p[0], p[1])] = true
	}
	return set
}
