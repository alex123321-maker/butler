package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/butler/butler/internal/logger"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func main() {
	direction := flag.String("direction", "up", "migration direction: up or down")
	migrationsDir := flag.String("migrations-dir", envOrDefault("BUTLER_POSTGRES_MIGRATIONS_DIR", "migrations"), "migration directory")
	postgresURL := flag.String("postgres-url", envOrDefault("BUTLER_POSTGRES_URL", ""), "postgres connection string")
	flag.Parse()
	if *postgresURL == "" {
		slog.Error("postgres-url is required", slog.String("flag", "postgres-url"), slog.String("env", "BUTLER_POSTGRES_URL"))
		os.Exit(1)
	}

	log := logger.New(logger.Options{Service: "migrator", Component: "cli", Level: slog.LevelInfo, Writer: os.Stdout})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store, err := postgresstore.Open(ctx, postgresstore.Config{
		URL:             *postgresURL,
		MaxConns:        2,
		MinConns:        1,
		MaxConnLifetime: 5 * time.Minute,
		MigrationsDir:   *migrationsDir,
	}, log)
	if err != nil {
		log.Error("failed to open postgres for migrations", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer store.Close()

	switch *direction {
	case "up":
		err = store.RunMigrations(ctx, *migrationsDir)
	case "down":
		err = store.RunDownMigrations(ctx, *migrationsDir)
	default:
		log.Error("invalid migration direction", slog.String("direction", *direction))
		os.Exit(1)
	}

	if err != nil {
		log.Error("migration command failed", slog.String("direction", *direction), slog.String("error", err.Error()))
		os.Exit(1)
	}

	log.Info("migration command completed", slog.String("direction", *direction))
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
