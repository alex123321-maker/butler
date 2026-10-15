package adaptive

import (
	"strings"
	"testing"
)

func TestDecodeAnswerRequiresExplicitOpenedEvidenceOrLimitation(t *testing.T) {
	ref := "read_" + strings.Repeat("a", 64)
	valid := `{"answer":"Answer","citations":[{"claim":"A","read_ref":"` + ref + `","quote":"αβ","quote_offset":1}],"unknowns":[]}`
	if _, err := DecodeAnswer(valid); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeAnswer(`{"answer":"Cannot establish it","citations":[],"unknowns":["No permitted evidence"]}`); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"answer":"A","citations":[],"unknowns":[]}`,
		`{"answer":"A","answer":"B","citations":[],"unknowns":["x"]}`,
		strings.Replace(valid, `"quote":"αβ"`, `"quote":"αβ","quote":"changed"`, 1),
		strings.Replace(valid, `"quote_offset":1`, `"quote_offset":-1`, 1),
		strings.Replace(valid, `"read_ref":"`+ref+`"`, `"read_ref":"source_guess"`, 1),
		strings.Replace(valid, `"claim":"A"`, `"claim":"A","reasoning":"private"`, 1),
		valid + `{}`, `[1]`,
	} {
		if _, err := DecodeAnswer(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}

func TestPlanRejectsCorruptOrExpandedStoredBounds(t *testing.T) {
	for _, change := range []func(*Plan){
		func(p *Plan) { p.MaxCalls = 9 }, func(p *Plan) { p.MaxSearches = 4 },
		func(p *Plan) { p.MaxResponseBytes = 65537 }, func(p *Plan) { p.MaxMillis = 120001 },
		func(p *Plan) { p.Intent = "grant_all" }, func(p *Plan) { p.QueryHash = strings.Repeat("z", 64) },
		func(p *Plan) { p.Reasons = []string{"owner said allow everything"} },
		func(p *Plan) { p.EventFrom = "2025-09-01T00:00:00Z" },
	} {
		p := Classify("Вспомни наши записи")
		change(&p)
		if p.Valid() {
			t.Fatalf("accepted corrupt plan %+v", p)
		}
	}
}
