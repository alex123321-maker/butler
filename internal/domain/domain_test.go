package domain

import "testing"

func TestValidateRunStateTransitionAllowsLifecyclePath(t *testing.T) {
	transitions := [][2]RunState{
		{RunStateCreated, RunStateQueued},
		{RunStateQueued, RunStateAcquired},
		{RunStateAcquired, RunStatePreparing},
		{RunStatePreparing, RunStateModelRunning},
		{RunStateModelRunning, RunStateToolPending},
		{RunStateToolPending, RunStateAwaitingApproval},
		{RunStateAwaitingApproval, RunStateToolRunning},
		{RunStateToolRunning, RunStateAwaitingModelResume},
		{RunStateAwaitingModelResume, RunStateModelRunning},
		{RunStateModelRunning, RunStateFinalizing},
		{RunStateFinalizing, RunStateCompleted},
	}

	for _, transition := range transitions {
		if err := ValidateRunStateTransition(transition[0], transition[1]); err != nil {
			t.Fatalf("expected %q -> %q to be valid, got %v", transition[0], transition[1], err)
		}
	}
}

func TestValidateRunStateTransitionAllowsErrorAndCancelPaths(t *testing.T) {
	transitions := [][2]RunState{
		{RunStatePreparing, RunStateFailed},
		{RunStateModelRunning, RunStateFailed},
		{RunStateToolRunning, RunStateFailed},
		{RunStateAwaitingApproval, RunStateFailed},
		{RunStateCreated, RunStateCancelled},
		{RunStateQueued, RunStateTimedOut},
		{RunStateFinalizing, RunStateCancelled},
	}

	for _, transition := range transitions {
		if err := ValidateRunStateTransition(transition[0], transition[1]); err != nil {
			t.Fatalf("expected %q -> %q to be valid, got %v", transition[0], transition[1], err)
		}
	}
}

func TestValidateRunStateTransitionRejectsInvalidPaths(t *testing.T) {
	transitions := [][2]RunState{
		{RunStateCreated, RunStatePreparing},
		{RunStateQueued, RunStateModelRunning},
		{RunStateAwaitingApproval, RunStateCompleted},
		{RunStateCompleted, RunStateFailed},
		{RunStateFailed, RunStateCancelled},
		{RunStateModelRunning, RunStateCompleted},
		{RunStateToolPending, RunStateAwaitingModelResume},
		{RunStatePreparing, RunStatePreparing},
	}

	for _, transition := range transitions {
		if err := ValidateRunStateTransition(transition[0], transition[1]); err == nil {
			t.Fatalf("expected %q -> %q to be invalid", transition[0], transition[1])
		}
	}
}

func TestTerminalRunStatesAreFinal(t *testing.T) {
	terminalStates := []RunState{
		RunStateCompleted,
		RunStateFailed,
		RunStateCancelled,
		RunStateTimedOut,
	}

	for _, state := range terminalStates {
		if !IsTerminalRunState(state) {
			t.Fatalf("expected %q to be terminal", state)
		}
		if err := ValidateRunStateTransition(state, RunStateQueued); err == nil {
			t.Fatalf("expected terminal state %q to reject further transitions", state)
		}
	}
}
