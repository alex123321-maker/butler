package orchestrator

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/transport"
)

type terminalWriteTasks struct {
	task.Manager
	failure    error
	contextErr error
}

func (s *terminalWriteTasks) SaveRunOutcome(ctx context.Context, _, _, _ string) error {
	s.contextErr = ctx.Err()
	return s.failure
}
func (s *terminalWriteTasks) BlockRun(ctx context.Context, _, _ string) error {
	s.contextErr = ctx.Err()
	return s.failure
}

type terminalWriteTransient struct {
	TransientWorkingStore
	failure error
}

func (s terminalWriteTransient) Save(context.Context, TransientWorkingState, time.Duration) (TransientWorkingState, error) {
	return TransientWorkingState{}, s.failure
}

func TestTerminalWriteFailuresStayJoinedAndSafe(t *testing.T) {
	for _, fault := range []string{"none", "working_memory", "transient_working_state", "run_failed_transition", "all"} {
		t.Run(fault, func(t *testing.T) {
			const marker = "PRIVATE_SYNTHETIC_TERMINAL_DIAGNOSTIC"
			actual := errors.New(marker)
			tasks := &terminalWriteTasks{}
			var transient TransientWorkingStore
			if fault == "working_memory" || fault == "all" {
				tasks.failure = actual
			}
			if fault == "transient_working_state" || fault == "all" {
				transient = terminalWriteTransient{failure: actual}
			}
			manager := newMemoryRunManager()
			manager.transitionErrs = map[commonv1.RunState]error{}
			current := &sessionv1.RunRecord{RunId: "run-1", SessionKey: "session", CurrentState: commonv1.RunState_RUN_STATE_MODEL_RUNNING}
			manager.records[current.RunId] = current
			if fault == "run_failed_transition" || fault == "all" {
				manager.transitionErrs[commonv1.RunState_RUN_STATE_FAILED] = actual
			}
			var output bytes.Buffer
			log := slog.New(slog.NewTextHandler(&output, nil))
			s := &Service{runs: manager, config: Config{Tasks: tasks, TransientStore: transient}, log: log}
			err := s.failRun(t.Context(), current, "lease", log, &transport.Error{Type: transport.ErrorTypeProviderProtocolError, Message: marker, SafeReason: marker})
			var provider *transport.Error
			if !errors.As(err, &provider) || provider.Type != transport.ErrorTypeProviderProtocolError || tasks.contextErr != nil {
				t.Fatal("original provider/context lost", err)
			}
			if strings.Contains(err.Error(), marker) || strings.Contains(output.String(), marker) || strings.Contains(current.ErrorMessage, marker) {
				t.Fatal("raw diagnostic exposed")
			}
			if fault == "none" {
				if _, ok := err.(*transport.Error); !ok {
					t.Fatal("ordinary provider error unnecessarily wrapped", err)
				}
				return
			}
			if _, ok := err.(*transport.Error); ok || !errors.Is(err, actual) {
				t.Fatal("terminal failure erased or made eligible as bare provider", err)
			}
			if fault != "all" && !strings.Contains(err.Error(), "terminal_"+fault+"_persistence_failed") {
				t.Fatal("fixed stage missing", err)
			}
		})
	}
}

func TestTerminalWriteBlockFailureIsReturnedSafely(t *testing.T) {
	actual := errors.New("PRIVATE_SYNTHETIC_BLOCK_DIAGNOSTIC")
	tasks := &terminalWriteTasks{failure: actual}
	s := &Service{config: Config{Tasks: tasks}}
	err := s.blockTaskRun(t.Context(), "run", "bounded_stop")
	if err == nil || !errors.Is(err, actual) || err.Error() != "terminal_task_block_persistence_failed" || tasks.contextErr != nil {
		t.Fatal("block persistence failure lost or exposed", err)
	}
	prior := &transport.Error{Type: transport.ErrorTypeProviderProtocolError, Message: "PRIVATE_SYNTHETIC_PRIOR_DIAGNOSTIC"}
	combined := err.withPreceding(prior)
	var provider *transport.Error
	if !errors.Is(combined, actual) || !errors.As(combined, &provider) || provider != prior || strings.Contains(combined.Error(), "PRIVATE_") {
		t.Fatal("preceding failure lost or exposed")
	}
}

func TestTerminalWriteSurvivesOwnedCancellation(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(&ownedCancellation{reason: "execution_authority_check_failed", failure: context.Canceled})
	prior := errors.New("bounded_original_stop")
	failure := &terminalWriteError{stage: "task_block", cause: context.Canceled, preceding: prior}
	for _, err := range []error{failure, errors.Join(prior, failure)} {
		got := preserveOwnedCancellation(ctx, err)
		var terminal *terminalWriteError
		if got != err || !errors.Is(got, prior) || !errors.As(got, &terminal) || terminal != failure || AuthorityObservationTimeout(got) {
			t.Fatal("terminal write failure replaced by owning cancellation", got)
		}
	}
}
