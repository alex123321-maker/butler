package input

import (
	"context"
	"encoding/json"
	"testing"
)

// Inject a process loss after the original result is stored, at each of the
// new observer checkpoints. No test oracle is a production effect receipt.
type browserObservationCrash struct {
	*Store
	stage      string
	nullMarker bool
	crash      context.CancelFunc
}

func (c browserObservationCrash) SaveCheckpoint(ctx context.Context, run, stage, payload string) error {
	if stage != "tool_result_recorded" {
		return c.Store.SaveCheckpoint(ctx, run, stage, payload)
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return err
	}
	marker := map[string]any{"version": 1, "tool_call_id": data["tool_call_id"], "effect_id": data["effect_id"], "read_call_id": "observer"}
	data["uncertain_browser"] = marker
	if c.nullMarker {
		data["uncertain_browser"] = nil
	}
	if c.stage == "tool_dispatching" {
		data["tool_call_id"], data["effect_id"], data["authorization_id"] = "observer", "read-effect", "read-grant"
	}
	raw, _ := json.Marshal(data)
	if err := c.Store.SaveCheckpoint(ctx, run, c.stage, string(raw)); err != nil {
		return err
	}
	c.crash()
	return ctx.Err()
}

func TestUncertainBrowserObservationCheckpointsNeverResumeIntegration(t *testing.T) {
	for _, stage := range []string{"browser_outcome_observing", "tool_dispatching", "browser_outcome_observed", "tool_result_recorded", "null-marker"} {
		t.Run(stage, func(t *testing.T) {
			f := newLifecycleFixture(t)
			worker, _ := f.worker(f.store, nil)
			worker.process(f.ctx, f.claim(t))
			if _, _, err := f.resolve(t, true, ownerActor()); err != nil {
				t.Fatal(err)
			}
			ctx, crash := context.WithCancel(f.ctx)
			defer crash()
			checkpointStage := stage
			if stage == "null-marker" {
				checkpointStage = "tool_result_recorded"
			}
			f.tools.unknown = true
			crashing, _ := f.worker(browserObservationCrash{Store: f.store, stage: checkpointStage, nullMarker: stage == "null-marker", crash: crash}, nil)
			crashing.process(ctx, f.claim(t))
			restarted, provider := f.worker(f.store, nil)
			restarted.process(f.ctx, f.reclaimAfterCrash(t))
			r := f.assertReceipt(t, "blocked")
			if r.ErrorCode != "effect_unknown" || len(provider.calls) != 0 || f.tools.calls != 1 {
				t.Fatal("observer checkpoint resumed provider or replayed an action", r.ErrorCode, len(provider.calls), f.tools.calls)
			}
			history, err := f.transcript.GetTranscript(f.ctx, f.key)
			if err != nil || len(history.ToolCalls) != 1 || history.ToolCalls[0].Status != "effect_unknown" {
				t.Fatal("original unknown transcript overwritten", err)
			}
		})
	}
}
