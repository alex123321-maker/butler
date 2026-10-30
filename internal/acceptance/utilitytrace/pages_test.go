package utilitytrace

import (
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/readcontrol"
)

func TestPinnedPagesRequireCompleteUnicodeCoverage(t *testing.T) {
	body := []rune("А🙂BCDя")
	makePage := func(start, end int) (readcontrol.Args, readcontrol.Item) {
		a := readcontrol.Args{SourceID: "source", Revision: 1, AccessRevision: 2, Hash: strings.Repeat("a", 64), Offset: start, MaxChars: 8}
		i := readcontrol.Item{Source: readcontrol.SourceRef{ID: a.SourceID, Revision: 1, AccessRevision: 2, Hash: a.Hash}, Text: string(body[start:end]), NextOffset: end, Truncated: end < len(body)}
		return a, i
	}
	for _, tc := range []struct {
		name     string
		pages    [][2]int
		complete bool
	}{
		{"two unicode pages", [][2]int{{0, 2}, {2, 6}}, true},
		{"snapshot order independent", [][2]int{{2, 6}, {0, 2}}, true},
		{"exact overlap does not invent coverage", [][2]int{{0, 3}, {2, 6}}, true},
		{"duplicate page leaves gap", [][2]int{{0, 2}, {0, 2}, {3, 6}}, false},
		{"missing beginning", [][2]int{{2, 6}}, false},
		{"missing tail", [][2]int{{0, 2}}, false},
		{"one whole source", [][2]int{{0, 6}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &sourcePages{body: body}
			for _, span := range tc.pages {
				a, i := makePage(span[0], span[1])
				if err := p.add(a, i); err != nil {
					t.Fatal(err)
				}
			}
			if p.complete() != tc.complete {
				t.Fatal("incorrect full-source coverage")
			}
		})
	}
	for name, mutate := range map[string]func(*readcontrol.Args, *readcontrol.Item){
		"wrong bytes":          func(a *readcontrol.Args, i *readcontrol.Item) { i.Text = "XX" },
		"wrong offset":         func(a *readcontrol.Args, i *readcontrol.Item) { a.Offset = 1 },
		"wrong next":           func(a *readcontrol.Args, i *readcontrol.Item) { i.NextOffset++ },
		"wrong truncate":       func(a *readcontrol.Args, i *readcontrol.Item) { i.Truncated = false },
		"redacted":             func(a *readcontrol.Args, i *readcontrol.Item) { i.Redacted = true },
		"conflict":             func(a *readcontrol.Args, i *readcontrol.Item) { i.Conflict = true },
		"fragment coordinates": func(a *readcontrol.Args, i *readcontrol.Item) { a.FragmentKey = "chunk" },
		"foreign source":       func(a *readcontrol.Args, i *readcontrol.Item) { i.Source.ID = "other" },
		"wrong revision":       func(a *readcontrol.Args, i *readcontrol.Item) { i.Source.Revision++ },
		"wrong access":         func(a *readcontrol.Args, i *readcontrol.Item) { i.Source.AccessRevision++ },
		"wrong hash":           func(a *readcontrol.Args, i *readcontrol.Item) { i.Source.Hash = strings.Repeat("b", 64) },
		"over request bound":   func(a *readcontrol.Args, i *readcontrol.Item) { a.MaxChars = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			a, i := makePage(0, 2)
			mutate(&a, &i)
			if (&sourcePages{body: body}).add(a, i) == nil {
				t.Fatal("invalid page accepted")
			}
		})
	}
}
