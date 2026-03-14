package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/butler/butler/apps/orchestrator/internal/app"
)

func main() {
	application, err := app.New(context.Background())
	if err != nil {
		slog.Error("failed to bootstrap orchestrator", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if err := application.Run(context.Background()); err != nil {
		os.Exit(1)
	}
}
