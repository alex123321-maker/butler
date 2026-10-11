package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/apps/tool-broker/internal/runtimeclient"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/singletabcontrol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type blockedTabReconciliation struct{}

func (blockedTabReconciliation) CheckReconciliationTx(context.Context, pgx.Tx, resources.Decision) error {
	return resources.ErrDenied
}

// Existing account/page networking belongs to the selected operator-owned
// Chrome profile. This policy narrows tab origins; it is not a network proxy.
func guardedOrigins(raw string) (map[string]bool, error) {
	var values []string
	if len(raw) > 8192 || json.Unmarshal([]byte(raw), &values) != nil || len(values) < 1 || len(values) > 32 {
		return nil, resourceexec.ErrDenied
	}
	out := map[string]bool{}
	for _, v := range values {
		canonical, err := resourceexec.SingleTabOrigin(v)
		if err != nil || canonical != v || out[v] {
			return nil, resourceexec.ErrDenied
		}
		out[v] = true
	}
	return out, nil
}

func configureGuardedTabRuntime(ctx context.Context, cfg config.ToolBrokerConfig, pool *pgxpool.Pool, registryStore *registry.Registry, router *runtimeclient.Router, budgets *budget.Controller, sources resourceexec.SourceGuard) (*resourceexec.Store, http.Handler, error) {
	if !cfg.GuardedTabRuntimeEnabled {
		return nil, nil, nil
	}
	if !cfg.GuardedTabRequestEnabled || pool == nil || registryStore == nil || router == nil || budgets == nil || sources == nil || !resourceexec.ValidSingleTabTarget(cfg.GuardedTabRuntimeTarget) || !resourceexec.ValidSingleTabTarget(cfg.Shared.HTTPAddr) || !resourceexec.ValidSingleTabTarget(cfg.Shared.GRPCAddr) || cfg.GuardedTabProfileID == "" || len(cfg.GuardedTabProfileID) > 128 || cfg.GuardedTabRuntimeHost == "" || len(cfg.GuardedTabRuntimeHost) > 128 {
		return nil, nil, fmt.Errorf("guarded tab runtime requires complete private loopback composition")
	}
	// Do not silently upgrade raw contracts or leave alternate single-tab tools.
	for _, contract := range registryStore.List("", true) {
		if strings.HasPrefix(contract.GetToolName(), "single_tab.") {
			return nil, nil, fmt.Errorf("guarded tab runtime requires a registry without legacy single_tab contracts")
		}
	}
	origins, err := guardedOrigins(cfg.GuardedTabAllowedOriginsJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("guarded tab runtime requires canonical operator origin policy")
	}
	token, err := actionauth.ReadServiceToken(cfg.GuardedTabRuntimeTokenFile)
	if err != nil {
		return nil, nil, fmt.Errorf("guarded tab runtime requires its private service token file")
	}
	if singletabcontrol.CheckSchema(ctx, pool) != nil {
		return nil, nil, fmt.Errorf("guarded tab runtime requires immutable selection schema")
	}
	var complete bool
	if pool.QueryRow(ctx, `SELECT to_regclass('execution_resources') IS NOT NULL AND to_regclass('execution_resource_leases') IS NOT NULL AND to_regclass('execution_resource_dispatches') IS NOT NULL AND to_regclass('execution_budget_reservations') IS NOT NULL`).Scan(&complete) != nil || !complete {
		return nil, nil, fmt.Errorf("guarded tab runtime requires resource and shared budget schema")
	}
	if resourceexec.CheckSingleTabReadiness(ctx, cfg.GuardedTabRuntimeTarget, cfg.GuardedTabRuntimeHost, token) != nil {
		return nil, nil, fmt.Errorf("exact authenticated installed guarded runtime is unavailable")
	}
	contracts := resourceexec.SingleTabContracts(cfg.GuardedTabRuntimeTarget)
	hashes := map[string]string{}
	for _, c := range contracts {
		hashes[c.ToolName], err = actionauth.ContractHash(c)
		if err != nil {
			return nil, nil, resourceexec.ErrDenied
		}
	}
	leases, err := resources.New(resources.WorkerGuard{}, blockedTabReconciliation{})
	if err != nil {
		return nil, nil, err
	}
	guard := singletabcontrol.Guard(cfg.GuardedTabRuntimeHost)
	policyGuard := func(ctx context.Context, tx pgx.Tx, b resources.Binding, id string) (resourceexec.SingleTabSession, error) {
		selected, e := guard(ctx, tx, b, id)
		if e != nil || !origins[selected.Origin] {
			return resourceexec.SingleTabSession{}, resourceexec.ErrDenied
		}
		return selected, nil
	}
	store, err := resourceexec.NewSingleTab(pool, leases, budgets, sources, policyGuard, resourceexec.Profile{ID: cfg.GuardedTabProfileID, RuntimeTarget: cfg.GuardedTabRuntimeTarget, ContractHashes: hashes})
	if err != nil {
		return nil, nil, err
	}
	handler, err := resourceexec.NewHTTPHandler(store, token)
	if err != nil {
		return nil, nil, err
	}
	if err = router.SetTargetAuthentication(cfg.GuardedTabRuntimeTarget, token); err != nil {
		return nil, nil, resourceexec.ErrDenied
	}
	if err = registryStore.RegisterOwned(contracts); err != nil {
		return nil, nil, err
	}
	return store, handler, nil
}
