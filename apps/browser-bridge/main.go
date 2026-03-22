package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/butler/butler/apps/browser-bridge/internal/app"
	"github.com/butler/butler/internal/logger"
)

func main() {
	log := logger.New(logger.Options{Service: "browser-bridge", Component: "main", Level: slog.LevelInfo, Writer: os.Stderr})
	application, err := app.New(context.Background(), os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		log.Error("failed to bootstrap browser bridge", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if err := application.Run(context.Background()); err != nil {
		log.Error("browser bridge stopped with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
