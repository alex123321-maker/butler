package utilitytrace

import (
	"errors"
	"sort"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/memory/readcontrol"
)

const PagedReadsVersion = "utility-trace-paged-reads-v1"

type sourcePages struct {
	body   []rune
	ref    acceptance.UtilityRef
	ranges [][2]int
}

// Source identity, current head, pin and permanent Broker evidence have already
// been verified by project. Check the request/result page geometry and bytes.
func (p *sourcePages) add(a readcontrol.Args, i readcontrol.Item) error {
	n := len([]rune(i.Text))
	end := a.Offset + n
	if a.FragmentKey != "" || i.FragmentKey != "" || i.Redacted || i.Conflict ||
		a.SourceID != i.Source.ID || a.Revision != i.Source.Revision || a.AccessRevision != i.Source.AccessRevision || a.Hash != i.Source.Hash ||
		a.Offset < 0 || a.Offset > len(p.body) || end > len(p.body) || n > a.MaxChars ||
		i.NextOffset != end || i.Truncated != (end < len(p.body)) ||
		string(p.body[a.Offset:end]) != i.Text || n == 0 && len(p.body) != 0 {
		return errors.New("paged source content or range mismatch")
	}
	p.ranges = append(p.ranges, [2]int{a.Offset, end})
	return nil
}

func (p *sourcePages) complete() bool {
	if len(p.ranges) == 0 {
		return false
	}
	sort.Slice(p.ranges, func(i, j int) bool { return p.ranges[i][0] < p.ranges[j][0] })
	next := 0
	for _, page := range p.ranges {
		if page[0] > next {
			return false
		}
		next = max(next, page[1])
	}
	return next == len(p.body)
}

func orderedPages(pages map[string]*sourcePages) []*sourcePages {
	keys := make([]string, 0, len(pages))
	for key := range pages {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]*sourcePages, 0, len(keys))
	for _, key := range keys {
		out = append(out, pages[key])
	}
	return out
}
