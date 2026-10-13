package acceptance

import (
	"regexp"
	"sort"
	"strings"
)

type CloudV2LeakageFinding struct {
	CaseID    string `json:"case_id"`
	OtherID   string `json:"other_case_id"`
	Relation  string `json:"relation"`
	Component string `json:"component"`
}
type CloudV2LeakageReport struct {
	Status                     string                  `json:"status"`
	Findings                   []CloudV2LeakageFinding `json:"findings"`
	SemanticIndependenceProven bool                    `json:"semantic_independence_proven"`
	Limits                     []string                `json:"limits"`
}

var cloudV2Numbers = regexp.MustCompile(`[0-9]+([.,][0-9]+)?`)
var cloudV2Named = regexp.MustCompile(`\p{Lu}[\p{Ll}\p{N}_-]{2,}`)
var cloudV2Quoted = regexp.MustCompile(`«[^»]+»|"[^"\n]+"`)

func cloudV2LeakageNormalized(e Example, body string) string {
	ids := []string{e.ID, e.Group}
	for id := range e.Documents {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return len(ids[i]) > len(ids[j]) })
	for _, id := range ids {
		if id != "" {
			body = strings.ReplaceAll(body, id, " object ")
		}
	}
	body = cloudV2Quoted.ReplaceAllString(body, " object ")
	body = cloudV2Named.ReplaceAllString(body, " object ")
	body = cloudV2Numbers.ReplaceAllString(body, " number ")
	return normalized(body)
}

// Names/IDs/numbers cannot make copied wording independent. Capitalized and
// quoted replacements deliberately over-screen; a human/independent role must
// resolve false positives and semantic paraphrases. This never approves a split.
func ScreenCloudV2Leakage(c CloudV2Corpus, previous Corpus) CloudV2LeakageReport {
	r := CloudV2LeakageReport{Status: "SCREEN_ONLY_MANUAL_REVIEW_REQUIRED", Findings: []CloudV2LeakageFinding{}, Limits: []string{"Lexical similarity is a screening aid, not semantic independence.", "Quoted/capitalized normalization can over-match; inspect every finding and semantic family before approval.", "Old observed corpus overlap cannot become an independent V2 holdout by renaming objects."}}
	current := cloudV2Examples(c).Examples
	compare := func(a, b Example, relation string) {
		other := cloudV2CaseID(b.ID)
		if relation == "old_observed_overlap" {
			other = cloudCaseID(b.ID)
		}
		if near(cloudV2LeakageNormalized(a, a.Input), cloudV2LeakageNormalized(b, b.Input)) {
			r.Findings = append(r.Findings, CloudV2LeakageFinding{cloudV2CaseID(a.ID), other, relation, "input"})
		}
		for _, aBody := range a.Documents {
			for _, bBody := range b.Documents {
				if near(cloudV2LeakageNormalized(a, aBody), cloudV2LeakageNormalized(b, bBody)) {
					r.Findings = append(r.Findings, CloudV2LeakageFinding{cloudV2CaseID(a.ID), other, relation, "document"})
				}
			}
		}
	}
	for i, a := range current {
		for _, b := range current[:i] {
			if a.Split != b.Split {
				compare(a, b, "cross_split")
			}
		}
		for _, b := range previous.Examples {
			compare(a, b, "old_observed_overlap")
		}
	}
	return r
}
