package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/butler/butler/apps/tool-workspace/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	a, err := app.New(ctx)
	if err == nil {
		slog.Info("workspace runtime started")
		err = a.Run(ctx)
	}
	if err != nil {
		slog.Error("workspace runtime stopped", slog.String("reason", "runtime configuration or service failure"))
		os.Exit(1)
	}
	slog.Info("workspace runtime stopped")
}
