package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/logger"
	redislib "github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	ErrLeaseConflict = fmt.Errorf("session lease already held")
	ErrLeaseNotFound = fmt.Errorf("lease not found")
)

type LeaseRecord struct {
	LeaseID    string    `json:"lease_id"`
	SessionKey string    `json:"session_key"`
	RunID      string    `json:"run_id"`
	OwnerID    string    `json:"owner_id"`
	TTLSeconds int64     `json:"ttl_seconds"`
	AcquiredAt time.Time `json:"acquired_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type LeaseManager interface {
	AcquireLease(ctx context.Context, params AcquireLeaseParams) (LeaseRecord, error)
	RenewLease(ctx context.Context, leaseID string, ttl time.Duration) (LeaseRecord, error)
	ReleaseLease(ctx context.Context, leaseID string) (bool, error)
}

type AcquireLeaseParams struct {
	SessionKey string
	RunID      string
	OwnerID    string
	TTL        time.Duration
}

type RedisLeaseManager struct {
	client *redislib.Client
	log    *slog.Logger
}

func NewRedisLeaseManager(client *redislib.Client, log *slog.Logger) *RedisLeaseManager {
	if log == nil {
		log = slog.Default()
	}
	return &RedisLeaseManager{
		client: client,
		log:    logger.WithComponent(log, "session-leases"),
	}
}

func (m *RedisLeaseManager) AcquireLease(ctx context.Context, params AcquireLeaseParams) (LeaseRecord, error) {
	if err := validateAcquireLeaseParams(params); err != nil {
		return LeaseRecord{}, err
	}

	now := time.Now().UTC()
	lease := LeaseRecord{
		LeaseID:    buildLeaseID(params.SessionKey, params.RunID, params.OwnerID),
		SessionKey: params.SessionKey,
		RunID:      params.RunID,
		OwnerID:    params.OwnerID,
		TTLSeconds: int64(params.TTL / time.Second),
		AcquiredAt: now,
		ExpiresAt:  now.Add(params.TTL),
	}
	payload, err := marshalLease(lease)
	if err != nil {
		return LeaseRecord{}, err
	}

	result, err := acquireLeaseScript.Run(ctx, m.client, []string{leaseKey(params.SessionKey), leaseIndexKey(lease.LeaseID)}, payload, params.TTL.Milliseconds(), params.SessionKey, params.RunID, params.OwnerID).Result()
	if err != nil {
		return LeaseRecord{}, fmt.Errorf("acquire lease: %w", err)
	}

	values, err := toResultSlice(result)
	if err != nil {
		return LeaseRecord{}, err
	}
	record, err := parseLeaseResult(values)
	if err != nil {
		return LeaseRecord{}, err
	}
	if values[0] == "-1" {
		m.log.Warn("lease conflict",
			slog.String("session_key", params.SessionKey),
			slog.String("run_id", params.RunID),
			slog.String("owner_id", params.OwnerID),
			slog.String("held_by_run_id", record.RunID),
		)
		return LeaseRecord{}, ErrLeaseConflict
	}

	created := values[0] == "1"
	m.log.Info("lease acquired",
		slog.String("lease_id", record.LeaseID),
		slog.String("session_key", record.SessionKey),
		slog.String("run_id", record.RunID),
		slog.String("owner_id", record.OwnerID),
		slog.Int64("ttl_seconds", record.TTLSeconds),
		slog.Bool("created", created),
	)

	return record, nil
}

func (m *RedisLeaseManager) RenewLease(ctx context.Context, leaseID string, ttl time.Duration) (LeaseRecord, error) {
	leaseID = strings.TrimSpace(leaseID)
	if leaseID == "" {
		return LeaseRecord{}, fmt.Errorf("lease_id is required")
	}
	if ttl <= 0 {
		return LeaseRecord{}, fmt.Errorf("ttl must be greater than zero")
	}

	result, err := renewLeaseScript.Run(ctx, m.client, []string{leaseIndexKey(leaseID)}, leaseID, ttl.Milliseconds(), int64(ttl/time.Second)).Result()
	if err != nil {
		return LeaseRecord{}, fmt.Errorf("renew lease: %w", err)
	}

	if result == nil {
		m.log.Warn("lease renew missed", slog.String("lease_id", leaseID))
		return LeaseRecord{}, ErrLeaseNotFound
	}

	record, err := unmarshalLease(result)
	if err != nil {
		return LeaseRecord{}, err
	}

	m.log.Info("lease renewed",
		slog.String("lease_id", record.LeaseID),
		slog.String("session_key", record.SessionKey),
		slog.Int64("ttl_seconds", record.TTLSeconds),
	)

	return record, nil
}

func (m *RedisLeaseManager) ReleaseLease(ctx context.Context, leaseID string) (bool, error) {
	leaseID = strings.TrimSpace(leaseID)
	if leaseID == "" {
		return false, fmt.Errorf("lease_id is required")
	}

	released, err := releaseLeaseScript.Run(ctx, m.client, []string{leaseIndexKey(leaseID)}, leaseID).Int()
	if err != nil {
		return false, fmt.Errorf("release lease: %w", err)
	}

	m.log.Info("lease released",
		slog.String("lease_id", leaseID),
		slog.Bool("released", released == 1),
	)

	return released == 1, nil
}

func (s *Server) AcquireLease(ctx context.Context, req *sessionv1.AcquireLeaseRequest) (*sessionv1.AcquireLeaseResponse, error) {
	if s.leases == nil {
		return nil, status.Error(codes.Unimplemented, "lease manager is not configured")
	}

	ttl := time.Duration(req.GetTtlSeconds()) * time.Second
	if ttl <= 0 {
		ttl = s.defaultLeaseTTL
	}

	record, err := s.leases.AcquireLease(ctx, AcquireLeaseParams{
		SessionKey: strings.TrimSpace(req.GetSessionKey()),
		RunID:      strings.TrimSpace(req.GetRunId()),
		OwnerID:    strings.TrimSpace(req.GetOwnerId()),
		TTL:        ttl,
	})
	if err != nil {
		return nil, leaseErrorToStatus(err)
	}

	return &sessionv1.AcquireLeaseResponse{Lease: leaseToProto(record)}, nil
}

func (s *Server) RenewLease(ctx context.Context, req *sessionv1.RenewLeaseRequest) (*sessionv1.RenewLeaseResponse, error) {
	if s.leases == nil {
		return nil, status.Error(codes.Unimplemented, "lease manager is not configured")
	}

	ttl := time.Duration(req.GetTtlSeconds()) * time.Second
	if ttl <= 0 {
		ttl = s.defaultLeaseTTL
	}

	record, err := s.leases.RenewLease(ctx, req.GetLeaseId(), ttl)
	if err != nil {
		return nil, leaseErrorToStatus(err)
	}

	return &sessionv1.RenewLeaseResponse{Lease: leaseToProto(record)}, nil
}

func (s *Server) ReleaseLease(ctx context.Context, req *sessionv1.ReleaseLeaseRequest) (*sessionv1.ReleaseLeaseResponse, error) {
	if s.leases == nil {
		return nil, status.Error(codes.Unimplemented, "lease manager is not configured")
	}

	released, err := s.leases.ReleaseLease(ctx, req.GetLeaseId())
	if err != nil {
		return nil, leaseErrorToStatus(err)
	}

	return &sessionv1.ReleaseLeaseResponse{Released: released}, nil
}

func leaseToProto(record LeaseRecord) *sessionv1.Lease {
	return &sessionv1.Lease{
		LeaseId:    record.LeaseID,
		SessionKey: record.SessionKey,
		RunId:      record.RunID,
		OwnerId:    record.OwnerID,
		TtlSeconds: record.TTLSeconds,
		AcquiredAt: record.AcquiredAt.UTC().Format(time.RFC3339Nano),
		ExpiresAt:  record.ExpiresAt.UTC().Format(time.RFC3339Nano),
	}
}

func leaseErrorToStatus(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrLeaseConflict):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, ErrLeaseNotFound):
		return status.Error(codes.NotFound, err.Error())
	case strings.Contains(err.Error(), "is required"), strings.Contains(err.Error(), "greater than zero"):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func validateAcquireLeaseParams(params AcquireLeaseParams) error {
	if strings.TrimSpace(params.SessionKey) == "" {
		return fmt.Errorf("session_key is required")
	}
	if strings.TrimSpace(params.RunID) == "" {
		return fmt.Errorf("run_id is required")
	}
	if strings.TrimSpace(params.OwnerID) == "" {
		return fmt.Errorf("owner_id is required")
	}
	if params.TTL <= 0 {
		return fmt.Errorf("ttl must be greater than zero")
	}
	return nil
}

func leaseKey(sessionKey string) string {
	return "butler:session_lease:" + sessionKey
}

func leaseIndexKey(leaseID string) string {
	return "butler:lease_index:" + leaseID
}

func buildLeaseID(sessionKey, runID, ownerID string) string {
	return strings.Join([]string{strings.TrimSpace(sessionKey), strings.TrimSpace(runID), strings.TrimSpace(ownerID)}, ":")
}

func marshalLease(record LeaseRecord) (string, error) {
	encoded, err := json.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("marshal lease: %w", err)
	}
	return string(encoded), nil
}

func unmarshalLease(value any) (LeaseRecord, error) {
	var payload string
	switch typed := value.(type) {
	case string:
		payload = typed
	case []byte:
		payload = string(typed)
	case nil:
		return LeaseRecord{}, ErrLeaseNotFound
	default:
		return LeaseRecord{}, fmt.Errorf("unexpected lease payload type %T", value)
	}

	var record LeaseRecord
	if err := json.Unmarshal([]byte(payload), &record); err != nil {
		return LeaseRecord{}, fmt.Errorf("unmarshal lease: %w", err)
	}
	return record, nil
}

func toResultSlice(value any) ([]string, error) {
	values, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected lease script result type %T", value)
	}
	result := make([]string, 0, len(values))
	for _, item := range values {
		switch typed := item.(type) {
		case string:
			result = append(result, typed)
		case []byte:
			result = append(result, string(typed))
		case int64:
			result = append(result, fmt.Sprintf("%d", typed))
		default:
			return nil, fmt.Errorf("unexpected lease script value type %T", item)
		}
	}
	return result, nil
}

func parseLeaseResult(values []string) (LeaseRecord, error) {
	if len(values) != 2 {
		return LeaseRecord{}, fmt.Errorf("unexpected lease script response length %d", len(values))
	}
	return unmarshalLease(values[1])
}

var acquireLeaseScript = redislib.NewScript(`
local existing = redis.call("GET", KEYS[1])
if not existing then
  redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
  redis.call("SET", KEYS[2], ARGV[3], "PX", ARGV[2])
  return {1, ARGV[1]}
end

local current = cjson.decode(existing)
if current.run_id == ARGV[4] and current.owner_id == ARGV[5] then
  redis.call("PEXPIRE", KEYS[1], ARGV[2])
  redis.call("SET", KEYS[2], ARGV[3], "PX", ARGV[2])
  return {0, existing}
end

return {-1, existing}
`)

var renewLeaseScript = redislib.NewScript(`
local sessionKey = redis.call("GET", KEYS[1])
if not sessionKey then
  return nil
end

local leaseKey = "butler:session_lease:" .. sessionKey
local existing = redis.call("GET", leaseKey)
if not existing then
  redis.call("DEL", KEYS[1])
  return nil
end

local current = cjson.decode(existing)
if current.lease_id ~= ARGV[1] then
  redis.call("DEL", KEYS[1])
  return nil
end

local now = redis.call("TIME")
local seconds = tonumber(now[1])
local micros = tonumber(now[2])
local ttlMillis = tonumber(ARGV[2])
local ttlSeconds = tonumber(ARGV[3])
local expiresAtMillis = seconds * 1000 + math.floor(micros / 1000) + ttlMillis
local expiresAtSeconds = math.floor(expiresAtMillis / 1000)
local expiresAtNanos = (expiresAtMillis % 1000) * 1000000

current.ttl_seconds = ttlSeconds
current.expires_at = os.date("!%Y-%m-%dT%H:%M:%S", expiresAtSeconds) .. string.format(".%09dZ", expiresAtNanos)

local payload = cjson.encode(current)
redis.call("SET", leaseKey, payload, "PX", ttlMillis)
redis.call("SET", KEYS[1], sessionKey, "PX", ttlMillis)
return payload
`)

var releaseLeaseScript = redislib.NewScript(`
local sessionKey = redis.call("GET", KEYS[1])
if not sessionKey then
  return 0
end

local leaseKey = "butler:session_lease:" .. sessionKey
local existing = redis.call("GET", leaseKey)
if not existing then
  redis.call("DEL", KEYS[1])
  return 0
end

local current = cjson.decode(existing)
if current.lease_id ~= ARGV[1] then
  return 0
end

redis.call("DEL", leaseKey)
redis.call("DEL", KEYS[1])
return 1
`)
