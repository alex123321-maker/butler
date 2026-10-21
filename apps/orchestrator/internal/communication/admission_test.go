package communication_test

import (
	"errors"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
)

func TestCommunicationTrustedAdmissionRecoveryPinsProfileIntegration(t *testing.T) {
	f := setup(t)
	methods := []string{"communication.message", "communication.widget"}
	if e := f.s.AdmitRun(f.ctx, f.p.RunID, false, methods); e != nil {
		t.Fatal("same admission did not recover", e)
	}
	if e := f.s.AdmitRun(f.ctx, f.p.RunID, true, methods); !errors.Is(e, communication.ErrConflict) {
		t.Fatal("recovery changed quiet policy", e)
	}
	if e := f.s.AdmitRun(f.ctx, f.p.RunID, false, methods[:1]); !errors.Is(e, communication.ErrConflict) {
		t.Fatal("recovery changed catalog", e)
	}
}
