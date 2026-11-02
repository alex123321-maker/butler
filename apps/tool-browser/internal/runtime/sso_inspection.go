package runtime

import (
	"context"
	"github.com/butler/butler/internal/browsersso"
	"github.com/butler/butler/internal/security/egress"
	"time"
)

type inspectionRequest struct {
	Scope    browsersso.Inspection `json:"scope"`
	Dispatch browsersso.Dispatch   `json:"dispatch"`
}

func (r *PlaywrightRunner) inspectSSO(ctx context.Context, req Request) (Result, error) {
	proxy := r.proxies[req.SessionID]
	if proxy == nil || req.SSOInspection == nil || !req.SSOInspection.Dispatch.ValidInspection(req.SSOInspection.Scope) {
		return Result{}, egress.ErrDenied
	}
	for _, target := range req.SSOInspection.Scope.Targets() {
		if _, err := r.policy.ValidateURL(target); err != nil {
			return Result{}, err
		}
	}
	proxy.Freeze()
	defer proxy.Freeze()
	deadline := req.SSOInspection.Dispatch.ExpiresAt
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if !deadline.After(time.Now()) {
		return Result{}, egress.ErrDenied
	}
	if err := proxy.AllowUntil(deadline); err != nil {
		return Result{}, err
	}
	return r.exchange(ctx, req)
}
