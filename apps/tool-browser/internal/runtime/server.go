package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/butler/butler/internal/browsercontrol"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/security/egress"
)

const maxSnapshotTextLen = 12000

var supportedTools = map[string]bool{
	"browser.read": true, "browser.open": true, "browser.close": true,
	"browser.navigate": true, "browser.snapshot": true, "browser.freeze_snapshot": true, "browser.click": true,
	"browser.fill": true, "browser.type": true, "browser.wait_for": true,
	"browser.prepare_fill": true, "browser.prepare_type": true,
	"browser.submit_get_form": true,
	"browser.follow_link":     true,
	"browser.extract_text":    true, "browser.set_cookie": true, "browser.restore_storage_state": true,
}

type Runner interface {
	Run(context.Context, Request) (Result, error)
}
type Server struct {
	runtimev1.UnimplementedToolRuntimeServiceServer
	runner   Runner
	sessions *sessionStore
	log      *slog.Logger
	// Set at startup when the private owner channel is enabled. Legacy isolated
	// browser composition has no owner epochs and retains its existing contract.
	RequireDispatchEpoch bool
}

// Request is a private execution message. Scope and expiry come from Go,
// not the model. Auth data is only broker-resolved material.
type Request struct {
	ToolName           string                  `json:"tool_name"`
	SessionID          string                  `json:"browser_session_id,omitempty"`
	URL                string                  `json:"url,omitempty"`
	ExpectedURL        string                  `json:"expected_url,omitempty"`
	ObservationVersion uint64                  `json:"observation_version,omitempty"`
	Selector           string                  `json:"selector,omitempty"`
	Fields             map[string]string       `json:"fields,omitempty"`
	GetFormPhase       string                  `json:"get_form_phase,omitempty"` // runner-only protocol
	GetFormNonce       string                  `json:"get_form_nonce,omitempty"`
	Value              string                  `json:"value,omitempty"`
	Text               string                  `json:"text,omitempty"`
	Timeout            int64                   `json:"timeout,omitempty"`
	ScopeOrigin        string                  `json:"scope_origin,omitempty"`
	ExpiresAt          string                  `json:"expires_at,omitempty"`
	ProxyURL           string                  `json:"proxy_url,omitempty"`
	SecretValues       []string                `json:"secret_values,omitempty"`
	AllowedDomains     []string                `json:"allowed_domains,omitempty"`
	OwnerCommand       *browsercontrol.Command `json:"owner_command,omitempty"`
	Cookie             *Cookie                 `json:"cookie,omitempty"`
	StorageState       *StorageState           `json:"storage_state,omitempty"`
}
type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HTTPOnly bool   `json:"http_only,omitempty"`
}
type StorageState struct {
	Cookies []Cookie        `json:"cookies,omitempty"`
	Origins []OriginStorage `json:"origins,omitempty"`
}
type OriginStorage struct {
	Origin       string         `json:"origin"`
	LocalStorage []StorageEntry `json:"local_storage,omitempty"`
}
type StorageEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type ResultLink struct {
	Text string `json:"text"`
	Href string `json:"href"`
}
type ResultError struct {
	ErrorType string `json:"error_type"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}
type Result struct {
	PreparationRejection string                `json:"preparation_rejection,omitempty"`
	InputDispatched      *bool                 `json:"input_dispatched,omitempty"`
	ReadRejection        string                `json:"read_rejection,omitempty"`
	NavigationDispatched *bool                 `json:"navigation_dispatched,omitempty"`
	Forms                []FormDescription     `json:"forms,omitempty"`
	GetFormTarget        string                `json:"get_form_target,omitempty"` // private helper plan; never tool output
	GetFormNonce         string                `json:"get_form_nonce,omitempty"`
	SessionID            string                `json:"browser_session_id,omitempty"`
	ObservationVersion   uint64                `json:"observation_version,omitempty"`
	ScopeOrigin          string                `json:"scope_origin,omitempty"`
	ExpiresAt            string                `json:"expires_at,omitempty"`
	FinalURL             string                `json:"final_url,omitempty"`
	Title                string                `json:"title,omitempty"`
	Text                 string                `json:"text,omitempty"`
	Matched              bool                  `json:"matched"`
	OK                   bool                  `json:"ok"`
	Links                []ResultLink          `json:"links,omitempty"`
	PageStatus           string                `json:"page_status,omitempty"`
	LocalPreparation     bool                  `json:"local_preparation,omitempty"`
	NetworkState         string                `json:"network_state,omitempty"`
	CredentialVerified   *bool                 `json:"credential_verified,omitempty"`
	Error                *ResultError          `json:"error,omitempty"`
	OwnerFrame           *browsercontrol.Frame `json:"owner_frame,omitempty"`
}

type FormDescription struct {
	Selector string                 `json:"selector"`
	Method   string                 `json:"method"`
	Action   string                 `json:"action"`
	Fields   []FormFieldDescription `json:"fields"`
}
type FormFieldDescription struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func NewServer(runner Runner, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{runner: runner, sessions: newSessionStore(), log: logger.WithComponent(log, "tool-browser-runtime")}
}
func (s *Server) Close() error {
	if closer, ok := s.runner.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}
func (s *Server) Execute(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	fail := func(class commonv1.ErrorClass, code, message string) (*runtimev1.ExecuteResponse, error) {
		details := mustJSON(map[string]string{"error_type": code})
		if guardedGETTool(req.GetToolCall().GetToolName()) && preNavigationReason(code) {
			details = preNavigationDetails(code)
		}
		if navigationProofContract(req.GetContract()) && (code == "page_changed" || code == "stale_observation" || code == "invalid_args") {
			details = preNavigationDetails(code)
		}
		if preparationProofContract(req.GetToolCall().GetToolName(), req.GetContract()) && preparationPrecondition(code) {
			details = preparationRejectionDetails(code)
		}
		return &runtimev1.ExecuteResponse{Result: failedResult(req, class, message, details)}, nil
	}
	call, contract := req.GetToolCall(), req.GetContract()
	if call == nil || contract == nil || s.runner == nil {
		return fail(commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "invalid_request", "tool call, contract and browser runner are required")
	}
	args, err := decodeRequest(call.GetToolName(), call.GetArgsJson(), req.GetResolvedCredentials())
	if err != nil {
		return fail(commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "invalid_args", err.Error())
	}
	if !supportedTools[args.ToolName] {
		return fail(commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "unsupported_tool", "unsupported browser tool")
	}
	if contract.GetToolName() != args.ToolName {
		return fail(commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "contract_mismatch", "browser tool contract does not match call")
	}
	if guardedGETTool(args.ToolName) && (contract.GetContractRevision() != "2" || contract.GetEffectContract().GetRetrySafety() != "read_only" || contract.GetEffectContract().GetVerificationMethod() != "guarded_browser_get_v1" || contract.GetSupportsCredentialRefs()) {
		return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "contract_mismatch", "GET navigation requires its guarded revision 2 contract")
	}
	if args.ToolName == "browser.freeze_snapshot" && (contract.GetContractRevision() != "1" || contract.GetEffectContract().GetRetrySafety() != "read_only" || contract.GetEffectContract().GetVerificationMethod() != "read_result" || contract.GetSupportsCredentialRefs()) {
		return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "invalid_contract", "frozen observation requires its exact read contract")
	}
	if (args.ToolName == "browser.read" || args.ToolName == "browser.open" || args.ToolName == "browser.navigate") && contract.GetContractRevision() != "3" && !(args.ToolName == "browser.navigate" && navigationProofContract(contract)) {
		return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "contract_mismatch", "public-origin navigation requires revision 3")
	}
	if (args.ToolName == "browser.prepare_fill" || args.ToolName == "browser.prepare_type") && ((contract.GetContractRevision() != "2" && contract.GetContractRevision() != "3" && contract.GetContractRevision() != "4") || contract.GetEffectContract().GetRetrySafety() != "unsafe" || contract.GetEffectContract().GetVerificationMethod() != "local_browser_preparation_v1") {
		return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "contract_mismatch", "local preparation requires its pinned revision 2 contract")
	}
	if err := validateSelectorRequired(args); err != nil {
		return fail(commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "invalid_selector", err.Error())
	}
	if args.Timeout < 0 || args.Timeout > 30000 {
		return fail(commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "invalid_timeout", "timeout must be between 0 and 30000 milliseconds")
	}
	if args.ToolName == "browser.read" || args.ToolName == "browser.open" || args.ToolName == "browser.navigate" {
		parsed, err := parseWebURL(args.URL)
		if err != nil {
			return fail(commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "invalid_url", err.Error())
		}
		if !domainAllowed(parsed.Hostname(), contract.GetAllowedDomains()) {
			return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "scope_denied", "target domain is not allowed")
		}
		if _, err := (egress.Policy{}).ValidateURL(args.URL); err != nil {
			return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "scope_denied", "target must be a public HTTP(S) destination")
		}
	}
	if args.ToolName != "browser.read" && strings.TrimSpace(call.GetRunId()) == "" {
		return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "owner_missing", "browser session requires a trusted run owner")
	}
	if req.GetContext().GetRunId() != call.GetRunId() {
		return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "owner_mismatch", "execution context does not match run owner")
	}
	var session *browserSession
	if args.ToolName == "browser.open" {
		session, err = s.sessions.open(call.GetRunId(), args.URL)
		if err != nil {
			return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "session_limit", err.Error())
		}
		session.mu.Lock()
		defer session.mu.Unlock()
		args.SessionID, args.ScopeOrigin, args.ExpiresAt = session.id, session.origin, session.expires.UTC().Format(time.RFC3339Nano)
		args.AllowedDomains = contract.GetAllowedDomains()
		session.allowedDomains = append([]string(nil), args.AllowedDomains...)
	} else if args.ToolName != "browser.read" {
		var code string
		session, code = s.sessions.acquire(args.SessionID, call.GetRunId())
		if code != "" {
			if code == "session_lost" && navigationProofContract(contract) {
				return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "Browser context unavailable before navigation", preNavigationDetails(code))}, nil
			}
			// Only this Go guard, before any runner call, may prove loss of this
			// context prevented a NEW local preparation. In-flight helper errors
			// below never receive this marker, even when named session_lost.
			if code == "session_lost" && preparationProofContract(args.ToolName, contract) && contract.GetContractRevision() == "4" {
				return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "Browser context unavailable before local input", preparationRejectionDetails(code))}, nil
			}
			return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, code, "browser session is unavailable: "+code)
		}
		defer session.mu.Unlock()
		if session.controller != "" {
			return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "owner_control", "owner is controlling this browser; wait for explicit return")
		}
		if s.RequireDispatchEpoch {
			epoch, ok := browsercontrol.IncomingDispatchEpoch(ctx)
			if !ok || epoch != session.controlEpoch {
				return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "stale_control_epoch", "browser control changed since admission; request a fresh snapshot")
			}
		}
		if session.closed {
			if navigationProofContract(contract) {
				return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "Browser context unavailable before navigation", preNavigationDetails("session_lost"))}, nil
			}
			if preparationProofContract(args.ToolName, contract) && contract.GetContractRevision() == "4" {
				return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "Browser context unavailable before local input", preparationRejectionDetails("session_lost"))}, nil
			}
			return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "session_lost", "browser session was closed or lost")
		}
		if !time.Now().Before(session.expires) {
			session.closed = true
			_, _ = s.runner.Run(ctx, Request{ToolName: "browser.close", SessionID: session.id})
			return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "session_expired", "browser session expired; explicitly open a new session")
		}
		parsed, _ := url.Parse(session.currentURL)
		if args.ToolName != "browser.close" && (parsed == nil || !domainAllowed(parsed.Hostname(), contract.GetAllowedDomains())) {
			return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "scope_denied", "current domain is not allowed by tool contract")
		}
		if isGuardedAction(args.ToolName) {
			if navigationProofContract(contract) && session.networkFrozen {
				return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "Frozen context refused navigation before dispatch", preNavigationDetails("network_frozen"))}, nil
			}
			if args.ObservationVersion == 0 || args.ObservationVersion != session.version {
				return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "stale_observation", "observe the current page before acting")
			}
			if args.ExpectedURL == "" || args.ExpectedURL != session.currentURL {
				return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "page_changed", "expected_url does not match the observed page")
			}
		}
		args.ScopeOrigin, args.ExpiresAt = originOf(session.currentURL), session.expires.UTC().Format(time.RFC3339Nano)
		if err := validateAuthScope(args, args.ScopeOrigin); err != nil {
			return fail(commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "scope_denied", err.Error())
		}
	}
	if args.ToolName == "browser.read" {
		args.AllowedDomains = contract.GetAllowedDomains()
	}
	runCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	result, err := s.runner.Run(runCtx, args)
	if err != nil {
		s.sessions.invalidateAll()
		return fail(commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, "session_lost", "browser process failed; prior sessions are invalid and any in-flight effect is unknown")
	}
	if session != nil {
		if result.Error == nil && result.NetworkState == "frozen" {
			session.networkFrozen = true
		}
		if result.Error != nil && (result.Error.ErrorType == "session_lost" || result.Error.ErrorType == "session_expired") {
			session.closed = true
			if result.Error.ErrorType == "session_lost" {
				s.sessions.invalidateAll()
			}
		}
		if args.ToolName == "browser.close" {
			session.closed = true
			s.sessions.remove(session.id)
		}
		if result.FinalURL != "" {
			session.currentURL = result.FinalURL
		}
		if result.ObservationVersion > 0 {
			session.version = result.ObservationVersion
		}
		if args.ToolName == "browser.open" && result.Error != nil {
			session.closed = true
			s.sessions.remove(session.id)
		}
		result.SessionID, result.ScopeOrigin, result.ExpiresAt = session.id, originOf(session.currentURL), session.expires.UTC().Format(time.RFC3339Nano)
	}
	result.Text = truncate(result.Text, maxSnapshotTextLen)
	if result.Error != nil {
		if preparationProofContract(args.ToolName, contract) && result.PreparationRejection == "pre_input_v1" && result.InputDispatched != nil && !*result.InputDispatched && preparationPrecondition(result.Error.ErrorType) {
			return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "Local preparation precondition failed", preparationRejectionDetails(result.Error.ErrorType))}, nil
		}
		result.PreparationRejection, result.InputDispatched = "", nil
		if guardedGETTool(args.ToolName) && result.ReadRejection == "pre_navigation_v1" && result.NavigationDispatched != nil && !*result.NavigationDispatched && preNavigationReason(result.Error.ErrorType) {
			return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "GET navigation precondition failed", preNavigationDetails(result.Error.ErrorType))}, nil
		}
		result.ReadRejection, result.NavigationDispatched = "", nil
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, result.Error.Message, buildResultJSON(args.ToolName, result))}, nil
	}
	s.log.Info("browser request executed", slog.String("tool_call_id", call.GetToolCallId()), slog.String("tool_name", call.GetToolName()))
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{ToolCallId: call.GetToolCallId(), RunId: call.GetRunId(), ToolName: call.GetToolName(), Status: "completed", ResultJson: buildResultJSON(args.ToolName, result), FinishedAt: time.Now().UTC().Format(time.RFC3339Nano)}}, nil
}

func preparationProofContract(tool string, c *toolbrokerv1.ToolContract) bool {
	return (tool == "browser.prepare_fill" || tool == "browser.prepare_type") && c.GetToolName() == tool && (c.GetContractRevision() == "3" || c.GetContractRevision() == "4") && c.GetEffectContract().GetRetrySafety() == "unsafe" && c.GetEffectContract().GetVerificationMethod() == "local_browser_preparation_v1"
}

func navigationProofContract(c *toolbrokerv1.ToolContract) bool {
	return c.GetToolName() == "browser.navigate" && c.GetContractRevision() == "4" && !c.GetSupportsCredentialRefs() && c.GetEffectContract().GetRetrySafety() == "read_only" && c.GetEffectContract().GetVerificationMethod() == "guarded_browser_get_v1"
}
func preparationPrecondition(reason string) bool {
	switch reason {
	case "page_changed", "stale_observation", "invalid_args", "invalid_selector":
		return true
	}
	return false
}
func preparationRejectionDetails(reason string) string {
	return mustJSON(map[string]any{"preparation_rejection": "pre_input_v1", "input_dispatched": false, "reason": reason})
}

func guardedGETTool(tool string) bool {
	return tool == "browser.submit_get_form" || tool == "browser.follow_link"
}
func preNavigationReason(reason string) bool {
	switch reason {
	case "page_changed", "stale_observation", "get_form_denied", "invalid_args", "invalid_selector":
		return true
	}
	return false
}
func preNavigationDetails(reason string) string {
	return mustJSON(map[string]any{"read_rejection": "pre_navigation_v1", "navigation_dispatched": false, "reason": reason})
}
func parseWebURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return nil, fmt.Errorf("url must be an absolute HTTP(S) URL without embedded credentials")
	}
	return u, nil
}
func originOf(raw string) string {
	u, err := parseWebURL(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Host)
	if u.Scheme == "https" && u.Port() == "443" {
		host = strings.TrimSuffix(host, ":443")
	}
	if u.Scheme == "http" && u.Port() == "80" {
		host = strings.TrimSuffix(host, ":80")
	}
	return strings.ToLower(u.Scheme) + "://" + host
}
func isGuardedAction(tool string) bool {
	return tool != "browser.read" && tool != "browser.open" && tool != "browser.snapshot" && tool != "browser.freeze_snapshot" && tool != "browser.extract_text" && tool != "browser.close"
}
func validateSelectorRequired(args Request) error {
	switch args.ToolName {
	case "browser.click", "browser.fill", "browser.type", "browser.prepare_fill", "browser.prepare_type", "browser.wait_for", "browser.extract_text", "browser.submit_get_form", "browser.follow_link":
		if strings.TrimSpace(args.Selector) == "" {
			return fmt.Errorf("selector is required")
		}
	}
	return nil
}
func buildResultJSON(_ string, result Result) string {
	result.OwnerFrame = nil
	result.GetFormTarget, result.GetFormNonce = "", ""
	return mustJSON(result)
}
func failedResult(req *runtimev1.ExecuteRequest, class commonv1.ErrorClass, message, detailsJSON string) *toolbrokerv1.ToolResult {
	result := &toolbrokerv1.ToolResult{Status: "failed", FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), Error: &toolbrokerv1.ToolError{ErrorClass: class, Message: message, Retryable: false, DetailsJson: detailsJSON}}
	if call := req.GetToolCall(); call != nil {
		result.ToolCallId, result.RunId, result.ToolName = call.GetToolCallId(), call.GetRunId(), call.GetToolName()
	}
	return result
}
func domainAllowed(host string, allowed []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate != "" && (candidate == "*" || host == candidate || strings.HasSuffix(host, "."+candidate)) {
			return true
		}
	}
	return false
}
func truncate(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}
func mustJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
