package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/apps/tool-broker/internal/runtimeclient"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browserspa"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/resources"
	"github.com/jackc/pgx/v5/pgxpool"
)

func configureServerSPA(ctx context.Context, cfg config.ToolBrokerConfig, pool *pgxpool.Pool, r *registry.Registry, router *runtimeclient.Router, budgets *budget.Controller, sources resourceexec.SourceGuard) (*resourceexec.Store, http.Handler, error) {
	if !cfg.BrowserSPAEnabled {
		return nil, nil, nil
	}
	if pool == nil || cfg.BrowserControlRuntimeURL == "" || cfg.GuardedTabRuntimeEnabled || cfg.BrowserControlTokenFile == "" || r == nil || router == nil || budgets == nil || sources == nil {
		return nil, nil, fmt.Errorf("SPA queries require private server-browser, resource and current authority composition")
	}
	token, err := actionauth.ReadServiceToken(cfg.BrowserControlTokenFile)
	if err != nil {
		return nil, nil, err
	}
	if err = browserspa.ProbeCapabilities(ctx, cfg.BrowserControlRuntimeURL, token); err != nil {
		return nil, nil, fmt.Errorf("bounded SPA runtime capability unavailable")
	}
	c := browserspa.Contract()
	hash, err := actionauth.ContractHash(c)
	if err != nil {
		return nil, nil, err
	}
	leases, err := resources.New(resources.WorkerGuard{}, blockedTabReconciliation{})
	if err != nil {
		return nil, nil, err
	}
	store, err := resourceexec.New(pool, leases, budgets, sources, resourceexec.Profile{ID: browserspa.Profile, RuntimeTarget: c.RuntimeTarget, ContractHashes: map[string]string{browserspa.Tool: hash}})
	if err != nil {
		return nil, nil, err
	}
	handler, err := resourceexec.NewHTTPHandler(store, token)
	if err != nil {
		return nil, nil, err
	}
	if err = router.SetTargetAuthentication(c.RuntimeTarget, token); err != nil {
		return nil, nil, err
	}
	if err = r.RegisterOwned([]*toolbrokerv1.ToolContract{c}); err != nil {
		return nil, nil, err
	}
	return store, handler, nil
}
