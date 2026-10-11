package broker

import (
	"context"

	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/singletabcontrol"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
)

// GuardedTabAdmission separates owning local selection from browser execution.
// Every other protected namespace remains denied by the exact single-tab store.
type GuardedTabAdmission struct{ Store *resourceexec.Store }

func (a GuardedTabAdmission) PrepareContext(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, grant actionauth.Grant) (context.Context, error) {
	if call.GetToolName() == singletabcontrol.BindTool {
		if !proto.Equal(contract, singletabcontrol.RequestContract()) || !exactLocalGuardedGrant(grant) {
			return ctx, resourceexec.ErrDenied
		}
		return ctx, nil
	}
	if a.Store == nil {
		return ctx, resourceexec.ErrDenied
	}
	return a.Store.PrepareContext(ctx, call, contract, grant)
}
func (a GuardedTabAdmission) CheckAdmissionTx(ctx context.Context, tx pgx.Tx, g actionauth.Grant) error {
	if g.Binding.ToolName == singletabcontrol.BindTool {
		if !exactLocalGuardedGrant(g) {
			return resourceexec.ErrDenied
		}
		return nil
	}
	if a.Store == nil {
		return resourceexec.ErrDenied
	}
	return a.Store.CheckAdmissionTx(ctx, tx, g)
}
func (a GuardedTabAdmission) ClaimActionTx(ctx context.Context, tx pgx.Tx, g actionauth.Grant, effect string) error {
	if g.Binding.ToolName == singletabcontrol.BindTool {
		if !exactLocalGuardedGrant(g) {
			return resourceexec.ErrDenied
		}
		return nil
	}
	if a.Store == nil {
		return resourceexec.ErrDenied
	}
	return a.Store.ClaimActionTx(ctx, tx, g, effect)
}

func exactLocalGuardedGrant(g actionauth.Grant) bool {
	h, e := actionauth.ContractHash(singletabcontrol.RequestContract())
	return e == nil && g.Binding.ToolName == singletabcontrol.BindTool && g.Binding.ContractHash == h
}
