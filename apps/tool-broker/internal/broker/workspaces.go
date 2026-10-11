package broker

import (
	"context"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/workspaceexec"
	"github.com/jackc/pgx/v5"
)

// ResourceAdmissions keeps each protected namespace on its exact admission
// path. Missing workspace/browser composition denies that namespace, while
// ordinary catalog operations retain their existing policy.
type ResourceAdmissions struct {
	Workspace *workspaceexec.DispatchStore
	Browser   *GuardedTabAdmission
}

func (a ResourceAdmissions) PrepareContext(ctx context.Context, call *toolbrokerv1.ToolCall, c *toolbrokerv1.ToolContract, g actionauth.Grant) (context.Context, error) {
	if workspaceexec.Protected(call.GetToolName()) {
		if a.Workspace == nil {
			return ctx, workspaceexec.ErrDenied
		}
		return a.Workspace.PrepareContext(ctx, call, c, g)
	}
	if resourceexec.Protected(call.GetToolName()) {
		if a.Browser == nil {
			return ctx, resourceexec.ErrDenied
		}
		return a.Browser.PrepareContext(ctx, call, c, g)
	}
	return ctx, nil
}
func (a ResourceAdmissions) CheckAdmissionTx(ctx context.Context, tx pgx.Tx, g actionauth.Grant) error {
	if workspaceexec.Protected(g.Binding.ToolName) {
		if a.Workspace == nil {
			return workspaceexec.ErrDenied
		}
		return a.Workspace.CheckAdmissionTx(ctx, tx, g)
	}
	if resourceexec.Protected(g.Binding.ToolName) {
		if a.Browser == nil {
			return resourceexec.ErrDenied
		}
		return a.Browser.CheckAdmissionTx(ctx, tx, g)
	}
	return nil
}
func (a ResourceAdmissions) ClaimActionTx(ctx context.Context, tx pgx.Tx, g actionauth.Grant, effect string) error {
	if workspaceexec.Protected(g.Binding.ToolName) {
		if a.Workspace == nil {
			return workspaceexec.ErrDenied
		}
		return a.Workspace.ClaimActionTx(ctx, tx, g, effect)
	}
	if resourceexec.Protected(g.Binding.ToolName) {
		if a.Browser == nil {
			return resourceexec.ErrDenied
		}
		return a.Browser.ClaimActionTx(ctx, tx, g, effect)
	}
	return nil
}

type workspaceExecutor struct {
	store *workspaceexec.DispatchStore
	next  RuntimeExecutor
}

func NewWorkspaceExecutor(store *workspaceexec.DispatchStore, next RuntimeExecutor) (RuntimeExecutor, error) {
	if store == nil || next == nil {
		return nil, workspaceexec.ErrDenied
	}
	return &workspaceExecutor{store, next}, nil
}
func (e *workspaceExecutor) Execute(ctx context.Context, call *toolbrokerv1.ToolCall, c *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	if workspaceexec.Protected(call.GetToolName()) {
		if len(secrets) != 0 || len(call.GetCredentialRefs()) != 0 {
			return nil, workspaceexec.ErrDenied
		}
		identity := actionauth.DispatchIdentityFromContext(ctx)
		ticket, err := e.store.TicketForAction(ctx, call, c, identity.ActionID)
		if err != nil {
			return nil, err
		}
		ctx = workspaceexec.WithTicket(ctx, ticket)
	}
	return e.next.Execute(ctx, call, c, secrets)
}
