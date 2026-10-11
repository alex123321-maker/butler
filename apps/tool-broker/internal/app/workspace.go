package app

import (
	"context"
	"fmt"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/apps/tool-broker/internal/runtimeclient"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/workspaceexec"
	"github.com/jackc/pgx/v5/pgxpool"
)

func configureWorkspace(ctx context.Context, cfg config.ToolBrokerConfig, pool *pgxpool.Pool, reg *registry.Registry, router *runtimeclient.Router, budgets *budget.Controller, sources *catalog.Store) (*workspaceexec.DispatchStore, error) {
	if !cfg.WorkspaceEnabled {
		return nil, nil
	}
	if pool == nil || reg == nil || router == nil || budgets == nil || sources == nil || cfg.WorkspaceProfileID == "" || len(cfg.WorkspaceProfileID) > 128 || !workspaceexec.ValidTarget(cfg.WorkspaceRuntimeTarget) {
		return nil, fmt.Errorf("workspace runtime requires complete private composition")
	}
	for _, c := range reg.List("", true) {
		if workspaceexec.Protected(c.GetToolName()) {
			return nil, fmt.Errorf("workspace runtime requires a registry without raw workspace contracts")
		}
	}
	token, err := actionauth.ReadServiceToken(cfg.WorkspaceRuntimeTokenFile)
	if err != nil {
		return nil, fmt.Errorf("workspace runtime requires its private service token file")
	}
	provenance, err := workspaceexec.NewProvenance(pool, sources)
	if err != nil {
		return nil, err
	}
	leases, err := resources.New(resources.WorkerGuard{}, blockedTabReconciliation{})
	if err != nil {
		return nil, err
	}
	profile := workspaceexec.Profile{ID: cfg.WorkspaceProfileID, Target: cfg.WorkspaceRuntimeTarget}
	d, err := workspaceexec.NewDispatchStore(pool, leases, budgets, provenance, profile)
	if err != nil || d.CheckReady(ctx) != nil {
		return nil, fmt.Errorf("workspace runtime requires current source/input/resource/dispatch schema and deletion anchor")
	}
	if workspaceexec.CheckReadiness(ctx, profile, token) != nil {
		return nil, fmt.Errorf("exact authenticated installed workspace runtime is unavailable")
	}
	if err = router.SetTargetAuthentication(profile.Target, token); err != nil {
		return nil, err
	}
	if err = reg.RegisterOwned(workspaceexec.Contracts(profile.Target)); err != nil {
		return nil, err
	}
	return d, nil
}
