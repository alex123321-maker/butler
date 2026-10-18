package app

import (
	"encoding/hex"
	"encoding/json"
	"errors"
)

const closedV9FactsSHA = "43ee9a114c83813205dd973bfe22e2f0f54982ce2b36de3bf23fa0bd8d05f49e"

// Closed accounting validates the original finite seal without requiring a
// past admission seal to remain unexpired. It never authorizes another call.
func closedV9PinsWitness(raw []byte, m closedOwnerManifest, previous *temporalV5History) (string, error) {
	var p temporalNativePins
	if previous == nil || json.Unmarshal(raw, &p) != nil || m.Version != 9 || p.WindowRevision != 9 || p.FactsRevision != "5" || p.FactsSHA != closedV9FactsSHA || p.Cases != 12 || p.MaxNative != 144 || p.MaxPerOriginal != 12 || p.MaxTools != 8 || p.Model != "gemini-3.8-flash-high" || p.Output != 32768 || p.Millis != 300000 || p.ForegroundSourceMillis != 2000 || p.ForegroundMillis != temporalV7ForegroundDuration.Milliseconds() || p.WholeMillis != temporalV7WholeDuration.Milliseconds() || p.OuterMillis != temporalV7OuterDuration.Milliseconds() || p.BrokerMillis != temporalV7BrokerDuration.Milliseconds() || p.HistorySHA != m.Previous.SHA256 || p.Day != m.Day || !p.DayDeadline.Equal(m.Deadline) || p.Expires.IsZero() || p.Expires.After(m.Deadline) {
		return "", errors.New("original closed V9 finite profile required")
	}
	for _, hash := range []string{p.Revision, p.SourceSHA, p.CollectorSHA, p.BrokerSHA, p.Test2JSONSHA, p.CorpusSHA, p.ControlsSHA, p.MethodSHA, p.HistorySHA, p.PolicySHA, p.FactsSHA, p.ScheduleSHA} {
		size := 64
		if hash == p.Revision {
			size = 40
		}
		if len(hash) != size {
			return "", errors.New("original closed V9 source digests required")
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return "", errors.New("invalid original closed V9 digest")
		}
	}
	policy, _ := json.Marshal(temporalNativePolicy(previous.Carry))
	if experienceDigest(string(policy)) != p.PolicySHA {
		return "", errors.New("closed V9 policy does not retain prior inclusive carry")
	}
	return p.Revision, nil
}
