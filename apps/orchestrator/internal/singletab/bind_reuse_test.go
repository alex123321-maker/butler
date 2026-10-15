package singletab

import (
	"context"
	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"testing"
	"time"
)

func TestBindRequestCannotReuseForeignResolvedOrExpiredApproval(t *testing.T) {
	now := time.Now().UTC()
	expired := now.Add(-time.Second)
	for _, tc := range []struct {
		name, run, session, status string
		expiry                     *time.Time
	}{
		{"foreign-run", "other", "session", approvals.StatusPending, nil},
		{"foreign-session", "run", "other", approvals.StatusPending, nil},
		{"resolved", "run", "session", approvals.StatusApproved, nil},
		{"expired", "run", "session", approvals.StatusPending, &expired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &memoryApprovalStore{record: approvals.Record{ApprovalID: "approval", ToolCallID: "call", ApprovalType: approvals.ApprovalTypeBrowserTabSelection, RunID: tc.run, SessionKey: tc.session, Status: tc.status, ExpiresAt: tc.expiry}}
			svc := NewService(store, newMemorySessionRepo(), &fakeGate{})
			svc.SetApprovalCreator(&fakeApprovalCreator{})
			_, err := svc.CreateBindRequest(context.Background(), CreateBindRequestParams{RunID: "run", SessionKey: "session", ToolCallID: "call", RequestedAt: now, Candidates: []BindCandidateInput{{InternalTabRef: "browser-a:17"}}})
			if err == nil {
				t.Fatal("invalid historical approval reused")
			}
		})
	}
}
