package app

import (
	"encoding/json"
	"testing"

	"github.com/butler/butler/internal/memory/candidatecontrol"
)

func TestMemoryCandidateRejectsSensitiveData(t *testing.T) {
	for _, a := range []candidatecontrol.Args{
		{Key: "password", Value: json.RawMessage(`"synthetic-fixture-value"`)},
		{Key: "language", Value: json.RawMessage(`{"cookie":"synthetic-fixture-value"}`)},
		{Key: "language", Value: json.RawMessage(`"password is synthetic-fixture-value"`)},
		{Key: "language", Value: json.RawMessage(`"ru"`), Summary: "token=synthetic-fixture-value"},
	} {
		if safeCandidate(a) {
			t.Fatal("sensitive candidate accepted")
		}
	}
	if !safeCandidate(candidatecontrol.Args{Key: "language", Value: json.RawMessage(`{"preference":"ru","count":10000000000000000001}`), Summary: "unverified candidate"}) {
		t.Fatal("bounded ordinary DATA denied")
	}
}
