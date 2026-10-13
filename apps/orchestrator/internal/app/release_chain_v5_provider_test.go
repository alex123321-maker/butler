package app

import (
	"context"
	"errors"
	"time"

	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
)

// All actual root/child entries share one window. The kernel budget wrapper
// remains the owning reservation ledger; this is an additional acceptance cap.
type releaseChainV5Provider struct {
	transport.ModelProvider
	checker         *releaseChainEntryChecker
	window          *releaseChainV5Window
	scenario        string
	expectedVersion string
	before          func(context.Context, string, string) error
}

// These are observations of the actual provider events consumed by this entry,
// before validation can replace an invalid packet with a safe error event.
type releaseChainV5Observations struct {
	finals, terminals, completed, cancelled, timedOut, transportErrors int
	stopFinals, maxTokensFinals                                        int
	finishReason                                                       *string
	finishUncertain                                                    bool
	usageUncertain, versionUncertain                                   bool
	providerStage                                                      *transport.ProviderStage
}

func (o *releaseChainV5Observations) observe(e transport.TransportEvent) {
	switch e.EventType {
	case transport.EventTypeAssistantFinal:
		o.finals++
		if e.AssistantFinal != nil {
			r := e.AssistantFinal.FinishReason
			if r == "STOP" {
				o.stopFinals++
			} else if r == "MAX_TOKENS" {
				o.maxTokensFinals++
			}
			if r != "STOP" && r != "MAX_TOKENS" || o.finishReason != nil && *o.finishReason != r {
				o.finishUncertain = true
				o.finishReason = nil
			} else if !o.finishUncertain {
				o.finishReason = &r
			}
		}
	case transport.EventTypeRunCompleted:
		o.terminals++
		o.completed++
	case transport.EventTypeRunCancelled:
		o.terminals++
		o.cancelled++
	case transport.EventTypeRunTimedOut:
		o.terminals++
		o.timedOut++
	case transport.EventTypeTransportError:
		o.terminals++
		o.transportErrors++
	}
}

func releaseChainV5FailureOutcome(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timed_out"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	return "transport_error"
}

func releaseChainV5CompleteObservation(kind transport.EventType) bool {
	switch kind {
	case transport.EventTypeAssistantFinal, transport.EventTypeToolCallRequested, transport.EventTypeToolCallBatchRequested,
		transport.EventTypeRunCompleted, transport.EventTypeRunCancelled, transport.EventTypeRunTimedOut, transport.EventTypeTransportError:
		return true
	default:
		return false
	}
}

func (*releaseChainV5Provider) InferenceLimits() (int64, int64) { return 1048576, 32768 }

func (p *releaseChainV5Provider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || r.Context.SessionKey != token.SessionKey || r.Context.ProviderName != antigravity.Name || r.Context.ModelName != antigravity.FlashHighModel {
		return nil, errNativeFixtureDenied
	}
	return p.entry(ctx, "start", r.Context.RunID, r, func(call context.Context) (transport.EventStream, error) { return p.ModelProvider.StartRun(call, r) })
}
func (p *releaseChainV5Provider) ContinueRun(ctx context.Context, r transport.ContinueRunRequest) (transport.EventStream, error) {
	return p.entry(ctx, "continue", r.RunID, r, func(call context.Context) (transport.EventStream, error) { return p.ModelProvider.ContinueRun(call, r) })
}
func (p *releaseChainV5Provider) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return p.entry(ctx, "submit", r.RunID, r, func(call context.Context) (transport.EventStream, error) {
		return p.ModelProvider.SubmitToolResult(call, r)
	})
}

func (p *releaseChainV5Provider) entry(ctx context.Context, kind, run string, input any, invoke func(context.Context) (transport.EventStream, error)) (transport.EventStream, error) {
	if p == nil || p.checker == nil || p.ModelProvider == nil || p.window == nil || p.expectedVersion == "" {
		return nil, errNativeFixtureDenied
	}
	if p.before != nil {
		if err := p.before(ctx, kind, run); err != nil {
			return nil, err
		}
	}
	ordinal, deadline, err := p.window.reserve(ctx, p.scenario, kind, p.checker.root, run, input, p.checker.guardTx)
	if err != nil {
		return nil, err
	}
	// This bound owns the whole auth/preflight/invoke/stream entry. The native
	// HTTP timeout is only an additional transport limit, not its replacement.
	attemptDeadline := time.Now().Add(120 * time.Second)
	if attemptDeadline.Before(deadline) {
		deadline = attemptDeadline
	}
	call, cancel := context.WithDeadline(ctx, deadline)
	var stages *releaseChainHighStageTrace
	if p.window.providerStages {
		stages = &releaseChainHighStageTrace{}
		parentCall := call
		call = transport.WithProviderStageObserver(call, func(stage transport.ProviderStage) {
			stages.observe(stage)
			transport.ObserveProviderStage(parentCall, stage)
		})
	}
	if call.Err() != nil {
		p.finish(ordinal, releaseChainV5FailureOutcome(call.Err()), nil, nil, "entry", transport.NormalizeError(call.Err(), p.Name()), &releaseChainV5Observations{providerStage: stages.seal()})
		cancel()
		return nil, call.Err()
	}
	stream, err := invoke(call)
	if err != nil {
		p.finish(ordinal, releaseChainV5FailureOutcome(err), nil, nil, "entry", transport.NormalizeError(err, p.Name()), &releaseChainV5Observations{providerStage: stages.seal()})
		cancel()
		return nil, err
	}
	if stream == nil {
		failure := &transport.Error{Type: transport.ErrorTypeProviderProtocolError}
		_ = p.finish(ordinal, "transport_error", nil, nil, "entry", failure, &releaseChainV5Observations{providerStage: stages.seal()})
		cancel()
		return nil, failure
	}
	out := make(chan transport.TransportEvent)
	go func() {
		defer close(out)
		defer cancel()
		outcome := "transport_error"
		failure := &transport.Error{Type: transport.ErrorTypeProviderProtocolError}
		var observed *transport.Usage
		var version *string
		var pendingFinal *transport.TransportEvent
		observations := &releaseChainV5Observations{}
		observedComplete := false
		usageUncertain, versionUncertain := false, false
		finishAttempted := false
		finish := func() error {
			finishAttempted = true
			observations.usageUncertain, observations.versionUncertain = usageUncertain, versionUncertain
			observations.providerStage = stages.seal()
			if !observedComplete || usageUncertain {
				observed = nil
			}
			if !observedComplete || versionUncertain {
				version = nil
			}
			return p.finish(ordinal, outcome, observed, version, "stream", failure, observations)
		}
		defer func() {
			if !finishAttempted {
				_ = finish()
			}
		}()
		for {
			select {
			case <-call.Done():
				outcome = releaseChainV5FailureOutcome(call.Err())
				failure = transport.NormalizeError(call.Err(), p.Name())
				return
			case event, ok := <-stream:
				if !ok {
					// EOF can race the provider closing its stream after this
					// request's context ended. Retain the actual context cause
					// rather than replacing it with the default protocol error.
					if err := call.Err(); err != nil {
						outcome = releaseChainV5FailureOutcome(err)
						failure = transport.NormalizeError(err, p.Name())
					}
					return
				}
				observations.observe(event)
				originalKind, originalVersion := event.EventType, event.ProviderModelVersion
				ownTerminal := event.RunID == run && (originalKind == transport.EventTypeRunCancelled || originalKind == transport.EventTypeRunTimedOut)
				if event.RunID != run {
					// Foreign events are observed lifecycle facts, but cannot supply
					// this request's cost or model version. Prior own complete
					// observations remain measured when this failure arrives.
					originalKind, originalVersion = "", nil
					event = transport.NewTransportErrorEvent(run, p.Name(), &transport.Error{Type: transport.ErrorTypeProviderProtocolError})
				}
				usage := event.Usage
				if event.AssistantFinal != nil && event.AssistantFinal.Usage != nil {
					if usage != nil && *usage != *event.AssistantFinal.Usage {
						usageUncertain = true
						event = transport.NewTransportErrorEvent(run, p.Name(), &transport.Error{Type: transport.ErrorTypeProviderProtocolError})
						usage = nil
					} else {
						usage = event.AssistantFinal.Usage
					}
				}
				if usage != nil {
					completeUsage := releaseChainV5CompleteObservation(originalKind)
					if !completeUsage || usage.Validate() != nil || observed != nil && *observed != *usage {
						usageUncertain = true
						if completeUsage {
							event = transport.NewTransportErrorEvent(run, p.Name(), &transport.Error{Type: transport.ErrorTypeProviderProtocolError})
						} else {
							event.Usage = nil
						}
					} else if observed == nil {
						copy := *usage
						observed = &copy
					}
				}
				if v := originalVersion; v != nil {
					completeVersion := releaseChainV5CompleteObservation(originalKind)
					if !completeVersion || *v != p.expectedVersion || version != nil && *version != *v {
						versionUncertain = true
						if completeVersion {
							event = transport.NewTransportErrorEvent(run, p.Name(), &transport.Error{Type: transport.ErrorTypeProviderProtocolError})
						} else {
							event.ProviderModelVersion = nil
						}
					} else {
						copy := *v
						version = &copy
					}
				}
				// The provider parsed this complete response packet before emitting
				// it. Later EOF/cancellation is a lifecycle failure, not evidence
				// that uniquely validated packet metadata was never observed.
				if releaseChainV5CompleteObservation(originalKind) {
					observedComplete = true
				}
				// A tool or final answer can lead to another request or an effect.
				// Stop before releasing it if this prospective experiment cannot
				// retain the complete cost and selected version. A failed request
				// still retains any unambiguous dimensions it actually reported.
				actionable := event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested || event.EventType == transport.EventTypeAssistantFinal
				fullTotal := observed != nil && (observed.TotalTokens > 0 || observed.InputTokens == 0 && observed.OutputTokens == 0)
				withinLimits := observed != nil && observed.InputTokens <= 1048576 && observed.OutputTokens <= 32768 && observed.TotalTokens <= observed.InputTokens+32768
				if actionable && (usageUncertain || versionUncertain || !fullTotal || version == nil || !withinLimits) {
					event = transport.NewTransportErrorEvent(run, p.Name(), &transport.Error{Type: transport.ErrorTypeProviderProtocolError})
				}
				// A V6 packet cannot be released from an absent or earlier phase.
				// The phase is diagnostic evidence, never a substitute for cost.
				if p.window.providerStages && (actionable || event.EventType == transport.EventTypeRunCompleted) {
					stage := stages.current()
					if stage == nil || *stage != transport.ProviderStageStream {
						event = transport.NewTransportErrorEvent(run, p.Name(), &transport.Error{Type: transport.ErrorTypeProviderProtocolError})
					}
				}
				if event.EventType == transport.EventTypeAssistantFinal && (pendingFinal != nil || event.AssistantFinal == nil || event.AssistantFinal.FinishReason != "STOP") ||
					event.EventType == transport.EventTypeRunCompleted && pendingFinal == nil ||
					event.EventType == transport.EventTypeToolCallRequested && (pendingFinal != nil || event.ToolCall == nil) ||
					event.EventType == transport.EventTypeToolCallBatchRequested && (pendingFinal != nil || event.ToolCallBatch == nil || len(event.ToolCallBatch.ToolCalls) == 0) {
					event = transport.NewTransportErrorEvent(run, p.Name(), &transport.Error{Type: transport.ErrorTypeProviderProtocolError})
				}
				// A structurally valid complete batch received by this generic
				// wrapper is a transport response, not
				// a protocol error. Persist its one request's cost, then pass it
				// unchanged to ordinary Core. Current Core advertises false batch
				// capability and rejects it before any Broker action. Current
				// Antigravity itself rejects multiple native functions upstream;
				// this branch is only exercised by the generic Go spy fixture.
				complete := event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested || event.EventType == transport.EventTypeRunCompleted
				if complete || event.EventType == transport.EventTypeTransportError {
					observedComplete = true
				}
				if complete {
					outcome = "completed"
					failure = nil
				} else if event.EventType == transport.EventTypeTransportError {
					failure = event.TransportError
				}
				if ownTerminal {
					if originalKind == transport.EventTypeRunTimedOut {
						outcome = "timed_out"
						if event.EventType != transport.EventTypeTransportError {
							failure = transport.NormalizeError(context.DeadlineExceeded, p.Name())
						}
					} else {
						outcome = "cancelled"
						if event.EventType != transport.EventTypeTransportError {
							failure = transport.NormalizeError(context.Canceled, p.Name())
						}
					}
					event = transport.NewTerminalEvent(run, originalKind, p.Name())
				}
				terminal := complete || ownTerminal || event.EventType == transport.EventTypeTransportError
				// The actual native packet must be durable before it can release a
				// tool proposal or final to the ordinary Core/Broker path. Failure
				// leaves the committed admission spent and its measurement unknown.
				if terminal {
					if err := finish(); err != nil {
						select {
						case out <- transport.NewTransportErrorEvent(run, p.Name(), &transport.Error{Type: transport.ErrorTypeProviderProtocolError}):
						case <-call.Done():
						}
						return
					}
					if ownTerminal || event.EventType == transport.EventTypeTransportError {
						event.Usage, event.ProviderModelVersion = observed, version // Measurements grant no action authority.
					}
				}
				if event.EventType == transport.EventTypeAssistantFinal {
					copy := event
					pendingFinal = &copy
					continue
				}
				if event.EventType == transport.EventTypeRunCompleted && pendingFinal != nil {
					select {
					case out <- *pendingFinal:
					case <-call.Done():
						outcome = releaseChainV5FailureOutcome(call.Err())
						failure = transport.NormalizeError(call.Err(), p.Name())
						return
					}
				}
				select {
				case out <- event:
				case <-call.Done():
					outcome = releaseChainV5FailureOutcome(call.Err())
					failure = transport.NormalizeError(call.Err(), p.Name())
					return
				}
				if terminal {
					return
				}
			}
		}
	}()
	return out, nil
}

// A measurement never creates authority or refunds an attempt. Unknown or
// incomplete cost remains NULL. The one terminal row cannot be overwritten.
func (p *releaseChainV5Provider) finish(ordinal int, outcome string, usage *transport.Usage, version *string, stage string, failure *transport.Error, observations *releaseChainV5Observations) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var in, out, total *int64
	if usage != nil {
		a, b := int64(usage.InputTokens), int64(usage.OutputTokens)
		in, out = &a, &b
		if usage.TotalTokens != 0 || usage.InputTokens == 0 && usage.OutputTokens == 0 {
			c := int64(usage.TotalTokens)
			total = &c
		}
	}
	var failureStage, code *string
	if outcome != "completed" {
		if stage != "entry" && stage != "stream" {
			stage = "stream"
		}
		c := transport.DiagnosticCode(failure)
		if c == "" {
			c = string(transport.ErrorTypeInternalTransportError)
		}
		failureStage, code = &stage, &c
	}
	var finals, terminals, completed, cancelled, timedOut, transportErrors, stopFinals, maxTokensFinals *int
	var finishReason *string
	var usageUncertain, versionUncertain, finishUncertain *bool
	var providerStage *transport.ProviderStage
	if observations != nil {
		finals, terminals, completed, cancelled, timedOut, transportErrors = &observations.finals, &observations.terminals, &observations.completed, &observations.cancelled, &observations.timedOut, &observations.transportErrors
		finishReason = observations.finishReason
		usageUncertain, versionUncertain = &observations.usageUncertain, &observations.versionUncertain
		stopFinals, maxTokensFinals, finishUncertain = &observations.stopFinals, &observations.maxTokensFinals, &observations.finishUncertain
		providerStage = observations.providerStage
	}
	tag, err := p.window.pool.Exec(ctx, `WITH finished AS MATERIALIZED (SELECT clock_timestamp() AS at) UPDATE `+p.window.table("fixture_chain_v5_high_requests")+` SET outcome=$2,input_tokens=$3,output_tokens=$4,total_tokens=$5,model_version=$6,finished_at=finished.at,elapsed_ms=GREATEST(0,floor(extract(epoch FROM finished.at-admitted_at)*1000)::bigint),failure_stage=$7,diagnostic_code=$8,final_count=$9,terminal_count=$10,completed_count=$11,cancelled_count=$12,timed_out_count=$13,transport_error_count=$14,finish_reason=$15,usage_observation_uncertain=$16,version_observation_uncertain=$17,stop_final_count=$18,max_tokens_final_count=$19,finish_observation_uncertain=$20,provider_operation_stage=$21 FROM finished WHERE ordinal=$1 AND outcome='unavailable'`, ordinal, outcome, in, out, total, version, failureStage, code, finals, terminals, completed, cancelled, timedOut, transportErrors, finishReason, usageUncertain, versionUncertain, stopFinals, maxTokensFinals, finishUncertain, providerStage)
	if err != nil || tag.RowsAffected() != 1 {
		return errNativeFixtureDenied
	}
	return nil
}
