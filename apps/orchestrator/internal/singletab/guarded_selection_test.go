package singletab

import (
	"context"
	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/internal/singletabcontrol"
	"testing"
)

func TestSingleTabResourceLegacyActivationRejectsGuardedSelection(t *testing.T) {
	repo := &memoryApprovalStore{record: approvals.Record{ApprovalID: "opaque-approval", RunID: "run", SessionKey: "session", ApprovalType: approvals.ApprovalTypeBrowserTabSelection, Status: approvals.StatusPending, ToolName: singletabcontrol.BindTool}}
	sessions := newMemorySessionRepo()
	service := NewService(repo, sessions, &fakeGate{})
	if _, e := service.ActivateFromApproval(context.Background(), ActivateFromApprovalParams{ApprovalID: "opaque-approval", CandidateToken: "opaque-token"}); e == nil {
		t.Fatal("legacy activation accepted guarded request")
	}
	if repo.record.Status != approvals.StatusPending {
		t.Fatal("legacy rejection altered approval")
	}
	if _, e := sessions.GetActiveSessionBySessionKey(context.Background(), "session"); e != ErrSessionNotFound {
		t.Fatal("legacy rejection created binding")
	}
}
