package toolcatalog

import (
	"fmt"
	"strings"
	"testing"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestToolSearchBeyond64PaginationAndStaleCursor(t *testing.T) {
	contracts := []*toolbrokerv1.ToolContract{}
	for i := 0; i < 130; i++ {
		contracts = append(contracts, &toolbrokerv1.ToolContract{ToolName: fmt.Sprintf("http.method_%03d", i), ToolClass: "http", InputSchemaJson: `{"type":"object"}`, Status: "active"})
	}
	last := SearchPage(contracts, Args{Query: "http.method_129"})
	if len(last.Items) != 1 || last.Items[0].Name != "http.method_129" {
		t.Fatal("late method unreachable", last)
	}
	seen := map[string]bool{}
	a := Args{Limit: 8}
	for {
		page := SearchPage(contracts, a)
		if page.Status != "ok" {
			t.Fatal(page)
		}
		for _, card := range page.Items {
			if seen[card.Name] {
				t.Fatal("duplicate page")
			}
			seen[card.Name] = true
		}
		if page.NextCursor == "" {
			break
		}
		a.Cursor = page.NextCursor
	}
	if len(seen) != 130 {
		t.Fatal("truncated catalog", len(seen))
	}
	first := SearchPage(contracts, Args{Limit: 8})
	contracts[0].ContractRevision = "2"
	if SearchPage(contracts, Args{Cursor: first.NextCursor}).Status != "stale_cursor" {
		t.Fatal("changed catalog reused old page")
	}
}

func TestToolSearchRussianAndExactRefs(t *testing.T) {
	c := &toolbrokerv1.ToolContract{ToolName: "schedule.create", ToolClass: "schedule_control", Description: "Persist a reminder", Status: "active"}
	for _, query := range []string{"напомни завтра", "создать напоминание", "таймер", "schedule.create"} {
		page := SearchPage([]*toolbrokerv1.ToolContract{c}, Args{Query: query})
		if len(page.Items) != 1 {
			t.Fatalf("query %q missed schedule", query)
		}
	}
	ref := Ref(c)
	c.RuntimeTarget = "different-runtime"
	if _, ok := Resolve([]*toolbrokerv1.ToolContract{c}, []string{ref}); ok {
		t.Fatal("reference silently upgraded")
	}
	if _, ok := Resolve(nil, []string{ref}); ok {
		t.Fatal("revoked method loaded")
	}
	for _, args := range []string{`{"tool_refs":[]}`, `{"tool_refs":["x"]}`, `{"query":"x","tool_refs":["` + strings.Repeat("x", 70) + `"]}`} {
		if _, err := Decode(Load, args); err == nil {
			t.Fatal("invalid load accepted", args)
		}
	}
	if _, err := Decode(Search, `{"query":"one","query":"two"}`); err == nil {
		t.Fatal("duplicate search arguments accepted")
	}
}

func TestToolSearchRussianOperationRanksBeforeCategoryOnlyMatch(t *testing.T) {
	contracts := []*toolbrokerv1.ToolContract{}
	for _, name := range []string{"browser.close", "browser.click", "browser.open", "browser.navigate"} {
		contracts = append(contracts, &toolbrokerv1.ToolContract{ToolName: name, ToolClass: "browser", Status: "active"})
	}
	page := SearchPage(contracts, Args{Query: "открыть страницу", Limit: 1})
	if len(page.Items) != 1 || page.Items[0].Name != "browser.open" {
		t.Fatal("operation intent lost in category matches", page)
	}
}
