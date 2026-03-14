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
		RunID:         newRunID(now),
		SessionKey:    params.SessionKey,
		InputEventID:  params.InputEventID,
		Status:        string(domain.RunStateCreated),
		AutonomyMode:  params.AutonomyMode,
		CurrentState:  string(domain.RunStateCreated),
		ModelProvider: params.ModelProvider,
		ResumesRunID:  params.ResumesRunID,
		StartedAt:     now,
		UpdatedAt:     now,
		MetadataJSON:  params.MetadataJSON,
	})
	if err != nil {
		return nil, err
	}

	s.log.Info("run created",
		slog.String("run_id", record.RunID),
		slog.String("session_key", record.SessionKey),
		slog.String("state", record.CurrentState),
	)

	return recordToProto(record), nil
}

func (s *Service) GetRun(ctx context.Context, runID string) (*sessionv1.RunRecord, error) {
	record, err := s.repo.GetRun(ctx, strings.TrimSpace(runID))
	if err != nil {
		return nil, err
	}
	return recordToProto(record), nil
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
	SessionKey    string
	InputEventID  string
	AutonomyMode  string
	ModelProvider string
	ResumesRunID  string
	MetadataJSON  string
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
		SessionKey:    strings.TrimSpace(req.GetSessionKey()),
		InputEventID:  strings.TrimSpace(req.GetInputEvent().GetEventId()),
		AutonomyMode:  mode,
		ModelProvider: strings.TrimSpace(req.GetModelProvider()),
		ResumesRunID:  strings.TrimSpace(req.GetResumesRunId()),
		MetadataJSON:  strings.TrimSpace(req.GetMetadataJson()),
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
	switch state {
	case commonv1.RunState_RUN_STATE_CREATED:
		return domain.RunStateCreated, nil
	case commonv1.RunState_RUN_STATE_QUEUED:
		return domain.RunStateQueued, nil
	case commonv1.RunState_RUN_STATE_ACQUIRED:
		return domain.RunStateAcquired, nil
	case commonv1.RunState_RUN_STATE_PREPARING:
		return domain.RunStatePreparing, nil
	case commonv1.RunState_RUN_STATE_MODEL_RUNNING:
		return domain.RunStateModelRunning, nil
	case commonv1.RunState_RUN_STATE_TOOL_PENDING:
		return domain.RunStateToolPending, nil
	case commonv1.RunState_RUN_STATE_AWAITING_APPROVAL:
		return domain.RunStateAwaitingApproval, nil
	case commonv1.RunState_RUN_STATE_TOOL_RUNNING:
		return domain.RunStateToolRunning, nil
	case commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME:
		return domain.RunStateAwaitingModelResume, nil
	case commonv1.RunState_RUN_STATE_FINALIZING:
		return domain.RunStateFinalizing, nil
	case commonv1.RunState_RUN_STATE_COMPLETED:
		return domain.RunStateCompleted, nil
	case commonv1.RunState_RUN_STATE_FAILED:
		return domain.RunStateFailed, nil
	case commonv1.RunState_RUN_STATE_CANCELLED:
		return domain.RunStateCancelled, nil
	case commonv1.RunState_RUN_STATE_TIMED_OUT:
		return domain.RunStateTimedOut, nil
	default:
		return "", fmt.Errorf("run state is required")
	}
}

func domainRunStateToProto(state string) commonv1.RunState {
	switch domain.RunState(state) {
	case domain.RunStateCreated:
		return commonv1.RunState_RUN_STATE_CREATED
	case domain.RunStateQueued:
		return commonv1.RunState_RUN_STATE_QUEUED
	case domain.RunStateAcquired:
		return commonv1.RunState_RUN_STATE_ACQUIRED
	case domain.RunStatePreparing:
		return commonv1.RunState_RUN_STATE_PREPARING
	case domain.RunStateModelRunning:
		return commonv1.RunState_RUN_STATE_MODEL_RUNNING
	case domain.RunStateToolPending:
		return commonv1.RunState_RUN_STATE_TOOL_PENDING
	case domain.RunStateAwaitingApproval:
		return commonv1.RunState_RUN_STATE_AWAITING_APPROVAL
	case domain.RunStateToolRunning:
		return commonv1.RunState_RUN_STATE_TOOL_RUNNING
	case domain.RunStateAwaitingModelResume:
		return commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME
	case domain.RunStateFinalizing:
		return commonv1.RunState_RUN_STATE_FINALIZING
	case domain.RunStateCompleted:
		return commonv1.RunState_RUN_STATE_COMPLETED
	case domain.RunStateFailed:
		return commonv1.RunState_RUN_STATE_FAILED
	case domain.RunStateCancelled:
		return commonv1.RunState_RUN_STATE_CANCELLED
	case domain.RunStateTimedOut:
		return commonv1.RunState_RUN_STATE_TIMED_OUT
	default:
		return commonv1.RunState_RUN_STATE_UNSPECIFIED
	}
}

func autonomyModeToString(mode commonv1.AutonomyMode) (string, error) {
	switch mode {
	case commonv1.AutonomyMode_AUTONOMY_MODE_0:
		return string(domain.AutonomyMode0), nil
	case commonv1.AutonomyMode_AUTONOMY_MODE_1:
		return string(domain.AutonomyMode1), nil
	case commonv1.AutonomyMode_AUTONOMY_MODE_2:
		return string(domain.AutonomyMode2), nil
	case commonv1.AutonomyMode_AUTONOMY_MODE_3:
		return string(domain.AutonomyMode3), nil
	default:
		return "", fmt.Errorf("autonomy_mode is required")
	}
}

func stringToAutonomyMode(value string) commonv1.AutonomyMode {
	switch value {
	case string(domain.AutonomyMode0):
		return commonv1.AutonomyMode_AUTONOMY_MODE_0
	case string(domain.AutonomyMode1):
		return commonv1.AutonomyMode_AUTONOMY_MODE_1
	case string(domain.AutonomyMode2):
		return commonv1.AutonomyMode_AUTONOMY_MODE_2
	case string(domain.AutonomyMode3):
		return commonv1.AutonomyMode_AUTONOMY_MODE_3
	default:
		return commonv1.AutonomyMode_AUTONOMY_MODE_UNSPECIFIED
	}
}

func errorClassToString(value commonv1.ErrorClass) (string, error) {
	switch value {
	case commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR:
		return string(domain.ErrorClassValidation), nil
	case commonv1.ErrorClass_ERROR_CLASS_TRANSPORT_ERROR:
		return string(domain.ErrorClassTransport), nil
	case commonv1.ErrorClass_ERROR_CLASS_TOOL_ERROR:
		return string(domain.ErrorClassTool), nil
	case commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED:
		return string(domain.ErrorClassPolicy), nil
	case commonv1.ErrorClass_ERROR_CLASS_CREDENTIAL_ERROR:
		return string(domain.ErrorClassCredential), nil
	case commonv1.ErrorClass_ERROR_CLASS_APPROVAL_ERROR:
		return string(domain.ErrorClassApproval), nil
	case commonv1.ErrorClass_ERROR_CLASS_TIMEOUT:
		return string(domain.ErrorClassTimeout), nil
	case commonv1.ErrorClass_ERROR_CLASS_CANCELLED:
		return string(domain.ErrorClassCancelled), nil
	case commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR:
		return string(domain.ErrorClassInternal), nil
	default:
		return "", fmt.Errorf("error_type is invalid")
	}
}

func stringToErrorClass(value string) commonv1.ErrorClass {
	switch value {
	case string(domain.ErrorClassValidation):
		return commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR
	case string(domain.ErrorClassTransport):
		return commonv1.ErrorClass_ERROR_CLASS_TRANSPORT_ERROR
	case string(domain.ErrorClassTool):
		return commonv1.ErrorClass_ERROR_CLASS_TOOL_ERROR
	case string(domain.ErrorClassPolicy):
		return commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED
	case string(domain.ErrorClassCredential):
		return commonv1.ErrorClass_ERROR_CLASS_CREDENTIAL_ERROR
	case string(domain.ErrorClassApproval):
		return commonv1.ErrorClass_ERROR_CLASS_APPROVAL_ERROR
	case string(domain.ErrorClassTimeout):
		return commonv1.ErrorClass_ERROR_CLASS_TIMEOUT
	case string(domain.ErrorClassCancelled):
		return commonv1.ErrorClass_ERROR_CLASS_CANCELLED
	case string(domain.ErrorClassInternal):
		return commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR
	default:
		return commonv1.ErrorClass_ERROR_CLASS_UNSPECIFIED
	}
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
