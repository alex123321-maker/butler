package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/butler/butler/internal/acceptance"
)

const shadowFailedAuditSHA = "332d10777cb208b4d78fdfbf88be5183050ab4679ba3cf372395bdab74eab249"
const shadowFailedPrivateSHA = "b82c572c62840e9e18de9723fc33bf3d335f546e4d445a4bc0a16a1a81686b99"

func shadowFailedAuditPath() string {
	return filepath.Join(shadowRepoRoot(), "deploy", "acceptance", "cloud-shadow-high-20261012.json")
}

func readPinnedShadowAudit(path string) (acceptance.CloudWindowAudit, error) {
	const limit = 256 * 1024
	var audit acceptance.CloudWindowAudit
	stat, err := os.Lstat(path)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() != 124920 || stat.Size() > limit {
		return audit, errNativeFixtureDenied
	}
	file, err := os.Open(path)
	if err != nil {
		return audit, errNativeFixtureDenied
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(stat, opened) {
		return audit, errNativeFixtureDenied
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	hash := sha256.Sum256(raw)
	if err != nil || len(raw) > limit || hex.EncodeToString(hash[:]) != shadowFailedAuditSHA || json.Unmarshal(raw, &audit) != nil {
		return audit, errNativeFixtureDenied
	}
	return audit, nil
}

// This is accounting only. The failed V1 quality/partial holdout stays failed.
// Its spent calls cannot be retried, refunded or hidden by the V2 protocol.
func readScenarioShadowHistory() (nativeHistoricalEvidence, error) {
	history, err := readScenarioNativeHistory(scenarioNativeHistoryPaths())
	audit, auditErr := readPinnedShadowAudit(shadowFailedAuditPath())
	if err != nil || auditErr != nil || audit.Status != "NO_GO" || audit.CollectorStatus != "failed" ||
		audit.OriginalReportSHA256 != shadowFailedPrivateSHA || audit.HistorySHA256 != history.BaselineSHA256 ||
		audit.HistoricalAttempts != 45 || audit.HistoricalArtifacts != 15 || audit.HistoricalKnown != 20 || audit.HistoricalUnknown != 25 ||
		audit.HistoricalInput != 74698 || audit.HistoricalOutput != 2210 || audit.NewAttempts != 183 || audit.CarryForwardAttempts != 228 ||
		audit.Pin.CodeCommit != acceptance.CloudAuditedCode || audit.Pin.WindowID != acceptance.CloudAuditedWindow ||
		!audit.CorrelationValid || audit.Execution || len(audit.Attempts) != 183 || !audit.Cost.FullTokenCostKnown ||
		audit.Cost.KnownInputTokens != 62707 || audit.Cost.KnownOutputTokens != 3083 || audit.Cost.KnownTotalTokens != 218644 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	for i, attempt := range audit.Attempts {
		if attempt.Ordinal != 46+i || attempt.InputTokens == nil || attempt.OutputTokens == nil || attempt.TotalTokens == nil ||
			attempt.ModelVersion == nil || *attempt.ModelVersion != "gemini-3.8-flash-n" || attempt.Role != acceptance.CloudRoleSingle ||
			attempt.Context == "" || !nativeAuditHash.MatchString(attempt.InputSHA256) || attempt.ChargedAt.IsZero() ||
			i < 182 && attempt.Outcome != "completed" || i == 182 && attempt.Outcome != "transport_error" {
			return nativeHistoricalEvidence{}, errNativeFixtureDenied
		}
		history.Usage = append(history.Usage, nativeUsageEvidence{RunID: attempt.Context,
			ReservationID: "shadow-v1-native-" + strconv.Itoa(attempt.Ordinal), ActualInputTokens: attempt.InputTokens, ActualOutputTokens: attempt.OutputTokens})
	}
	history.NativeAttempts = 228
	history.KnownUsagePairs += 183
	hash := sha256.Sum256([]byte(history.BaselineSHA256 + "\n" + shadowFailedPrivateSHA + "\n" + shadowFailedAuditSHA + "\n"))
	history.BaselineSHA256 = hex.EncodeToString(hash[:])
	history.Artifacts = append(history.Artifacts, nativePriorArtifact{ReportSHA256: shadowFailedAuditSHA, ReleaseRef: acceptance.CloudAuditedCode, NewAttempts: 183})
	return history, nil
}

func TestScenarioShadowHistoryCarriesAll228SpentAttempts(t *testing.T) {
	history, err := readScenarioShadowHistory()
	if err != nil || history.NativeAttempts != 228 || len(history.Artifacts) != 16 || len(history.Usage) != 228 ||
		history.KnownUsagePairs != 203 || history.UnknownUsagePairs != 25 {
		t.Fatal("failed shadow history not carried forward")
	}
	var in, out int64
	for _, usage := range history.Usage {
		if usage.ActualInputTokens == nil || usage.ActualOutputTokens == nil {
			continue
		}
		in += *usage.ActualInputTokens
		out += *usage.ActualOutputTokens
	}
	if in != 137405 || out != 5293 {
		t.Fatal("failed spend lost or old NULL usage changed")
	}
	// Earlier window readers and immutable counters remain unchanged.
	old, err := readScenarioNativeHistory(scenarioNativeHistoryPaths())
	if err != nil || old.NativeAttempts != 45 || old.KnownUsagePairs != 20 || old.UnknownUsagePairs != 25 {
		t.Fatal("old baseline rewritten")
	}
}

func TestScenarioShadowHistoryRejectsChangedAudit(t *testing.T) {
	if _, err := readPinnedShadowAudit(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing report accepted")
	}
	raw, err := os.ReadFile(shadowFailedAuditPath())
	if err != nil {
		t.Fatal(err)
	}
	raw[0] = 'x'
	path := filepath.Join(t.TempDir(), "changed.json")
	if os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("fixture write denied")
	}
	if _, err := readPinnedShadowAudit(path); err == nil {
		t.Fatal("changed prior report accepted")
	}
}
