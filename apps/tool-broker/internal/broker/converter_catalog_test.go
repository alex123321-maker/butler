package broker

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/generatedtools"
)

type converterCatalogContext struct {
	context actionauth.ContextSnapshot
}

func (f converterCatalogContext) GetActionContext(context.Context, string) (actionauth.ContextSnapshot, error) {
	return f.context, nil
}

func (converterCatalogContext) VerifyActionApproval(context.Context, actionauth.ApprovalRequest) (actionauth.ApprovalSnapshot, error) {
	return actionauth.ApprovalSnapshot{}, fmt.Errorf("approval is absent from catalog-only fixture")
}

func TestConverterCatalogOmitsUnavailableCollectionPreservesDeveloperRoute(t *testing.T) {
	// Use the real owned registry constructor to avoid a separate schema mirror.
	reg := loadTestRegistry(t)
	if err := reg.RegisterOwned(generatedtools.Contracts()); err != nil {
		t.Fatal(err)
	}
	a, err := newAuthFixture().GetActionContext(context.Background(), "run")
	if err != nil {
		t.Fatal(err)
	}
	catalog := &TaskCatalog{Registry: reg, Contexts: converterCatalogContext{a}}
	contracts, err := catalog.ListForRun(context.Background(), "run", "", 64)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, c := range contracts {
		names[c.GetToolName()] = true
	}
	if names["converters.collect"] || !names["converters.discover"] || !names["converters.develop"] || !names["converters.test"] || !names["converters.trial"] || !names["converters.activate"] {
		t.Fatal("unavailable collection advertised or direct developer route removed")
	}
	permissions, err := generatedtools.DeveloperPermissions()
	if err != nil {
		t.Fatal(err)
	}
	a.Delegation = &delegation.Contract{TaskID: a.TaskID, RootTaskID: "root", ParentTaskID: "parent", ParentRevision: 1, ConstraintsRevision: 1, Revision: 1, Depth: 1, State: "executing", Deadline: time.Now().Add(time.Hour), Permissions: permissions}
	catalog.Contexts = converterCatalogContext{a}
	contracts, err = catalog.ListForRun(context.Background(), "run", "", 64)
	if err != nil || len(contracts) != 3 {
		t.Fatal("child's fixed three-tool route changed", err)
	}
	for _, c := range contracts {
		if c.GetToolName() != "converters.discover" && c.GetToolName() != "converters.develop" && c.GetToolName() != "converters.test" {
			t.Fatal("parent-only capability reached child")
		}
	}
}

func TestConverterDeveloperAdmissionDeclaresPhaseBoundary(t *testing.T) {
	description := generatedtools.DeveloperAdmissionContract().GetDescription()
	for _, fact := range []string{"finish this parent attempt", "without claiming the task is complete", "fresh parent attempt", "tested immutable final/report"} {
		if !strings.Contains(description, fact) {
			t.Fatal("missing parent-phase boundary")
		}
	}
}
