package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/toolcatalog"
	"strings"
)

func TestToolDiscoveryThroughAuthorizedBroker(t *testing.T) {
	reg := loadTestRegistry(t)
	if err := reg.RegisterOwned(toolcatalog.Contracts()); err != nil {
		t.Fatal(err)
	}
	server := NewServer(reg, nil, nil, nil)
	f := setTestAuthority(server, nil)
	a, _ := f.GetActionContext(context.Background(), "run")
	a.InputClaimHash = strings.Repeat("a", 64)
	server.SetActionAuthorizer(actionauth.NewService(f, discoveryContext{authFixture: f, current: a}, nil))
	catalog := &TaskCatalog{Registry: reg, Contexts: converterCatalogContext{a}}
	server.SetScopedCatalog(catalog)
	server.SetCatalogExecutor(&ToolCatalogExecutor{Catalog: catalog})
	result := dispatchForTest(t, server, f, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "search", ToolName: toolcatalog.Search, ArgsJson: `{"query":"http.request"}`}, false)
	var page toolcatalog.Result
	if result.Status != "completed" || json.Unmarshal([]byte(result.ResultJson), &page) != nil || len(page.Items) != 1 {
		t.Fatalf("search failed: %+v %s", result, page.Status)
	}
	raw, _ := json.Marshal(toolcatalog.Args{Refs: []string{page.Items[0].Ref}})
	load := dispatchForTest(t, server, f, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "load", ToolName: toolcatalog.Load, ArgsJson: string(raw)}, false)
	if json.Unmarshal([]byte(load.ResultJson), &page) != nil || page.Status != "ok" || len(page.Loaded) != 1 {
		t.Fatal("load failed", load)
	}
	// A child can inspect only its fixed scope, even though the two read-only
	// discovery controls are standard kernel operations.
	a.Delegation = &delegation.Contract{}
	catalog.Contexts = converterCatalogContext{a}
	pageResult := dispatchForTest(t, server, f, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "child-search", ToolName: toolcatalog.Search, ArgsJson: `{"query":"http.request"}`}, false)
	if json.Unmarshal([]byte(pageResult.ResultJson), &page) != nil || page.Status != "empty" {
		t.Fatal("child discovered parent tool", pageResult)
	}
	a.DenyTools = []string{toolcatalog.Load}
	catalog.Contexts = converterCatalogContext{a}
	contracts, err := catalog.ListForRun(context.Background(), "run", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range contracts {
		if c.ToolName == toolcatalog.Load {
			t.Fatal("explicit denial ignored")
		}
	}
}

func TestToolCatalogDoesNotTruncateCoreBeforeSearch(t *testing.T) {
	reg := loadTestRegistry(t)
	contracts := []*toolbrokerv1.ToolContract{}
	for i := 0; i < 100; i++ {
		c := toolcatalog.Contract(toolcatalog.Search)
		c.ToolName = fmt.Sprintf("doctor.extra_%03d", i)
		contracts = append(contracts, c)
	}
	if err := reg.RegisterOwned(contracts); err != nil {
		t.Fatal(err)
	}
	a, _ := newAuthFixture().GetActionContext(context.Background(), "run")
	catalog := &TaskCatalog{Registry: reg, Contexts: converterCatalogContext{a}}
	all, err := catalog.ListForRun(context.Background(), "run", "", 0)
	if err != nil || len(all) < 100 {
		t.Fatal("core catalog truncated", len(all), err)
	}
}

type discoveryContext struct {
	*authFixture
	current actionauth.ContextSnapshot
}

func (d discoveryContext) GetActionContext(context.Context, string) (actionauth.ContextSnapshot, error) {
	return d.current, nil
}
