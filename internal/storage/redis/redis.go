package redis

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	configpkg "github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/logger"
	redislib "github.com/redis/go-redis/v9"
)

type Store struct {
	client *redislib.Client
	log    *slog.Logger
	config Config
}

type Config struct {
	URL string
}

func ConfigFromShared(cfg configpkg.RedisConfig) Config {
	return Config{URL: cfg.URL}
}

func Open(ctx context.Context, cfg Config, log *slog.Logger) (*Store, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, fmt.Errorf("redis url is required")
	}

	options, err := redislib.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse redis config: %w", err)
	}

	log = resolveLogger(log)
	log.Info("connecting to redis",
		slog.String("component", "redis"),
		slog.String("url", logger.MaskSecret(cfg.URL)),
	)

	client := redislib.NewClient(options)
	store := &Store{client: client, log: log, config: cfg}

	if err := store.HealthCheck(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}

	log.Info("redis connected", slog.String("component", "redis"))
	return store, nil
}

func (s *Store) Client() *redislib.Client {
	return s.client
}

func (s *Store) HealthCheck(ctx context.Context) error {
	if err := s.client.Ping(ctx).Err(); err != nil {
		s.log.Error("redis health check failed", slog.String("component", "redis"), slog.String("error", err.Error()))
		return fmt.Errorf("redis health check: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	if s == nil || s.client == nil {
		return nil
	}

	s.log.Info("closing redis client", slog.String("component", "redis"))
	if err := s.client.Close(); err != nil {
		s.log.Error("redis close failed", slog.String("component", "redis"), slog.String("error", err.Error()))
		return fmt.Errorf("close redis client: %w", err)
	}
	return nil
}

func resolveLogger(log *slog.Logger) *slog.Logger {
	if log != nil {
		return log
	}
	return slog.Default()
}
