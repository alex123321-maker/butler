package broker

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentialcontrol"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/protobuf/proto"
)

// CredentialExecutor is a Broker-owned local capability, behind ordinary live
// action authorization and irreversible dispatch. Model arguments carry no owner.
type CredentialExecutor struct {
	Vault    *credentials.Vault
	Contexts actionauth.ContextReader
	Next     RuntimeExecutor
}

func (e *CredentialExecutor) Execute(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	if call == nil {
		return nil, credentials.ErrVault
	}
	if !credentialcontrol.IsTool(call.ToolName) {
		if e.Next == nil {
			return nil, credentials.ErrVault
		}
		return e.Next.Execute(ctx, call, contract, secrets)
	}
	if e.Vault == nil || e.Contexts == nil || len(secrets) != 0 || len(call.CredentialRefs) != 0 || !proto.Equal(contract, credentialcontrol.Contract(call.ToolName)) {
		return nil, credentials.ErrVault
	}
	identity := actionauth.DispatchIdentityFromContext(ctx)
	if identity.ActionID == "" || identity.Fingerprint == "" {
		return nil, credentials.ErrVault
	}
	a, err := credentialcontrol.DecodeToolArgs(call.ToolName, call.ArgsJson)
	if err != nil {
		return nil, err
	}
	c, err := e.Contexts.GetActionContext(ctx, call.RunId)
	if err != nil || c.OwnerID == "" || c.RunID != call.RunId || c.Delegation != nil {
		return nil, credentials.ErrVault
	}
	p := credentials.Principal{OwnerID: c.OwnerID, SessionKey: c.SessionKey, RunID: c.RunID, TaskID: c.TaskID, TaskRevision: c.TaskRevision, ConstraintsRevision: c.ConstraintsRevision}
	var payload any
	if call.ToolName == "credentials.search" {
		entries, err := e.Vault.List(ctx, c.OwnerID, a.Query)
		if err != nil {
			return nil, err
		}
		payload = map[string]any{"entries": entries}
	} else if call.ToolName == "credentials.request_status" {
		r, err := e.Vault.RequestStatus(ctx, p, a.RequestID)
		if err != nil {
			return nil, err
		}
		payload = map[string]any{"request": r}
	} else {
		// Browser state is attested by the Broker's session store, not model URL prose.
		var expected string
		if e.Vault.Pool.QueryRow(ctx, `SELECT observed_url FROM server_browser_sessions WHERE browser_session_id=$1 AND run_id=$2 AND owner_id=$3 AND mode='agent' AND expires_at>now() AND NOT fresh_needed`, a.BrowserSessionID, c.RunID, c.OwnerID).Scan(&expected) != nil || expected != a.ExpectedURL {
			return nil, credentials.ErrVault
		}
		kind := strings.TrimPrefix(call.ToolName, "credentials.request_")
		fields := a.Fields
		if kind == "otp" {
			fields = []string{"otp"}
		}
		var r credentials.AccessRequest
		if kind == "login" {
			r, err = e.Vault.CreateLoginRequest(ctx, p, call.ToolCallId, a.RecordID, a.Purpose, *a.LoginScope)
		} else {
			r, err = e.Vault.CreateRequest(ctx, p, call.ToolCallId, kind, a.RecordID, a.ExpectedURL, a.BrowserSessionID, a.Purpose, fields, a.AccessRequestID)
		}
		if err != nil {
			return nil, err
		}
		payload = map[string]any{"request": r}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, credentials.ErrVault
	}
	return &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, Status: "completed", ResultJson: string(raw)}, nil
}
