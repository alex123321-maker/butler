package readcontrol

import (
	"encoding/json"
	"testing"
)

func TestLimitedReadIsNotEmptyLookupOrEvidence(t *testing.T) {
	for _, reason := range []string{"memory_evidence_read_bound", "memory_evidence_search_exhausted", "memory_evidence_range_already_read"} {
		r := Limited(reason)
		b, _ := json.Marshal(r)
		got, err := DecodeResult(b)
		if err != nil || got.Status != "limited" || got.LimitReason != reason || len(got.Items) != 0 {
			t.Fatal("typed limit lost", got, err)
		}
	}
	for _, change := range []func(*Result){
		func(r *Result) { r.Status = "empty" },
		func(r *Result) { r.LimitReason = "" },
		func(r *Result) { r.LimitReason = "owner_revoked" },
		func(r *Result) { r.Items = []Item{{Text: "pretend evidence"}} },
		func(r *Result) { r.Truncated = true },
		func(r *Result) { r.UnknownValidity = 1 },
	} {
		r := Limited("memory_evidence_search_exhausted")
		change(&r)
		b, _ := json.Marshal(r)
		if _, err := DecodeResult(b); err == nil {
			t.Fatal("limit manufactured evidence or hid a fatal failure", string(b))
		}
	}
}
