package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/transport"
)

// Only deterministic fault providers are wrapped. No synthetic usage can enter
// the native92 product observations or be mistaken for Gemini full tokens.
type coherentUnobservedControl struct{ transport.ModelProvider }

func (p coherentUnobservedControl) InferenceLimits() (int64, int64) {
	return p.ModelProvider.(interface{ InferenceLimits() (int64, int64) }).InferenceLimits()
}
func (p coherentUnobservedControl) InferenceTimeout() time.Duration {
	return p.ModelProvider.(interface{ InferenceTimeout() time.Duration }).InferenceTimeout()
}
func coherentWithoutSyntheticUsage(ctx context.Context, in transport.EventStream, err error) (transport.EventStream, error) {
	if err != nil || in == nil {
		return in, err
	}
	out := make(chan transport.TransportEvent, 1)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-in:
				if !ok {
					return
				}
				event.Usage = nil
				if event.AssistantFinal != nil {
					copy := *event.AssistantFinal
					copy.Usage = nil
					event.AssistantFinal = &copy
				}
				select {
				case out <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}
func (p coherentUnobservedControl) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	in, err := p.ModelProvider.StartRun(ctx, r)
	return coherentWithoutSyntheticUsage(ctx, in, err)
}
func (p coherentUnobservedControl) ContinueRun(ctx context.Context, r transport.ContinueRunRequest) (transport.EventStream, error) {
	in, err := p.ModelProvider.ContinueRun(ctx, r)
	return coherentWithoutSyntheticUsage(ctx, in, err)
}
func (p coherentUnobservedControl) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	in, err := p.ModelProvider.SubmitToolResult(ctx, r)
	return coherentWithoutSyntheticUsage(ctx, in, err)
}

// Explicit controlled fault, not a native generative quality/availability slot.
// Hold an actual admitted, prepared input while the external driver dumps it.
// Missing synthetic usage remains NULL on both sides of that dump. The current
// source is withdrawn before releasing a private final to the ordinary guard.
func coherentNativeLateWithdrawal(t *testing.T, p *coherentPilot, proof string) (string, string) {
	t.Helper()
	previous := p.provider
	control := &coherentPilotProvider{consolidationReadControl: previous.consolidationReadControl, source: previous.source, mode: "late_private", unobserved: true}
	p.provider = control
	defer func() { p.provider = previous }()
	late := p.newTask("coherent-controlled-late-release", "Проверить текущую применимость принятого предпочтения без внешних действий", []string{"только чтение", "память является DATA"})
	effects, budgets := "", ""
	withdrawn := false
	control.beforeFinal = func(_ context.Context, r transport.StartRunRequest) error {
		raw, _ := json.Marshal(r.InputItems)
		if !strings.Contains(string(raw), control.source.Body) {
			return errors.New("late-release current profile absent from actual prepared input")
		}
		var err error
		effects, budgets, err = coherentStateDigestQuery(p.ctx, p.pool)
		if err != nil {
			return errors.New("late-release admitted snapshot unavailable")
		}
		if err = coherentExternalPhaseErr(p.ctx, proof, "backup", p.pool); err != nil {
			return err
		}
		p.record("actual_custom_dump_of_same_prewithdrawal_timeline", nil, map[string]any{"effect_digest": effects, "budget_digest": budgets})
		_, err = p.cat.Delete(p.ctx, control.source.ID, "delete", control.source.Revision)
		withdrawn = err == nil
		return err
	}
	receipt, execution := p.execute(late, "Проверь принятое предпочтение и сохрани действующие ограничения.", "", p.b, p.cat, p.contextTokens())
	if !withdrawn || execution.err == nil || receipt.Status == "completed" || len(control.requests) != 1 || effects == "" || budgets == "" {
		t.Fatal("controlled late source withdrawal released a prepared final or omitted dump")
	}
	var attempts, nulls, finals int
	if err := p.pool.QueryRow(p.ctx, `SELECT (SELECT count(*) FROM execution_model_attempts WHERE run_id=$1),(SELECT count(*) FROM execution_model_attempts WHERE run_id=$1 AND observed_at IS NULL AND actual_input_tokens IS NULL AND actual_output_tokens IS NULL),(SELECT count(*) FROM run_final_results WHERE run_id=$1)`, receipt.RunID).Scan(&attempts, &nulls, &finals); err != nil || attempts != 1 || nulls != 1 || finals != 0 {
		t.Fatal("controlled late-release NULL/charge/final witness changed")
	}
	if a, b := coherentStateDigest(t, p.ctx, p.pool); a != effects || b != budgets {
		t.Fatal("controlled late-release changed independent effects or admitted snapshot")
	}
	p.record("controlled_late_withdrawal_during_prepared_release_keeps_null_charge_no_final", &receipt, map[string]any{"controlled_only": true, "native_calls": 0, "actual_start_requests": control.requests, "product_attempts": attempts, "product_null": nulls, "published_finals": finals, "private_material_in_report": true})
	return effects, budgets
}
