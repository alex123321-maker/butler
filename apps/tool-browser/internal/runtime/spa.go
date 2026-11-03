package runtime

import (
	"context"
	"time"

	"github.com/butler/butler/internal/browserspa"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/security/spaproxy"
)

type spaRequest struct {
	Args  browserspa.Args `json:"args"`
	Phase string          `json:"phase"`
	SPKI  string          `json:"spki"`
	Nonce string          `json:"nonce,omitempty"`
}

func (s *Server) executeSPA(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	fail := func(code string) (*runtimev1.ExecuteResponse, error) {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "bounded SPA query unavailable or denied", mustJSON(map[string]string{"error_type": code}))}, nil
	}
	call := req.GetToolCall()
	a, err := browserspa.Parse(call.GetArgsJson())
	_, live := resourceexec.LiveCheck(ctx)
	if !s.SPAEnabled || !live || err != nil || !browserspa.ValidContract(req.GetContract()) || len(req.GetResolvedCredentials()) != 0 || len(call.GetCredentialRefs()) != 0 || call.GetRunId() == "" || call.GetRunId() != req.GetContext().GetRunId() {
		return fail("spa_profile_denied")
	}
	result, err := s.runner.Run(ctx, Request{ToolName: browserspa.Tool, URL: a.URL, SPA: &spaRequest{Args: a}})
	if err != nil {
		s.sessions.invalidateAll()
		return fail("spa_runtime_lost")
	}
	if !result.OK || result.Error != nil {
		code := "spa_query_failed"
		if result.Error != nil && browserspa.ValidDiagnostic(result.Error.ErrorType) {
			code = result.Error.ErrorType
		}
		return fail(code)
	}
	if result.SPAProfile != browserspa.Profile || result.NetworkState != "frozen" || result.SessionID != "" || !browserspa.NetworkURL(a.URL, result.FinalURL) {
		return fail("spa_proof_invalid")
	}
	sanitizeFormMetadata(&result, true)
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{ToolCallId: call.GetToolCallId(), RunId: call.GetRunId(), ToolName: call.GetToolName(), Status: "completed", ResultJson: buildResultJSON(call.GetToolName(), result), FinishedAt: time.Now().UTC().Format(time.RFC3339Nano)}}, nil
}

func (r *PlaywrightRunner) readSPA(ctx context.Context, req Request) (Result, error) {
	live, ok := resourceexec.LiveCheck(ctx)
	if !ok || req.SPA == nil || req.SPA.Args.Validate() != nil {
		return Result{}, browserspa.ErrDenied
	}
	owned, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	deadline, _ := owned.Deadline()
	p, err := spaproxy.New(owned, spaproxy.Options{URL: req.URL, Deadline: deadline, Policy: r.policy, Check: live, Allows: func(target string) bool { return browserspa.NetworkURL(req.URL, target) }})
	if err != nil {
		return Result{Error: &ResultError{ErrorType: "spa_profile_denied", Message: "bounded SPA relay or current authority unavailable"}}, nil
	}
	defer p.Close()
	if r.cmd == nil {
		if err = r.start(owned); err != nil {
			return Result{}, err
		}
	}
	req.ProxyURL = p.URL()
	req.ExpiresAt = deadline.UTC().Format(time.RFC3339Nano)
	req.SPA.Phase, req.SPA.SPKI = "run", p.SPKI
	prepared, err := r.exchange(owned, req)
	if err != nil || !prepared.OK {
		if err == nil {
			_, reason := p.Snapshot()
			if reason != "" {
				return Result{Error: &ResultError{ErrorType: browserspa.Diagnostic(reason), Message: "bounded SPA network request denied"}}, nil
			}
		}
		return prepared, err
	}
	p.Close() // terminal; no later call or helper phase can thaw this relay.
	stats, failure := p.Snapshot()
	req.SPA.Phase, req.SPA.Nonce = "observe", prepared.SPANonce
	result, err := r.exchange(owned, req) // also destroys the private browser and worker.
	if err != nil {
		return Result{}, err
	}
	if failure != "" {
		return Result{Error: &ResultError{ErrorType: browserspa.Diagnostic(failure), Message: "SPA network profile refused the request"}}, nil
	}
	if stats.PendingQueries != 0 || stats.Queries != stats.CompletedQueries {
		return Result{Error: &ResultError{ErrorType: "spa_query_unsettled", Message: "SPA query response did not settle"}}, nil
	}
	if !result.OK || live(owned, result.FinalURL) != nil {
		return Result{Error: &ResultError{ErrorType: "spa_authority_denied", Message: "current SPA observation authority required"}}, nil
	}
	result.SPAProfile, result.SPANetwork, result.NetworkState = browserspa.Profile, &stats, "frozen"
	result.SPANonce = ""
	return result, nil
}
