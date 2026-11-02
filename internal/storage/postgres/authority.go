package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OpenWithAuthorityReserve reserves independent capacity for short lease/Task
// checks and longer token-only source checks. The total maximum is unchanged;
// business work retains at least two connections. At the minimum cap of three,
// source reads use business capacity, never the single authority connection.
func OpenWithAuthorityReserve(ctx context.Context, cfg Config, log *slog.Logger) (*Store, error) {
	parsed, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL authority reserve configuration")
	}
	if cfg.MaxConns <= 0 {
		cfg.MaxConns = parsed.MaxConns
	}
	if cfg.MinConns == 0 {
		cfg.MinConns = parsed.MinConns
	}
	if cfg.MaxConns < 3 || cfg.MinConns < 0 || cfg.MinConns > cfg.MaxConns {
		return nil, fmt.Errorf("orchestrator requires at least three PostgreSQL connections, including one authority reserve; min_conns must not exceed max_conns")
	}
	write := cfg
	write.MaxConns--
	var sourceReserve int32
	if cfg.MaxConns >= 4 {
		sourceReserve = 1
		write.MaxConns--
	}
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
	if sourceReserve != 0 {
		source := read.Copy()
		source.MinConns = 0
		if cfg.MinConns > write.MinConns+read.MinConns {
			source.MinConns = 1
		}
		source.ConnConfig.RuntimeParams["application_name"] = "butler-source-observer"
		s.sourceReads, err = pgxpool.NewWithConfig(ctx, source)
		if err == nil {
			err = s.sourceReads.Ping(ctx)
		}
		if err != nil {
			s.Close()
			return nil, fmt.Errorf("open PostgreSQL source reserve: %w", err)
		}
	}
	s.log.Info("reserved PostgreSQL authority capacity", slog.Int64("total_max_conns", int64(cfg.MaxConns)), slog.Int64("business_max_conns", int64(write.MaxConns)), slog.Int("authority_max_conns", 1), slog.Int64("source_max_conns", int64(sourceReserve)))
	return s, nil
}

// SourceReadPool is used only by source-observation composition, with the same
// current proofs/deadlines. Capacity selection never proves authority itself.
func (s *Store) SourceReadPool() *pgxpool.Pool {
	if s.sourceReads != nil {
		return s.sourceReads
	}
	return s.pool
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
