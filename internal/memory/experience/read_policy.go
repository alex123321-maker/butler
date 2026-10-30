package experience

import (
	"errors"
	"sort"
	"unicode/utf8"

	"github.com/butler/butler/internal/memory/readcontrol"
)

// These prospective limits bound optional reads, not task success or authority.
const MaxReadCalls = 8
const MaxSearchCalls = 3
const MaxUnproductiveSearches = 2

var ErrReadBound = errors.New("experience read allowance exhausted")
var ErrSearchBound = errors.New("experience discovery exhausted")
var ErrRepeatedRead = errors.New("experience range already read")

// Only exact optional-read limits are recoverable. A wrapped/joined storage or
// authority failure must never be downgraded into an allowance result.
func ReadLimitReason(err error) string {
	switch err {
	case ErrReadBound:
		return "memory_evidence_read_bound"
	case ErrSearchBound:
		return "memory_evidence_search_exhausted"
	case ErrRepeatedRead:
		return "memory_evidence_range_already_read"
	}
	return ""
}

type ReadObservation struct {
	Tool, ArgsJSON, ResultJSON, Status string
	Confirmed                          bool
}

type readRange struct{ start, end int }
type readSource struct {
	ref      readcontrol.SourceRef
	fragment string
}

// CheckReadProposal uses only durable receipts from this exact current run.
// Search discovery is distinct from opened source ranges. Failed/unconfirmed
// attempts spend calls but can never establish new evidence or read coverage.
// No semantic claim, task completion or negative fact is inferred from novelty.
func CheckReadProposal(history []ReadObservation, tool, raw string) error {
	if tool != readcontrol.Search && tool != readcontrol.Open && tool != readcontrol.Neighbors {
		return nil
	}
	a, err := readcontrol.DecodeArgs(tool, raw)
	if err != nil {
		return err
	}
	seen := map[readcontrol.SourceRef]bool{}
	opened := map[readSource][]readRange{}
	ends := map[readSource]int{}
	calls, searches, stagnant := 0, 0, 0
	for _, h := range history {
		if h.Tool != readcontrol.Search && h.Tool != readcontrol.Open && h.Tool != readcontrol.Neighbors {
			continue
		}
		calls++
		var result readcontrol.Result
		valid := h.Status == "completed" && h.Confirmed
		if valid {
			result, err = readcontrol.DecodeResult([]byte(h.ResultJSON))
			if err != nil {
				return ErrEvidence
			}
		}
		if h.Tool == readcontrol.Search {
			searches++
			novel := false
			for _, item := range result.Items {
				if item.Source.ID == "" || item.Source.Revision < 1 || item.Source.AccessRevision < 1 || !hexHash.MatchString(item.Source.Hash) {
					return ErrEvidence
				}
				if !seen[item.Source] {
					novel = true
					seen[item.Source] = true
				}
			}
			if novel {
				stagnant = 0
			} else {
				stagnant++
			}
		} else if valid && result.Status == "ok" {
			previous, err := readcontrol.DecodeArgs(h.Tool, h.ArgsJSON)
			if err != nil {
				return ErrEvidence
			}
			for _, item := range result.Items {
				if item.Source.ID != previous.SourceID || item.Source.Revision != previous.Revision || item.Source.AccessRevision != previous.AccessRevision || item.Source.Hash != previous.Hash {
					return ErrEvidence
				}
				if h.Tool == readcontrol.Open && item.FragmentKey != previous.FragmentKey {
					return ErrEvidence
				}
				start := previous.Offset
				if h.Tool == readcontrol.Neighbors {
					start = 0
				}
				key := readSource{item.Source, item.FragmentKey}
				opened[key] = append(opened[key], readRange{start, start + utf8.RuneCountInString(item.Text)})
				if !item.Truncated {
					ends[key] = start + utf8.RuneCountInString(item.Text)
				}
			}
		}
	}
	if calls >= MaxReadCalls {
		return ErrReadBound
	}
	if tool == readcontrol.Search && (searches >= MaxSearchCalls || stagnant >= MaxUnproductiveSearches) {
		return ErrSearchBound
	}
	if tool == readcontrol.Open {
		key := readSource{readcontrol.SourceRef{ID: a.SourceID, Revision: a.Revision, AccessRevision: a.AccessRevision, Hash: a.Hash}, a.FragmentKey}
		ranges := opened[key]
		sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
		position := a.Offset
		for _, interval := range ranges {
			if interval.start > position {
				break
			}
			if interval.end > position {
				position = interval.end
			}
		}
		end := a.Offset + a.MaxChars
		if eof, ok := ends[key]; ok && eof < end {
			end = eof
		}
		if position >= end {
			return ErrRepeatedRead
		}
	}
	return nil
}
