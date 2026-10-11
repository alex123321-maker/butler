package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

type attemptReaderFunc func(context.Context, string) ([]delegation.AttemptReport, error)

func (f attemptReaderFunc) LoadFailedDelegationAttempts(ctx context.Context, id string) ([]delegation.AttemptReport, error) {
	return f(ctx, id)
}

func attemptContextReport() delegation.AttemptReport {
	return delegation.AttemptReport{TaskID: "failed-child", ParentTaskID: "child", TaskRevision: 1, ConstraintsRevision: 1, ContractRevision: 1, TaskStatus: "blocked", ObservedAt: time.Now().UTC(), Attempts: []delegation.FailedAttempt{{RunID: "failed-run", State: "failed", ErrorCode: "transport_error", DiagnosticCode: "rate_limited"}}, Actions: []delegation.AttemptAction{{RunID: "failed-run", ActionID: "actual-action", EffectID: "permanent-effect", State: "effect_unknown"}}, ModelUsage: []delegation.AttemptUsage{{RunID: "failed-run", OperationID: "actual-attempt", ReservationID: "spent-reservation"}}, Sources: []delegation.SourceRef{{ID: "contract", Revision: 1, AccessRevision: 1, Hash: strings.Repeat("a", 64)}}, Uncertainty: []delegation.Reason{{Kind: "execution_failed"}, {Kind: "verification_missing"}}}
}

func TestDelegationFailedAttemptContextPreparationAndRecovery(t *testing.T) {
	ctx, _, _ := delegatedContextFixture()
	tasks := &delegatedTasks{}
	report, calls := attemptContextReport(), 0
	s := &Service{sessions: delegatedSessions{}, config: Config{Tasks: tasks, MemoryBundles: &stubMemoryBundleService{}, FailedDelegationAttempts: attemptReaderFunc(func(_ context.Context, run string) ([]delegation.AttemptReport, error) {
		calls++
		if !tasks.pinned || run != "child-run" {
			return nil, errors.New("observation requires an attached current parent")
		}
		return []delegation.AttemptReport{report}, nil
	})}}
	event := ingress.InputEvent{EventID: "owner-input", SessionKey: "child-session", Source: "web", PayloadJSON: `{"task_id":"child","text":"Continue the current parent task"}`}
	p, err := s.prepareRun(ctx, event)
	if err != nil || calls != 1 {
		t.Fatal("parent observation preparation", err, calls)
	}
	if len(p.AttemptSourceRefs) != 1 || len(p.SourceDataRefs) != 0 {
		t.Fatal("attempt roots changed the child's immutable selected context")
	}
	var live transport.InputItem
	for _, item := range p.InputItems {
		if isFailedAttemptItem(item) {
			live = item
		}
	}
	if live.Role != "user" || content.ItemOrigin(live).Kind != content.SubagentOutput || content.ItemOrigin(live).IsInstruction() || !strings.Contains(live.Content, `"input_tokens":null`) || !strings.Contains(live.Content, "effect_unknown") {
		t.Fatal("failed observations lost DATA/unknown provenance", live.Content)
	}
	durable := durableSourcePreparation(p)
	if strings.Contains(mustMarshalJSON(durable), "rate_limited") || strings.Contains(mustMarshalJSON(durable), "permanent-effect") || !strings.Contains(mustMarshalJSON(durable), "reload_current_attempts") || !strings.Contains(mustMarshalJSON(p), "rate_limited") {
		t.Fatal("checkpoint cached a failed observation or mutated live input")
	}
	report.Actions[0].State = "effect_absent"
	if err = s.rehydrateSourcePreparation(ctx, &durable); err != nil || calls != 2 || strings.Contains(mustMarshalJSON(durable), "effect_unknown") || !strings.Contains(mustMarshalJSON(durable), "effect_absent") {
		t.Fatal("recovery reused old journal evidence", err, calls)
	}
	// Turning off the owning adapter cannot replay a persisted report body.
	s.config.FailedDelegationAttempts = nil
	if err = s.rehydrateSourcePreparation(ctx, &durable); !errors.Is(err, delegation.ErrDenied) || strings.Contains(mustMarshalJSON(durable), "effect_absent") {
		t.Fatal("missing current reader replayed a cached report", err)
	}
}

func TestDelegationFailedAttemptContextRecoveryFailureDropsCachedReport(t *testing.T) {
	ctx, _, _ := delegatedContextFixture()
	p := preparedRun{CurrentRunID: "child-run", AttemptSourceRefs: []string{"contract"}, InputItems: []transport.InputItem{failedAttemptItem(attemptContextReport(), []string{"contract"})}}
	s := &Service{config: Config{FailedDelegationAttempts: attemptReaderFunc(func(context.Context, string) ([]delegation.AttemptReport, error) { return nil, delegation.ErrDenied })}}
	if err := s.rehydrateSourcePreparation(ctx, &p); !errors.Is(err, delegation.ErrDenied) || len(p.InputItems) != 0 || len(p.AttemptSourceRefs) != 0 {
		t.Fatal("revoked/deleted source retained old observation", err)
	}
}

func TestDelegationFailedAttemptContextRejectsWrongParentAndRun(t *testing.T) {
	ctx, _, _ := delegatedContextFixture()
	for _, name := range []string{"wrong-parent", "wrong-run", "too-many", "invalid-source"} {
		t.Run(name, func(t *testing.T) {
			tasks := &delegatedTasks{}
			parent, _ := tasks.Get(ctx, "child")
			p := preparedRun{CurrentRunID: "child-run", Task: &parent}
			r := attemptContextReport()
			reports := []delegation.AttemptReport{r}
			switch name {
			case "wrong-parent":
				reports[0].ParentTaskID = "foreign"
			case "wrong-run":
				p.CurrentRunID = "foreign-run"
			case "too-many":
				reports = make([]delegation.AttemptReport, 9)
			case "invalid-source":
				reports[0].Sources[0].Hash = "unproved"
			}
			s := &Service{config: Config{FailedDelegationAttempts: attemptReaderFunc(func(context.Context, string) ([]delegation.AttemptReport, error) { return reports, nil })}}
			if err := s.attachFailedDelegationAttempts(ctx, &p); !errors.Is(err, delegation.ErrDenied) {
				t.Fatal("unbound observation reached model context", err)
			}
		})
	}
}
