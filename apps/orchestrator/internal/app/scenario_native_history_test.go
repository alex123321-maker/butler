package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/butler/butler/internal/transport/antigravity"
)

const scenarioInitiativeReportSHA = "0ded1f418c3ecf9fad177fe5141350e8c5ca8002452fb462e3114219c4ab4ca9"

// Accounting preparation, not a new allowance. Every old failed and accepted
// report remains immutable; no provider, import, counter or marker is accessed.
func scenarioNativeHistoryPaths() []string {
	paths := initiativeHighHistoryPaths()
	return append(paths, filepath.Join(filepath.Dir(filepath.Dir(paths[13])), "initiatives", "native-high-20261012.json"))
}

func readScenarioNativeHistory(paths []string) (nativeHistoricalEvidence, error) {
	if len(paths) != 15 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	history, err := readInitiativeHighHistory(paths[:14])
	if err != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	raw, err := readHighRemainderPriorArtifact(paths[14], scenarioInitiativeReportSHA)
	var previous initiativeHighEvidence
	if err != nil || json.Unmarshal(raw, &previous) != nil || previous.Status != "passed" ||
		previous.Model != antigravity.FlashHighModel || previous.ReleaseRef != "250cf05be67b3ba19bcdcc6a7afabf13d5733e79" ||
		previous.NativeTotal != 45 || previous.NewAttempts != 4 || previous.MaxTotal != 45 || previous.MaxNew != 4 ||
		previous.TaskID == "" || previous.RunID == "" || previous.CheapChecks != 3 || !previous.CorePassed ||
		previous.ModelVerdict != "unverified" || previous.TaskStatus != "completed" ||
		previous.DigestStatus != "sent" || previous.DigestACK != "101" ||
		len(previous.Attempts) != 4 || len(previous.Usage) != 4 ||
		string(nativeJSON(previous.Historical)) != string(nativeJSON(history)) {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	reservations := map[string]bool{}
	for i, attempt := range previous.Attempts {
		kind := "tool_result"
		if i == 0 {
			kind = "start"
		}
		usage := previous.Usage[i]
		if attempt.Attempt != 42+i || attempt.Kind != kind || attempt.RunID != previous.RunID ||
			!nativeAuditHash.MatchString(attempt.InputHash) || attempt.CreatedAt.IsZero() ||
			i > 0 && attempt.CreatedAt.Before(previous.Attempts[i-1].CreatedAt) ||
			usage.RunID != previous.RunID || usage.ReservationID == "" || reservations[usage.ReservationID] {
			return nativeHistoricalEvidence{}, errNativeFixtureDenied
		}
		reservations[usage.ReservationID] = true
		if i < 3 {
			if usage.ActualInputTokens != nil || usage.ActualOutputTokens != nil {
				return nativeHistoricalEvidence{}, errNativeFixtureDenied
			}
		} else if usage.ActualInputTokens == nil || usage.ActualOutputTokens == nil ||
			*usage.ActualInputTokens != 5640 || *usage.ActualOutputTokens != 75 {
			return nativeHistoricalEvidence{}, errNativeFixtureDenied
		}
	}
	history.Usage = append(history.Usage, previous.Usage...)
	history.NativeAttempts = 45
	history.KnownUsagePairs++
	history.UnknownUsagePairs += 3
	if len(history.Usage) != 45 || history.KnownUsagePairs != 20 || history.UnknownUsagePairs != 25 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	baseline := sha256.Sum256([]byte(history.BaselineSHA256 + "\n" + scenarioInitiativeReportSHA + "\n"))
	history.BaselineSHA256 = hex.EncodeToString(baseline[:])
	history.Artifacts = append(history.Artifacts, nativePriorArtifact{
		ReportSHA256: scenarioInitiativeReportSHA, ReleaseRef: previous.ReleaseRef, NewAttempts: 4,
	})
	return history, nil
}

func TestScenarioNativeHistoryCarriesAll45Attempts(t *testing.T) {
	history, err := readScenarioNativeHistory(scenarioNativeHistoryPaths())
	if err != nil || history.NativeAttempts != 45 || len(history.Artifacts) != 15 ||
		history.KnownUsagePairs != 20 || history.UnknownUsagePairs != 25 || len(history.Usage) != 45 {
		t.Fatal("current immutable native history unavailable")
	}
	var input, output int64
	for _, usage := range history.Usage {
		if usage.ActualInputTokens == nil || usage.ActualOutputTokens == nil {
			if usage.ActualInputTokens != nil || usage.ActualOutputTokens != nil {
				t.Fatal("partial measured pair in immutable history")
			}
			continue
		}
		input += *usage.ActualInputTokens
		output += *usage.ActualOutputTokens
	}
	if input != 74698 || output != 2210 {
		t.Fatal("current known subtotal changed or NULL pairs were backfilled")
	}
	for _, incomplete := range [][]string{nil, scenarioNativeHistoryPaths()[:14]} {
		if _, err := readScenarioNativeHistory(incomplete); err != errNativeFixtureDenied {
			t.Fatal("incomplete native history allowed")
		}
	}
}

func TestScenarioNativeHistoryRejectsEveryMissingOrChangedPin(t *testing.T) {
	files := scenarioNativeHistoryPaths()
	for index, original := range files {
		t.Run(filepath.Base(original), func(t *testing.T) {
			paths := append([]string(nil), files...)
			copyPath := filepath.Join(t.TempDir(), "history.json")
			paths[index] = copyPath
			if _, err := readScenarioNativeHistory(paths); err != errNativeFixtureDenied {
				t.Fatal("missing historical artifact accepted")
			}
			raw, err := os.ReadFile(original)
			if err != nil || os.WriteFile(copyPath, raw, 0600) != nil {
				t.Fatal("public history copy unavailable")
			}
			if _, err := readScenarioNativeHistory(paths); err != nil {
				t.Fatal("exact public artifact copy denied")
			}
			if os.WriteFile(copyPath, append(raw, '\n'), 0600) != nil {
				t.Fatal("public tamper fixture unavailable")
			}
			if _, err := readScenarioNativeHistory(paths); err != errNativeFixtureDenied {
				t.Fatal("changed historical artifact accepted")
			}
		})
	}
}
