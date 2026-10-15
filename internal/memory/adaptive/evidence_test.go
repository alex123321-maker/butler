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

func TestAdaptiveBrokerResultDigestPreservesEveryFieldAndNumber(t *testing.T) {
	a, err := brokerResultDigest([]byte(`{"text":"α","count":9007199254740993,"locator":{"timestamp":"T"}}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := brokerResultDigest([]byte(`{"locator":{"timestamp":"T"},"count":9007199254740993,"text":"α"}`))
	if err != nil || a != b {
		t.Fatal("key-order changed proof", err)
	}
	for _, raw := range []string{`{"text":"α","count":9007199254740992,"locator":{"timestamp":"T"}}`, `{"text":"α","count":9007199254740993,"locator":{"timestamp":"U"}}`} {
		c, err := brokerResultDigest([]byte(raw))
		if err != nil || c == a {
			t.Fatal("result evidence removed", err)
		}
	}
}

func TestAdaptivePublicReferenceSyntaxCannotInventCitations(t *testing.T) {
	a := Answer{Answer: "Supported [1]. URL https://example.test/path.", Citations: []Citation{{Claim: "One", Quote: "URL https://example.test/path"}}}
	if err := checkReferenceSyntax(a); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Invented [2]", "Invented [0]", "Invented [1,2]", "[label](https://example.test/path)", "[label] (https://example.test/path)", "URL https://example.test/else", "URL https://example.test/path.evil"} {
		bad := a
		bad.Answer = text
		if checkReferenceSyntax(bad) == nil {
			t.Fatalf("accepted %s", text)
		}
	}
	for _, text := range []string{"source_unopened", "read_unopened", "message:unopened"} {
		bad := a
		bad.Answer = text
		if checkPublicHandles(bad, map[string]bool{}) == nil {
			t.Fatal("undeclared handle", text)
		}
	}
	a.Answer = "source_opened read_opened message:opened"
	if err := checkPublicHandles(a, map[string]bool{"source_opened": true, "read_opened": true, "message:opened": true}); err != nil {
		t.Fatal(err)
	}
}

func TestAdaptiveFooterIdentityIsLiteralEvenWithMarkdownAndControlCharacters(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"fragment", "`\"fragment\"`"},
		{"`[link](https://example.invalid)`", "``\"`[link](https://example.invalid)`\"``"},
		{"line\nnext", "`\"line\\nnext\"`"},
	} {
		if got := inlineLiteral(tc.value); got != tc.want {
			t.Fatalf("%q => %q", tc.value, got)
		}
	}
}
