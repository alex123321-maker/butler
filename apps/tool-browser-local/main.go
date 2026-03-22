package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/butler/butler/apps/tool-browser-local/internal/app"
	"github.com/butler/butler/internal/logger"
)

func main() {
	log := logger.New(logger.Options{Service: "tool-browser-local", Component: "main", Level: slog.LevelInfo, Writer: os.Stdout})
	application, err := app.New(context.Background())
	if err != nil {
		log.Error("failed to bootstrap tool browser local runtime", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if err := application.Run(context.Background()); err != nil {
		os.Exit(1)
	}
}
