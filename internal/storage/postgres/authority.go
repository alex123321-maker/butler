package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OpenWithAuthorityReserve reserves one of the configured connections for
// bounded lease/Task authority observations. Long channel polls, retained feeds
// and fenced business writes cannot consume it. The total configured maximum
// is unchanged; ordinary writes still need at least two connections.
func OpenWithAuthorityReserve(ctx context.Context, cfg Config, log *slog.Logger) (*Store, error) {
	if cfg.MaxConns <= 0 {
		parsed, err := pgxpool.ParseConfig(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("parse PostgreSQL authority reserve configuration")
		}
		cfg.MaxConns = parsed.MaxConns
	}
	if cfg.MaxConns < 3 || cfg.MinConns > cfg.MaxConns {
		return nil, fmt.Errorf("orchestrator requires at least three PostgreSQL connections, including one authority reserve; min_conns must not exceed max_conns")
	}
	write := cfg
	write.MaxConns--
	if write.MinConns > write.MaxConns {
		write.MinConns = write.MaxConns
	}
	s, err := Open(ctx, write, log)
	if err != nil {
		return nil, err
	}
	read := s.pool.Config().Copy()
	read.MaxConns = 1
	read.MinConns = 0
	if cfg.MinConns > write.MinConns {
		read.MinConns = 1
	}
	read.ConnConfig.RuntimeParams["application_name"] = "butler-authority-observer"
	s.authorityReads, err = pgxpool.NewWithConfig(ctx, read)
	if err == nil {
		err = s.authorityReads.Ping(ctx)
	}
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("open PostgreSQL authority reserve: %w", err)
	}
	s.log.Info("reserved PostgreSQL authority capacity", slog.Int64("total_max_conns", int64(cfg.MaxConns)), slog.Int64("business_max_conns", int64(write.MaxConns)), slog.Int("authority_max_conns", 1))
	return s, nil
}

// AuthorityReadPool is wired only to trusted lease/Task observation methods.
// It does not grant authority; these readers still query current PostgreSQL
// state and retain their existing locks, revisions, deadlines and denial rules.
func (s *Store) AuthorityReadPool() *pgxpool.Pool {
	if s.authorityReads != nil {
		return s.authorityReads
	}
	return s.pool
}
