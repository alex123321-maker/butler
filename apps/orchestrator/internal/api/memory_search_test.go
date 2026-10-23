package api

import (
	"context"
	"errors"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
)

func TestOwnerSearchRejectsInvalidStrategyBeforeRetrieval(t *testing.T) {
	s := &MemoryCatalogServer{}
	for _, q := range []ownerSearchRequest{
		{Query: "x", Limit: 6, Strategy: "cloud"}, {Query: "x", Limit: 6, Offset: 1, Strategy: "hybrid"},
		{Limit: 6, Strategy: "hybrid"}, {Query: "x", Limit: 21}, {Query: "x", Limit: 6, Offset: 1001},
	} {
		if _, err := s.searchMemory(context.Background(), q); !errors.Is(err, catalog.ErrInvalid) {
			t.Fatal(q, err)
		}
	}
}
func TestOwnerSearchFusionKeepsLexicalCursorItemsAndDeduplicates(t *testing.T) {
	item := func(id string) catalog.BrowseItem { return catalog.BrowseItem{Source: catalog.Source{ID: id}} }
	items := mergeOwnerMatches([]catalog.BrowseItem{item("text"), item("both")}, []catalog.BrowseItem{item("semantic"), item("both"), item("semantic")})
	if len(items) != 3 || items[0].ID != "both" || items[0].Match != "both" {
		t.Fatal(items)
	}
	if items[1].ID != "text" || items[1].Match != "text" || items[2].Match != "semantic" {
		t.Fatal(items)
	}
}
