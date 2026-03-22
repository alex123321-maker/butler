package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/run"
)

type fakeSystemTaskReader struct {
	byStatus map[string][]run.TaskRow
	err      error
}

func (f *fakeSystemTaskReader) ListTasks(_ context.Context, params run.TaskListParams) ([]run.TaskRow, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byStatus[params.Status], nil
}

type fakeSystemApprovalsRepo struct {
	items []approvals.Record
	err   error
}

func (f *fakeSystemApprovalsRepo) CreateApproval(ctx context.Context, params approvals.CreateParams) (approvals.Record, error) {
	return approvals.Record{}, nil
}
func (f *fakeSystemApprovalsRepo) GetApprovalByToolCallID(ctx context.Context, toolCallID string) (approvals.Record, error) {
	return approvals.Record{}, approvals.ErrApprovalNotFound
}
func (f *fakeSystemApprovalsRepo) GetApprovalByID(ctx context.Context, approvalID string) (approvals.Record, error) {
	return approvals.Record{}, approvals.ErrApprovalNotFound
}
func (f *fakeSystemApprovalsRepo) ListApprovals(ctx context.Context, status, runID, sessionKey string, limit, offset int) ([]approvals.Record, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}
func (f *fakeSystemApprovalsRepo) CreateTabCandidates(ctx context.Context, params []approvals.CreateTabCandidateParams) error {
	return nil
}
func (f *fakeSystemApprovalsRepo) ListTabCandidates(ctx context.Context, approvalID string) ([]approvals.TabCandidate, error) {
	return nil, nil
}
func (f *fakeSystemApprovalsRepo) SelectTabCandidate(ctx context.Context, approvalID, candidateToken string, selectedAt time.Time) (approvals.TabCandidate, error) {
	return approvals.TabCandidate{}, approvals.ErrTabCandidateNotFound
}
func (f *fakeSystemApprovalsRepo) ResolveApproval(ctx context.Context, params approvals.ResolveParams) (approvals.Record, error) {
	return approvals.Record{}, nil
}
func (f *fakeSystemApprovalsRepo) InsertEvent(ctx context.Context, event approvals.Event) error {
	return nil
}

func TestSystemServer_DegradedAndHealthyStates(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	server := NewSystemServer(
		nil,
		&fakeSystemTaskReader{byStatus: map[string][]run.TaskRow{"failed": {{RunID: "run-1", ErrorSummary: "error", UpdatedAt: now}}}},
		&fakeSystemApprovalsRepo{items: []approvals.Record{{ApprovalID: "a1", Status: approvals.StatusPending}}},
		"openai",
		true,
		true,
		"dual",
		true,
		90,
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v2/system", nil)
	recorder := httptest.NewRecorder()
	server.HandleGetSystem().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	health := payload["health"].(map[string]any)
	if health["status"] != "degraded" {
		t.Fatalf("expected degraded health, got %v", health["status"])
	}
	if payload["pending_approvals"].(float64) != 1 {
		t.Fatalf("expected pending approvals=1, got %v", payload["pending_approvals"])
	}
	singleTabExtension, ok := payload["single_tab_extension"].(map[string]any)
	if !ok {
		t.Fatalf("expected single_tab_extension section, got %T", payload["single_tab_extension"])
	}
	if singleTabExtension["transport_mode"] != "dual" {
		t.Fatalf("expected transport mode dual, got %v", singleTabExtension["transport_mode"])
	}
	if singleTabExtension["relay_enabled"] != true {
		t.Fatalf("expected relay_enabled=true, got %v", singleTabExtension["relay_enabled"])
	}
	instances, ok := singleTabExtension["instances"].([]any)
	if !ok {
		t.Fatalf("expected instances array, got %T", singleTabExtension["instances"])
	}
	if len(instances) != 0 {
		t.Fatalf("expected empty instances list without db, got %v", instances)
	}
}

func TestClassifyExtensionInstanceState(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	fresh := now.Add(-15 * time.Second)
	stale := now.Add(-5 * time.Minute)

	cases := []struct {
		name          string
		lastSeenAt    *time.Time
		active        int
		disconnected  int
		ttlSeconds    int
		expectedState string
	}{
		{
			name:          "unknown without heartbeat",
			lastSeenAt:    nil,
			active:        0,
			disconnected:  0,
			ttlSeconds:    90,
			expectedState: "unknown",
		},
		{
			name:          "online with fresh heartbeat",
			lastSeenAt:    &fresh,
			active:        1,
			disconnected:  0,
			ttlSeconds:    90,
			expectedState: "online",
		},
		{
			name:          "stale with old heartbeat",
			lastSeenAt:    &stale,
			active:        1,
			disconnected:  0,
			ttlSeconds:    90,
			expectedState: "stale",
		},
		{
			name:          "disconnected without active sessions",
			lastSeenAt:    &stale,
			active:        0,
			disconnected:  2,
			ttlSeconds:    90,
			expectedState: "disconnected",
		},
		{
			name:          "default ttl fallback",
			lastSeenAt:    &fresh,
			active:        1,
			disconnected:  0,
			ttlSeconds:    0,
			expectedState: "online",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state := classifyExtensionInstanceState(tc.lastSeenAt, tc.active, tc.disconnected, tc.ttlSeconds)
			if state != tc.expectedState {
				t.Fatalf("expected state %q, got %q", tc.expectedState, state)
			}
		})
	}
}

func TestSystemServer_ListExtensionInstancesWithoutDatabase(t *testing.T) {
	t.Parallel()

	server := NewSystemServer(nil, nil, nil, "openai", true, false, "remote_preferred", true, 90)
	request := httptest.NewRequest(http.MethodGet, "/api/v2/single-tab/extension-instances?limit=5&state=online,stale", nil)
	recorder := httptest.NewRecorder()
	server.HandleListExtensionInstances().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	items, ok := payload["items"].([]any)
	if !ok {
		t.Fatalf("expected items array, got %T", payload["items"])
	}
	if len(items) != 0 {
		t.Fatalf("expected empty items without db, got %v", items)
	}
	meta, ok := payload["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta section, got %T", payload["meta"])
	}
	if meta["limit"] != float64(5) {
		t.Fatalf("expected meta.limit=5, got %v", meta["limit"])
	}
	stateFilter, ok := meta["state_filter"].([]any)
	if !ok {
		t.Fatalf("expected state_filter array, got %T", meta["state_filter"])
	}
	if len(stateFilter) != 2 {
		t.Fatalf("expected 2 state filters, got %v", stateFilter)
	}
}

func TestSystemServer_ListExtensionInstancesRejectsInvalidStateFilter(t *testing.T) {
	t.Parallel()

	server := NewSystemServer(nil, nil, nil, "openai", true, false, "dual", true, 90)
	request := httptest.NewRequest(http.MethodGet, "/api/v2/single-tab/extension-instances?state=bad", nil)
	recorder := httptest.NewRecorder()
	server.HandleListExtensionInstances().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	errValue, _ := payload["error"].(string)
	if !strings.Contains(errValue, "state filter") {
		t.Fatalf("expected state filter validation error, got %q", errValue)
	}
}

func TestNormalizeExtensionInstanceLimit(t *testing.T) {
	t.Parallel()

	if got := normalizeExtensionInstanceLimit(""); got != defaultExtensionInstancesLimit {
		t.Fatalf("expected default limit %d, got %d", defaultExtensionInstancesLimit, got)
	}
	if got := normalizeExtensionInstanceLimit("0"); got != 1 {
		t.Fatalf("expected min limit 1, got %d", got)
	}
	if got := normalizeExtensionInstanceLimit("999"); got != maxExtensionInstancesLimit {
		t.Fatalf("expected max limit %d, got %d", maxExtensionInstancesLimit, got)
	}
	if got := normalizeExtensionInstanceLimit("17"); got != 17 {
		t.Fatalf("expected parsed limit 17, got %d", got)
	}
}

func TestParseExtensionInstanceStateFilter(t *testing.T) {
	t.Parallel()

	filter, states, err := parseExtensionInstanceStateFilter("online, stale,online")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filter) != 2 {
		t.Fatalf("expected 2 states in filter map, got %d", len(filter))
	}
	if len(states) != 2 || states[0] != "online" || states[1] != "stale" {
		t.Fatalf("unexpected state order: %v", states)
	}
	if _, _, err := parseExtensionInstanceStateFilter("invalid"); err == nil {
		t.Fatal("expected error for invalid state")
	}
}
