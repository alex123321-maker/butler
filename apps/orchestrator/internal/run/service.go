package run

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/butler/butler/internal/domain"
	"github.com/butler/butler/internal/domain/convert"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/logger"
)

type Service struct {
	repo Repository
	log  *slog.Logger
}

func NewService(repo Repository, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{repo: repo, log: logger.WithComponent(log, "run")}
}

func (s *Service) CreateRun(ctx context.Context, req *sessionv1.CreateRunRequest) (*sessionv1.RunRecord, error) {
	params, err := validateCreateRunRequest(req)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	record, err := s.repo.CreateRun(ctx, Record{
		RunID:          newRunID(now),
		SessionKey:     params.SessionKey,
		InputEventID:   params.InputEventID,
		IdempotencyKey: params.IdempotencyKey,
		Status:         string(domain.RunStateCreated),
		AutonomyMode:   params.AutonomyMode,
		CurrentState:   string(domain.RunStateCreated),
		ModelProvider:  params.ModelProvider,
		ResumesRunID:   params.ResumesRunID,
		StartedAt:      now,
		UpdatedAt:      now,
		MetadataJSON:   params.MetadataJSON,
	})
	if err != nil {
		if params.IdempotencyKey != "" && err == ErrRunDuplicate {
			duplicate, lookupErr := s.repo.FindRunByIdempotencyKey(ctx, params.SessionKey, params.IdempotencyKey)
			if lookupErr != nil {
				return nil, lookupErr
			}
			return recordToProto(duplicate), nil
		}
		return nil, err
	}

	s.log.Info("run created",
		slog.String("run_id", record.RunID),
		slog.String("session_key", record.SessionKey),
		slog.String("state", record.CurrentState),
	)

	return recordToProto(record), nil
}

func (s *Service) FindRunByIdempotencyKey(ctx context.Context, sessionKey, idempotencyKey string) (*sessionv1.RunRecord, error) {
	record, err := s.repo.FindRunByIdempotencyKey(ctx, strings.TrimSpace(sessionKey), strings.TrimSpace(idempotencyKey))
	if err != nil {
		return nil, err
	}
	return recordToProto(record), nil
}

func (s *Service) GetRun(ctx context.Context, runID string) (*sessionv1.RunRecord, error) {
	record, err := s.repo.GetRun(ctx, strings.TrimSpace(runID))
	if err != nil {
		return nil, err
	}
	return recordToProto(record), nil
}

func (s *Service) PersistProviderSessionRef(ctx context.Context, runID, providerSessionRef string) (*sessionv1.RunRecord, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}

	record, err := s.repo.UpdateProviderSessionRef(ctx, UpdateProviderSessionRefParams{
		RunID:              runID,
		ProviderSessionRef: strings.TrimSpace(providerSessionRef),
		UpdatedAt:          time.Now().UTC(),
	})
	if err != nil {
		return nil, err
	}

	s.log.Info("run provider session updated",
		slog.String("run_id", record.RunID),
		slog.Bool("has_provider_session_ref", record.ProviderSessionRef != ""),
	)

	return recordToProto(record), nil
}

func (s *Service) ListRunsBySessionKey(ctx context.Context, sessionKey string) ([]*sessionv1.RunRecord, error) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return nil, fmt.Errorf("session_key is required")
	}

	records, err := s.repo.ListRunsBySessionKey(ctx, sessionKey)
	if err != nil {
		return nil, err
	}

	runs := make([]*sessionv1.RunRecord, 0, len(records))
	for _, record := range records {
		runs = append(runs, recordToProto(record))
	}
	return runs, nil
}

func (s *Service) TransitionRun(ctx context.Context, req *sessionv1.UpdateRunStateRequest) (*sessionv1.RunRecord, error) {
	params, err := validateTransitionRequest(req)
	if err != nil {
		return nil, err
	}

	if err := domain.ValidateRunStateTransition(params.FromState, params.ToState); err != nil {
		return nil, err
	}

	finishedAt := params.FinishedAt
	if finishedAt == nil && domain.IsTerminalRunState(params.ToState) {
		now := time.Now().UTC()
		finishedAt = &now
	}

	record, err := s.repo.UpdateRun(ctx, UpdateParams{
		RunID:        params.RunID,
		CurrentState: string(params.FromState),
		NextState:    string(params.ToState),
		Status:       string(params.ToState),
		LeaseID:      params.LeaseID,
		ErrorType:    params.ErrorType,
		ErrorMessage: params.ErrorMessage,
		FinishedAt:   finishedAt,
		UpdatedAt:    time.Now().UTC(),
	})
	if err != nil {
		return nil, err
	}

	s.log.Info("run transitioned",
		slog.String("run_id", record.RunID),
		slog.String("from_state", string(params.FromState)),
		slog.String("to_state", string(params.ToState)),
	)

	return recordToProto(record), nil
}

type createParams struct {
	SessionKey     string
	InputEventID   string
	IdempotencyKey string
	AutonomyMode   string
	ModelProvider  string
	ResumesRunID   string
	MetadataJSON   string
}

type transitionParams struct {
	RunID        string
	FromState    domain.RunState
	ToState      domain.RunState
	LeaseID      string
	ErrorType    string
	ErrorMessage string
	FinishedAt   *time.Time
}

func validateCreateRunRequest(req *sessionv1.CreateRunRequest) (createParams, error) {
	if strings.TrimSpace(req.GetSessionKey()) == "" {
		return createParams{}, fmt.Errorf("session_key is required")
	}
	if req.GetInputEvent() == nil {
		return createParams{}, fmt.Errorf("input_event is required")
	}
	if strings.TrimSpace(req.GetInputEvent().GetEventId()) == "" {
		return createParams{}, fmt.Errorf("input_event.event_id is required")
	}
	if strings.TrimSpace(req.GetModelProvider()) == "" {
		return createParams{}, fmt.Errorf("model_provider is required")
	}

	mode, err := autonomyModeToString(req.GetAutonomyMode())
	if err != nil {
		return createParams{}, err
	}

	return createParams{
		SessionKey:     strings.TrimSpace(req.GetSessionKey()),
		InputEventID:   strings.TrimSpace(req.GetInputEvent().GetEventId()),
		IdempotencyKey: strings.TrimSpace(req.GetInputEvent().GetIdempotencyKey()),
		AutonomyMode:   mode,
		ModelProvider:  strings.TrimSpace(req.GetModelProvider()),
		ResumesRunID:   strings.TrimSpace(req.GetResumesRunId()),
		MetadataJSON:   strings.TrimSpace(req.GetMetadataJson()),
	}, nil
}

func validateTransitionRequest(req *sessionv1.UpdateRunStateRequest) (transitionParams, error) {
	if strings.TrimSpace(req.GetRunId()) == "" {
		return transitionParams{}, fmt.Errorf("run_id is required")
	}
	fromState, err := protoRunStateToDomain(req.GetFromState())
	if err != nil {
		return transitionParams{}, err
	}
	toState, err := protoRunStateToDomain(req.GetToState())
	if err != nil {
		return transitionParams{}, err
	}

	finishedAt, err := parseFinishedAt(req.GetFinishedAt())
	if err != nil {
		return transitionParams{}, err
	}
	if finishedAt != nil && !domain.IsTerminalRunState(toState) {
		return transitionParams{}, fmt.Errorf("finished_at is only allowed for terminal states")
	}

	errorType := ""
	if req.GetErrorType() != commonv1.ErrorClass_ERROR_CLASS_UNSPECIFIED {
		errorType, err = errorClassToString(req.GetErrorType())
		if err != nil {
			return transitionParams{}, err
		}
	}

	return transitionParams{
		RunID:        strings.TrimSpace(req.GetRunId()),
		FromState:    fromState,
		ToState:      toState,
		LeaseID:      strings.TrimSpace(req.GetLeaseId()),
		ErrorType:    errorType,
		ErrorMessage: strings.TrimSpace(req.GetErrorMessage()),
		FinishedAt:   finishedAt,
	}, nil
}

func parseFinishedAt(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, fmt.Errorf("finished_at must be RFC3339")
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func protoRunStateToDomain(state commonv1.RunState) (domain.RunState, error) {
	return convert.ProtoToRunState(state)
}

func domainRunStateToProto(state string) commonv1.RunState {
	return convert.RunStateStringToProto(state)
}

func autonomyModeToString(mode commonv1.AutonomyMode) (string, error) {
	return convert.ProtoToAutonomyModeString(mode)
}

func stringToAutonomyMode(value string) commonv1.AutonomyMode {
	return convert.AutonomyModeStringToProto(value)
}

func errorClassToString(value commonv1.ErrorClass) (string, error) {
	return convert.ProtoToErrorClassString(value)
}

func stringToErrorClass(value string) commonv1.ErrorClass {
	return convert.ErrorClassStringToProto(value)
}

func recordToProto(record Record) *sessionv1.RunRecord {
	finishedAt := ""
	if record.FinishedAt != nil {
		finishedAt = record.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	return &sessionv1.RunRecord{
		RunId:              record.RunID,
		SessionKey:         record.SessionKey,
		InputEventId:       record.InputEventID,
		Status:             record.Status,
		AutonomyMode:       stringToAutonomyMode(record.AutonomyMode),
		CurrentState:       domainRunStateToProto(record.CurrentState),
		ModelProvider:      record.ModelProvider,
		ProviderSessionRef: record.ProviderSessionRef,
		LeaseId:            record.LeaseID,
		ResumesRunId:       record.ResumesRunID,
		StartedAt:          record.StartedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:          record.UpdatedAt.UTC().Format(time.RFC3339Nano),
		FinishedAt:         finishedAt,
		ErrorType:          stringToErrorClass(record.ErrorType),
		ErrorMessage:       record.ErrorMessage,
		MetadataJson:       record.MetadataJSON,
	}
}

func newRunID(now time.Time) string {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return fmt.Sprintf("run-%d", now.UnixNano())
	}
	return fmt.Sprintf("run-%d-%s", now.UnixNano(), hex.EncodeToString(suffix[:]))
}
