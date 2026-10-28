package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/skills"
	"github.com/butler/butler/internal/transport"
)

type instructionSkillCatalog struct{}

func (instructionSkillCatalog) Index(context.Context, string) ([]skills.Descriptor, error) {
	return []skills.Descriptor{{ID: "fixture", Digest: "digest", Description: "UNTRUSTED_DESCRIPTION"}}, nil
}
func (instructionSkillCatalog) Pinned(context.Context, string) ([]skills.Package, error) {
	return []skills.Package{{Digest: "digest", Manifest: skills.Manifest{Description: "UNTRUSTED_PROCEDURE"}}}, nil
}

func TestSkillSelectionGuidanceKeepsProcedureDataAndOwnerConstraint(t *testing.T) {
	s := &Service{config: Config{Skills: instructionSkillCatalog{}}}
	owner := content.CurrentOwnerMessage("Do not use skills for this request", "input:owner")
	p := preparedRun{CurrentRunID: "run", Task: &task.Record{}, InputItems: []transport.InputItem{owner}}
	for i := 0; i < 2; i++ {
		if err := s.attachSkillContext(context.Background(), &p, false); err != nil {
			t.Fatal(err)
		}
	}
	count := 0
	for _, item := range p.InputItems {
		origin := content.ItemOrigin(item)
		if origin.IsInstruction() {
			count++
			if item.Content != skillSelectionInstruction || item.Role != "system" || strings.Contains(item.Content, "UNTRUSTED_") {
				t.Fatal("procedure prose became guidance")
			}
		}
	}
	if count != 1 || p.InputItems[0].Content != owner.Content {
		t.Fatal("duplicate guidance or changed owner request")
	}
	if !strings.Contains(skillSelectionInstruction, "prohibition") || !strings.Contains(skillSelectionInstruction, "already present") || !strings.Contains(skillSelectionInstruction, "revoked") {
		t.Fatal("selection omitted current owner, loaded or lifecycle restrictions")
	}
	old := content.Instruction("obsolete guidance", content.Origin{Kind: content.KernelInstruction, SourceRefs: []string{skillSelectionInstructionRef}, Sensitivity: content.Public})
	recovered, err := s.refreshRecoveredToolContext(context.Background(), "run", []transport.InputItem{owner, old}, nil)
	if err != nil {
		t.Fatal(err)
	}
	count = 0
	body := false
	for _, item := range recovered.InputItems {
		if content.ItemOrigin(item).IsInstruction() {
			count++
			if item.Content != skillSelectionInstruction {
				t.Fatal("old guidance survived recovery")
			}
		} else if strings.Contains(item.Content, "UNTRUSTED_PROCEDURE") {
			body = true
		}
	}
	if count != 1 || !body {
		t.Fatal("recovery omitted current guidance or pinned DATA")
	}
}
func TestSkillSelectionGuidanceRequiresConfiguredTaskContext(t *testing.T) {
	for _, s := range []*Service{{}, {config: Config{Skills: instructionSkillCatalog{}}}} {
		p := preparedRun{CurrentRunID: "run"}
		if err := s.attachSkillContext(context.Background(), &p, false); err != nil || len(p.InputItems) != 0 {
			t.Fatal("taskless or disabled context changed")
		}
	}
	s := &Service{}
	p := preparedRun{CurrentRunID: "run", Task: &task.Record{}}
	if err := s.attachSkillContext(context.Background(), &p, false); err != nil || len(p.InputItems) != 0 {
		t.Fatal("disabled skill context changed")
	}
}
