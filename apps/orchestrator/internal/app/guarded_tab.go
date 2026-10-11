package app

import (
	"context"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/singletabcontrol"
	"github.com/jackc/pgx/v5/pgxpool"
)

func configureGuardedTabOwner(ctx context.Context, pool *pgxpool.Pool, cfg config.OrchestratorConfig, sources resourceexec.SourceGuard) (api.GuardedTabDecisions, error) {
	if !cfg.GuardedTabOwnerEnabled {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if singletabcontrol.CheckSchema(ctx, pool) != nil {
		return nil, singletabcontrol.ErrDenied
	}
	token, err := actionauth.ReadServiceToken(cfg.GuardedTabTokenFile)
	if err != nil {
		return nil, singletabcontrol.ErrDenied
	}
	metadata, err := singletabcontrol.NewMetadataClient(cfg.GuardedTabMetadataURL, token)
	if err != nil {
		return nil, singletabcontrol.ErrDenied
	}
	return singletabcontrol.NewWaiting(pool, metadata, sources, cfg.GuardedTabHost)
}
