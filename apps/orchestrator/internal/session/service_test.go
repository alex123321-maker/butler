package session

import (
	"context"
	"testing"
	"time"

	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCreateSessionCreatesNewRecord(t *testing.T) {
	repo := &memoryRepository{}
	server := NewServer(repo, nil, nil, time.Minute, nil)

	resp, err := server.CreateSession(context.Background(), &sessionv1.CreateSessionRequest{
		SessionKey:   "telegram:chat:42",
		UserId:       "user-42",
		Channel:      "telegram",
		MetadataJson: "{\"topic\":\"demo\"}",
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if !resp.GetCreated() {
		t.Fatal("expected created=true")
	}
	if resp.GetSession().GetSessionKey() != "telegram:chat:42" {
		t.Fatalf("unexpected session_key %q", resp.GetSession().GetSessionKey())
	}
	if resp.GetSession().GetMetadataJson() != "{\"topic\":\"demo\"}" {
		t.Fatalf("unexpected metadata_json %q", resp.GetSession().GetMetadataJson())
	}
	if repo.createCalls != 1 {
		t.Fatalf("expected single repository create call, got %d", repo.createCalls)
	}
}

func TestCreateSessionIsIdempotent(t *testing.T) {
	createdAt := time.Date(2026, time.March, 15, 10, 0, 0, 0, time.UTC)
	repo := &memoryRepository{
		records: map[string]SessionRecord{
			"telegram:chat:42": {
				SessionKey:   "telegram:chat:42",
				UserID:       "user-42",
				Channel:      "telegram",
				MetadataJSON: "{}",
				CreatedAt:    createdAt,
				UpdatedAt:    createdAt,
			},
		},
	}
	server := NewServer(repo, nil, nil, time.Minute, nil)

	resp, err := server.CreateSession(context.Background(), &sessionv1.CreateSessionRequest{
		SessionKey: "telegram:chat:42",
		UserId:     "user-42",
		Channel:    "telegram",
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if resp.GetCreated() {
		t.Fatal("expected created=false for existing session")
	}
	if resp.GetSession().GetCreatedAt() != createdAt.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected created_at %q", resp.GetSession().GetCreatedAt())
	}
}

func TestGetSessionReturnsStoredRecord(t *testing.T) {
	createdAt := time.Date(2026, time.March, 15, 11, 0, 0, 0, time.UTC)
	repo := &memoryRepository{
		records: map[string]SessionRecord{
			"web:user:abc": {
				SessionKey:   "web:user:abc",
				UserID:       "abc",
				Channel:      "web",
				MetadataJSON: "{}",
				CreatedAt:    createdAt,
				UpdatedAt:    createdAt,
			},
		},
	}
	server := NewServer(repo, nil, nil, time.Minute, nil)

	resp, err := server.GetSession(context.Background(), &sessionv1.GetSessionRequest{SessionKey: "web:user:abc"})
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if resp.GetSession().GetUserId() != "abc" {
		t.Fatalf("unexpected user_id %q", resp.GetSession().GetUserId())
	}
}

func TestResolveSessionKeyPrefersChatID(t *testing.T) {
	server := NewServer(&memoryRepository{}, nil, nil, time.Minute, nil)

	resp, err := server.ResolveSessionKey(context.Background(), &sessionv1.ResolveSessionKeyRequest{
		Channel:        "telegram",
		ExternalUserId: "user-42",
		ExternalChatId: "chat-99",
	})
	if err != nil {
		t.Fatalf("ResolveSessionKey returned error: %v", err)
	}
	if resp.GetSessionKey() != "telegram:chat:chat-99" {
		t.Fatalf("unexpected session key %q", resp.GetSessionKey())
	}
}

func TestGetSessionReturnsNotFound(t *testing.T) {
	server := NewServer(&memoryRepository{}, nil, nil, time.Minute, nil)

	_, err := server.GetSession(context.Background(), &sessionv1.GetSessionRequest{SessionKey: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}

type memoryRepository struct {
	records     map[string]SessionRecord
	createCalls int
}

func (r *memoryRepository) CreateSession(_ context.Context, params CreateSessionParams) (SessionRecord, bool, error) {
	r.createCalls++
	if r.records == nil {
		r.records = make(map[string]SessionRecord)
	}
	if existing, ok := r.records[params.SessionKey]; ok {
		return existing, false, nil
	}
	now := time.Date(2026, time.March, 15, 9, 0, 0, 0, time.UTC)
	record := SessionRecord{
		SessionKey:   params.SessionKey,
		UserID:       params.UserID,
		Channel:      params.Channel,
		MetadataJSON: params.MetadataJSON,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	r.records[params.SessionKey] = record
	return record, true, nil
}

func (r *memoryRepository) GetSessionByKey(_ context.Context, sessionKey string) (SessionRecord, error) {
	if r.records == nil {
		return SessionRecord{}, ErrSessionNotFound
	}
	record, ok := r.records[sessionKey]
	if !ok {
		return SessionRecord{}, ErrSessionNotFound
	}
	return record, nil
}

func TestNormalizeMetadataJSONRejectsInvalidJSON(t *testing.T) {
	_, err := normalizeMetadataJSON("{broken}")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if err.Error() != "metadata_json must be valid JSON" {
		t.Fatalf("unexpected error %q", err.Error())
	}
}

func TestAcquireLeaseUsesDefaultTTL(t *testing.T) {
	leases := &memoryLeaseManager{}
	server := NewServer(&memoryRepository{}, leases, nil, 45*time.Second, nil)

	resp, err := server.AcquireLease(context.Background(), &sessionv1.AcquireLeaseRequest{
		SessionKey: "telegram:chat:1",
		RunId:      "run-1",
		OwnerId:    "owner-1",
	})
	if err != nil {
		t.Fatalf("AcquireLease returned error: %v", err)
	}
	if resp.GetLease().GetTtlSeconds() != 45 {
		t.Fatalf("expected default ttl 45, got %d", resp.GetLease().GetTtlSeconds())
	}
}

func TestAcquireLeaseDetectsConflict(t *testing.T) {
	leases := &memoryLeaseManager{}
	server := NewServer(&memoryRepository{}, leases, nil, time.Minute, nil)

	_, err := server.AcquireLease(context.Background(), &sessionv1.AcquireLeaseRequest{
		SessionKey: "telegram:chat:1",
		RunId:      "run-1",
		OwnerId:    "owner-1",
		TtlSeconds: 60,
	})
	if err != nil {
		t.Fatalf("initial AcquireLease returned error: %v", err)
	}

	_, err = server.AcquireLease(context.Background(), &sessionv1.AcquireLeaseRequest{
		SessionKey: "telegram:chat:1",
		RunId:      "run-2",
		OwnerId:    "owner-2",
		TtlSeconds: 60,
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected lease conflict, got %v", err)
	}
}

func TestRenewLeaseExtendsExistingLease(t *testing.T) {
	leases := &memoryLeaseManager{}
	server := NewServer(&memoryRepository{}, leases, nil, time.Minute, nil)

	acquireResp, err := server.AcquireLease(context.Background(), &sessionv1.AcquireLeaseRequest{
		SessionKey: "telegram:chat:1",
		RunId:      "run-1",
		OwnerId:    "owner-1",
		TtlSeconds: 30,
	})
	if err != nil {
		t.Fatalf("AcquireLease returned error: %v", err)
	}

	renewResp, err := server.RenewLease(context.Background(), &sessionv1.RenewLeaseRequest{
		LeaseId:    acquireResp.GetLease().GetLeaseId(),
		TtlSeconds: 90,
	})
	if err != nil {
		t.Fatalf("RenewLease returned error: %v", err)
	}
	if renewResp.GetLease().GetTtlSeconds() != 90 {
		t.Fatalf("expected renewed ttl 90, got %d", renewResp.GetLease().GetTtlSeconds())
	}
}

func TestReleaseLeaseRemovesExistingLease(t *testing.T) {
	leases := &memoryLeaseManager{}
	server := NewServer(&memoryRepository{}, leases, nil, time.Minute, nil)

	acquireResp, err := server.AcquireLease(context.Background(), &sessionv1.AcquireLeaseRequest{
		SessionKey: "telegram:chat:1",
		RunId:      "run-1",
		OwnerId:    "owner-1",
		TtlSeconds: 60,
	})
	if err != nil {
		t.Fatalf("AcquireLease returned error: %v", err)
	}

	releaseResp, err := server.ReleaseLease(context.Background(), &sessionv1.ReleaseLeaseRequest{LeaseId: acquireResp.GetLease().GetLeaseId()})
	if err != nil {
		t.Fatalf("ReleaseLease returned error: %v", err)
	}
	if !releaseResp.GetReleased() {
		t.Fatal("expected released=true")
	}
}

func TestAcquireLeaseAllowsReacquireAfterExpiry(t *testing.T) {
	leases := newMemoryLeaseManager(time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC))
	server := NewServer(&memoryRepository{}, leases, nil, time.Minute, nil)

	_, err := server.AcquireLease(context.Background(), &sessionv1.AcquireLeaseRequest{
		SessionKey: "telegram:chat:1",
		RunId:      "run-1",
		OwnerId:    "owner-1",
		TtlSeconds: 10,
	})
	if err != nil {
		t.Fatalf("AcquireLease returned error: %v", err)
	}

	leases.Advance(11 * time.Second)

	resp, err := server.AcquireLease(context.Background(), &sessionv1.AcquireLeaseRequest{
		SessionKey: "telegram:chat:1",
		RunId:      "run-2",
		OwnerId:    "owner-2",
		TtlSeconds: 10,
	})
	if err != nil {
		t.Fatalf("AcquireLease after expiry returned error: %v", err)
	}
	if resp.GetLease().GetRunId() != "run-2" {
		t.Fatalf("expected reacquired lease for run-2, got %q", resp.GetLease().GetRunId())
	}
}

type memoryLeaseManager struct {
	leases map[string]LeaseRecord
	now    time.Time
}

func newMemoryLeaseManager(now time.Time) *memoryLeaseManager {
	return &memoryLeaseManager{now: now, leases: make(map[string]LeaseRecord)}
}

func (m *memoryLeaseManager) currentTime() time.Time {
	if m.now.IsZero() {
		return time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC)
	}
	return m.now
}

func (m *memoryLeaseManager) Advance(delta time.Duration) {
	m.now = m.currentTime().Add(delta)
}

func (m *memoryLeaseManager) AcquireLease(_ context.Context, params AcquireLeaseParams) (LeaseRecord, error) {
	if err := validateAcquireLeaseParams(params); err != nil {
		return LeaseRecord{}, err
	}
	if m.leases == nil {
		m.leases = make(map[string]LeaseRecord)
	}
	now := m.currentTime()
	if existing, ok := m.leases[params.SessionKey]; ok && existing.ExpiresAt.After(now) && (existing.RunID != params.RunID || existing.OwnerID != params.OwnerID) {
		return LeaseRecord{}, ErrLeaseConflict
	}
	record := LeaseRecord{
		LeaseID:    buildLeaseID(params.SessionKey, params.RunID, params.OwnerID),
		SessionKey: params.SessionKey,
		RunID:      params.RunID,
		OwnerID:    params.OwnerID,
		TTLSeconds: int64(params.TTL / time.Second),
		AcquiredAt: now,
		ExpiresAt:  now.Add(params.TTL),
	}
	m.leases[params.SessionKey] = record
	return record, nil
}

func (m *memoryLeaseManager) RenewLease(_ context.Context, leaseID string, ttl time.Duration) (LeaseRecord, error) {
	for sessionKey, record := range m.leases {
		if record.LeaseID != leaseID {
			continue
		}
		now := m.currentTime()
		if !record.ExpiresAt.After(now) {
			delete(m.leases, sessionKey)
			return LeaseRecord{}, ErrLeaseNotFound
		}
		record.TTLSeconds = int64(ttl / time.Second)
		record.ExpiresAt = now.Add(ttl)
		m.leases[sessionKey] = record
		return record, nil
	}
	return LeaseRecord{}, ErrLeaseNotFound
}

func (m *memoryLeaseManager) ReleaseLease(_ context.Context, leaseID string) (bool, error) {
	for sessionKey, record := range m.leases {
		if record.LeaseID == leaseID {
			delete(m.leases, sessionKey)
			return true, nil
		}
	}
	return false, nil
}
