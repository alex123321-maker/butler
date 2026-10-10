// Package runtime interprets reviewed HTTP-JSON contracts. It has no package
// loader, process execution, filesystem tool, secret store or orchestration.
package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/butler/butler/internal/extensions"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress"
	"github.com/butler/butler/internal/security/redact"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type Server struct {
	runtimev1.UnimplementedToolRuntimeServiceServer
	profiles map[string]bool
	policy   egress.Policy
}

func NewServer(profiles map[string]extensions.Profile) *Server {
	return NewServerWithEgress(profiles, egress.Policy{})
}

// The resolver/dial seam is construction-only for public-origin fault fixtures.
// Production never replaces the public/private network policy.
func NewServerWithEgress(profiles map[string]extensions.Profile, policy egress.Policy) *Server {
	return &Server{profiles: extensions.ProfilePins(profiles), policy: policy}
}

type credentialArgument struct {
	Type  string `json:"type"`
	Alias string `json:"alias"`
	Field string `json:"field"`
}
type arguments struct {
	URL           string              `json:"url"`
	Body          json.RawMessage     `json:"body"`
	Authorization *credentialArgument `json:"authorization"`
}

func failure(call *toolbrokerv1.ToolCall, message string) *runtimev1.ExecuteResponse {
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{ToolCallId: call.GetToolCallId(), RunId: call.GetRunId(), ToolName: call.GetToolName(), Status: "failed", FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), ResultJson: "{}", Error: &toolbrokerv1.ToolError{ErrorClass: commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, Message: message, Retryable: false}}}
}
func (s *Server) Execute(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	call, contract := req.GetToolCall(), req.GetContract()
	e := contract.GetExtension()
	meta := req.GetContext()
	if call == nil || contract == nil || extensions.ValidateProtocol(e, s.profiles) != nil || call.GetToolName() != contract.GetToolName() || !strings.HasPrefix(call.GetToolName(), "extension."+e.GetExtensionId()+".") || meta.GetActionId() == "" || meta.GetActionFingerprint() == "" || meta.GetRunId() != call.GetRunId() || meta.GetToolCallId() != call.GetToolCallId() || meta.GetToolName() != call.GetToolName() || meta.GetTimeoutMs() <= 0 {
		return failure(call, "reviewed runtime authorization metadata required"), nil
	}
	var args arguments
	if extensions.DecodeArguments(call.GetArgsJson(), &args) != nil || !extensions.ProtocolResourceAllowed(e, args.URL) {
		return failure(call, "extension arguments or resource denied"), nil
	}
	if e.GetMethod() == "GET" || e.GetMethod() == "HEAD" {
		if args.Body != nil {
			return failure(call, "read operations cannot send a body"), nil
		}
	}
	secrets := []string{}
	authorization := ""
	if args.Authorization != nil {
		ref := args.Authorization
		allowed := false
		for _, input := range e.GetCredentials() {
			if input.GetInput() == "authorization" && input.GetField() == ref.Field {
				allowed = true
			}
		}
		if !allowed || ref.Type != "credential_ref" || ref.Alias == "" || len(req.GetResolvedCredentials()) != 1 || len(call.GetCredentialRefs()) != 1 {
			return failure(call, "undeclared credential sink denied"), nil
		}
		provided := call.GetCredentialRefs()[0]
		resolved := req.GetResolvedCredentials()[0]
		if provided.GetType() != "credential_ref" || provided.GetAlias() != ref.Alias || provided.GetField() != ref.Field || resolved.GetAlias() != ref.Alias || resolved.GetField() != ref.Field || resolved.GetValue() == "" || strings.ContainsAny(resolved.GetValue(), "\r\n\x00") {
			return failure(call, "deferred credential binding denied"), nil
		}
		authorization = "Bearer " + resolved.GetValue()
		secrets = append(secrets, resolved.GetValue())
	} else {
		if len(req.GetResolvedCredentials()) != 0 || len(call.GetCredentialRefs()) != 0 {
			return failure(call, "unsolicited credential material denied"), nil
		}
		for _, input := range e.GetCredentials() {
			if input.GetRequired() {
				return failure(call, "mandatory credential reference missing"), nil
			}
		}
	}
	timeout := time.Duration(e.GetTimeoutMs()) * time.Millisecond
	if private := time.Duration(meta.GetTimeoutMs()) * time.Millisecond; private < timeout {
		timeout = private
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ctx = egress.WithCounter(ctx, egress.NewCounter(e.GetEgressBytes()))
	request, err := http.NewRequestWithContext(ctx, e.GetMethod(), args.URL, bytes.NewReader(args.Body))
	if err != nil {
		return failure(call, "extension request invalid"), nil
	}
	request.Header.Set("Accept", "application/json")
	if args.Body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if receipt := e.GetReceipt(); receipt != nil {
		if !proto.Equal(receipt, contract.GetEffectContract()) || meta.GetIdempotencyKey() == "" || strings.ContainsAny(meta.GetIdempotencyKey()+meta.GetActionFingerprint(), "\r\n\x00") || !strings.HasPrefix(args.URL, receipt.GetScopeOrigin()+"/") {
			return failure(call, "reviewed recipient identity required"), nil
		}
		request.Header.Set(receipt.GetIdempotencyHeader(), meta.GetIdempotencyKey())
		request.Header.Set("Butler-Action-Fingerprint", meta.GetActionFingerprint())
	} else if meta.GetIdempotencyKey() != "" || contract.GetEffectContract().GetRetrySafety() == "external_idempotency" {
		return failure(call, "unreviewed recipient guarantee denied"), nil
	}
	if egress.CheckRequestBytes(request, e.GetEgressBytes()) != nil {
		return failure(call, "extension egress allowance exceeded"), nil
	}
	response, err := s.client().Do(request)
	if err != nil {
		return failure(call, "extension request failed or destination denied"), nil
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, e.GetOutputBytes()+1))
	if err != nil || int64(len(body)) > e.GetOutputBytes() {
		return failure(call, "extension response allowance exceeded"), nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !json.Valid(body) {
		return failure(call, "extension returned an invalid HTTP-JSON result"), nil
	}
	encoded, _ := json.Marshal(map[string]any{"status_code": response.StatusCode, "body": redact.Values(string(body), secrets...)})
	if int64(len(encoded)) > e.GetOutputBytes() {
		return failure(call, "extension response envelope allowance exceeded"), nil
	}
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{ToolCallId: call.GetToolCallId(), RunId: call.GetRunId(), ToolName: call.GetToolName(), Status: "completed", ResultJson: string(encoded), FinishedAt: time.Now().UTC().Format(time.RFC3339Nano)}}, nil
}
func (s *Server) client() *http.Client {
	return &http.Client{Transport: egress.NewTransport(s.policy), CheckRedirect: func(*http.Request, []*http.Request) error { return egress.ErrDenied }}
}
func (s *Server) ProbeExtension(ctx context.Context, req *runtimev1.ProbeExtensionRequest) (*runtimev1.ProbeExtensionResponse, error) {
	e := req.GetExtension()
	if req.GetProbeId() == "" || extensions.ValidateProtocol(e, s.profiles) != nil || e.GetMethod() != "GET" || e.GetEffect() != "read" || len(e.GetCredentials()) != 0 || !extensions.ProtocolResourceAllowed(e, req.GetUrl()) {
		return nil, status.Error(codes.PermissionDenied, "reviewed fixed readiness probe required")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(e.GetTimeoutMs())*time.Millisecond)
	defer cancel()
	ctx = egress.WithCounter(ctx, egress.NewCounter(e.GetEgressBytes()))
	request, err := http.NewRequestWithContext(ctx, "GET", req.GetUrl(), nil)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid readiness URL")
	}
	request.Header.Set("Accept", "application/json")
	if egress.CheckRequestBytes(request, e.GetEgressBytes()) != nil {
		return nil, status.Error(codes.ResourceExhausted, "readiness byte allowance exceeded")
	}
	response, err := s.client().Do(request)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "readiness failed or destination denied")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, e.GetOutputBytes()+1))
	if err != nil || int64(len(body)) > e.GetOutputBytes() {
		return nil, status.Error(codes.ResourceExhausted, "readiness response allowance exceeded")
	}
	var result struct {
		Ready bool `json:"ready"`
	}
	ready := response.StatusCode >= 200 && response.StatusCode < 300 && json.Unmarshal(body, &result) == nil && result.Ready
	sum := sha256.Sum256(body)
	return &runtimev1.ProbeExtensionResponse{Ready: ready, ResponseDigest: hex.EncodeToString(sum[:])}, nil
}
