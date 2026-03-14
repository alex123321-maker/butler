package postgres

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/logger"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool   *pgxpool.Pool
	log    *slog.Logger
	config Config
}

type Config struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MigrationsDir   string
}

func ConfigFromShared(cfg config.PostgresConfig) Config {
	return Config{
		URL:             cfg.URL,
		MaxConns:        cfg.MaxConns,
		MinConns:        cfg.MinConns,
		MaxConnLifetime: time.Duration(cfg.MaxConnLifetime) * time.Second,
		MigrationsDir:   cfg.MigrationsDir,
	}
}

func Open(ctx context.Context, cfg Config, log *slog.Logger) (*Store, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, fmt.Errorf("postgres url is required")
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}

	if cfg.MaxConns > 0 {
		poolConfig.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns > 0 {
		poolConfig.MinConns = cfg.MinConns
	}
	if cfg.MaxConnLifetime > 0 {
		poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	}

	log = resolveLogger(log)
	log.Info("connecting to postgres",
		slog.String("component", "postgres"),
		slog.String("dsn", logger.MaskSecret(cfg.URL)),
		slog.Int64("max_conns", int64(poolConfig.MaxConns)),
		slog.Int64("min_conns", int64(poolConfig.MinConns)),
	)

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Error("postgres connection failed", slog.String("component", "postgres"), slog.String("error", err.Error()))
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}

	store := &Store{pool: pool, log: log, config: cfg}
	if err := store.HealthCheck(ctx); err != nil {
		store.Close()
		return nil, err
	}

	log.Info("postgres connected", slog.String("component", "postgres"))
	return store, nil
}

func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

func (s *Store) HealthCheck(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		s.log.Error("postgres health check failed", slog.String("component", "postgres"), slog.String("error", err.Error()))
		return fmt.Errorf("postgres health check: %w", err)
	}
	return nil
}

func (s *Store) Close() {
	if s == nil || s.pool == nil {
		return
	}
	counts := s.pool.Stat()
	s.log.Info("closing postgres pool",
		slog.String("component", "postgres"),
		slog.Int64("total_conns", int64(counts.TotalConns())),
	)
	s.pool.Close()
}

func (s *Store) RunMigrations(ctx context.Context, dir string) error {
	return s.runMigrations(ctx, dir, migrationDirectionUp)
}

func (s *Store) RunDownMigrations(ctx context.Context, dir string) error {
	return s.runMigrations(ctx, dir, migrationDirectionDown)
}

type migrationDirection string

const (
	migrationDirectionUp   migrationDirection = "up"
	migrationDirectionDown migrationDirection = "down"
)

func (s *Store) runMigrations(ctx context.Context, dir string, direction migrationDirection) error {
	if strings.TrimSpace(dir) == "" {
		dir = s.config.MigrationsDir
	}
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("migrations directory is required")
	}

	entries, err := migrationFiles(dir, direction)
	if err != nil {
		return err
	}

	if _, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	for _, path := range entries {
		version := migrationVersion(path, direction)
		var alreadyApplied bool
		if err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", version).Scan(&alreadyApplied); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if direction == migrationDirectionUp && alreadyApplied {
			continue
		}
		if direction == migrationDirectionDown && !alreadyApplied {
			continue
		}

		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}

		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", version, err)
		}

		if _, err := tx.Exec(ctx, string(contents)); err != nil {
			_ = tx.Rollback(ctx)
			s.log.Error("migration failed", slog.String("component", "postgres"), slog.String("migration", version), slog.String("error", err.Error()))
			return fmt.Errorf("execute migration %s: %w", version, err)
		}
		if direction == migrationDirectionUp {
			if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("record migration %s: %w", version, err)
			}
		} else {
			if _, err := tx.Exec(ctx, "DELETE FROM schema_migrations WHERE version = $1", version); err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("remove migration record %s: %w", version, err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", version, err)
		}

		message := "migration applied"
		if direction == migrationDirectionDown {
			message = "migration rolled back"
		}
		s.log.Info(message, slog.String("component", "postgres"), slog.String("migration", version))
	}

	return nil
}

func migrationFiles(dir string, direction migrationDirection) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !isMigrationFile(entry, direction) {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}

	sort.Strings(files)
	if direction == migrationDirectionDown {
		reverse(files)
	}
	return files, nil
}

func isMigrationFile(entry fs.DirEntry, direction migrationDirection) bool {
	name := entry.Name()
	suffix := ".up.sql"
	if direction == migrationDirectionDown {
		suffix = ".down.sql"
	}
	return strings.HasSuffix(name, suffix)
}

func reverse(values []string) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func migrationVersion(path string, direction migrationDirection) string {
	name := filepath.Base(path)
	if direction == migrationDirectionDown {
		return strings.TrimSuffix(name, ".down.sql") + ".up.sql"
	}
	return name
}

func resolveLogger(log *slog.Logger) *slog.Logger {
	if log != nil {
		return log
	}
	return slog.Default()
}
