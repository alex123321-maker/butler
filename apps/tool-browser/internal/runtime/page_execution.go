package runtime

import (
	"context"
	"log/slog"
	"time"

	"github.com/butler/butler/internal/browsercontrol"
	"github.com/butler/butler/internal/browserpage"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress"
)

func (s *Server) executePage(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	stage := "contract"
	fail := func() (*runtimev1.ExecuteResponse, error) {
		s.log.Info("managed browser request rejected", slog.String("stage", stage))
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, browserpage.ErrDenied.Error(), `{}`)}, nil
	}
	call := req.GetToolCall()
	a, e := browserpage.Parse(call.ToolName, call.ArgsJson)
	if e != nil || !browserpage.ValidContract(req.Contract) || len(req.ResolvedCredentials) != 0 || len(call.CredentialRefs) != 0 || req.GetContext().GetRunId() != call.RunId || call.RunId == "" {
		return fail()
	}
	var session *browserSession
	stage = "session"
	if call.ToolName == browserpage.Open {
		u, e := parseWebURL(a.URL)
		if e != nil || !domainAllowed(u.Hostname(), req.Contract.AllowedDomains) {
			return fail()
		}
		if _, e = (egress.Policy{}).ValidateURL(a.URL); e != nil {
			return fail()
		}
		session, e = s.sessions.open(call.RunId, a.URL)
		if e != nil {
			return fail()
		}
		session.mu.Lock()
		session.managedPages = true
		session.pages = map[string]browserpage.Report{}
		session.allowedDomains = append([]string(nil), req.Contract.AllowedDomains...)
	} else {
		var code string
		session, code = s.sessions.acquire(a.SessionID, call.RunId)
		if code != "" {
			return fail()
		}
	}
	defer session.mu.Unlock()
	if !session.managedPages || session.closed || session.controller != "" || !session.expires.After(time.Now()) {
		return fail()
	}
	if call.ToolName != browserpage.Open {
		stage = "control_epoch"
		epoch, ok := browsercontrol.IncomingDispatchEpoch(ctx)
		if !ok || epoch != session.controlEpoch {
			return fail()
		}
	}
	request := Request{ToolName: browserpage.Legacy(call.ToolName), SessionID: session.id, ManagedPages: true, URL: a.URL, Selector: a.Selector, Value: a.Value, Text: a.Text, ScopeOrigin: session.origin, ExpiresAt: session.expires.UTC().Format(time.RFC3339Nano), AllowedDomains: session.allowedDomains}
	if browserpage.Bound(call.ToolName) {
		stage = "page_binding"
		bound, ok := browsercontrol.IncomingPageDispatch(ctx)
		page, known := session.pages[a.Page.ID]
		if !ok || !known || bound.Ref != *a.Page || bound.Epoch != session.controlEpoch || bound.DocumentID != page.DocumentID || bound.RuntimeVersion != page.RuntimeVersion || bound.URL != page.URL || page.State != "ready" || originOf(bound.URL) != session.origin {
			return fail()
		}
		request.PageID, request.DocumentID, request.ObservationVersion, request.ExpectedURL = page.ID, page.DocumentID, uint64(page.RuntimeVersion), page.URL
	}
	runCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	result, e := s.runner.Run(runCtx, request)
	stage = "runner"
	if e != nil {
		s.sessions.invalidateAll()
		return fail()
	}
	if result.Error != nil || !result.OK {
		stage = "helper"
		if result.Error != nil {
			switch result.Error.ErrorType {
			case "stale_page", "page_lost", "session_lost", "timeout", "operation_failed":
				stage = "helper_" + result.Error.ErrorType
			}
		}
		if call.ToolName == browserpage.Open {
			session.closed = true
			s.sessions.remove(session.id)
		}
		return fail()
	}
	if len(result.PrivatePages) == 0 || len(result.PrivatePages) > browsercontrol.MaxLifetimePages {
		stage = "page_report_count"
		return fail()
	}
	pages := map[string]browserpage.Report{}
	stage = "page_report"
	for _, page := range result.PrivatePages {
		if !browserpage.ID(page.ID) || pages[page.ID].ID != "" || page.OpenerID != "" && !browserpage.ID(page.OpenerID) {
			stage = "report_identity"
			return fail()
		}
		if page.State == "ready" && !browserpage.ID(page.DocumentID) {
			stage = "report_document"
			return fail()
		}
		if page.State == "ready" && page.RuntimeVersion <= 0 {
			stage = "report_version"
			return fail()
		}
		if page.State == "ready" && originOf(page.URL) != session.origin {
			stage = "report_origin"
			return fail()
		}
		if page.State != "ready" && page.State != "pending" && page.State != "closed" && page.State != "lost" {
			stage = "report_state"
			return fail()
		}
		pages[page.ID] = page
	}
	if browserpage.Bound(call.ToolName) && call.ToolName != browserpage.Close && result.SelectedPageID != a.Page.ID {
		return fail()
	}
	session.pages = pages
	result.SessionID, result.ScopeOrigin, result.ExpiresAt = session.id, session.origin, session.expires.UTC().Format(time.RFC3339Nano)
	result.Text = truncate(result.Text, maxSnapshotTextLen)
	// No raw helper error or private authentication field is forwarded. Broker
	// must mediate private_pages before this result reaches any model transcript.
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, Status: "completed", ResultJson: buildResultJSON(call.ToolName, result), FinishedAt: time.Now().UTC().Format(time.RFC3339Nano)}}, nil
}
