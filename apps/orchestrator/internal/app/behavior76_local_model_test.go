package app

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
)

const behavior76LocalModelMode = "whole_fixture_after_exact_empty_answer_v1"
const behavior76LocalHTTPMode = "whole_fixture_after_exact_http_do_no_response_v1"

func behavior76ExactHTTPEvents(events []map[string]any) bool {
	if len(events) != 2 || len(events[0]) != 1 || events[0]["type"] != transport.EventTypeRunStarted || len(events[1]) != 4 {
		return false
	}
	e := events[1]
	return e["type"] == transport.EventTypeTransportError && e["error_code"] == "transport_connection_error" && e["safe_reason"] == "http_do_failed_without_response" && e["http_status"] == 0
}

func (p *behavior76NativeProvider) localFailureClass(events []map[string]any, usage *transport.Usage, version string) (cause, stage, code string) {
	if p.scope == nil {
		return "", "", ""
	}
	if p.scope.localEmpty && behavior76ExactEmptyEvents(events) && (!p.scope.localResponse || behavior76ResponseMetadata("stream_empty_answer", usage, version)) {
		return "model_empty_answer", "response_stream", "provider_protocol_error"
	}
	if p.scope.localHTTP && usage == nil && version == "" && behavior76ExactHTTPEvents(events) {
		return "model_http_no_response", "http_round_trip", "transport_connection_error"
	}
	if p.scope.localResponse && behavior76ExactResponseEvents(events) && behavior76ResponseMetadata(events[len(events)-1]["safe_reason"].(string), usage, version) {
		return "model_response_rejected", "response_stream", "provider_protocol_error"
	}
	return "", "", ""
}

// This predicate is deliberately narrower than a diagnostic-code match. It
// describes the entire safe event stream, not a model assertion about failure.
func behavior76ExactEmptyEvents(events []map[string]any) bool {
	if len(events) != 2 || len(events[0]) != 1 || events[0]["type"] != transport.EventTypeRunStarted {
		return false
	}
	e := events[1]
	if len(e) != 4 && len(e) != 5 {
		return false
	}
	if len(e) == 5 {
		if u, ok := e["usage"].(*transport.Usage); !ok || u == nil || u.Validate() != nil {
			return false
		}
	}
	return e["type"] == transport.EventTypeTransportError && e["error_code"] == "provider_protocol_error" && e["safe_reason"] == "stream_empty_answer" && e["http_status"] == 0
}

// Called only after underlying channel EOF. The same lock covers native
// reservations, ordinary Input claims, recipient admissions and this barrier.
// Persistence failure is never retried or interpreted as a local exception.
func (p *behavior76NativeProvider) stopCompletedLocalFailure(ctx context.Context, n int, run string, persist func(string, []map[string]any, *transport.Usage, string) bool, events []map[string]any, usage *transport.Usage, version string) bool {
	s := p.scope
	cause, stage, _ := p.localFailureClass(events, usage, version)
	if s == nil || cause == "" {
		p.sealed.Store(true)
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fail := func() bool { p.sealed.Store(true); return false }
	wantMode := "fixture-local-model-v8"
	if s.localHTTP {
		wantMode = "fixture-local-http-v9"
	}
	if s.localResponse {
		wantMode = "fixture-local-response-v10"
	}
	if s.binding.Mode != wantMode || s.binding.Window != p.window || (p.slot != s.binding.Arm && !strings.HasPrefix(p.slot, s.binding.Arm+"/")) {
		return fail()
	}
	if ctx.Err() != nil || s.global.Load() || (s.authorityTrace != nil && s.authorityTrace.failed()) {
		return fail()
	}
	if !persist("transport_error", events, usage, version) {
		return fail()
	}
	proof, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != run {
		return fail()
	}
	var live bool
	var owners []byte
	err := s.pool.QueryRow(proof, `SELECT o.lease_id=$2 AND o.run_id=$1 AND o.fence_epoch=$3 AND o.expires_at>clock_timestamp()
 AND (r.task_id IS NULL OR EXISTS(SELECT 1 FROM tasks t WHERE t.task_id=r.task_id AND t.revision=r.task_revision AND t.status='active')),
 (SELECT jsonb_agg(jsonb_build_object('input_id',a.input_id,'run_id',a.run_id,'session_key',a.session_key,'session_sequence',a.session_sequence) ORDER BY a.input_id)
 FROM accepted_inputs a WHERE a.run_id=r.run_id AND a.session_key=r.session_key AND a.task_id IS NOT DISTINCT FROM r.task_id AND a.status='running' AND a.claim_expires_at>clock_timestamp())
 FROM runs r JOIN session_ownership o USING(session_key) WHERE r.run_id=$1`, run, token.LeaseID, token.FenceEpoch).Scan(&live, &owners)
	if err != nil || !live || len(owners) == 0 || string(owners) == "null" || p.fixture.sources.CheckRunSources(proof, run) != nil {
		return fail()
	}
	var identities []json.RawMessage
	if json.Unmarshal(owners, &identities) != nil || len(identities) != 1 {
		return fail()
	}
	var evidence, root []byte
	var in, out, total, thinking any
	if usage != nil {
		in, out, total = usage.InputTokens, usage.OutputTokens, usage.TotalTokens
		if usage.ThinkingTokens != nil {
			thinking = *usage.ThinkingTokens
		}
	}
	err = p.journal.QueryRow(proof, `SELECT (to_jsonb(a)-'input'-'started_at') || jsonb_build_object('owner_inputs',$3::jsonb,'native_admission_cutoff_n',(SELECT max(n) FROM public.behavior76_attempts WHERE window_id=a.window_id)),to_jsonb(a)-'input'-'started_at'
 FROM public.behavior76_attempts a WHERE a.window_id=$1 AND a.n=$2 AND a.status='transport_error' AND a.stage=$12 AND a.run_id=$4 AND a.slot=$5 AND a.kind IN ('start','continue','submit') AND a.events=$6::jsonb AND a.model_version IS NOT DISTINCT FROM NULLIF($7,'') AND a.input_tokens IS NOT DISTINCT FROM $8::bigint AND a.output_tokens IS NOT DISTINCT FROM $9::bigint AND a.total_tokens IS NOT DISTINCT FROM $10::bigint AND a.thinking_tokens IS NOT DISTINCT FROM $11::bigint AND a.milliseconds>=0`, p.window, n, owners, run, p.slot, behavior76JSON(events), version, in, out, total, thinking, stage).Scan(&evidence, &root)
	if err != nil || !json.Valid(evidence) {
		return fail()
	}
	// Reconcile late UNKNOWNs even when another first cause already exists.
	if err = s.reconcileLocked(proof); err != nil && err != errBehavior76FixtureStopped {
		return fail()
	}
	if !s.stopped {
		if _, err = s.pool.Exec(proof, `INSERT INTO behavior76_fixture_stops(singleton,cause,binding,effects,native_evidence) VALUES(true,$3,$1,'[]',$2)`, behavior76JSON(s.binding), evidence, cause); err != nil {
			return fail()
		}
		s.stopped, s.stopCause = true, cause
		s.signalLocked()
	}
	if s.localResponse {
		hash := behavior76Hash(root)
		if original, loaded := p.localFailureProofs.LoadOrStore(run, hash); loaded && original != hash {
			return fail()
		}
	}
	return true
}

// Core sanitizes diagnostics on failRun. Exempt only its unwrapped, exact
// normalized error for this actual failed run/Input and a persisted eligible
// native row. A different or wrapped cleanup/transition error stays GLOBAL.
func (p *behavior76NativeProvider) localModelExecutionError(event ingress.InputEvent, executionErr error) bool {
	e, ok := executionErr.(*transport.Error)
	if !ok || e.SafeReason != "" || e.HTTPStatus != 0 || e.Retryable || e.ProviderCode != "" || e.ProviderName != "" || len(e.ProviderDetails) != 0 || p.scope == nil || p.sealed.Load() {
		return false
	}
	code, stage := "provider_protocol_error", "response_stream"
	if e.Type == transport.ErrorTypeTransportConnectionError && p.scope.localHTTP {
		code, stage = "transport_connection_error", "http_round_trip"
	} else if e.Type != transport.ErrorTypeProviderProtocolError || !p.scope.localEmpty {
		return false
	}
	if e.Message != code {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var run string
	if err := p.fixture.pool.QueryRow(ctx, `SELECT r.run_id FROM accepted_inputs a JOIN runs r ON r.run_id=a.run_id AND r.session_key=a.session_key WHERE a.event_id=$1 AND a.session_key=$2 AND r.current_state='failed' AND r.error_type='transport_error' AND r.error_message=$3`, event.EventID, event.SessionKey, code).Scan(&run); err != nil {
		return false
	}
	var events, root []byte
	var in, out, total, thinking *int64
	var version *string
	if err := p.journal.QueryRow(ctx, `SELECT events,input_tokens,output_tokens,total_tokens,thinking_tokens,model_version,to_jsonb(a)-'input'-'started_at' FROM public.behavior76_attempts a WHERE window_id=$1 AND slot=$2 AND run_id=$3 AND status='transport_error' AND stage=$4 AND ($5=false OR (input_tokens IS NULL AND output_tokens IS NULL AND total_tokens IS NULL AND thinking_tokens IS NULL AND model_version IS NULL)) AND ($6=false OR (SELECT count(*)=1 FROM public.behavior76_attempts b WHERE b.window_id=a.window_id AND b.slot=a.slot AND b.run_id=a.run_id AND b.status='transport_error')) ORDER BY n DESC LIMIT 1`, p.window, p.slot, run, stage, code == "transport_connection_error", p.scope.localResponse).Scan(&events, &in, &out, &total, &thinking, &version, &root); err != nil {
		return false
	}
	if p.scope.localResponse {
		original, ok := p.localFailureProofs.Load(run)
		if !ok || original != behavior76Hash(root) {
			return false
		}
	}
	// Compare the strict JSON shape using typed rehydration for the optional usage.
	var chain []map[string]json.RawMessage
	if json.Unmarshal(events, &chain) != nil || len(chain) < 2 || (!p.scope.localResponse && len(chain) != 2) {
		return false
	}
	normalized := make([]map[string]any, len(chain))
	for i, item := range chain {
		normalized[i] = map[string]any{}
		for key, value := range item {
			if strings.TrimSpace(string(value)) == "null" {
				return false
			}
			switch key {
			case "type":
				var t transport.EventType
				if json.Unmarshal(value, &t) != nil {
					return false
				}
				normalized[i][key] = t
			case "usage":
				u, ok := behavior76ExactStoredUsage(value)
				if !ok {
					return false
				}
				normalized[i][key] = u
			case "text_delta":
				d, ok := behavior76ExactStoredDelta(value)
				if !ok {
					return false
				}
				normalized[i][key] = d
			case "http_status":
				var status int
				if json.Unmarshal(value, &status) != nil {
					return false
				}
				normalized[i][key] = status
			default:
				var v string
				if json.Unmarshal(value, &v) != nil {
					return false
				}
				normalized[i][key] = v
			}
		}
	}
	if p.scope.localResponse {
		u, _ := normalized[len(normalized)-1]["usage"].(*transport.Usage)
		v := ""
		if version != nil {
			v = *version
			if v == "" {
				return false
			}
		}
		reason, _ := normalized[len(normalized)-1]["safe_reason"].(string)
		if !behavior76StoredMetadataMatches(u, in, out, total, thinking) || !behavior76ResponseMetadata(reason, u, v) {
			return false
		}
	}
	if code == "transport_connection_error" {
		return p.scope.localStopped() && behavior76ExactHTTPEvents(normalized)
	}
	return p.scope.localStopped() && (behavior76ExactEmptyEvents(normalized) || (p.scope.localResponse && behavior76ExactResponseEvents(normalized)))
}

// PostgreSQL JSONB has already canonicalized duplicate keys. Struct decoding
// alone would still admit null, case-folded aliases and ignored extra fields.
func behavior76ExactStoredUsage(raw json.RawMessage) (*transport.Usage, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil || (len(fields) != 3 && len(fields) != 4) {
		return nil, false
	}
	u := &transport.Usage{}
	for key, value := range fields {
		var n int
		if string(value) == "null" || json.Unmarshal(value, &n) != nil {
			return nil, false
		}
		switch key {
		case "InputTokens":
			u.InputTokens = n
		case "OutputTokens":
			u.OutputTokens = n
		case "TotalTokens":
			u.TotalTokens = n
		case "ThinkingTokens":
			u.ThinkingTokens = &n
		default:
			return nil, false
		}
	}
	for _, key := range []string{"InputTokens", "OutputTokens", "TotalTokens"} {
		if _, ok := fields[key]; !ok {
			return nil, false
		}
	}
	return u, u.Validate() == nil
}
