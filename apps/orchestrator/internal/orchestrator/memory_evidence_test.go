package orchestrator

import (
	"context"
	"errors"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
	"testing"
)

type evidencePolicyFixture struct{ err error }

func (p evidencePolicyFixture) CheckProposal(context.Context, string, string, string) error {
	return p.err
}

func TestMemoryEvidenceLimitsDeferToOwningReaderAndRecoveryGuidance(t *testing.T) {
	s := &Service{config: Config{MemoryEvidence: evidencePolicyFixture{experience.ErrSearchBound}}}
	var err error
	var stopped *budget.LoopError
	for _, limit := range []error{experience.ErrSearchBound, experience.ErrReadBound, experience.ErrRepeatedRead} {
		s.config.MemoryEvidence = evidencePolicyFixture{limit}
		if err = s.guardMemoryEvidenceProposal(context.Background(), "run", "memory.search", `{"query":"again"}`); err != nil {
			t.Fatal("optional limit stopped task before owning Broker result", err)
		}
	}
	if err = s.guardMemoryEvidenceProposal(context.Background(), "run", "extension.project.inspect", `{}`); err != nil {
		t.Fatal("optional read stop blocked independent task action", err)
	}
	private := errors.New("private storage diagnostic")
	for _, failure := range []error{private, errors.Join(private, experience.ErrSearchBound)} {
		s.config.MemoryEvidence = evidencePolicyFixture{failure}
		if err = s.guardMemoryEvidenceProposal(context.Background(), "run", "memory.search", `{"query":"x"}`); err == nil || err.Error() != "memory_evidence_state_unavailable" || !errors.Is(err, private) || errors.As(err, &stopped) {
			t.Fatal("storage/authority error exposed or reclassified", err)
		}
	}
	s.config.MemoryEvidence = evidencePolicyFixture{experience.ErrSearchBound}
	owner := content.CurrentOwnerMessage("Use only currently permitted independent proof", "input:owner")
	old := content.Instruction("obsolete", content.Origin{Kind: content.KernelInstruction, SourceRefs: []string{memoryEvidenceInstructionRef}})
	for n := 0; n < 2; n++ {
		p, err := s.refreshRecoveredToolContext(context.Background(), "run", []transport.InputItem{owner, old}, nil)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, item := range p.InputItems {
			if content.ItemOrigin(item).IsInstruction() {
				count++
				if item.Content != memoryEvidenceInstruction {
					t.Fatal("obsolete guidance survived")
				}
			}
		}
		if count != 1 || p.InputItems[0].Content != owner.Content {
			t.Fatal("guidance duplicated or owner changed")
		}
		old = p.InputItems[len(p.InputItems)-1]
	}
	disabled := &Service{}
	p, err := disabled.refreshRecoveredToolContext(context.Background(), "run", []transport.InputItem{owner, old}, nil)
	if err != nil || len(p.InputItems) != 1 {
		t.Fatal("disabled policy leaked guidance")
	}
}
