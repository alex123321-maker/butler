package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/butler/butler/apps/orchestrator/internal/app"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/logger"
)

func main() {
	profile := flag.String("communication-profile", "", "private immutable owner communication profile")
	flag.Parse()
	log := logger.New(logger.Options{Service: "orchestrator", Component: "main", Level: slog.LevelInfo, Writer: os.Stdout})
	var application *app.App
	var err error
	if *profile == "" {
		application, err = app.New(context.Background())
	} else {
		var p communicationcontrol.OwnerProfile
		p, err = communicationcontrol.LoadOwnerProfile(*profile)
		if err == nil {
			application, err = app.NewForOwnerCommunication(context.Background(), p)
		}
	}
	if err != nil {
		log.Error("failed to bootstrap orchestrator", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if err := application.Run(context.Background()); err != nil {
		os.Exit(1)
	}
}
