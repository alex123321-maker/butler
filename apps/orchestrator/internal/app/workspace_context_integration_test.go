package app

import (
	"testing"

	"github.com/butler/butler/internal/actionauth"
)

func TestWorkspaceOwningContextPinsCurrentInputIntegration(t *testing.T) {
	f := newToolControlDispatchFixture(t, nil, false, true)
	f.reader.memoryCandidateReady = false
	f.reader.workspaceReady = true
	c, err := f.reader.GetActionContext(f.f.ctx, f.call.RunId)
	if err != nil || !c.WorkspaceReady || c.InputClaimHash != actionauth.InputClaimHash(f.inputClaim.InputID, f.inputClaim.ClaimID) {
		t.Fatal("actual owning workspace input projection", err)
	}
	if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()-interval '1 second' WHERE run_id=$1`, f.call.RunId); err != nil {
		t.Fatal(err)
	}
	c, err = f.reader.GetActionContext(f.f.ctx, f.call.RunId)
	if err != nil || c.InputClaimHash != "" {
		t.Fatal("expired input projected authority", err)
	}
}
