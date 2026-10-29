package experience

import (
	"encoding/json"
	"errors"
	"github.com/butler/butler/internal/memory/readcontrol"
	"strings"
	"testing"
)

func TestExperienceReadPolicyNoveltyRangesAndFailures(t *testing.T) {
	ref := readcontrol.SourceRef{ID: "source", Revision: 1, AccessRevision: 1, Hash: strings.Repeat("a", 64)}
	encode := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	search := func(items ...readcontrol.Item) ReadObservation {
		r := readcontrol.Empty("empty")
		if len(items) > 0 {
			r.Status = "ok"
			r.Items = items
		}
		return ReadObservation{Tool: readcontrol.Search, ArgsJSON: `{"query":"different words"}`, ResultJSON: encode(r), Status: "completed", Confirmed: true}
	}
	open := func(offset int, text string, truncated bool) ReadObservation {
		r := readcontrol.Empty("ok")
		r.Items = []readcontrol.Item{{Source: ref, Text: text, Truncated: truncated}}
		return ReadObservation{Tool: readcontrol.Open, ArgsJSON: encode(readcontrol.Args{SourceID: ref.ID, Revision: 1, AccessRevision: 1, Hash: ref.Hash, Offset: offset, MaxChars: 1000}), ResultJSON: encode(r), Status: "completed", Confirmed: true}
	}
	args := func(offset, n int) string {
		return encode(readcontrol.Args{SourceID: ref.ID, Revision: 1, AccessRevision: 1, Hash: ref.Hash, Offset: offset, MaxChars: n})
	}
	for _, tc := range []struct {
		name      string
		history   []ReadObservation
		tool, raw string
		want      error
	}{
		{"two_empty", []ReadObservation{search(), search()}, readcontrol.Search, `{"query":"brand new phrasing"}`, ErrSearchBound},
		{"same_source_new_queries", []ReadObservation{search(readcontrol.Item{Source: ref}), search(readcontrol.Item{Source: ref}), search(readcontrol.Item{Source: ref})}, readcontrol.Search, `{"query":"fourth"}`, ErrSearchBound},
		{"open_after_discovery_stop", []ReadObservation{search(), search()}, readcontrol.Open, args(0, 1000), nil},
		{"covered_union_unicode", []ReadObservation{open(0, strings.Repeat("я", 500), true), open(500, strings.Repeat("я", 500), true)}, readcontrol.Open, args(100, 800), ErrRepeatedRead},
		{"overlap_with_new_suffix", []ReadObservation{open(0, strings.Repeat("я", 500), true)}, readcontrol.Open, args(400, 200), nil},
		{"gap_not_covered", []ReadObservation{open(0, strings.Repeat("я", 100), true), open(200, strings.Repeat("я", 100), true)}, readcontrol.Open, args(0, 300), nil},
		{"short_eof", []ReadObservation{open(0, "короткая запись", false)}, readcontrol.Open, args(0, 4096), ErrRepeatedRead},
		{"past_eof", []ReadObservation{open(0, "короткая запись", false)}, readcontrol.Open, args(99, 100), ErrRepeatedRead},
		{"other_task_action", []ReadObservation{search(), search()}, "extension.project.inspect", `{}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := CheckReadProposal(tc.history, tc.tool, tc.raw); !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
	failed := make([]ReadObservation, MaxReadCalls)
	for i := range failed {
		failed[i] = ReadObservation{Tool: readcontrol.Open, Status: "effect_unknown"}
	}
	if !errors.Is(CheckReadProposal(failed, readcontrol.Open, args(0, 100)), ErrReadBound) {
		t.Fatal("failures refunded read allowance")
	}
	unconfirmed := search(readcontrol.Item{Source: ref})
	unconfirmed.Confirmed = false
	if !errors.Is(CheckReadProposal([]ReadObservation{unconfirmed, unconfirmed}, readcontrol.Search, `{"query":"retry"}`), ErrSearchBound) {
		t.Fatal("unconfirmed data manufactured progress")
	}
	changed := ref
	changed.Revision++
	if err := CheckReadProposal([]ReadObservation{search(), search(readcontrol.Item{Source: changed})}, readcontrol.Search, `{"query":"next"}`); err != nil {
		t.Fatal("new source version did not count as discovery", err)
	}
}
