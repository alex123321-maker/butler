package broker

import (
	"context"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/singletabcontrol"
)

func TestResourceAdmissionsCannotSubstituteBrowserForWorkspace(t *testing.T) {
	ctx := context.Background()
	a := ResourceAdmissions{Browser: &GuardedTabAdmission{}}
	for _, name := range []string{"workspace.create", "workspace.unknown", "single_tab.status", "browser.snapshot"} {
		call := &toolbrokerv1.ToolCall{ToolName: name}
		g := actionauth.Grant{Binding: actionauth.ActionBinding{ToolName: name}}
		if _, err := a.PrepareContext(ctx, call, nil, g); err == nil {
			t.Fatal("missing protected store admitted", name)
		}
		if err := a.CheckAdmissionTx(ctx, nil, g); err == nil {
			t.Fatal("missing protected check admitted", name)
		}
		if err := a.ClaimActionTx(ctx, nil, g, "fixture-effect"); err == nil {
			t.Fatal("missing protected claim admitted", name)
		}
	}
	c := singletabcontrol.RequestContract()
	h, err := actionauth.ContractHash(c)
	if err != nil {
		t.Fatal(err)
	}
	g := actionauth.Grant{Binding: actionauth.ActionBinding{ToolName: singletabcontrol.BindTool, ContractHash: h}}
	if _, err = a.PrepareContext(ctx, &toolbrokerv1.ToolCall{ToolName: g.Binding.ToolName}, c, g); err != nil {
		t.Fatal("exact local binding", err)
	}
	if err = a.CheckAdmissionTx(ctx, nil, g); err != nil {
		t.Fatal(err)
	}
	if err = a.ClaimActionTx(ctx, nil, g, "fixture-effect"); err != nil {
		t.Fatal(err)
	}
	forged := g
	forged.Binding.ToolName = "workspace.create"
	if _, err = a.PrepareContext(ctx, &toolbrokerv1.ToolCall{ToolName: forged.Binding.ToolName}, c, forged); err == nil {
		t.Fatal("local browser binding crossed namespace")
	}
}
