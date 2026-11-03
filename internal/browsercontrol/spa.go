package browsercontrol

import (
	"context"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browserspa"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func (s *Store) recordSPA(ctx context.Context, call *toolbrokerv1.ToolCall, grant actionauth.Grant, result *toolbrokerv1.ToolResult) error {
	b := grant.Binding
	deny := func() error { result.ResultJson = "{}"; return ErrDenied }
	a, err := browserspa.Parse(call.ArgsJson)
	hash, he := actionauth.ArgumentsHash(call.ArgsJson)
	if err != nil || he != nil || hash != b.ArgsHash || b.Resource != a.URL || b.ToolName != browserspa.Tool || b.Effect != actionauth.EffectRead || len(b.CredentialRefs) != 0 || s.Contexts == nil {
		return deny()
	}
	actual, err := browserspa.ReadResultURL(result.GetResultJson(), a)
	if err != nil {
		return deny()
	}
	c, err := s.Contexts.GetActionContext(ctx, call.RunId)
	if err != nil || c.Authority == nil || c.Delegation != nil || c.OwnerID != b.OwnerID || c.TaskID != b.TaskID || grant.InputClaimHash == "" || c.InputClaimHash != grant.InputClaimHash {
		return deny()
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return deny()
	}
	defer tx.Rollback(ctx)
	var at time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at) != nil || actionauth.CheckTaskActionTx(ctx, tx, b, at) != nil {
		return deny()
	}
	projected := b
	projected.Resource = actual
	if actionauth.CheckTaskActionTx(ctx, tx, projected, at) != nil || actionauth.CheckDenyOverrides(projected, c.DenyTools, c.DenyResources) != nil || actionauth.CheckOwningInputTx(ctx, tx, grant) != nil || tx.Commit(ctx) != nil {
		return deny()
	}
	return nil
}
