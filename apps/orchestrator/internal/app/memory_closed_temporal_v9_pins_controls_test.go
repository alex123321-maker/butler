package app

import (
	"strings"
	"testing"
	"time"
)

func TestClosedV9PinsRejectDifferentProfile(t *testing.T) {
	now := time.Date(2026, 10, 18, 8, 0, 0, 0, time.UTC)
	previous := &temporalV5History{Hash: strings.Repeat("a", 64)}
	m := closedOwnerManifest{Version: 9, Day: "2026-10-18", Deadline: now.Truncate(24 * time.Hour).Add(24 * time.Hour), Previous: consolidationHistoryRef{SHA256: previous.Hash}}
	policy := mustTemporalJSON(t, temporalNativePolicy(previous.Carry))
	digest := strings.Repeat("b", 64)
	p := temporalNativePins{WindowRevision: 9, Revision: strings.Repeat("c", 40), SourceSHA: digest, CollectorSHA: digest, BrokerSHA: digest, Test2JSONSHA: digest, CorpusSHA: digest, ControlsSHA: digest, MethodSHA: digest, HistorySHA: previous.Hash, PolicySHA: experienceDigest(string(policy)), FactsRevision: "5", FactsSHA: closedV9FactsSHA, ScheduleSHA: digest, Day: m.Day, DayDeadline: m.Deadline, Expires: now.Add(time.Hour), Cases: 12, MaxNative: 144, MaxPerOriginal: 12, MaxTools: 8, Model: "gemini-3.8-flash-high", Output: 32768, Millis: 300000, ForegroundSourceMillis: 2000, ForegroundMillis: temporalV7ForegroundDuration.Milliseconds(), WholeMillis: temporalV7WholeDuration.Milliseconds(), OuterMillis: temporalV7OuterDuration.Milliseconds(), BrokerMillis: temporalV7BrokerDuration.Milliseconds()}
	if source, err := closedV9PinsWitness(mustTemporalJSON(t, p), m, previous); err != nil || source != p.Revision {
		t.Fatal("valid past finite seal rejected", err)
	}
	for _, name := range []string{"revision", "facts", "facts_digest", "native", "per_original", "output", "source_bound", "foreground", "whole", "outer", "broker", "day", "deadline", "expiry", "history", "policy", "source_digest", "collector_digest"} {
		t.Run(name, func(t *testing.T) {
			copy := p
			switch name {
			case "revision":
				copy.WindowRevision = 8
			case "facts":
				copy.FactsRevision = "4"
			case "facts_digest":
				copy.FactsSHA = digest
			case "native":
				copy.MaxNative = 145
			case "per_original":
				copy.MaxPerOriginal = 13
			case "output":
				copy.Output = 32769
			case "source_bound":
				copy.ForegroundSourceMillis = 200
			case "foreground":
				copy.ForegroundMillis++
			case "whole":
				copy.WholeMillis++
			case "outer":
				copy.OuterMillis++
			case "broker":
				copy.BrokerMillis++
			case "day":
				copy.Day = "2026-10-15"
			case "deadline":
				copy.DayDeadline = copy.DayDeadline.Add(time.Hour)
			case "expiry":
				copy.Expires = m.Deadline.Add(time.Second)
			case "history":
				copy.HistorySHA = digest
			case "policy":
				copy.PolicySHA = digest
			case "source_digest":
				copy.SourceSHA = strings.Repeat("z", 64)
			case "collector_digest":
				copy.CollectorSHA = ""
			}
			if _, err := closedV9PinsWitness(mustTemporalJSON(t, copy), m, previous); err == nil {
				t.Fatal("changed closed finite seal accepted")
			}
		})
	}
}
