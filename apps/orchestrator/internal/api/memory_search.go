package api

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/butler/butler/internal/memory/catalog"
)

type OwnerMemorySearch interface {
	Candidates(context.Context, string) ([]catalog.BrowseItem, []string, error)
}
type ownerSearchRequest struct {
	Query    string `json:"query"`
	Limit    int    `json:"limit"`
	Offset   int    `json:"offset"`
	Strategy string `json:"strategy"`
}

func (s *MemoryCatalogServer) searchMemory(ctx context.Context, q ownerSearchRequest) (catalog.BrowsePage, error) {
	q.Query = strings.TrimSpace(q.Query)
	if !utf8.ValidString(q.Query) || len(q.Query) > 2048 || q.Limit < 1 || q.Limit > 20 || q.Offset < 0 || q.Offset > 1000 || q.Strategy != "" && q.Strategy != "text" && q.Strategy != "hybrid" || q.Strategy == "hybrid" && (q.Offset != 0 || q.Query == "") {
		return catalog.BrowsePage{}, catalog.ErrInvalid
	}
	var semantic []catalog.BrowseItem
	var diagnostics []string
	if q.Strategy == "hybrid" {
		diagnostics = []string{"semantic_unavailable"}
		if s.Search != nil {
			bounded, cancel := context.WithTimeout(ctx, 25*time.Second)
			var err error
			semantic, diagnostics, err = s.Search.Candidates(bounded, q.Query)
			cancel()
			if ctx.Err() != nil {
				return catalog.BrowsePage{}, ctx.Err()
			}
			if err != nil {
				semantic = nil
				diagnostics = []string{"semantic_unavailable"}
			}
		}
	}
	// Read lexical source heads after inference, never keep pre-inference text
	// across a potentially slow model call or return stale semantic candidates.
	page, err := s.Store.Browse(ctx, q.Query, q.Limit, q.Offset)
	if err != nil {
		return page, err
	}
	for i := range page.Items {
		page.Items[i].Match = "text"
	}
	if q.Strategy != "hybrid" {
		return page, nil
	}
	page.Diagnostics = diagnostics
	if len(semantic) > 0 {
		page.Items = mergeOwnerMatches(page.Items, semantic)
		page.Mode = "hybrid"
	} else if len(diagnostics) == 1 && diagnostics[0] == "semantic_indexed_only" {
		page.Diagnostics = append(diagnostics, "semantic_empty")
	}
	return page, nil
}

// Reciprocal ranks combine two different measures without pretending cosine
// distance and text rank are comparable. Keep every lexical item so next_offset
// can continue the original text cursor without omissions. At most 20+4 items.
func mergeOwnerMatches(text, semantic []catalog.BrowseItem) []catalog.BrowseItem {
	type ranked struct {
		item  catalog.BrowseItem
		score float64
	}
	byID := map[string]*ranked{}
	for li, list := range [][]catalog.BrowseItem{text, semantic} {
		seen := map[string]bool{}
		for rank, item := range list {
			if seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			found := byID[item.ID]
			if found == nil {
				label := "text"
				if li == 1 {
					label = "semantic"
				}
				item.Match = label
				found = &ranked{item: item}
				byID[item.ID] = found
			} else if li == 1 {
				found.item.Match = "both"
			}
			found.score += 1 / float64(60+rank+1)
		}
	}
	values := make([]*ranked, 0, len(byID))
	for _, value := range byID {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].score != values[j].score {
			return values[i].score > values[j].score
		}
		if values[i].item.Match != values[j].item.Match {
			return values[i].item.Match == "text"
		}
		return values[i].item.ID < values[j].item.ID
	})
	result := make([]catalog.BrowseItem, 0, len(values))
	for _, value := range values {
		result = append(result, value.item)
	}
	return result
}
