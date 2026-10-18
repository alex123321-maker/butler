package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type temporalNativeEntry struct {
	ID            int64                              `json:"id"`
	RunID         string                             `json:"run_id"`
	OperationID   string                             `json:"operation_id"`
	ReservationID string                             `json:"reservation_id"`
	Kind          string                             `json:"kind"`
	Start         *transport.StartRunRequest         `json:"start,omitempty"`
	Continuation  *transport.SubmitToolResultRequest `json:"continuation,omitempty"`
	Events        []transport.TransportEvent         `json:"events"`
	Usage         *transport.Usage                   `json:"usage"`
	Millis        int64                              `json:"millis"`
	InputBytes    int                                `json:"serialized_input_bytes"`
	FailureCode   string                             `json:"failure_code,omitempty"`
	HTTPStatus    int                                `json:"http_status,omitempty"`
	FailureReason string                             `json:"failure_reason,omitempty"`
	Stages        []transport.ProviderStage          `json:"stages"`
}

// No answering logic, retries or repair. Every start/tool continuation is one
// durable admission with its own full native usage (including thinking).
type temporalNativeProvider struct {
	transport.ModelProvider
	pool    *pgxpool.Pool
	proof   string
	mu      sync.Mutex
	entries []*temporalNativeEntry
	active  sync.WaitGroup
	sealErr error
}

func (p *temporalNativeProvider) InferenceLimits() (int64, int64) {
	return p.ModelProvider.(interface{ InferenceLimits() (int64, int64) }).InferenceLimits()
}
func (p *temporalNativeProvider) InferenceTimeout() time.Duration {
	return p.ModelProvider.(interface{ InferenceTimeout() time.Duration }).InferenceTimeout()
}
func (p *temporalNativeProvider) reserve(ctx context.Context, run, kind string, request any) (*temporalNativeEntry, error) {
	if err := p.auditVisible(ctx, run, request); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	operation, reservation, err := temporalNativeAdmission(ctx, tx, run)
	if err != nil {
		return nil, err
	}
	var count int
	var id int64
	if tx.QueryRow(ctx, `SELECT native_entries FROM temporal_native_cases c JOIN runs r ON r.session_key=c.session_key JOIN accepted_inputs i ON i.run_id=r.run_id AND i.input_id=c.input_id WHERE r.run_id=$1 AND c.status='running' AND i.status='running' AND i.max_attempts=1 AND i.attempts=1 FOR UPDATE OF c`, run).Scan(&count) != nil || count >= temporalMaxPerOriginal {
		return nil, errors.New("unsealed, repeated or exhausted original")
	}
	if tx.QueryRow(ctx, `UPDATE temporal_native_window SET paid_entries=paid_entries+1 WHERE singleton AND sealed AND NOT closed AND paid_entries<$1 RETURNING paid_entries`, temporalMaxNative).Scan(&id) != nil {
		return nil, errors.New("prospective native allowance unavailable")
	}
	if _, err = tx.Exec(ctx, `UPDATE temporal_native_cases c SET native_entries=native_entries+1,run_id=$1 FROM runs r WHERE r.run_id=$1 AND r.session_key=c.session_key`, run); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO temporal_native_entries(id,run_id,operation_id,reservation_id,kind,request_json,input_bytes)VALUES($1,$2,$3,$4,$5,$6,$7)`, id, run, operation, reservation, kind, raw, len(raw)); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	c := &temporalNativeEntry{ID: id, RunID: run, OperationID: operation, ReservationID: reservation, Kind: kind, InputBytes: len(raw), Events: []transport.TransportEvent{}, Stages: []transport.ProviderStage{}}
	if kind == "start" {
		v := request.(transport.StartRunRequest)
		c.Start = &v
	} else {
		v := request.(transport.SubmitToolResultRequest)
		c.Continuation = &v
	}
	p.mu.Lock()
	p.entries = append(p.entries, c)
	p.mu.Unlock()
	return c, nil
}

// The context reference is observability, not authority. Corroborate the
// actual durable owning foreground attempt before recording a native entry.
func temporalNativeAdmission(ctx context.Context, tx pgx.Tx, run string) (string, string, error) {
	operation, ok := budget.ModelOperationFromContext(ctx)
	if !ok {
		return "", "", errors.New("native entry lacks model operation reference")
	}
	var reservation string
	if tx.QueryRow(ctx, `SELECT a.reservation_id FROM execution_model_attempts a JOIN execution_budget_reservations r ON r.reservation_id=a.reservation_id WHERE a.operation_id=$1 AND a.run_id=$2 AND r.operation_id=a.operation_id AND r.owner_id=$3`, operation, run, temporalNativeOwner).Scan(&reservation) != nil || reservation == "" {
		return "", "", errors.New("native entry lacks durable owning model admission")
	}
	return operation, reservation, nil
}

func (p *temporalNativeProvider) auditVisible(ctx context.Context, run string, request any) error {
	raw, err := json.Marshal(request)
	if err != nil {
		return err
	}
	visible := string(raw)
	var selectedRaw []byte
	if p.pool.QueryRow(ctx, `SELECT c.prepared_json FROM temporal_native_cases c JOIN runs r ON r.session_key=c.session_key WHERE r.run_id=$1`, run).Scan(&selectedRaw) != nil {
		return errors.New("actual context lacks sealed original binding")
	}
	var selected temporalPreparedCase
	if json.Unmarshal(selectedRaw, &selected) != nil {
		return errors.New("sealed context binding invalid")
	}
	allowed := map[string]bool{}
	for _, id := range selected.ContextSources {
		allowed[id] = true
	}
	if r, ok := request.(transport.StartRunRequest); ok {
		var input strings.Builder
		for _, item := range r.InputItems {
			input.WriteString(item.Content)
			input.WriteByte('\n')
		}
		if !strings.Contains(input.String(), temporalNativePreference) || !strings.Contains(input.String(), selected.ContextMarker) {
			return errors.New("mandatory current scoped preference omitted")
		}
	}
	rows, err := p.pool.Query(ctx, `SELECT prepared_json FROM temporal_native_cases ORDER BY position`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var c temporalPreparedCase
		if rows.Scan(&raw) != nil || json.Unmarshal(raw, &c) != nil {
			return errors.New("context audit bindings unavailable")
		}
		if strings.Contains(visible, c.ID) {
			return errors.New("private corpus class label entered model context")
		}
		if c.ID != selected.ID && strings.Contains(visible, c.ContextMarker) {
			return errors.New("foreign case primary context entered model context")
		}
		for _, id := range c.ContextSources {
			if !allowed[id] && strings.Contains(visible, id) {
				return errors.New("foreign case source entered model context")
			}
		}
	}
	if rows.Err() != nil {
		return errors.New("context audit incomplete")
	}
	for _, hidden := range []string{"Сирень-17", "Туя-61", "expected_typed_query", "recorded_anchors", "prospective_draft", "temporal.case."} {
		if strings.Contains(visible, hidden) {
			return errors.New("hidden rubric or revoked content entered model context")
		}
	}
	return nil
}
func (p *temporalNativeProvider) call(ctx context.Context, c *temporalNativeEntry, invoke func(context.Context) (transport.EventStream, error)) (transport.EventStream, error) {
	started := time.Now()
	ctx = transport.WithProviderStageObserver(ctx, func(stage transport.ProviderStage) {
		p.mu.Lock()
		if len(c.Stages) < 32 {
			c.Stages = append(c.Stages, stage)
		}
		p.mu.Unlock()
	})
	finish := func() {
		p.mu.Lock()
		c.Millis = time.Since(started).Milliseconds()
		raw, _ := json.Marshal(c)
		p.mu.Unlock()
		bound, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_, dbErr := p.pool.Exec(bound, `UPDATE temporal_native_entries SET observation_json=$2,finished_at=clock_timestamp() WHERE id=$1`, c.ID, raw)
		// Full per-entry output is immutable before the next original runs.
		fileErr := temporalWriteExclusive(filepath.Join(p.proof, "entry-"+temporalNumber(c.ID)+".json"), raw)
		if dbErr != nil || fileErr != nil {
			p.mu.Lock()
			p.sealErr = errors.New("native observation failed to seal")
			p.mu.Unlock()
		}
	}
	stream, err := invoke(ctx)
	if err != nil {
		p.mu.Lock()
		c.FailureCode = transport.DiagnosticCode(err)
		c.HTTPStatus = transport.DiagnosticHTTPStatus(err)
		c.FailureReason = transport.DiagnosticReason(err)
		p.mu.Unlock()
		finish()
		return nil, err
	}
	out := make(chan transport.TransportEvent, 32)
	p.active.Add(1)
	go func() {
		defer p.active.Done()
		defer close(out)
		defer finish()
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-stream:
				if !ok {
					return
				}
				p.mu.Lock()
				// Raw transport errors may contain internals; the export retains
				// only fixed diagnostics. Model/source text is synthetic evidence.
				copy := e
				if e.TransportError != nil {
					c.FailureCode = transport.DiagnosticCode(e.TransportError)
					c.HTTPStatus = transport.DiagnosticHTTPStatus(e.TransportError)
					c.FailureReason = transport.DiagnosticReason(e.TransportError)
					copy.TransportError = nil
					copy.PayloadJSON = ""
				}
				if e.EventType == transport.EventTypeTransportWarning {
					copy.PayloadJSON = ""
				}
				if e.Usage != nil && e.Usage.Validate() == nil {
					u := *e.Usage
					c.Usage = &u
				}
				c.Events = append(c.Events, copy)
				p.mu.Unlock()
				select {
				case out <- e:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}
func (p *temporalNativeProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	c, e := p.reserve(ctx, r.Context.RunID, "start", r)
	if e != nil {
		return nil, e
	}
	return p.call(ctx, c, func(ctx context.Context) (transport.EventStream, error) { return p.ModelProvider.StartRun(ctx, r) })
}
func (p *temporalNativeProvider) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	c, e := p.reserve(ctx, r.RunID, "tool_result", r)
	if e != nil {
		return nil, e
	}
	return p.call(ctx, c, func(ctx context.Context) (transport.EventStream, error) {
		return p.ModelProvider.SubmitToolResult(ctx, r)
	})
}
func (p *temporalNativeProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, errors.New("native screen never resumes or replays an original")
}
func temporalNumber(id int64) string {
	return fmt.Sprintf("%03d", id)
}
func temporalWriteExclusive(path string, raw []byte) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(append(raw, '\n')); e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	return e
}
