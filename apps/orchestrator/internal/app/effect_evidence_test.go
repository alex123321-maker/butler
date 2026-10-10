package app

import (
	"context"
	"errors"
	"github.com/butler/butler/internal/actionauth"
	"testing"
)

type journalFixture struct{ err error }

func (f journalFixture) GetRunEvidence(context.Context, string) ([]actionauth.RunEffect, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []actionauth.RunEffect{{ActionID: "permanent-effect", EffectState: actionauth.StateUnknown, EvidenceRefs: []string{"evidence-digest"}}}, nil
}
func TestCompletionEvidenceKeepsPermanentJournalSemantics(t *testing.T) {
	result, err := (effectEvidence{journal: journalFixture{}}).ReadRunEffects(context.Background(), "run")
	if err != nil || len(result) != 1 || result[0].EffectState != "effect_unknown" || result[0].ActionID != "permanent-effect" || result[0].EvidenceRefs[0] != "evidence-digest" {
		t.Fatal("dispatch completion changed evidence", result, err)
	}
	if _, err = (effectEvidence{journal: journalFixture{err: errors.New("store unavailable")}}).ReadRunEffects(context.Background(), "run"); err == nil {
		t.Fatal("missing evidence hidden")
	}
}
