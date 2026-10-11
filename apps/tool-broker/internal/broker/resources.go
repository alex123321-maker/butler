package broker

import (
	"context"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/resourceexec"
)

type ResourceAdmission interface {
	PrepareContext(context.Context, *toolbrokerv1.ToolCall, *toolbrokerv1.ToolContract, actionauth.Grant) (context.Context, error)
}

func (s *Server) SetResourceAdmission(a ResourceAdmission) { s.resourceAdmission = a }

// NewResourceExecutor is optional trusted composition around the actual router.
// It cannot claim an action or obtain credentials. A missing committed mapping
// denies protected tools, including incomplete startup composition.
func NewResourceExecutor(store *resourceexec.Store, next RuntimeExecutor) (RuntimeExecutor, error) {
	if store == nil || next == nil {
		return nil, resourceexec.ErrDenied
	}
	return &resourceExecutor{store, next}, nil
}

type resourceExecutor struct {
	store *resourceexec.Store
	next  RuntimeExecutor
}

func (e *resourceExecutor) Execute(ctx context.Context, c *toolbrokerv1.ToolCall, k *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	if resourceexec.Protected(c.GetToolName()) {
		identity := actionauth.DispatchIdentityFromContext(ctx)
		ticket, err := e.store.TicketForAction(ctx, c, k, identity.ActionID)
		if err != nil {
			return nil, err
		}
		ctx = resourceexec.WithTicket(ctx, ticket)
	}
	return e.next.Execute(ctx, c, k, secrets)
}
