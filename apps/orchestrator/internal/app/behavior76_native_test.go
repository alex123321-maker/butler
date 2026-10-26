package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync/atomic"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

const behavior76JournalDDL = `CREATE TABLE IF NOT EXISTS public.behavior76_windows(
 id text PRIMARY KEY,manifest jsonb NOT NULL,expires_at timestamptz NOT NULL,
 admitted integer NOT NULL DEFAULT 0,closed boolean NOT NULL DEFAULT false);
 CREATE TABLE IF NOT EXISTS public.behavior76_attempts(
 window_id text NOT NULL REFERENCES public.behavior76_windows(id),n integer NOT NULL,
 slot text NOT NULL,run_id text NOT NULL,kind text NOT NULL,input jsonb NOT NULL,
 status text NOT NULL DEFAULT 'unknown',events jsonb,stage text NOT NULL DEFAULT '',
 milliseconds bigint,input_tokens bigint,output_tokens bigint,total_tokens bigint,
 thinking_tokens bigint,model_version text,started_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(window_id,n));`

var errBehavior76Native = errors.New("local behavior76 native boundary unavailable")

type behavior76NativeProvider struct {
	transport.ModelProvider
	fixture      *behavior76Fixture
	journal      *pgxpool.Pool
	window, slot string
	sealed       *atomic.Bool
	entryLimit   int // Trusted experiment admission metadata, never a model input.
	scope        *behavior76FixtureScope
}

func (p *behavior76NativeProvider) InferenceLimits() (int64, int64) { return 1048576, 8192 }
func (p *behavior76NativeProvider) InferenceTimeout() time.Duration { return 120 * time.Second }

func (p *behavior76NativeProvider) invoke(ctx context.Context, kind, run string, input any, call func(context.Context) (transport.EventStream, error)) (transport.EventStream, error) {
	if p.sealed.Load() {
		return nil, errBehavior76Native
	}
	if p.scope != nil {
		if err := p.scope.observe(); err != nil {
			return nil, err
		}
	} else if p.stopOnUnknownEffect() {
		return nil, errBehavior76Native
	}
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != run {
		p.sealed.Store(true)
		return nil, errBehavior76Native
	}
	proof, cancel := context.WithTimeout(ctx, 2*time.Second)
	var live bool
	err := p.fixture.pool.QueryRow(proof, `SELECT o.lease_id=$2 AND o.run_id=$1 AND o.fence_epoch=$3 AND o.expires_at>clock_timestamp()
 AND (r.task_id IS NULL OR EXISTS(SELECT 1 FROM tasks t WHERE t.task_id=r.task_id AND t.revision=r.task_revision AND t.status='active'))
 FROM runs r JOIN session_ownership o USING(session_key) WHERE r.run_id=$1`, run, token.LeaseID, token.FenceEpoch).Scan(&live)
	if err == nil && live {
		err = p.fixture.sources.CheckRunSources(proof, run)
	}
	cancel()
	if err != nil || !live {
		p.sealed.Store(true)
		return nil, errBehavior76Native
	}
	switch q := input.(type) {
	case transport.StartRunRequest:
		q.Context.ProviderSessionRef = nil
		input = q
	case transport.ContinueRunRequest:
		q.ProviderSessionRef = nil
		input = q
	case transport.SubmitToolResultRequest:
		q.ProviderSessionRef = nil
		input = q
	}
	raw, err := json.Marshal(input)
	if err != nil {
		p.sealed.Store(true)
		return nil, errBehavior76Native
	}
	limit := p.entryLimit
	if limit == 0 {
		limit = behavior76CEntryLimit
	}
	if limit < 1 || limit > 768 {
		p.sealed.Store(true)
		return nil, errBehavior76Native
	}
	var n int
	reserve := func() error {
		tx, err := p.journal.Begin(ctx)
		if err != nil {
			p.sealed.Store(true)
			return errBehavior76Native
		}
		defer tx.Rollback(ctx)
		err = tx.QueryRow(ctx, `UPDATE public.behavior76_windows w SET admitted=admitted+1 WHERE id=$1 AND NOT closed AND expires_at>clock_timestamp() AND admitted<$4 AND (manifest->>'max_native_entries')::integer=$4
 AND (SELECT count(*) FROM public.behavior76_attempts a WHERE a.window_id=w.id AND a.slot=$2)<8
	 AND (SELECT count(*) FROM public.behavior76_attempts a WHERE a.window_id=w.id AND a.run_id=$3)<4 RETURNING admitted`, p.window, p.slot, run, limit).Scan(&n)
		if err != nil {
			p.sealed.Store(true)
			return errBehavior76Native
		}
		if _, err = tx.Exec(ctx, `INSERT INTO public.behavior76_attempts(window_id,n,slot,run_id,kind,input) VALUES($1,$2,$3,$4,$5,$6)`, p.window, n, p.slot, run, kind, raw); err != nil {
			p.sealed.Store(true)
			return errBehavior76Native
		}
		if err = tx.Commit(ctx); err != nil {
			p.sealed.Store(true)
			return errBehavior76Native
		}
		return nil
	}
	release := func() {}
	if p.scope != nil {
		release, err = p.scope.begin(ctx, reserve)
	} else {
		err = reserve()
	}
	if err != nil && n == 0 {
		return nil, err
	}
	if release == nil {
		release = func() {}
	}
	// The UNKNOWN reservation is committed before any native auth or HTTP. Opaque
	// native continuity stays inside the adapter, never in the research journal.
	var stage atomic.Value
	stage.Store("")
	observed := transport.WithProviderStageObserver(ctx, func(s transport.ProviderStage) { stage.Store(string(s)) })
	start := time.Now()
	var persisted, persistedOK bool
	persist := func(status string, events []map[string]any, usage *transport.Usage, version string) bool {
		if persisted {
			return persistedOK
		}
		persisted = true
		done, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		var in, out, total, thinking any
		if usage != nil {
			in = usage.InputTokens
			out = usage.OutputTokens
			total = usage.TotalTokens
			if usage.ThinkingTokens != nil {
				thinking = *usage.ThinkingTokens
			}
		}
		tag, e := p.journal.Exec(done, `UPDATE public.behavior76_attempts SET status=$3,events=$4,stage=$5,milliseconds=$6,input_tokens=$7,output_tokens=$8,total_tokens=$9,thinking_tokens=$10,model_version=NULLIF($11,'') WHERE window_id=$1 AND n=$2 AND status='unknown' AND run_id=$12 AND slot=$13 AND kind=$14`, p.window, n, status, behavior76JSON(events), stage.Load().(string), time.Since(start).Milliseconds(), in, out, total, thinking, version, run, p.slot, kind)
		if e != nil || tag.RowsAffected() != 1 {
			p.sealed.Store(true)
			return false
		}
		persistedOK = true
		return true
	}
	finish := func(status string, events []map[string]any, usage *transport.Usage, version string) {
		defer release()
		persist(status, events, usage, version)
	}
	if err != nil || p.sealed.Load() {
		finish("not_entered_global_stop", nil, nil, "")
		return nil, errBehavior76Native
	}
	stream, err := call(observed)
	if err != nil {
		finish("entry_failed", []map[string]any{{"type": "entry_failed", "error_code": transport.DiagnosticCode(err), "safe_reason": transport.DiagnosticReason(err), "http_status": transport.DiagnosticHTTPStatus(err)}}, nil, "")
		p.sealed.Store(true)
		return nil, errBehavior76Native
	}
	return p.observeStream(ctx, stream, finish, func(events []map[string]any, usage *transport.Usage, version, observedRun string) bool {
		if observedRun != run {
			p.sealed.Store(true)
			return false
		}
		return p.stopCompletedLocalFailure(ctx, n, run, persist, events, usage, version)
	}), nil
}

// A clean channel close is not a completed provider operation. In particular,
// Core's owning watchdog can cancel HTTP before any event arrives. Preserve
// its NULL usage and close the whole experiment before a later admission.
func (p *behavior76NativeProvider) observeStream(ctx context.Context, stream transport.EventStream, finish func(string, []map[string]any, *transport.Usage, string), localEOF ...func([]map[string]any, *transport.Usage, string, string) bool) transport.EventStream {
	// Two trailing normalized events fit after a handoff (proposal, optional
	// run_completed). Original events remain ordered and fully journalled.
	output := make(chan transport.TransportEvent, 2)
	go func() {
		defer close(output)
		status, version := "stream_closed", ""
		var events []map[string]any
		var usage *transport.Usage
		var held *transport.TransportEvent
		var observedRun string
		var observedVersion bool
		var handoff []transport.TransportEvent
		recorded := false
		record := func() {
			if !recorded {
				recorded = true
				finish(status, events, usage, version)
			}
		}
		defer func() {
			if status == "stream_closed" || status == "cancelled" || behavior76AuthorityFailure(context.Cause(ctx)) {
				p.sealed.Store(true)
			}
			record()
		}()
		for {
			var event transport.TransportEvent
			var open bool
			select {
			case event, open = <-stream:
			default:
				// Prefer an already available terminal/EOF over the owning
				// consumer's cancellation after a successful tool handoff.
				select {
				case event, open = <-stream:
				case <-ctx.Done():
					status = "cancelled"
					return
				}
			}
			if !open {
				if len(handoff) > 0 {
					if ctx.Err() != nil {
						status = "cancelled"
						p.sealed.Store(true)
						return
					}
					record()
					if p.sealed.Load() {
						return
					}
					if ctx.Err() != nil {
						status = "cancelled"
						p.sealed.Store(true)
						return
					}
					// Reserve space for the complete bounded tail BEFORE exposing
					// its proposal. Earlier prefix events may still occupy output.
					// This is the only producer; a consumer can only increase room.
					for cap(output)-len(output) < len(handoff) {
						timer := time.NewTimer(time.Millisecond)
						select {
						case <-timer.C:
						case <-ctx.Done():
							timer.Stop()
							status = "cancelled"
							return
						}
					}
					if ctx.Err() != nil || p.sealed.Load() {
						status = "cancelled"
						return
					}
					for _, terminal := range handoff {
						output <- terminal // Reserved capacity: cannot block after proposal.
					}
					return
				}
				if held != nil {
					if ctx.Err() != nil || len(localEOF) != 1 || !localEOF[0](events, usage, version, observedRun) {
						p.sealed.Store(true)
					}
					select {
					case output <- *held:
					case <-ctx.Done():
						status = "cancelled"
					}
				}
				return
			}
			if held != nil {
				// Any event after the proposed empty terminal makes the entire
				// sequence ineligible. Preserve it; no local reinterpretation.
				p.sealed.Store(true)
			}
			if p.scope != nil && (p.scope.localEmpty || p.scope.localHTTP) {
				if observedRun == "" {
					observedRun = event.RunID
				}
				if event.RunID == "" || event.RunID != observedRun {
					p.sealed.Store(true)
				}
			}
			if event.Usage != nil {
				v := *event.Usage
				usage = &v
			} else if event.AssistantFinal != nil && event.AssistantFinal.Usage != nil {
				v := *event.AssistantFinal.Usage
				usage = &v
			}
			if event.ProviderModelVersion != nil {
				observedVersion = true
				version = *event.ProviderModelVersion
			}
			safe := map[string]any{"type": event.EventType}
			if event.AssistantDelta != nil && event.AssistantDelta.DeltaType == "text" {
				safe["text_delta"] = event.AssistantDelta
			}
			if event.ToolCall != nil {
				safe["tool_call"] = event.ToolCall
			}
			if event.ToolCallBatch != nil {
				safe["tool_batch"] = event.ToolCallBatch
			}
			if event.AssistantFinal != nil {
				safe["final"] = event.AssistantFinal
				if status != "transport_error" {
					status = "completed"
				}
			}
			if event.Usage != nil {
				safe["usage"] = event.Usage
			}
			if event.TransportError != nil {
				safe["error_code"] = transport.DiagnosticCode(event.TransportError)
				safe["safe_reason"] = transport.DiagnosticReason(event.TransportError)
				safe["http_status"] = transport.DiagnosticHTTPStatus(event.TransportError)
				status = "transport_error"
				cause, _, _ := p.localFailureClass(append(events, safe), usage, version)
				if cause == "model_http_no_response" && observedVersion {
					cause = ""
				}
				if len(localEOF) != 1 || event.TransportError.HTTPStatus != 0 || cause == "" {
					p.sealed.Store(true)
				}
			}
			if event.EventType == transport.EventTypeToolCallBatchRequested || (event.EventType == transport.EventTypeToolCallRequested && (event.ToolCall == nil || event.ToolCall.ToolCallRef == "" || event.ToolCall.ToolName == "" || !json.Valid([]byte(event.ToolCall.ArgsJSON)))) {
				// This composition explicitly does not support batch calls. A
				// malformed/unsupported proposal cannot become a valid handoff.
				status = "transport_error"
				p.sealed.Store(true)
			}
			if status != "transport_error" && event.EventType == transport.EventTypeToolCallRequested && event.ToolCall != nil {
				// A complete normalized function proposal is a logical provider
				// handoff, not a text-final or task-completion claim.
				status = "tool_handoff"
			}
			if event.EventType == transport.EventTypeRunCancelled || event.EventType == transport.EventTypeRunTimedOut {
				status = "cancelled"
				p.sealed.Store(true)
			}
			if event.EventType == transport.EventTypeRunCompleted && status != "transport_error" && status != "completed" && status != "tool_handoff" {
				status = "transport_error"
				safe["invalid_terminal_without_final_or_handoff"] = true
				p.sealed.Store(true)
			}
			events = append(events, safe)
			if len(handoff) > 0 {
				if len(handoff) == 1 && event.EventType == transport.EventTypeRunCompleted && event.AssistantDelta == nil && event.AssistantFinal == nil && event.ToolCall == nil && event.ToolCallBatch == nil && event.TransportError == nil && event.Usage == nil {
					handoff = append(handoff, event)
				} else {
					status = "transport_error"
					safe["invalid_handoff_tail"] = true
					p.sealed.Store(true)
				}
				continue
			}
			if held != nil {
				continue // Drain and record all late events before returning the error.
			}
			if event.TransportError != nil && !p.sealed.Load() && p.scope != nil && (p.scope.localEmpty || p.scope.localHTTP) {
				copy := event
				held = &copy
				continue
			}
			if status == "tool_handoff" && event.EventType == transport.EventTypeToolCallRequested {
				// Core's budget wrapper cancels its child context as soon as it
				// accepts a tool proposal. Require real EOF and journal success
				// BEFORE publication, so that ordinary cancellation cannot race
				// the provider's final channel close. No pre-EOF cancel is waived.
				handoff = append(handoff, event)
				continue
			}
			select {
			case output <- event:
			case <-ctx.Done():
				status = "cancelled"
				return
			}
		}
	}()
	return output
}
func (p *behavior76NativeProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	want := (prompt.Assembly{EffectiveBasePrompt: os.Getenv(prompt.SettingKeyBasePrompt) + "\n\n" + behavior76OutputContract}).TrustedInstructions()
	matched := 0
	for _, item := range q.InputItems {
		if item.Content == want {
			matched++
		}
	}
	if matched != 1 || os.Getenv(prompt.SettingKeyBasePromptEnabled) != "true" {
		p.sealed.Store(true)
		return nil, errBehavior76Native
	}
	return p.invoke(ctx, "start", q.Context.RunID, q, func(c context.Context) (transport.EventStream, error) { return p.ModelProvider.StartRun(c, q) })
}
func (p *behavior76NativeProvider) ContinueRun(ctx context.Context, q transport.ContinueRunRequest) (transport.EventStream, error) {
	return p.invoke(ctx, "continue", q.RunID, q, func(c context.Context) (transport.EventStream, error) { return p.ModelProvider.ContinueRun(c, q) })
}
func (p *behavior76NativeProvider) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return p.invoke(ctx, "submit", q.RunID, q, func(c context.Context) (transport.EventStream, error) { return p.ModelProvider.SubmitToolResult(c, q) })
}
