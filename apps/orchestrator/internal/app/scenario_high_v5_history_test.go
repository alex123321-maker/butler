package app

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strconv"
	"testing"

	"github.com/butler/butler/internal/acceptance"
)

const highV5ClosedV4AuditSHA = "431e4371e7975604971019930c1f912eb9c610fa7ad316738cb50601ab8507cb"
const highV5ClosedV4PGSHA = "dc7689428e55575ce7023de8282da3f73192e1064845d385bd7776217d9b2f31"
const highV5ClosedV4DiagnosticsSHA = "02c7129de44b2980c1758c3061025792da5e37c6f0c26d58d781c3d9e4641ac7"
const highV5ClosedV4ReportSHA = "8c675d8136419a371142fa9e90c56423e7f1a216188e7fcc6d4ad4f25884ab8d"
const highV5ClosedV4Release = "925e1729923fd9865622c475893d7bffb21699e8"
const highV5HistorySHA = "5a91d598ae6f94a82fdab88d6305a214104d4ecd1bd59274546b1e4269071694"

type highV5ClosedV4Audit struct {
	Status      string                          `json:"status"`
	Report      string                          `json:"report_sha256"`
	PG          string                          `json:"pg_sha256"`
	Diagnostics string                          `json:"diagnostics_sha256"`
	Rows        int                             `json:"exact_diagnostic_rows"`
	Originals   int                             `json:"exact_private_originals"`
	Correct     int                             `json:"correct_original_witnesses"`
	Window      acceptance.CloudV4WindowAudit   `json:"window_audit"`
	Correlation acceptance.CloudV4PGCorrelation `json:"pg_correlation"`
	Calls       int                             `json:"auditor_native_calls"`
	Ready       bool                            `json:"issue33_ready"`
}

func readHighV5ClosedV4() (highV5ClosedV4Audit, acceptance.CloudV4PGWitness, error) {
	var a highV5ClosedV4Audit
	var p acceptance.CloudV4PGWitness
	var d any
	if readPinnedHighHistory(highHistoryPath("cloud-fresh-v4-final-audit-20261013.json"), highV5ClosedV4AuditSHA, 15764, &a) != nil ||
		readPinnedHighHistory(highHistoryPath("cloud-fresh-v4-pg-witness-20261013.json"), highV5ClosedV4PGSHA, 5836, &p) != nil ||
		readPinnedHighHistory(highHistoryPath("cloud-fresh-v4-pg-diagnostics-20261013.json"), highV5ClosedV4DiagnosticsSHA, 6496, &d) != nil {
		return a, p, errNativeFixtureDenied
	}
	return a, p, nil
}

func validateHighV5ClosedV4(h nativeHistoricalEvidence, a highV5ClosedV4Audit, p acceptance.CloudV4PGWitness) error {
	w, c := a.Window, a.Correlation
	if h.NativeAttempts != 268 || len(h.Usage) != 268 || h.BaselineSHA256 != highV4HistorySHA || h.KnownUsagePairs != 242 || h.UnknownUsagePairs != 26 ||
		a.Status != "V4_PRIVATE_FINAL_SQL_CORRELATION_MATCH" || a.Report != highV5ClosedV4ReportSHA || a.PG != highV5ClosedV4PGSHA || a.Diagnostics != highV5ClosedV4DiagnosticsSHA ||
		a.Rows != 11 || a.Originals != 11 || a.Correct != 10 || a.Calls != 0 || a.Ready ||
		w.Status != "NO_GO" || w.CollectorStatus != "failed" || w.Pin.CodeCommit != highV5ClosedV4Release || w.HistorySHA256 != h.BaselineSHA256 ||
		w.HistoricalAttempts != 268 || w.HistoricalKnown != 242 || w.HistoricalUnknown != 26 || w.NewAttempts != 11 || w.CarryForwardAttempts != 279 ||
		!w.CorrelationValid || !w.QualityValid || w.Quality.Complete || w.Quality.ThresholdsPassed || w.Execution || w.Comparison.ThresholdsPassed ||
		w.Cost.Attempts != 11 || w.Cost.KnownInputTokens != 5134 || w.Cost.KnownOutputTokens != 299 || w.Cost.KnownTotalTokens != 12107 || !w.Cost.FullTokenCostKnown ||
		c.Status != "PG_WITNESS_CORRELATION_MATCH" || c.ReportSHA256 != a.Report || c.WitnessSHA256 != a.PG || c.TotalAttempts != 279 || c.NewAttempts != 11 || c.Rows != 11 || !c.Matched || c.QualityPassed || c.InferenceProven || c.Execution ||
		p.Version != acceptance.CloudV4PGWitnessVersion || p.Window.Attempts != 279 || p.Window.BaselineHash != highV4HistorySHA || len(p.Attempts) != 11 || len(w.Attempts) != 11 {
		return errNativeFixtureDenied
	}
	for i, x := range w.Attempts {
		y := p.Attempts[i]
		outcome := "completed"
		if i == 10 {
			outcome = "transport_error"
		}
		if x.Ordinal != 269+i || y.Ordinal != x.Ordinal || y.Context != x.Context || y.InputHash != x.InputSHA256 || x.Outcome != outcome || y.Outcome != x.Outcome ||
			x.InputTokens == nil || x.OutputTokens == nil || x.TotalTokens == nil || x.ModelVersion == nil || *x.ModelVersion != "gemini-3.8-flash-n" ||
			!reflect.DeepEqual(x.InputTokens, y.InputTokens) || !reflect.DeepEqual(x.OutputTokens, y.OutputTokens) || !reflect.DeepEqual(x.TotalTokens, y.TotalTokens) {
			return errNativeFixtureDenied
		}
	}
	return nil
}

// Pure cumulative history279. Closed NO_GO is spend evidence, never a release
// prerequisite. Neither the discarded ordinal266 body nor its cause is invented.
func readScenarioHighV5History() (nativeHistoricalEvidence, error) {
	h, err := readScenarioHighV4History()
	a, p, readErr := readHighV5ClosedV4()
	if err != nil || readErr != nil || validateHighV5ClosedV4(h, a, p) != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	for _, x := range a.Window.Attempts {
		h.Usage = append(h.Usage, nativeUsageEvidence{RunID: x.Context, ReservationID: "shadow-v4-native-" + strconv.Itoa(x.Ordinal), ActualInputTokens: x.InputTokens, ActualOutputTokens: x.OutputTokens})
	}
	h.NativeAttempts, h.KnownUsagePairs = 279, 253
	sum := sha256.Sum256([]byte(h.BaselineSHA256 + "\n" + highV5ClosedV4ReportSHA + "\n" + highV5ClosedV4AuditSHA + "\n" + highV5ClosedV4PGSHA + "\n" + highV5ClosedV4DiagnosticsSHA + "\n"))
	h.BaselineSHA256 = hex.EncodeToString(sum[:])
	h.Artifacts = append(h.Artifacts, nativePriorArtifact{ReportSHA256: highV5ClosedV4AuditSHA, ReleaseRef: highV5ClosedV4Release, NewAttempts: 11})
	return h, nil
}

func TestScenarioHighV5HistoryCarriesAll279SpentAttempts(t *testing.T) {
	h, err := readScenarioHighV5History()
	if err != nil || h.NativeAttempts != 279 || len(h.Usage) != 279 || len(h.Artifacts) != 20 || h.KnownUsagePairs != 253 || h.UnknownUsagePairs != 26 || h.BaselineSHA256 != highV5HistorySHA {
		t.Fatal("actual closed V4 accounting lost", err)
	}
	var in, out int64
	unknown := 0
	for _, u := range h.Usage {
		if u.ActualInputTokens == nil || u.ActualOutputTokens == nil {
			unknown++
			continue
		}
		in += *u.ActualInputTokens
		out += *u.ActualOutputTokens
	}
	if in != 160557 || out != 6242 || unknown != 26 || h.Usage[247].ActualInputTokens != nil || *h.Usage[265].ActualInputTokens != 526 || *h.Usage[278].ActualInputTokens != 509 {
		t.Fatal("unknown or failed usage changed")
	}
	t.Logf("actual_history279_sha256=%s", h.BaselineSHA256)
}

func TestScenarioHighV5HistoryRejectsFalsePassCostOrOmissions(t *testing.T) {
	h, _ := readScenarioHighV4History()
	for _, mode := range []string{"pass", "quality", "invoice", "unknown", "cost", "omission", "refund", "pg", "ready"} {
		t.Run(mode, func(t *testing.T) {
			a, p, err := readHighV5ClosedV4()
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "pass":
				a.Window.Status = "PASS"
			case "quality":
				a.Window.Quality.ThresholdsPassed = true
			case "invoice":
				a.Window.Cost.FullTokenCostKnown = false
			case "unknown":
				a.Window.HistoricalUnknown = 0
			case "cost":
				a.Window.Cost.KnownTotalTokens++
			case "omission":
				p.Attempts = p.Attempts[:10]
			case "refund":
				a.Window.CarryForwardAttempts--
			case "pg":
				p.Attempts[10].Outcome = "completed"
			case "ready":
				a.Ready = true
			}
			if validateHighV5ClosedV4(h, a, p) == nil {
				t.Fatal("closed failed scope promoted or spend lost")
			}
		})
	}
}
