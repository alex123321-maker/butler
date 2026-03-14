package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CreateSessionParams struct {
	SessionKey   string
	UserID       string
	Channel      string
	MetadataJSON string
}

type RunManager interface {
	CreateRun(ctx context.Context, req *sessionv1.CreateRunRequest) (*sessionv1.RunRecord, error)
	GetRun(ctx context.Context, runID string) (*sessionv1.RunRecord, error)
	FindRunByIdempotencyKey(ctx context.Context, sessionKey, idempotencyKey string) (*sessionv1.RunRecord, error)
	TransitionRun(ctx context.Context, req *sessionv1.UpdateRunStateRequest) (*sessionv1.RunRecord, error)
}

type Server struct {
	sessionv1.UnimplementedSessionServiceServer

	repo            Repository
	leases          LeaseManager
	runs            RunManager
	defaultLeaseTTL time.Duration
	log             *slog.Logger
}

func NewServer(repo Repository, leases LeaseManager, runs RunManager, defaultLeaseTTL time.Duration, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	if defaultLeaseTTL <= 0 {
		defaultLeaseTTL = 60 * time.Second
	}

	return &Server{
		repo:            repo,
		leases:          leases,
		runs:            runs,
		defaultLeaseTTL: defaultLeaseTTL,
		log:             logger.WithComponent(log, "session"),
	}
}

func (s *Server) CreateSession(ctx context.Context, req *sessionv1.CreateSessionRequest) (*sessionv1.CreateSessionResponse, error) {
	params, err := validateCreateSessionRequest(req)
	if err != nil {
		return nil, invalidArgumentError(err)
	}

	record, created, err := s.repo.CreateSession(ctx, params)
	if err != nil {
		s.log.Error("create session failed",
			slog.String("session_key", params.SessionKey),
			slog.String("channel", params.Channel),
			slog.String("error", err.Error()),
		)
		return nil, status.Error(codes.Internal, "create session failed")
	}

	s.log.Info("session upserted",
		slog.String("session_key", record.SessionKey),
		slog.String("channel", record.Channel),
		slog.String("user_id", record.UserID),
		slog.Bool("created", created),
	)

	return &sessionv1.CreateSessionResponse{
		Session: sessionToProto(record),
		Created: created,
	}, nil
}

func (s *Server) GetSession(ctx context.Context, req *sessionv1.GetSessionRequest) (*sessionv1.GetSessionResponse, error) {
	sessionKey := strings.TrimSpace(req.GetSessionKey())
	if sessionKey == "" {
		return nil, status.Error(codes.InvalidArgument, "session_key is required")
	}

	record, err := s.repo.GetSessionByKey(ctx, sessionKey)
	if err != nil {
		if err == ErrSessionNotFound {
			s.log.Warn("session lookup missed", slog.String("session_key", sessionKey))
			return nil, status.Error(codes.NotFound, "session not found")
		}
		s.log.Error("get session failed",
			slog.String("session_key", sessionKey),
			slog.String("error", err.Error()),
		)
		return nil, status.Error(codes.Internal, "get session failed")
	}

	s.log.Info("session fetched",
		slog.String("session_key", record.SessionKey),
		slog.String("channel", record.Channel),
	)

	return &sessionv1.GetSessionResponse{Session: sessionToProto(record)}, nil
}

func (s *Server) ResolveSessionKey(_ context.Context, req *sessionv1.ResolveSessionKeyRequest) (*sessionv1.ResolveSessionKeyResponse, error) {
	channel := strings.TrimSpace(req.GetChannel())
	if channel == "" {
		return nil, status.Error(codes.InvalidArgument, "channel is required")
	}

	chatID := strings.TrimSpace(req.GetExternalChatId())
	userID := strings.TrimSpace(req.GetExternalUserId())
	if chatID == "" && userID == "" {
		return nil, status.Error(codes.InvalidArgument, "external_chat_id or external_user_id is required")
	}

	sessionKey := resolveSessionKey(channel, chatID, userID)
	s.log.Info("session key resolved",
		slog.String("channel", channel),
		slog.String("session_key", sessionKey),
	)

	return &sessionv1.ResolveSessionKeyResponse{SessionKey: sessionKey}, nil
}

func resolveSessionKey(channel, externalChatID, externalUserID string) string {
	channel = strings.TrimSpace(channel)
	if externalChatID != "" {
		return strings.Join([]string{channel, "chat", strings.TrimSpace(externalChatID)}, ":")
	}
	return strings.Join([]string{channel, "user", strings.TrimSpace(externalUserID)}, ":")
}

func validateCreateSessionRequest(req *sessionv1.CreateSessionRequest) (CreateSessionParams, error) {
	params := CreateSessionParams{
		SessionKey: strings.TrimSpace(req.GetSessionKey()),
		UserID:     strings.TrimSpace(req.GetUserId()),
		Channel:    strings.TrimSpace(req.GetChannel()),
	}

	if params.SessionKey == "" {
		return CreateSessionParams{}, fmt.Errorf("session_key is required")
	}
	if params.UserID == "" {
		return CreateSessionParams{}, fmt.Errorf("user_id is required")
	}
	if params.Channel == "" {
		return CreateSessionParams{}, fmt.Errorf("channel is required")
	}

	metadataJSON, err := normalizeMetadataJSON(req.GetMetadataJson())
	if err != nil {
		return CreateSessionParams{}, err
	}
	params.MetadataJSON = metadataJSON

	return params, nil
}

func normalizeMetadataJSON(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "{}", nil
	}

	if !json.Valid([]byte(trimmed)) {
		return "", fmt.Errorf("metadata_json must be valid JSON")
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(trimmed)); err != nil {
		return "", fmt.Errorf("metadata_json must be valid JSON")
	}

	return compact.String(), nil
}

func sessionToProto(record SessionRecord) *sessionv1.Session {
	return &sessionv1.Session{
		SessionId:    record.SessionKey,
		SessionKey:   record.SessionKey,
		UserId:       record.UserID,
		Channel:      record.Channel,
		MetadataJson: record.MetadataJSON,
		CreatedAt:    record.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:    record.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func invalidArgumentError(err error) error {
	return status.Error(codes.InvalidArgument, err.Error())
}

func (s *Server) CreateRun(ctx context.Context, req *sessionv1.CreateRunRequest) (*sessionv1.CreateRunResponse, error) {
	if s.runs == nil {
		return nil, status.Error(codes.Unimplemented, "run manager is not configured")
	}
	if req.GetInputEvent() != nil {
		idempotencyKey := strings.TrimSpace(req.GetInputEvent().GetIdempotencyKey())
		if idempotencyKey != "" {
			existing, err := s.runs.FindRunByIdempotencyKey(ctx, req.GetSessionKey(), idempotencyKey)
			if err == nil {
				s.log.Info("duplicate input event resolved to existing run",
					slog.String("session_key", req.GetSessionKey()),
					slog.String("run_id", existing.GetRunId()),
					slog.String("idempotency_key", idempotencyKey),
				)
				return &sessionv1.CreateRunResponse{Run: existing}, nil
			}
			if !errors.Is(err, runservice.ErrRunNotFound) {
				return nil, mapRunError(err)
			}
		}
	}
	run, err := s.runs.CreateRun(ctx, req)
	if err != nil {
		return nil, mapRunError(err)
	}
	return &sessionv1.CreateRunResponse{Run: run}, nil
}

func (s *Server) UpdateRunState(ctx context.Context, req *sessionv1.UpdateRunStateRequest) (*sessionv1.UpdateRunStateResponse, error) {
	if s.runs == nil {
		return nil, status.Error(codes.Unimplemented, "run manager is not configured")
	}
	run, err := s.runs.TransitionRun(ctx, req)
	if err != nil {
		return nil, mapRunError(err)
	}
	return &sessionv1.UpdateRunStateResponse{Run: run}, nil
}

func (s *Server) GetRun(ctx context.Context, req *sessionv1.GetRunRequest) (*sessionv1.GetRunResponse, error) {
	if s.runs == nil {
		return nil, status.Error(codes.Unimplemented, "run manager is not configured")
	}
	runID := strings.TrimSpace(req.GetRunId())
	if runID == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id is required")
	}
	run, err := s.runs.GetRun(ctx, runID)
	if err != nil {
		return nil, mapRunError(err)
	}
	return &sessionv1.GetRunResponse{Run: run}, nil
}

func mapRunError(err error) error {
	if err == nil {
		return nil
	}
	if status.Code(err) != codes.Unknown {
		return err
	}
	message := err.Error()
	switch {
	case errors.Is(err, runservice.ErrRunNotFound):
		return status.Error(codes.NotFound, message)
	case errors.Is(err, runservice.ErrRunDuplicate):
		return status.Error(codes.AlreadyExists, message)
	case strings.Contains(message, "is required"), strings.Contains(message, "must be"), strings.Contains(message, "not allowed"):
		return status.Error(codes.InvalidArgument, message)
	default:
		return status.Error(codes.Internal, message)
	}
}
