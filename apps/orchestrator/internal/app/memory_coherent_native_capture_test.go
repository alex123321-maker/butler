package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The92 window has one retained owning database and no case clones. Capture is
// independent of normal product admission; it never supplies an answer, tool
// proposal, authority, usage estimate or a replacement reservation.
type coherentNativeCapture struct {
	transport.ModelProvider
	mu       sync.Mutex
	proof    string
	pool     *pgxpool.Pool
	deadline time.Time
	slots    []coherentNativeSlot
	active   int
	entries  []*experienceNativeEntry
	joined   chan struct{}
	failed   error
	tail     bool
}

func coherentNativeWriteNew(parent, name string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(parent, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return f.Sync()
}
func (p *coherentNativeCapture) wait(ctx context.Context) error {
	p.mu.Lock()
	joined := p.joined
	p.mu.Unlock()
	if joined != nil {
		select {
		case <-joined:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
func (p *coherentNativeCapture) phase(ctx context.Context, name string) error {
	if err := p.wait(ctx); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failed != nil || p.tail {
		return errors.New("closed coherent phase sequence")
	}
	next := p.active + 1
	if p.active >= 0 {
		used := 0
		for _, e := range p.entries {
			if e.Slot == p.slots[p.active].Name {
				used++
			}
		}
		if used == 0 {
			return errors.New("native phase cannot complete without an actual provider entry")
		}
		p.slots[p.active].State = "completed"
		p.slots[p.active].Blocker = ""
		if err := coherentNativeWriteNew(p.proof, fmt.Sprintf("slot-%02d-end.json", p.active+1), p.slots[p.active]); err != nil {
			return err
		}
	}
	if name == "mechanics_tail" && next == len(p.slots) {
		p.active = -1
		p.tail = true
		return nil
	}
	if next >= len(p.slots) || p.slots[next].Name != name {
		return errors.New("unexpected coherent slot/order")
	}
	p.active = next
	p.slots[next].State = "running"
	p.slots[next].Blocker = ""
	return coherentNativeWriteNew(p.proof, fmt.Sprintf("slot-%02d-start.json", next+1), p.slots[next])
}
func (p *coherentNativeCapture) count() int { p.mu.Lock(); defer p.mu.Unlock(); return len(p.entries) }
func (p *coherentNativeCapture) completeRead(run string, source catalog.Source) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	original := []rune(source.Body)
	covered := make([]bool, len(original))
	pages := 0
	for _, e := range p.entries {
		if e.RunID != run || e.Kind != "tool_result" {
			continue
		}
		var request transport.SubmitToolResultRequest
		var response struct {
			Status string             `json:"status"`
			Result readcontrol.Result `json:"result"`
		}
		if json.Unmarshal(e.Request, &request) != nil || json.Unmarshal([]byte(request.ToolResultJSON), &response) != nil || response.Status != "completed" || response.Result.Status != "ok" {
			continue
		}
		for _, item := range response.Result.Items {
			if item.Source.ID != source.ID {
				continue
			}
			text := []rune(item.Text)
			end := item.NextOffset
			start := end - len(text)
			if item.Source.Revision != source.Revision || item.Source.AccessRevision != source.AccessRevision || item.Source.Hash != source.Hash || item.Redacted || start < 0 || end > len(original) || string(original[start:end]) != item.Text {
				return errors.New("native current consolidated page differs from exact original")
			}
			for i := start; i < end; i++ {
				covered[i] = true
			}
			pages++
		}
	}
	if pages == 0 || len(covered) == 0 {
		return errors.New("native consolidated original was not opened")
	}
	for _, have := range covered {
		if !have {
			return errors.New("native consolidated original paging incomplete")
		}
	}
	return nil
}
func (p *coherentNativeCapture) reserve(ctx context.Context, run, kind string, request any) (*experienceNativeEntry, error) {
	if err := p.wait(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failed != nil || p.pool == nil || p.ModelProvider == nil || p.active < 0 || p.tail || !time.Now().Before(p.deadline) || len(p.entries) >= coherentNativeMaximum() {
		return nil, errors.New("coherent native entry closed")
	}
	slot := p.slots[p.active]
	used := 0
	for _, e := range p.entries {
		if e.Slot == slot.Name {
			used++
		}
	}
	if used >= slot.Maximum {
		return nil, errors.New("coherent original slot exhausted")
	}
	op, ok := budget.ModelOperationFromContext(ctx)
	if !ok || op == "" {
		return nil, errors.New("owning coherent admission absent")
	}
	var reservation string
	err := p.pool.QueryRow(ctx, `SELECT a.reservation_id FROM (
 SELECT operation_id,reservation_id,run_id,provider,model FROM execution_model_attempts
 UNION ALL SELECT operation_id,reservation_id,operation_id,provider,model FROM memory_model_attempts
) a JOIN execution_budget_reservations r USING(reservation_id)
WHERE a.operation_id=$1 AND a.run_id=$2 AND a.provider='antigravity' AND a.model='gemini-3.8-flash-high' AND r.owner_id='owner'`, op, run).Scan(&reservation)
	if err != nil || reservation == "" {
		return nil, errors.New("coherent product reservation not corroborated")
	}
	raw, err := json.Marshal(request)
	if err != nil || len(raw) > 8388608 {
		return nil, errors.New("coherent request capture exceeds bound")
	}
	e := &experienceNativeEntry{Sequence: len(p.entries) + 1, Slot: slot.Name, RunID: run, Operation: op, Reservation: reservation, Kind: kind, Request: raw, Events: []json.RawMessage{}}
	// The durable independent method-entry witness precedes real transport.
	if _, err = p.pool.Exec(ctx, `INSERT INTO coherent_native_entries(sequence,slot,operation_id,reservation_id,run_id,kind,request_json) VALUES($1,$2,$3,$4,$5,$6,$7)`, e.Sequence, e.Slot, e.Operation, e.Reservation, e.RunID, e.Kind, e.Request); err != nil {
		p.failed = errors.New("coherent independent entry journal denied")
		return nil, p.failed
	}
	if err = coherentNativeWriteNew(p.proof, fmt.Sprintf("entry-%03d-start.json", e.Sequence), e); err != nil {
		p.failed = err
		return nil, err
	}
	p.entries = append(p.entries, e)
	p.joined = make(chan struct{})
	return e, nil
}
func (p *coherentNativeCapture) observe(ctx context.Context, e *experienceNativeEntry, start time.Time, in transport.EventStream, providerErr error) (transport.EventStream, error) {
	finish := func(failure string) {
		p.mu.Lock()
		defer p.mu.Unlock()
		millis := time.Since(start).Milliseconds()
		e.Millis = &millis
		e.Finished = true
		e.Failure = failure
		if ctx.Err() != nil {
			e.Canceled = "context_canceled_or_expired"
		}
		retained, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		raw, _ := json.Marshal(e)
		if _, err := p.pool.Exec(retained, `UPDATE coherent_native_entries SET observation=$2 WHERE sequence=$1 AND observation IS NULL`, e.Sequence, raw); err != nil {
			p.failed = errors.New("coherent entry closure journal failed")
		}
		if err := coherentNativeWriteNew(p.proof, fmt.Sprintf("entry-%03d-end.json", e.Sequence), e); err != nil {
			p.failed = err
		}
		close(p.joined)
	}
	if providerErr != nil {
		code := transport.DiagnosticCode(providerErr)
		if code == "" {
			code = "provider_method_error_unknown"
		}
		finish(code)
		return nil, providerErr
	}
	if in == nil {
		finish("nil_provider_stream")
		return nil, errors.New("native coherent stream absent")
	}
	out := make(chan transport.TransportEvent, 64)
	go func() {
		failure := ""
		captured := 0
		terminal := false
		usageConflict := false
		defer close(out)
		defer func() { finish(failure) }()
		for {
			select {
			case <-ctx.Done():
				if !terminal {
					failure = "context_canceled_or_expired"
				}
				return
			case event, ok := <-in:
				if !ok {
					return
				}
				safe := experienceNativeSafeEvent(event)
				raw, err := json.Marshal(safe)
				captured += len(raw)
				if err != nil || len(raw) > 8*1024*1024 || captured > 32*1024*1024 {
					failure = "provider_event_capture_rejected"
					return
				}
				p.mu.Lock()
				e.Events = append(e.Events, raw)
				if safe.Usage != nil && !usageConflict {
					u := *safe.Usage
					if e.Usage != nil && *e.Usage != u {
						usageConflict, e.Usage = true, nil
						failure = "conflicting_provider_usage"
					} else {
						e.Usage = &u
					}
				}
				if event.TransportError != nil {
					failure = transport.DiagnosticCode(event.TransportError)
				}
				terminal = terminal || event.EventType == transport.EventTypeAssistantFinal || event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested
				p.mu.Unlock()
				select {
				case out <- event:
				case <-ctx.Done():
					failure = "context_canceled_or_expired"
					return
				}
			}
		}
	}()
	return out, nil
}
func (p *coherentNativeCapture) stage(ctx context.Context, e *experienceNativeEntry) context.Context {
	prior := ctx
	return transport.WithProviderStageObserver(ctx, func(s transport.ProviderStage) {
		transport.ObserveProviderStage(prior, s)
		p.mu.Lock()
		defer p.mu.Unlock()
		if !e.Finished && len(e.Stages) < 32 {
			e.Stages = append(e.Stages, s)
		}
	})
}
func (p *coherentNativeCapture) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	e, err := p.reserve(ctx, r.Context.RunID, "start", r)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	stream, err := p.ModelProvider.StartRun(p.stage(ctx, e), r)
	return p.observe(ctx, e, start, stream, err)
}
func (p *coherentNativeCapture) ContinueRun(ctx context.Context, r transport.ContinueRunRequest) (transport.EventStream, error) {
	e, err := p.reserve(ctx, r.RunID, "continuation", r)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	stream, err := p.ModelProvider.ContinueRun(p.stage(ctx, e), r)
	return p.observe(ctx, e, start, stream, err)
}
func (p *coherentNativeCapture) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	e, err := p.reserve(ctx, r.RunID, "tool_result", r)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	stream, err := p.ModelProvider.SubmitToolResult(p.stage(ctx, e), r)
	return p.observe(ctx, e, start, stream, err)
}
func (p *coherentNativeCapture) InferenceLimits() (int64, int64) {
	return p.ModelProvider.(interface{ InferenceLimits() (int64, int64) }).InferenceLimits()
}
func (p *coherentNativeCapture) InferenceTimeout() time.Duration {
	return p.ModelProvider.(interface{ InferenceTimeout() time.Duration }).InferenceTimeout()
}
func (p *coherentNativeCapture) close(ctx context.Context, failed bool) error {
	if err := p.wait(ctx); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.slots {
		if p.slots[i].State == "running" {
			p.slots[i].State = "failed"
			p.slots[i].Blocker = "whole_aborted_before_slot_sealed"
		}
		if p.slots[i].State == "not_executed" {
			p.slots[i].Blocker = "whole_closed_before_execution"
		}
	}
	return coherentNativeWriteNew(p.proof, "native-window-final.json", map[string]any{"slots": p.slots, "entries": p.entries, "maximum_entries": coherentNativeMaximum(), "capture_error": p.failed != nil, "whole_failed": failed, "mechanics_tail_reached": p.tail, "semantic_qualified": false, "native_http_reach_is_not_inferred": true})
}
