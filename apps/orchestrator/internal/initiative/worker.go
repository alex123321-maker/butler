package initiative

import (
	"context"
	"errors"
	"time"
)

// Observer is an installed adapter with a fixed source binding. Models cannot
// register an adapter or choose its targets. Empty checks spend no model turns.
// Every real external observation must use the Broker's current-authority path;
// the built-in heartbeat consumes only already accepted typed observations.
type Observer interface {
	Check(context.Context, Source, ClaimedTick) (*Event, error)
}

type Worker struct {
	Store    *Store
	Promoter *Promoter
	Observer Observer
}

// Sweep is called by application maintenance, never a request's lifetime. Tick
// leases and deterministic event IDs survive restart and lost acknowledgement.
func (w *Worker) Sweep(ctx context.Context) error {
	if w.Store == nil || w.Promoter == nil {
		return ErrInvalid
	}
	if _, err := w.Store.AdvanceDue(ctx, time.Now().UTC(), 50); err != nil {
		return err
	}
	for i := 0; i < 20; i++ {
		tick, err := w.Store.ClaimTick(ctx, time.Minute)
		if errors.Is(err, ErrNotFound) {
			break
		}
		if err != nil {
			return err
		}
		if err = w.check(ctx, tick); err != nil {
			return err
		}
	}
	ready, err := w.Store.Ready(ctx, 20)
	if err != nil {
		return err
	}
	for _, c := range ready {
		if _, err = w.Promoter.Promote(ctx, c.ID, c.OwnerID); err != nil && !errors.Is(err, ErrConflict) {
			return err
		}
	}
	if err := w.Store.RefreshCompleted(ctx); err != nil {
		return err
	}
	if err := w.Promoter.RecoverMobileInputs(ctx); err != nil {
		return err
	}
	return w.Store.QueueDigests(ctx)
}

func (w *Worker) check(ctx context.Context, tick ClaimedTick) error {
	source, err := scanSource(w.Store.pool.QueryRow(ctx, `SELECT s.source_json,s.revision,s.enabled FROM initiative_sources s JOIN initiative_schedules q ON q.source_id=s.source_id WHERE q.schedule_id=$1 AND q.revision=$2 AND q.enabled AND s.source_id=$3`, tick.ScheduleID, tick.ScheduleRevision, tick.SourceID))
	if errors.Is(err, ErrNotFound) {
		return w.Store.FinishTick(ctx, tick, "skipped", "schedule_changed")
	}
	if err != nil {
		return err
	}
	if !source.Enabled || source.Revision != tick.SourceRevision {
		return w.Store.FinishTick(ctx, tick, "skipped", "source_changed")
	}
	var event *Event
	if w.Observer != nil {
		// Bound the adapter by the same durable claim; an unacknowledged timeout
		// retries observation, never an external mutation or model generation.
		observe, cancel := context.WithTimeout(ctx, 30*time.Second)
		event, err = w.Observer.Check(observe, source, tick)
		cancel()
		if err != nil {
			return w.Store.FinishTick(ctx, tick, "failed", "observer_failed")
		}
	}
	if event == nil {
		event = &Event{Kind: "periodic", ConditionDigest: hash("empty_check"), ResourceRevision: hash([]string{source.ID, tick.SlotKey}), EvidenceDigest: hash([]string{tick.ScheduleID, tick.SlotKey}), Confidence: 100, ObservedAt: tick.ScheduledAt}
	}
	event.ID = "tick:" + hash([]any{tick.ScheduleID, tick.ScheduleRevision, tick.SlotKey})
	event.SourceID = source.ID
	if _, _, err = w.Store.ObserveTick(ctx, source.OwnerID, *event, tick, time.Now().UTC()); err != nil {
		if errors.Is(err, ErrConflict) {
			return w.Store.FinishTick(ctx, tick, "skipped", "source_changed")
		}
		return err
	}
	reason := "observation_recorded"
	if event.Kind == "periodic" {
		reason = "empty_check"
	}
	return w.Store.FinishTick(ctx, tick, "checked", reason)
}

// Only Task Service's verified completion can complete an initiative. Provider
// final text, run.completed and an unverified result cannot set this state.
func (s *Store) RefreshCompleted(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `UPDATE initiatives i SET status='completed',reason='verified_task_result',updated_at=clock_timestamp() FROM tasks t JOIN task_results r ON r.task_id=t.task_id AND r.result_id=t.completion_result_id WHERE i.task_id=t.task_id AND i.status='promoted' AND t.status='completed' AND r.verdict='verified'`)
	return err
}
