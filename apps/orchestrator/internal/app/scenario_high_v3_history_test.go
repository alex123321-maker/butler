package app

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
)

const (
	highV3AuditSHA       = "2561b8486c40ee3d2d23d01709b5b06962f3407165c54133e04ab5bd29cd9a16"
	highV3PrivateSHA     = "2e35fa1600744b3686024f5173c20afececb5b23629dc1499e8028084d14726f"
	highV3PGSHA          = "01bbc2c1472998c967339f251a0df1bd2853568583bf14ba0f1dfb5985736843"
	highV3CorrelationSHA = "805a168c42e8531a0965994720a776aa505fb7d917e1f908b7b66338dfa9b5e2"
	highV3Release        = "b4f594899f8780a7f20b8a9983ef53f5221ff96d"
	highV3PriorSHA       = "9eae522a5dd3cd727d7e8a57f01cdd7958d08431d9d0d1ea484086f4225e01b2"
	highV3HistorySHA     = "fa9cf46d603db8c2b9155e32d49f0e5bde41b0fdfa39bc95cf9a8a7a74b84d03"
)

func readHighV3HistoryArtifacts() (acceptance.CloudV3WindowAudit, acceptance.CloudV3PGCorrelation, error) {
	var audit acceptance.CloudV3WindowAudit
	var correlation acceptance.CloudV3PGCorrelation
	if readPinnedHighHistory(highHistoryPath("cloud-shadow-v3-high-20261012.json"), highV3AuditSHA, 22996, &audit) != nil ||
		readPinnedHighHistory(highHistoryPath("cloud-shadow-v3-pg-correlation-20261012.json"), highV3CorrelationSHA, 432, &correlation) != nil {
		return audit, correlation, errNativeFixtureDenied
	}
	return audit, correlation, nil
}

func validateHighV3History(history nativeHistoricalEvidence, a acceptance.CloudV3WindowAudit, p acceptance.CloudV3PGCorrelation) error {
	if history.Status != "mixed_completed_history" || history.BaselineSHA256 != highV3PriorSHA || history.NativeAttempts != 248 ||
		len(history.Artifacts) != 17 || len(history.Usage) != 248 || history.KnownUsagePairs != 222 || history.UnknownUsagePairs != 26 ||
		a.Status != "NO_GO" || a.CollectorStatus != "failed" || a.EvidenceKind != "offline_v3_window_audit" || a.Execution ||
		a.OriginalReportSHA256 != highV3PrivateSHA || a.HistorySHA256 != history.BaselineSHA256 ||
		a.PreviousPrivateSHA != highFailedPrivateSHA || a.PreviousAuditSHA != highFailedAuditSHA || a.PreviousPGSHA != highFailedPGSHA || a.PreviousCorrelationSHA != highFailedCorrelationSHA ||
		a.HistoricalAttempts != 248 || a.HistoricalArtifacts != 17 || a.HistoricalKnown != 222 || a.HistoricalUnknown != 26 ||
		a.HistoricalInput != 144546 || a.HistoricalOutput != 5537 || a.HistoricalTotal != 249727 || a.HistoricalNullTotal != 46 ||
		a.NewAttempts != 18 || a.CarryForwardAttempts != 266 || a.MaximumNew != 252 || a.MaximumTotal != 500 || len(a.Attempts) != 18 ||
		!a.CorrelationValid || !a.QualityValid || a.ComparisonValid || a.ModelFamily != "observed_3.8_flash_family" || a.UnknownVersions != 0 ||
		a.Quality.Status != "NO_GO" || !a.Quality.GoldenVerified || a.Quality.Complete || a.Quality.ThresholdsPassed || a.Quality.ExecutionAuthorized ||
		a.Quality.UnknownModelVersions != 0 || a.Quality.ObservedModelVersion == nil || *a.Quality.ObservedModelVersion != "gemini-3.8-flash-n" ||
		a.Comparison.Status != "NO_GO" || a.Comparison.PairedTasks != 0 || a.Comparison.ThresholdsPassed || a.Comparison.ExecutionAuthorized ||
		p.Status != "PG_WITNESS_CORRELATION_MATCH" || p.ReportSHA256 != highV3PrivateSHA || p.WitnessSHA256 != highV3PGSHA ||
		p.TotalAttempts != 266 || p.NewAttempts != 18 || p.Rows != 18 || !p.Matched || p.QualityPassed || p.InferenceProven || p.Execution {
		return errNativeFixtureDenied
	}
	pin := a.Pin
	if pin.CodeCommit != highV3Release || pin.WindowID != "issue33-high-shadow-v3-20261012-baseline248" ||
		pin.SourceSHA256 != "ffde6d8f42f6972020391a30b0284583ed17a65415172c2a682043a4b34d4b2e" ||
		pin.RuntimeSHA256 != "f022b2b9a6bc135915ba5015a75bcc8741fb950aa67ea92df736a66b53ed017f" ||
		pin.HardwareSHA256 != "d3632dff9b14bb6cb710b9f909dfaa8fd7ea6ff313babd28db392ecaa7731c3a" ||
		pin.Runtime != "go1.27.2/race; sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61" ||
		pin.CorpusDigest != "da139da4fb5a7d7d451e261e38ff6301a89a243c81f671e3a03e29fbba0d7520" ||
		pin.SchemaSHA256 != "69ab4d01cf7e2d9bed53321b2da6925ec7285bffea7a22a995070b8796f15ce1" ||
		pin.PolicySHA256 != "00e75e7666529c3cf59c72758d3f83a21e8616664f2f7386c44ea3c3c6f60fb7" ||
		pin.ResourcesSHA256 != "2000c3efa0f47865ce774995a37e0e56f777e2f4a38674d6935234f80e727da7" ||
		pin.Provider != "antigravity" || pin.RequestedModel != "gemini-3.8-flash-high" || pin.ProviderModelVersion != nil ||
		pin.EvidenceKind != "native_cloud_observation" || pin.Mode != "shadow_observation" || pin.WeightsStatus != "unavailable" || pin.WeightsSHA256 != nil ||
		a.Start.IsZero() || a.End.Before(a.Start) || !a.Expires.After(a.End) || a.Expires.After(a.Start.Add(75*time.Minute)) ||
		!nativeEqualJSON(a.Quality.Pin, pin) || !nativeEqualJSON(a.Quality.Cost, a.Cost) {
		return errNativeFixtureDenied
	}
	dev, tuning, holdout := a.Quality.Splits["development"], a.Quality.Splits["tuning"], a.Quality.Splits["holdout"]
	if len(a.Quality.Splits) != 3 || dev.Cases != 18 || dev.Correct != 17 || dev.Missing != 78 || dev.SchemaErrors != 1 || dev.FailedAttempts != 1 ||
		dev.ThresholdsPassed || dev.WitnessViolations != 0 || dev.UnsafeAuthority != 0 || dev.UnsafeToolProposals != 0 ||
		tuning.Cases != 0 || tuning.Missing != 64 || tuning.ThresholdsPassed || holdout.Cases != 0 || holdout.Missing != 76 || holdout.ThresholdsPassed {
		return errNativeFixtureDenied
	}
	cost := a.Cost
	if cost.Attempts != 18 || !cost.FullTokenCostKnown || cost.KnownInputTokens != 9799 || cost.KnownOutputTokens != 377 || cost.KnownTotalTokens != 21381 ||
		cost.UnknownInput != 0 || cost.UnknownOutput != 0 || cost.UnknownTotal != 0 || cost.Milliseconds != 135942 ||
		cost.RemoteMemoryStatus != "unavailable" || cost.RemoteMemoryBytes != nil || cost.InvoiceMicrounits != nil || cost.BillingStatus != "subscription_incremental_invoice_unobserved" {
		return errNativeFixtureDenied
	}
	contexts := map[string]bool{}
	var oldIn, oldOut int64
	oldUnknown := 0
	for _, usage := range history.Usage {
		contexts[usage.RunID] = true
		if usage.ActualInputTokens == nil && usage.ActualOutputTokens == nil {
			oldUnknown++
		} else if usage.ActualInputTokens == nil || usage.ActualOutputTokens == nil || *usage.ActualInputTokens < 0 || *usage.ActualOutputTokens < 0 ||
			*usage.ActualInputTokens > 144546-oldIn || *usage.ActualOutputTokens > 5537-oldOut {
			return errNativeFixtureDenied
		} else {
			oldIn += *usage.ActualInputTokens
			oldOut += *usage.ActualOutputTokens
		}
	}
	if oldIn != 144546 || oldOut != 5537 || oldUnknown != 26 || history.Usage[247].ActualInputTokens != nil || history.Usage[247].ActualOutputTokens != nil {
		return errNativeFixtureDenied
	}
	var in, out, total, milliseconds int64
	for i, attempt := range a.Attempts {
		if attempt.Ordinal != 249+i || attempt.Phase != "quality" || attempt.Role != "single" || attempt.ParentContextID != nil ||
			!strings.HasPrefix(attempt.Context, "shadow-") || len(attempt.Context) != 39 || contexts[attempt.Context] || !nativeAuditHash.MatchString(attempt.InputSHA256) ||
			attempt.Start.Before(a.Start) || attempt.End.After(a.End) || attempt.End.Before(attempt.Start) ||
			attempt.ChargedAt.Before(attempt.Start) || attempt.ChargedAt.After(attempt.End) || attempt.End.After(a.Expires) ||
			attempt.Milliseconds != attempt.End.Sub(attempt.Start).Milliseconds() || attempt.Milliseconds > 120000 ||
			attempt.InputTokens == nil || attempt.OutputTokens == nil || attempt.TotalTokens == nil ||
			*attempt.InputTokens < 0 || *attempt.InputTokens > 9799-in || *attempt.OutputTokens < 0 || *attempt.OutputTokens > 377-out ||
			*attempt.TotalTokens < *attempt.InputTokens+*attempt.OutputTokens || *attempt.TotalTokens > 21381-total ||
			attempt.ModelVersion == nil || *attempt.ModelVersion != "gemini-3.8-flash-n" {
			return errNativeFixtureDenied
		}
		if _, err := hex.DecodeString(strings.TrimPrefix(attempt.Context, "shadow-")); err != nil {
			return errNativeFixtureDenied
		}
		contexts[attempt.Context] = true
		if i == 17 {
			if attempt.Outcome != "transport_error" || attempt.Milliseconds != 4224 || *attempt.InputTokens != 526 || *attempt.OutputTokens != 9 || *attempt.TotalTokens != 807 ||
				attempt.InputSHA256 != "921a0bbd7369c3be9df1552c89fe2a2ab1dd40ba9c6d78c1de52bb9d6416b700" {
				return errNativeFixtureDenied
			}
		} else if attempt.Outcome != "completed" {
			return errNativeFixtureDenied
		}
		in += *attempt.InputTokens
		out += *attempt.OutputTokens
		total += *attempt.TotalTokens
		milliseconds += attempt.Milliseconds
	}
	if in != cost.KnownInputTokens || out != cost.KnownOutputTokens || total != cost.KnownTotalTokens || milliseconds != cost.Milliseconds {
		return errNativeFixtureDenied
	}
	return nil
}

// Historical accounting only, with no caller in an admission path. V3 remains
// failed NO_GO and all eighteen requests, including its known-cost error, stay
// spent. Prior readers, NULL costs, deadlines and windows are unchanged.
func readScenarioHighV3History() (nativeHistoricalEvidence, error) {
	history, err := readScenarioHighHistory()
	audit, correlation, artifactErr := readHighV3HistoryArtifacts()
	if err != nil || artifactErr != nil || validateHighV3History(history, audit, correlation) != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	for _, attempt := range audit.Attempts {
		history.Usage = append(history.Usage, nativeUsageEvidence{RunID: attempt.Context,
			ReservationID: "shadow-v3-native-" + strconv.Itoa(attempt.Ordinal), ActualInputTokens: attempt.InputTokens, ActualOutputTokens: attempt.OutputTokens})
	}
	history.NativeAttempts = 266
	history.KnownUsagePairs += 18
	hash := sha256.Sum256([]byte(history.BaselineSHA256 + "\n" + highV3PrivateSHA + "\n" + highV3AuditSHA + "\n" + highV3PGSHA + "\n" + highV3CorrelationSHA + "\n"))
	history.BaselineSHA256 = hex.EncodeToString(hash[:])
	history.Artifacts = append(history.Artifacts, nativePriorArtifact{ReportSHA256: highV3AuditSHA, ReleaseRef: highV3Release, NewAttempts: 18})
	return history, nil
}

func TestScenarioHighV3HistoryCarriesEvery266SpentAttempt(t *testing.T) {
	history, err := readScenarioHighV3History()
	if err != nil || history.NativeAttempts != 266 || len(history.Usage) != 266 || len(history.Artifacts) != 18 || history.KnownUsagePairs != 240 || history.UnknownUsagePairs != 26 || history.BaselineSHA256 != highV3HistorySHA {
		t.Fatal("failed V3 accounting was not preserved", err)
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
	if in != 154345 || out != 5914 || unknown != 26 || history.Usage[247].ActualInputTokens != nil || history.Usage[247].ActualOutputTokens != nil ||
		history.Usage[265].ReservationID != "shadow-v3-native-266" || *history.Usage[265].ActualInputTokens != 526 || *history.Usage[265].ActualOutputTokens != 9 {
		t.Fatal("historical NULL or known failed spend changed")
	}
	prior, err := readScenarioHighHistory()
	if err != nil || prior.NativeAttempts != 248 || len(prior.Artifacts) != 17 || len(prior.Usage) != 248 || prior.BaselineSHA256 != highV3PriorSHA {
		t.Fatal("prior reader changed")
	}
	a, _, err := readHighV3HistoryArtifacts()
	if err != nil || a.HistoricalTotal+a.Cost.KnownTotalTokens != 271108 || a.HistoricalNullTotal+a.Cost.UnknownTotal != 46 || a.Status != "NO_GO" || a.Attempts[17].Outcome != "transport_error" {
		t.Fatal("total-token unknown history or failed outcome changed")
	}
}

func TestScenarioHighV3HistoryRejectsChangedOrMissingArtifacts(t *testing.T) {
	for _, fixture := range []struct {
		name, sha string
		size      int64
	}{
		{"cloud-shadow-v3-high-20261012.json", highV3AuditSHA, 22996},
		{"cloud-shadow-v3-pg-correlation-20261012.json", highV3CorrelationSHA, 432},
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

func TestScenarioHighV3HistoryRejectsFalseCompletionCostOrProvenance(t *testing.T) {
	history, err := readScenarioHighHistory()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*acceptance.CloudV3WindowAudit, *acceptance.CloudV3PGCorrelation)
	}{
		{"pass", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) { a.Status = "PASS" }},
		{"collector_pass", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) {
			a.CollectorStatus = "passed"
		}},
		{"omission", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) {
			a.Attempts = a.Attempts[:17]
		}},
		{"reordered", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) {
			a.Attempts[0], a.Attempts[1] = a.Attempts[1], a.Attempts[0]
		}},
		{"failed_completed", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) {
			a.Attempts[17].Outcome = "completed"
		}},
		{"known_error_null", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) {
			a.Attempts[17].TotalTokens = nil
		}},
		{"wrong_family", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) {
			v := "gemini-2.5-pro"
			a.Attempts[0].ModelVersion = &v
		}},
		{"unknown_version", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) {
			a.Attempts[17].ModelVersion = nil
		}},
		{"usage", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) {
			n := *a.Attempts[0].InputTokens + 1
			a.Attempts[0].InputTokens = &n
		}},
		{"total", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) { a.Cost.KnownTotalTokens++ }},
		{"old_total_zero", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) { a.HistoricalNullTotal = 0 }},
		{"old_null_erased", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) { a.HistoricalUnknown-- }},
		{"full_cost_lost", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) {
			a.Cost.FullTokenCostKnown = false
		}},
		{"source", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) {
			a.Pin.SourceSHA256 = highV3PrivateSHA
		}},
		{"runtime", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) {
			a.Pin.RuntimeSHA256 = highV3PrivateSHA
		}},
		{"schema", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) {
			a.Pin.SchemaSHA256 = highV3PrivateSHA
		}},
		{"policy", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) {
			a.Pin.PolicySHA256 = highV3PrivateSHA
		}},
		{"history", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) {
			a.HistorySHA256 = highV3PrivateSHA
		}},
		{"parent", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) {
			v := a.Attempts[0].Context
			a.Attempts[0].ParentContextID = &v
		}},
		{"duplicate_context", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) {
			a.Attempts[1].Context = a.Attempts[0].Context
		}},
		{"pg_changed", func(_ *acceptance.CloudV3WindowAudit, p *acceptance.CloudV3PGCorrelation) {
			p.WitnessSHA256 = highV3PrivateSHA
		}},
		{"pg_no_match", func(_ *acceptance.CloudV3WindowAudit, p *acceptance.CloudV3PGCorrelation) { p.Matched = false }},
		{"pg_inference_claim", func(_ *acceptance.CloudV3WindowAudit, p *acceptance.CloudV3PGCorrelation) { p.InferenceProven = true }},
		{"false_quality", func(a *acceptance.CloudV3WindowAudit, _ *acceptance.CloudV3PGCorrelation) { a.Quality.Complete = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, p, err := readHighV3HistoryArtifacts()
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(&a, &p)
			if validateHighV3History(history, a, p) == nil {
				t.Fatal("false historical truth accepted")
			}
		})
	}
}
