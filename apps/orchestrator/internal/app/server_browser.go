package app

import (
	"context"
	"github.com/butler/butler/internal/browsercontrol"
	"github.com/butler/butler/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func browserOwnerWait(cfg config.OrchestratorConfig, pool *pgxpool.Pool) func(context.Context, string) error {
	if cfg.BrowserControlURL == "" {
		return nil
	}
	return (&browsercontrol.Store{Pool: pool}).WaitRun
}
