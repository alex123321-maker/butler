package actionauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type EffectState string

const (
	StatePrepared  EffectState = "prepared"
	StateAllowed   EffectState = "allowed"
	StateExecuting EffectState = "executing"
	StateConfirmed EffectState = "effect_confirmed"
	StateAbsent    EffectState = "effect_absent"
	StateUnknown   EffectState = "effect_unknown"
)

// EffectPolicy is pinned with the exact contract, not inferred from model text
// or a caller-supplied HTTP header. ExternalReceipt is a recipient protocol.
type EffectPolicy struct {
	RetrySafety        string `json:"retry_safety"`
	VerificationMethod string `json:"verification_method"`
	IdempotencyHeader  string `json:"idempotency_header,omitempty"`
	ScopeOrigin        string `json:"scope_origin,omitempty"`
	VerificationURL    string `json:"verification_url,omitempty"`
}

type EffectRecord struct {
	ActionID        string        `json:"action_id"`
	Fingerprint     string        `json:"fingerprint"`
	Binding         ActionBinding `json:"binding"`
	Policy          EffectPolicy  `json:"policy"`
	State           EffectState   `json:"state"`
	IdempotencyKey  string        `json:"idempotency_key,omitempty"`
	AuthorizationID string        `json:"authorization_id,omitempty"`
	DispatchOutcome string        `json:"dispatch_outcome,omitempty"`
	EvidenceRefs    []string      `json:"evidence_refs"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

// Evidence contains provenance and digests only. Raw output, credential values,
// receipt IDs, cookies and URL queries are not copied into the journal.
type EffectEvidence struct {
	EvidenceID       string      `json:"evidence_id"`
	ActionID         string      `json:"action_id"`
	SourceActionID   string      `json:"source_action_id"`
	SourceToolCallID string      `json:"source_tool_call_id"`
	SourceRunID      string      `json:"source_run_id"`
	Source           string      `json:"source"`
	Method           string      `json:"method"`
	Verdict          EffectState `json:"verdict"`
	ResultDigest     string      `json:"result_digest"`
	ArtifactDigest   string      `json:"artifact_digest,omitempty"`
	ObservedAt       time.Time   `json:"observed_at"`
	RecordedAt       time.Time   `json:"recorded_at"`
}

type EffectStore interface {
	GetEffect(context.Context, string) (EffectRecord, error)
	RecordDispatchOutcome(context.Context, string, string, EffectEvidence) (EffectRecord, error)
	ReconcileEffect(context.Context, string, string, EffectEvidence) (EffectRecord, error)
	GetEffectEvidence(context.Context, string) (EffectEvidence, error)
	MarkInterruptedEffect(context.Context, string) error
}

// EffectIdentity enables a trusted task checkpoint to carry a logical reference
// between runs. Execution-bound approval IDs remain distinct. Reusing a logical
// reference with changed args, contract, owner or revisions is rejected by PG.
func EffectIdentity(b ActionBinding, logicalRef string) (string, string, error) {
	copy := b
	copy.ActionID, copy.ActionHash, copy.RunID, copy.ToolCallID = "", "", "", ""
	encoded, err := json.Marshal(copy)
	if err != nil {
		return "", "", err
	}
	if logicalRef == "" {
		return b.ActionID, digest(string(encoded)), nil
	}
	if b.TaskID == "" || len(logicalRef) > 160 || strings.TrimSpace(logicalRef) != logicalRef || strings.ContainsAny(logicalRef, "\x00\r\n") {
		return "", "", fmt.Errorf("logical action reference requires a trusted task checkpoint")
	}
	return "effect_" + digest(b.OwnerID+"\x00"+b.TaskID+"\x00"+logicalRef), digest(string(encoded)), nil
}

func ResolveEffectPolicy(b ActionBinding, contract *toolbrokerv1.ToolContract) (EffectPolicy, error) {
	p := EffectPolicy{RetrySafety: "unsafe", VerificationMethod: "none"}
	if c := contract.GetEffectContract(); c != nil {
		p = EffectPolicy{c.GetRetrySafety(), c.GetVerificationMethod(), c.GetIdempotencyHeader(), c.GetScopeOrigin(), c.GetVerificationUrl()}
	}
	if p.RetrySafety == "" {
		p.RetrySafety = "unsafe"
	}
	if p.VerificationMethod == "" {
		p.VerificationMethod = "none"
	}
	if b.Effect == EffectRead {
		if p.RetrySafety != "unsafe" && p.RetrySafety != "read_only" && p.RetrySafety != "external_idempotency" {
			return p, fmt.Errorf("read effect contract is incompatible")
		}
		p.RetrySafety, p.VerificationMethod, p.IdempotencyHeader = "read_only", "read_result", ""
		return p, nil
	}
	switch p.RetrySafety {
	case "unsafe":
		if p.IdempotencyHeader != "" {
			return p, fmt.Errorf("idempotency header requires an external recipient guarantee")
		}
		if p.VerificationMethod != "none" && !(p.VerificationMethod == "local_runtime_ack" && b.Effect == EffectPrepare) {
			return p, fmt.Errorf("external effect requires a supported receipt contract")
		}
	case "external_idempotency":
		if b.ToolName != "http.request" || b.Effect != EffectCommit || p.VerificationMethod != "external_receipt_v1" || (p.IdempotencyHeader != "Idempotency-Key" && p.IdempotencyHeader != "X-Idempotency-Key") {
			return p, fmt.Errorf("unsupported external idempotency contract")
		}
		resource, err := url.Parse(b.Resource)
		scope, e2 := url.Parse(p.ScopeOrigin)
		if err != nil || e2 != nil || scope.User != nil || scope.RawQuery != "" || scope.Fragment != "" || (scope.Path != "" && scope.Path != "/") || scope.Host == "" || (scope.Scheme != "http" && scope.Scheme != "https") || resource.Scheme+"://"+resource.Host != scope.Scheme+"://"+scope.Host {
			return p, fmt.Errorf("external idempotency requires an exact scoped origin")
		}
		p.ScopeOrigin = scope.Scheme + "://" + scope.Host
		if p.VerificationURL != "" {
			if strings.Count(p.VerificationURL, "{idempotency_key}") != 1 {
				return p, fmt.Errorf("verification URL requires one system idempotency placeholder")
			}
			u, e := url.Parse(strings.ReplaceAll(p.VerificationURL, "{idempotency_key}", "key"))
			if e != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Scheme+"://"+u.Host != p.ScopeOrigin {
				return p, fmt.Errorf("verification URL must retain the exact recipient origin")
			}
		}
	default:
		return p, fmt.Errorf("unsupported retry safety")
	}
	return p, nil
}

type DispatchIdentity struct{ ActionID, IdempotencyKey, Fingerprint string }
type dispatchContextKey struct{}

func WithDispatchIdentity(ctx context.Context, identity DispatchIdentity) context.Context {
	return context.WithValue(ctx, dispatchContextKey{}, identity)
}
func DispatchIdentityFromContext(ctx context.Context) DispatchIdentity {
	value, _ := ctx.Value(dispatchContextKey{}).(DispatchIdentity)
	return value
}

func verificationResource(r EffectRecord) string {
	return strings.ReplaceAll(r.Policy.VerificationURL, "{idempotency_key}", r.IdempotencyKey)
}

func (s *Service) EffectForGrant(ctx context.Context, g Grant) (EffectRecord, error) {
	store, ok := s.store.(EffectStore)
	if !ok {
		return EffectRecord{}, fmt.Errorf("durable effect journal is unavailable")
	}
	intent, err := s.store.GetIntent(ctx, g.Binding.ActionID)
	if err != nil {
		return EffectRecord{}, err
	}
	record, err := store.GetEffect(ctx, intent.EffectID)
	if err != nil || record.Fingerprint != intent.EffectFingerprint {
		return EffectRecord{}, fmt.Errorf("journal identity is unavailable or changed")
	}
	return record, nil
}

// RecordedEffect returns metadata only for the same exact, already-dispatched
// proposal. It prevents a repeated RPC from hiding an uncertain effect as an
// ordinary validation failure; it never creates execution permission.
func (s *Service) RecordedEffect(ctx context.Context, id string, call *toolbrokerv1.ToolCall) (EffectRecord, error) {
	g, err := s.store.GetGrant(ctx, id)
	if err != nil || call == nil {
		return EffectRecord{}, fmt.Errorf("recorded execution unavailable")
	}
	hash, err := ArgumentsHash(call.GetArgsJson())
	if err != nil || g.Binding.RunID != call.GetRunId() || g.Binding.ToolCallID != call.GetToolCallId() || g.Binding.ToolName != call.GetToolName() || g.Binding.ArgsHash != hash || g.State == "authorized" || g.State == "revoked" {
		return EffectRecord{}, fmt.Errorf("recorded execution does not match")
	}
	return s.EffectForGrant(ctx, g)
}

// Outcome is called with already-redacted trusted runtime output, not with model
// verdicts. A completed dispatch without supported external evidence stays unknown.
func (s *Service) Outcome(ctx context.Context, g Grant, result *toolbrokerv1.ToolResult, runtimeError bool) (EffectRecord, error) {
	store, ok := s.store.(EffectStore)
	if !ok {
		return EffectRecord{}, fmt.Errorf("durable effect journal is unavailable")
	}
	record, err := s.EffectForGrant(ctx, g)
	if err != nil {
		return record, err
	}
	e := EffectEvidence{EvidenceID: "evidence_" + randomID(), ActionID: record.ActionID, SourceActionID: g.Binding.ActionID, SourceToolCallID: g.Binding.ToolCallID, SourceRunID: g.Binding.RunID, Source: "runtime_response", Method: record.Policy.VerificationMethod, Verdict: StateUnknown, ObservedAt: s.now(), RecordedAt: s.now()}
	outcome := "unknown"
	if !runtimeError && result != nil && result.GetRunId() == g.Binding.RunID && result.GetToolCallId() == g.Binding.ToolCallID && result.GetToolName() == g.Binding.ToolName && result.GetStatus() == "completed" {
		outcome = "dispatch_completed"
		e.ResultDigest = digest(result.GetResultJson())
		switch record.Policy.VerificationMethod {
		case "read_result":
			if g.Binding.Effect == EffectRead {
				e.Verdict = StateConfirmed
			}
		case "local_runtime_ack":
			if g.Binding.Effect == EffectPrepare {
				e.Verdict = StateConfirmed
			}
		case "external_receipt_v1":
			e.Verdict, e.ArtifactDigest = receiptVerdict(record, result.GetResultJson(), false)
		}
	}
	return store.RecordDispatchOutcome(ctx, g.ID, outcome, e)
}

// The explicitly contracted recipient returns this envelope in an HTTP body.
// A generic 2xx/201, arbitrary Location header or model summary is insufficient.
func receiptVerdict(record EffectRecord, raw string, lookup bool) (EffectState, string) {
	if _, err := canonicalJSON(raw); err != nil {
		return StateUnknown, ""
	}
	var response struct {
		StatusCode int    `json:"status_code"`
		Truncated  bool   `json:"truncated"`
		Body       string `json:"body"`
	}
	if len(raw) > 512*1024 || json.Unmarshal([]byte(raw), &response) != nil || response.StatusCode < 200 || response.StatusCode >= 300 || response.Truncated {
		return StateUnknown, ""
	}
	var body struct {
		Receipt struct {
			Key         string `json:"idempotency_key"`
			Fingerprint string `json:"action_fingerprint"`
			State       string `json:"effect_state"`
			Artifact    string `json:"artifact_id"`
		} `json:"butler_effect_receipt"`
	}
	if _, err := canonicalJSON(response.Body); err != nil {
		return StateUnknown, ""
	}
	if json.Unmarshal([]byte(response.Body), &body) != nil || body.Receipt.Key == "" || body.Receipt.Key != record.IdempotencyKey || body.Receipt.Fingerprint != record.Fingerprint {
		return StateUnknown, ""
	}
	if body.Receipt.State == string(StateConfirmed) && body.Receipt.Artifact != "" && len(body.Receipt.Artifact) <= 512 {
		return StateConfirmed, digest(body.Receipt.Artifact)
	}
	if lookup && body.Receipt.State == string(StateAbsent) && body.Receipt.Artifact == "" {
		return StateAbsent, ""
	}
	return StateUnknown, ""
}

// Reconcile consumes an independently dispatched and journaled read result.
// It cannot accept a verifier agent's text or caller-chosen success evidence.
func (s *Service) Reconcile(ctx context.Context, targetID string, observation Grant, result *toolbrokerv1.ToolResult) (EffectRecord, error) {
	store, ok := s.store.(EffectStore)
	if !ok {
		return EffectRecord{}, fmt.Errorf("durable effect journal is unavailable")
	}
	target, err := store.GetEffect(ctx, targetID)
	if err != nil {
		return target, err
	}
	source, err := s.EffectForGrant(ctx, observation)
	if err != nil {
		return target, err
	}
	expected := verificationResource(target)
	if target.Policy.VerificationMethod != "external_receipt_v1" || expected == "" || source.State != StateConfirmed || observation.Binding.Effect != EffectRead || observation.Binding.ToolName != "http.request" || observation.Binding.Resource != expected || source.Binding.OwnerID != target.Binding.OwnerID || source.Binding.TaskID != target.Binding.TaskID || result == nil || result.GetToolCallId() != observation.Binding.ToolCallID || result.GetRunId() != observation.Binding.RunID || result.GetToolName() != observation.Binding.ToolName || result.GetStatus() != "completed" {
		return target, fmt.Errorf("independent authorized recipient observation is required")
	}
	verdict, artifact := receiptVerdict(target, result.GetResultJson(), true)
	if verdict == StateUnknown {
		return target, fmt.Errorf("observation does not prove the external effect")
	}
	var captured EffectEvidence
	for _, ref := range source.EvidenceRefs {
		candidate, e := store.GetEffectEvidence(ctx, ref)
		if e == nil && candidate.SourceActionID == observation.Binding.ActionID && candidate.SourceRunID == observation.Binding.RunID && candidate.SourceToolCallID == observation.Binding.ToolCallID && candidate.ResultDigest == digest(result.GetResultJson()) && candidate.Source == "runtime_response" && candidate.Verdict == StateConfirmed {
			captured = candidate
			break
		}
	}
	if captured.EvidenceID == "" {
		return target, fmt.Errorf("verification output must match independently captured runtime evidence")
	}
	e := EffectEvidence{EvidenceID: "evidence_" + randomID(), ActionID: targetID, SourceActionID: observation.Binding.ActionID, SourceToolCallID: observation.Binding.ToolCallID, SourceRunID: observation.Binding.RunID, Source: "independent_lookup", Method: "external_receipt_v1", Verdict: verdict, ResultDigest: digest(result.GetResultJson()), ArtifactDigest: artifact, ObservedAt: captured.ObservedAt, RecordedAt: s.now()}
	return store.ReconcileEffect(ctx, targetID, source.ActionID, e)
}
