package acceptance

import (
	"encoding/json"
	"strings"
)

// These additive diagnostics explain the frozen source_fabrications metric.
// They neither change its verdict nor infer that an anchored paraphrase is a
// fabricated fact. Counts refer to cases, and reason counts may overlap.
type CloudWitnessCounts struct {
	Cases                          int `json:"witness_contract_failure_cases"`
	InvalidSource                  int `json:"invalid_source_ref_cases"`
	DuplicateRef                   int `json:"duplicate_retrieval_ref_cases"`
	EmptyKey                       int `json:"empty_key_cases"`
	EmptyQuote                     int `json:"empty_quote_cases"`
	QuoteAbsent                    int `json:"quote_absent_from_original_cases"`
	ValueQuoteMismatch             int `json:"value_quote_mismatch_cases"`
	AnchoredValueQuoteMismatch     int `json:"anchored_value_quote_mismatch_cases"`
	OnlyAnchoredValueQuoteMismatch int `json:"only_anchored_value_quote_mismatch_cases"`
	OtherWitnessFailure            int `json:"other_witness_failure_cases"`
}

type CloudWitnessDiagnostics struct {
	Total  CloudWitnessCounts            `json:"total"`
	Splits map[string]CloudWitnessCounts `json:"splits"`
	Limits []string                      `json:"limits"`
}

func cloudWitnessReasons(e Example, raw []byte) CloudWitnessCounts {
	x := CloudWitnessCounts{}
	if !VerifyCloudResponse(e, raw).SourceFabrication {
		return x
	}
	x.Cases = 1
	set := func(n *int, condition bool) {
		if condition {
			*n = 1
		}
	}
	switch e.Kind {
	case "retrieval":
		var out struct {
			Refs []string `json:"refs"`
		}
		if DecodeStrict(raw, &out, "refs") != nil {
			break
		}
		seen := map[string]bool{}
		for _, id := range out.Refs {
			_, ok := e.Documents[id]
			set(&x.InvalidSource, !ok)
			set(&x.DuplicateRef, seen[id])
			seen[id] = true
		}
	case "extraction":
		var out struct {
			Facts []json.RawMessage `json:"facts"`
		}
		if DecodeStrict(raw, &out, "facts") != nil {
			break
		}
		for _, rawFact := range out.Facts {
			var f struct {
				Key      string `json:"key"`
				Value    string `json:"value"`
				SourceID string `json:"source_id"`
				Quote    string `json:"quote"`
			}
			if DecodeStrict(rawFact, &f, "key", "value", "source_id", "quote") != nil {
				continue
			}
			body, ok := e.Documents[f.SourceID]
			anchored := ok && f.Key != "" && f.Quote != "" && strings.Contains(body, f.Quote)
			set(&x.InvalidSource, !ok)
			set(&x.EmptyKey, f.Key == "")
			set(&x.EmptyQuote, f.Quote == "")
			set(&x.QuoteAbsent, ok && f.Quote != "" && !strings.Contains(body, f.Quote))
			set(&x.ValueQuoteMismatch, f.Value != f.Quote)
			set(&x.AnchoredValueQuoteMismatch, anchored && f.Value != f.Quote)
		}
	}
	if x.InvalidSource+x.DuplicateRef+x.EmptyKey+x.EmptyQuote+x.QuoteAbsent == 0 && x.AnchoredValueQuoteMismatch == 1 {
		x.OnlyAnchoredValueQuoteMismatch = 1
	} else {
		x.OtherWitnessFailure = 1
	}
	return x
}

func (c *CloudWitnessCounts) add(x CloudWitnessCounts) {
	c.Cases += x.Cases
	c.InvalidSource += x.InvalidSource
	c.DuplicateRef += x.DuplicateRef
	c.EmptyKey += x.EmptyKey
	c.EmptyQuote += x.EmptyQuote
	c.QuoteAbsent += x.QuoteAbsent
	c.ValueQuoteMismatch += x.ValueQuoteMismatch
	c.AnchoredValueQuoteMismatch += x.AnchoredValueQuoteMismatch
	c.OnlyAnchoredValueQuoteMismatch += x.OnlyAnchoredValueQuoteMismatch
	c.OtherWitnessFailure += x.OtherWitnessFailure
}

func cloudWitnessDiagnostics(c Corpus, attempts []CloudAttempt) CloudWitnessDiagnostics {
	r := CloudWitnessDiagnostics{Splits: map[string]CloudWitnessCounts{}, Limits: []string{
		"Diagnostics only; the original frozen scorer, goldens and release verdict are unchanged.",
		"Source-fabrications is the historical metric name for exact witness-contract violations, not a literal hallucination count.",
		"Reason counts can overlap; only anchored value/quote mismatch and other witness failure partition the cases.",
		"Anchored quote does not establish semantic equivalence of a differently worded value; no model judge or retuning is used.",
	}}
	selected := map[string]Example{}
	for _, e := range c.Examples {
		if !e.NoModel {
			selected[cloudCaseID(e.ID)] = e
		}
	}
	for split := range FrozenCloudPolicy().SplitCases {
		r.Splits[split] = CloudWitnessCounts{}
	}
	for _, a := range attempts {
		e, ok := selected[a.CaseID]
		if !ok || a.Outcome != "completed" {
			continue
		}
		x := cloudWitnessReasons(e, a.Output)
		r.Total.add(x)
		m := r.Splits[e.Split]
		m.add(x)
		r.Splits[e.Split] = m
	}
	return r
}
