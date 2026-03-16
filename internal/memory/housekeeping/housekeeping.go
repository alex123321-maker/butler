package housekeeping

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/butler/butler/internal/logger"
)

type WorkingPruner interface {
	DeleteStaleBefore(context.Context, time.Time) (int64, error)
}

type ProfilePruner interface {
	DeleteInactiveBefore(context.Context, time.Time) (int64, error)
}

type EpisodicPruner interface {
	Prune(context.Context, time.Time, int) (int64, error)
}

type ChunkPruner interface {
	Prune(context.Context, time.Time, int) (int64, error)
}

type Config struct {
	WorkingRetention time.Duration
	ProfileRetention time.Duration
	EpisodeRetention time.Duration
	ChunkRetention   time.Duration
	KeepEpisodes     int
	KeepChunks       int
	Log              *slog.Logger
}

type Service struct {
	working WorkingPruner
	profile ProfilePruner
	episode EpisodicPruner
	chunks  ChunkPruner
	config  Config
	log     *slog.Logger
}

type Result struct {
	WorkingDeleted int64
	ProfileDeleted int64
	EpisodeDeleted int64
	ChunkDeleted   int64
}

func New(working WorkingPruner, profile ProfilePruner, episode EpisodicPruner, chunks ChunkPruner, cfg Config) *Service {
	if cfg.WorkingRetention <= 0 {
		cfg.WorkingRetention = 7 * 24 * time.Hour
	}
	if cfg.ProfileRetention <= 0 {
		cfg.ProfileRetention = 30 * 24 * time.Hour
	}
	if cfg.EpisodeRetention <= 0 {
		cfg.EpisodeRetention = 30 * 24 * time.Hour
	}
	if cfg.ChunkRetention <= 0 {
		cfg.ChunkRetention = 45 * 24 * time.Hour
	}
	if cfg.KeepEpisodes <= 0 {
		cfg.KeepEpisodes = 20
	}
	if cfg.KeepChunks <= 0 {
		cfg.KeepChunks = 20
	}
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{working: working, profile: profile, episode: episode, chunks: chunks, config: cfg, log: logger.WithComponent(log, "memory-housekeeping")}
}

func (s *Service) RunOnce(ctx context.Context, now time.Time) (Result, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	result := Result{}
	var err error
	if s.working != nil {
		result.WorkingDeleted, err = s.working.DeleteStaleBefore(ctx, now.Add(-s.config.WorkingRetention))
		if err != nil {
			return Result{}, fmt.Errorf("prune working memory: %w", err)
		}
	}
	if s.profile != nil {
		result.ProfileDeleted, err = s.profile.DeleteInactiveBefore(ctx, now.Add(-s.config.ProfileRetention))
		if err != nil {
			return Result{}, fmt.Errorf("prune profile memory: %w", err)
		}
	}
	if s.episode != nil {
		result.EpisodeDeleted, err = s.episode.Prune(ctx, now.Add(-s.config.EpisodeRetention), s.config.KeepEpisodes)
		if err != nil {
			return Result{}, fmt.Errorf("prune episodic memory: %w", err)
		}
	}
	if s.chunks != nil {
		result.ChunkDeleted, err = s.chunks.Prune(ctx, now.Add(-s.config.ChunkRetention), s.config.KeepChunks)
		if err != nil {
			return Result{}, fmt.Errorf("prune chunk memory: %w", err)
		}
	}
	s.log.Info("memory housekeeping completed",
		slog.Int64("working_deleted", result.WorkingDeleted),
		slog.Int64("profile_deleted", result.ProfileDeleted),
		slog.Int64("episode_deleted", result.EpisodeDeleted),
		slog.Int64("chunk_deleted", result.ChunkDeleted),
	)
	return result, nil
}
