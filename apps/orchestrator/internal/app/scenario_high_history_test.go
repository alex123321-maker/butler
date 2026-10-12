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
	"time"

	"github.com/butler/butler/internal/acceptance"
)

const (
	highFailedAuditSHA       = "f00347b79875ca9f516c2308b66f70c851606ddfd058d00125adc45aab432f7b"
	highFailedPrivateSHA     = "383917dda36e8d1e1fc1c1d06fdf8ee5ceaad713e2668cf154d6f5baa15c4711"
	highFailedPGSHA          = "e732ad5b14daad1fe77acd4ee6dc9230109177dd9427c417fda31c21c3b5748f"
	highFailedCorrelationSHA = "9c374c2c10f5a87a4bee9767be38abfa41bf93cd6a29dac36b4226c4f4bf2fde"
	highFailedRelease        = "409847057692cf6a9a30b4cdbd6ef55fa7711ca0"
	highPriorHistorySHA      = "87d943b378ec8475f16a972dc52f9b85e3c80a29c49919912b6f897dee02a240"
)

func highHistoryPath(name string) string {
	return filepath.Join(shadowRepoRoot(), "deploy", "acceptance", name)
}

// Only immutable content-free artifacts are read. No private report, provider,
// credential, storage mutation, scoring or window admission is involved.
func readPinnedHighHistory(path, expectedSHA string, size int64, destination any) error {
	const limit = 256 * 1024
	stat, err := os.Lstat(path)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() != size || size > limit {
		return errNativeFixtureDenied
	}
	file, err := os.Open(path)
	if err != nil {
		return errNativeFixtureDenied
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(stat, opened) {
		return errNativeFixtureDenied
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	hash := sha256.Sum256(raw)
	if err != nil || int64(len(raw)) != size || hex.EncodeToString(hash[:]) != expectedSHA || json.Unmarshal(raw, destination) != nil {
		return errNativeFixtureDenied
	}
	return nil
}

func readHighHistoryArtifacts() (acceptance.CloudV2WindowAudit, acceptance.CloudV2PGCorrelation, error) {
	var audit acceptance.CloudV2WindowAudit
	var correlation acceptance.CloudV2PGCorrelation
	if readPinnedHighHistory(highHistoryPath("cloud-shadow-v2-high-20261012.json"), highFailedAuditSHA, 23550, &audit) != nil ||
		readPinnedHighHistory(highHistoryPath("cloud-shadow-v2-pg-correlation-20261012.json"), highFailedCorrelationSHA, 432, &correlation) != nil {
		return audit, correlation, errNativeFixtureDenied
	}
	return audit, correlation, nil
}

func validateHighHistory(history nativeHistoricalEvidence, a acceptance.CloudV2WindowAudit, p acceptance.CloudV2PGCorrelation) error {
	if history.BaselineSHA256 != highPriorHistorySHA || history.NativeAttempts != 228 || len(history.Artifacts) != 16 ||
		history.KnownUsagePairs != 203 || history.UnknownUsagePairs != 25 || len(history.Usage) != 228 ||
		a.Status != "NO_GO" || a.CollectorStatus != "failed" || a.EvidenceKind != "offline_v2_window_audit" ||
		a.OriginalReportSHA256 != highFailedPrivateSHA || a.HistorySHA256 != history.BaselineSHA256 ||
		a.PreviousPrivateSHA != shadowFailedPrivateSHA || a.PreviousAuditSHA != shadowFailedAuditSHA ||
		a.HistoricalAttempts != 228 || a.HistoricalArtifacts != 16 || a.HistoricalKnown != 203 || a.HistoricalUnknown != 25 ||
		a.HistoricalInput != 137405 || a.HistoricalOutput != 5293 || a.HistoricalTotal != 218644 || a.HistoricalNullTotal != 45 ||
		a.NewAttempts != 20 || a.CarryForwardAttempts != 248 || a.MaximumNew != 252 || a.MaximumTotal != 480 ||
		!a.CorrelationValid || a.Execution || len(a.Attempts) != 20 || !a.QualityValid || a.ComparisonValid ||
		a.Quality.Status != "NO_GO" || !a.Quality.GoldenVerified || a.Quality.Complete || a.Quality.ThresholdsPassed || a.Quality.ExecutionAuthorized ||
		a.Comparison.Status != "NO_GO" || a.Comparison.PairedTasks != 0 || a.Comparison.ThresholdsPassed || a.Comparison.ExecutionAuthorized ||
		a.ModelFamily != "observed_3.8_flash_family" || a.UnknownVersions != 1 ||
		p.Status != "PG_WITNESS_CORRELATION_MATCH" || p.ReportSHA256 != highFailedPrivateSHA || p.WitnessSHA256 != highFailedPGSHA ||
		p.TotalAttempts != 248 || p.NewAttempts != 20 || p.Rows != 20 || !p.Matched || p.QualityPassed || p.InferenceProven || p.Execution {
		return errNativeFixtureDenied
	}
	pin := a.Pin
	if pin.CodeCommit != highFailedRelease || pin.WindowID != acceptance.CloudV2AuditedWindow ||
		pin.SourceSHA256 != "7d08028f8dde629c8cec5f53184d973e86f33590f52be0da4df1ec3965543c6d" ||
		pin.HardwareSHA256 != "d3632dff9b14bb6cb710b9f909dfaa8fd7ea6ff313babd28db392ecaa7731c3a" ||
		pin.RuntimeSHA256 != "9e70fe8cd56036a37385ca2e5deae7e091c623d7973ee8c35137bbae5e5e58d9" ||
		pin.Runtime != "go1.27.2/race; sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61" ||
		pin.SchemaSHA256 != "8f4bcb0eb38a854c7307b3a54d36c4deac7779fe30024d216b6c37f14d8bbe4a" ||
		pin.PolicySHA256 != "d68d1a104c0cf108c9b78c5293ccfe792a92525b88920954ec4b095faa3b4632" ||
		pin.ResourcesSHA256 != "df078aa7d2a0a0410be62f7259fe49a93ad31f572dc2a002f5639950abc1ab2c" ||
		pin.CorpusDigest != "62768fbcc7f389825946bb1632884c1de5ef835644ae473a68bb1ffc3caca172" ||
		pin.EvidenceKind != "native_cloud_observation" || pin.Provider != "antigravity" || pin.RequestedModel != acceptance.CloudRequestedModel ||
		pin.ProviderModelVersion != nil || pin.WeightsStatus != "unavailable" || pin.WeightsSHA256 != nil || pin.Mode != "shadow_observation" ||
		a.Start.IsZero() || a.End.Before(a.Start) || !a.Expires.After(a.End) || a.Expires.After(a.Start.Add(75*time.Minute)) {
		return errNativeFixtureDenied
	}
	cost := a.Cost
	if cost.Attempts != 20 || cost.FullTokenCostKnown || cost.KnownInputTokens != 7141 || cost.KnownOutputTokens != 244 || cost.KnownTotalTokens != 31083 ||
		cost.UnknownInput != 1 || cost.UnknownOutput != 1 || cost.UnknownTotal != 1 || cost.Milliseconds != 379814 ||
		cost.RemoteMemoryStatus != "unavailable" || cost.RemoteMemoryBytes != nil || cost.InvoiceMicrounits != nil ||
		cost.BillingStatus != "subscription_incremental_invoice_unobserved" {
		return errNativeFixtureDenied
	}
	var in, out, total, milliseconds int64
	contexts := map[string]bool{}
	for i, attempt := range a.Attempts {
		if attempt.Ordinal != 229+i || attempt.Phase != "quality" || attempt.Role != acceptance.CloudRoleSingle ||
			!nativeAuditHash.MatchString(attempt.InputSHA256) || attempt.Context == "" || contexts[attempt.Context] ||
			attempt.Start.Before(a.Start) || attempt.End.After(a.End) || attempt.End.Before(attempt.Start) ||
			attempt.ChargedAt.Before(attempt.Start) || attempt.ChargedAt.After(attempt.End) ||
			attempt.Milliseconds != attempt.End.Sub(attempt.Start).Milliseconds() {
			return errNativeFixtureDenied
		}
		contexts[attempt.Context] = true
		milliseconds += attempt.Milliseconds
		if i == 19 {
			if attempt.Outcome != "timeout" || attempt.InputTokens != nil || attempt.OutputTokens != nil || attempt.TotalTokens != nil || attempt.ModelVersion != nil ||
				attempt.Milliseconds != 120027 || attempt.InputSHA256 != "a0b6cc914753e61c4d3085c463f574e5e8724cac42cf5e8ae4d4f8835b0672f6" {
				return errNativeFixtureDenied
			}
			continue
		}
		if attempt.Outcome != "completed" || attempt.InputTokens == nil || attempt.OutputTokens == nil || attempt.TotalTokens == nil ||
			attempt.ModelVersion == nil || *attempt.ModelVersion != "gemini-3.8-flash-n" ||
			*attempt.InputTokens < 0 || *attempt.InputTokens > 7141 || *attempt.OutputTokens < 0 || *attempt.OutputTokens > 244 ||
			*attempt.TotalTokens < *attempt.InputTokens+*attempt.OutputTokens || *attempt.TotalTokens > 31083 {
			return errNativeFixtureDenied
		}
		in += *attempt.InputTokens
		out += *attempt.OutputTokens
		total += *attempt.TotalTokens
	}
	if in != cost.KnownInputTokens || out != cost.KnownOutputTokens || total != cost.KnownTotalTokens || milliseconds != cost.Milliseconds {
		return errNativeFixtureDenied
	}
	return nil
}

// Historical accounting only: no caller is wired into admission. The failed
// V2 remains NO_GO; all twenty calls are spent, including the unknown timeout.
func readScenarioHighHistory() (nativeHistoricalEvidence, error) {
	history, err := readScenarioShadowHistory()
	audit, correlation, artifactErr := readHighHistoryArtifacts()
	if err != nil || artifactErr != nil || validateHighHistory(history, audit, correlation) != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	for _, attempt := range audit.Attempts {
		history.Usage = append(history.Usage, nativeUsageEvidence{RunID: attempt.Context,
			ReservationID: "shadow-v2-native-" + strconv.Itoa(attempt.Ordinal), ActualInputTokens: attempt.InputTokens, ActualOutputTokens: attempt.OutputTokens})
	}
	history.NativeAttempts = 248
	history.KnownUsagePairs += 19
	history.UnknownUsagePairs++
	hash := sha256.Sum256([]byte(history.BaselineSHA256 + "\n" + highFailedPrivateSHA + "\n" + highFailedAuditSHA + "\n" + highFailedPGSHA + "\n" + highFailedCorrelationSHA + "\n"))
	history.BaselineSHA256 = hex.EncodeToString(hash[:])
	history.Artifacts = append(history.Artifacts, nativePriorArtifact{ReportSHA256: highFailedAuditSHA, ReleaseRef: highFailedRelease, NewAttempts: 20})
	return history, nil
}

func TestScenarioHighHistoryCarriesEvery248SpentAttempt(t *testing.T) {
	history, err := readScenarioHighHistory()
	if err != nil || history.NativeAttempts != 248 || len(history.Usage) != 248 || len(history.Artifacts) != 17 || history.KnownUsagePairs != 222 || history.UnknownUsagePairs != 26 {
		t.Fatal("failed V2 accounting was not preserved", err)
	}
	var in, out int64
	unknown := 0
	for _, usage := range history.Usage {
		if usage.ActualInputTokens == nil || usage.ActualOutputTokens == nil {
			unknown++
			continue
		}
		in += *usage.ActualInputTokens
		out += *usage.ActualOutputTokens
	}
	if in != 144546 || out != 5537 || unknown != 26 || history.Usage[247].ActualInputTokens != nil || history.Usage[247].ReservationID != "shadow-v2-native-248" {
		t.Fatal("historical known subtotals or timeout NULL changed")
	}
	old, err := readScenarioShadowHistory()
	if err != nil || old.NativeAttempts != 228 || len(old.Artifacts) != 16 || old.BaselineSHA256 != highPriorHistorySHA {
		t.Fatal("old history changed")
	}
	if history.BaselineSHA256 != "9eae522a5dd3cd727d7e8a57f01cdd7958d08431d9d0d1ea484086f4225e01b2" {
		t.Fatal("additive provenance fold absent")
	}
}

func TestScenarioHighHistoryRejectsChangedOrMissingArtifacts(t *testing.T) {
	for _, fixture := range []struct {
		name, sha string
		size      int64
	}{
		{"cloud-shadow-v2-high-20261012.json", highFailedAuditSHA, 23550},
		{"cloud-shadow-v2-pg-correlation-20261012.json", highFailedCorrelationSHA, 432},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var destination any
			if readPinnedHighHistory(filepath.Join(t.TempDir(), "missing"), fixture.sha, fixture.size, &destination) == nil {
				t.Fatal("missing accepted")
			}
			raw, err := os.ReadFile(highHistoryPath(fixture.name))
			if err != nil {
				t.Fatal(err)
			}
			raw[0] = 'x'
			path := filepath.Join(t.TempDir(), "changed.json")
			if os.WriteFile(path, raw, 0600) != nil || readPinnedHighHistory(path, fixture.sha, fixture.size, &destination) == nil {
				t.Fatal("changed artifact accepted")
			}
		})
	}
}

func TestScenarioHighHistoryRejectsFalseCompletionCostOrProvenance(t *testing.T) {
	history, err := readScenarioShadowHistory()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*acceptance.CloudV2WindowAudit, *acceptance.CloudV2PGCorrelation)
	}{
		{"pass", func(a *acceptance.CloudV2WindowAudit, _ *acceptance.CloudV2PGCorrelation) { a.Status = "PASS" }},
		{"collector_pass", func(a *acceptance.CloudV2WindowAudit, _ *acceptance.CloudV2PGCorrelation) {
			a.CollectorStatus = "passed"
		}},
		{"omission", func(a *acceptance.CloudV2WindowAudit, _ *acceptance.CloudV2PGCorrelation) {
			a.Attempts = a.Attempts[:19]
		}},
		{"reordered", func(a *acceptance.CloudV2WindowAudit, _ *acceptance.CloudV2PGCorrelation) {
			a.Attempts[0], a.Attempts[1] = a.Attempts[1], a.Attempts[0]
		}},
		{"timeout_completed", func(a *acceptance.CloudV2WindowAudit, _ *acceptance.CloudV2PGCorrelation) {
			a.Attempts[19].Outcome = "completed"
		}},
		{"unknown_zero", func(a *acceptance.CloudV2WindowAudit, _ *acceptance.CloudV2PGCorrelation) {
			n := int64(0)
			a.Attempts[19].InputTokens = &n
		}},
		{"unknown_version_filled", func(a *acceptance.CloudV2WindowAudit, _ *acceptance.CloudV2PGCorrelation) {
			v := "gemini-3.8-flash-n"
			a.Attempts[19].ModelVersion = &v
		}},
		{"wrong_family", func(a *acceptance.CloudV2WindowAudit, _ *acceptance.CloudV2PGCorrelation) {
			v := "gemini-2.5-pro"
			a.Attempts[0].ModelVersion = &v
		}},
		{"changed_usage", func(a *acceptance.CloudV2WindowAudit, _ *acceptance.CloudV2PGCorrelation) {
			n := *a.Attempts[0].InputTokens + 1
			a.Attempts[0].InputTokens = &n
		}},
		{"changed_total", func(a *acceptance.CloudV2WindowAudit, _ *acceptance.CloudV2PGCorrelation) { a.Cost.KnownTotalTokens++ }},
		{"full_cost", func(a *acceptance.CloudV2WindowAudit, _ *acceptance.CloudV2PGCorrelation) {
			a.Cost.FullTokenCostKnown = true
		}},
		{"source", func(a *acceptance.CloudV2WindowAudit, _ *acceptance.CloudV2PGCorrelation) {
			a.Pin.SourceSHA256 = shadowFailedPrivateSHA
		}},
		{"policy", func(a *acceptance.CloudV2WindowAudit, _ *acceptance.CloudV2PGCorrelation) {
			a.Pin.PolicySHA256 = shadowFailedPrivateSHA
		}},
		{"history", func(a *acceptance.CloudV2WindowAudit, _ *acceptance.CloudV2PGCorrelation) {
			a.HistorySHA256 = shadowFailedPrivateSHA
		}},
		{"pg_changed", func(_ *acceptance.CloudV2WindowAudit, p *acceptance.CloudV2PGCorrelation) {
			p.WitnessSHA256 = shadowFailedPrivateSHA
		}},
		{"pg_no_match", func(_ *acceptance.CloudV2WindowAudit, p *acceptance.CloudV2PGCorrelation) { p.Matched = false }},
		{"pg_quality_claim", func(_ *acceptance.CloudV2WindowAudit, p *acceptance.CloudV2PGCorrelation) { p.QualityPassed = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, p, err := readHighHistoryArtifacts()
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(&a, &p)
			if validateHighHistory(history, a, p) == nil {
				t.Fatal("false historical truth accepted")
			}
		})
	}
}
