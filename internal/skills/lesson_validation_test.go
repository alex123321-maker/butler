package skills

import (
	"context"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
)

type lessonAuthority struct {
	*fixtureAuthority
	parents []catalog.SourceRef
	deny    bool
}

func (a *lessonAuthority) VerifyLessonLineage(context.Context, string, LessonRequest) ([]catalog.SourceRef, error) {
	if a.deny {
		return nil, ErrEvidence
	}
	return a.parents, nil
}

func TestSkillLessonVerificationCannotDowngradeOrInventParents(t *testing.T) {
	m, base := fixtureManifest(t)
	one := catalog.SourceRef{ID: "lesson-one", Revision: 1, AccessRevision: 1, Hash: validationHashFixture()}
	two := one
	two.ID = "lesson-two"
	m.LessonSources = []catalog.SourceRef{one, two}
	p := packageOf(t, m)
	if _, err := Validate(context.Background(), p, "owner", base, base); err == nil {
		t.Fatal("missing private lesson verifier accepted")
	}
	a := &lessonAuthority{fixtureAuthority: base, parents: []catalog.SourceRef{one, two}}
	if report, err := Validate(context.Background(), p, "owner", a, a); err != nil || len(report.LessonParents) != 2 {
		t.Fatal(err)
	}
	a.parents = []catalog.SourceRef{one}
	if _, err := Validate(context.Background(), p, "owner", a, a); err == nil {
		t.Fatal("incomplete proof accepted")
	}
	a.parents = []catalog.SourceRef{one, two}
	a.parents[1].Hash = one.ID
	if _, err := Validate(context.Background(), p, "owner", a, a); err == nil {
		t.Fatal("different parent accepted")
	}
	m.LessonSources = nil
	p = packageOf(t, m)
	a.deny = true
	if _, err := Validate(context.Background(), p, "owner", a, a); err == nil {
		t.Fatal("omitted lesson lineage fell back to legacy admission")
	}
}
func validationHashFixture() string {
	return "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
}
