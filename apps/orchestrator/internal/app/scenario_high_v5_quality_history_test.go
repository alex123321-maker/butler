package app

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/butler/butler/internal/transport"
)

const highV5CompatibilityReportSHA = "05419e82a3c9b32a1250c4c67f5c1b127aa6919a6f6c043f058b89ca80f5330b"
const highV5CompatibilityAuditSHA = "d3877d3bc9a2bdffca14c040e4aa8c397e3b5da806c0ff0e3a33a47221fd7cfe"
const highV5CompatibilityPGSHA = "5ad40ecd6a33e7643a7c38e02422cdcc47e445973bdfc2386d50d80574343b48"
const highV5CompatibilityRelease = "a2004acad0c6dc37c2cdf5db71e1a63d5e57e3dc"
const highV5CompatibilitySourceSHA = "890aa41435d36e418788a2d01285e56e81b2c025b9617bb80ef00f5a7adeeef4"
const highV5CompatibilityRuntimeSHA = "93b8876a9cba071e5aff4d1771e93932d78c7164b2dfdaf63de812879b9b406f"
const highV5CompatibilityHardwareSHA = "5e102e3738d1a2e6720f8dba6f17d0acb4dec4c2e953262a9baf68c3e6760c00"
const highV5QualityHistorySHA = "fb248690c6daa51c65a82a7b67f155511b369d3aadf6028338561d02fe4e951b"
const highV5QualityHistoryFileSHA = "1299b074c6b317727495fdb2502456eb18829e4e05df9fae75a15a434215b56d"
const highV5QualityHistoricalKnownTotal int64 = 289855
const highV5QualityHistoricalUnknownTotal = 46

func readHighV5Compatibility() (schemaDiagnosticReport, schemaDiagnosticFinalAudit, schemaDiagnosticPGWitness, error) {
	var r schemaDiagnosticReport
	var a schemaDiagnosticFinalAudit
	var p schemaDiagnosticPGWitness
	if readPinnedHighHistory(highHistoryPath("schema-dialect-diagnostic-report-20261013.json"), highV5CompatibilityReportSHA, 6561, &r) != nil ||
		readPinnedHighHistory(highHistoryPath("schema-dialect-final-audit-20261013.json"), highV5CompatibilityAuditSHA, 5832, &a) != nil ||
		readPinnedHighHistory(highHistoryPath("schema-dialect-pg-witness-20261013.json"), highV5CompatibilityPGSHA, 5718, &p) != nil {
		return r, a, p, errNativeFixtureDenied
	}
	return r, a, p, nil
}

// This pure accounting reader consumes the exact independently checked safe
// metadata. Original private bytes remain only in the frozen diagnostic proof.
// It cannot reopen an old window or authenticate/invoke a provider.
func validateHighV5Compatibility(h nativeHistoricalEvidence, r schemaDiagnosticReport, a schemaDiagnosticFinalAudit, p schemaDiagnosticPGWitness) error {
	if h.NativeAttempts != 279 || len(h.Usage) != 279 || len(h.Artifacts) != 20 || h.KnownUsagePairs != 253 || h.UnknownUsagePairs != 26 || h.BaselineSHA256 != highV5HistorySHA ||
		r.Status != "diagnostic_completed" || r.Window != schemaDiagnosticWindowID || r.Baseline != 279 || r.Ceiling != 283 || r.Ready || r.Quality || len(r.Attempts) != 4 ||
		a.Status != "SCHEMA_DIALECT_COMPATIBILITY_PASS" || !a.Compatibility || a.Ready || a.Quality || a.Calls != 0 || a.ReportSHA != highV5CompatibilityReportSHA || a.PGSHA != highV5CompatibilityPGSHA || a.HistorySHA != highV5HistorySHA || a.ProtocolSHA != schemaDiagnosticProtocolSHA() ||
		a.Baseline != 279 || a.New != 4 || a.Total != 283 || a.Rows != 4 || a.Raw != 4 || a.NativeExact != 2 || a.KnownInput != 2116 || a.KnownOutput != 242 || a.KnownTotal != 4676 || a.UnknownInput != 0 || a.UnknownOutput != 0 || a.UnknownTotal != 0 || !reflect.DeepEqual(a.Attempts, r.Attempts) ||
		p.Version != "butler-schema-dialect-pg-witness-v1" || !p.Window.Singleton || p.Window.Attempts != 283 || p.Window.History != highV5HistorySHA || p.Window.Protocol != a.ProtocolSHA || p.Window.Expires.IsZero() || len(p.Attempts) != 4 || len(p.Retention) != 4 {
		return errNativeFixtureDenied
	}
	entries, err := schemaDiagnosticInputs()
	if err != nil || len(entries) != 4 {
		return errNativeFixtureDenied
	}
	var in, out, total int64
	for i, x := range r.Attempts {
		y, ret, e := p.Attempts[i], p.Retention[i], entries[i]
		input, err := makeSchemaDiagnosticInput(e)
		user, uerr := schemaDiagnosticUser(e)
		if err != nil || uerr != nil || x.Ordinal != 280+i || y.Ordinal != x.Ordinal || ret.Ordinal != x.Ordinal || x.CaseID != e.ID || y.CaseID != e.ID || x.Dialect != e.Dialect || y.Dialect != e.Dialect ||
			x.Context != schemaDiagnosticContext(e) || y.Context != x.Context || x.InputSHA != shadowV4Hash(input) || y.InputSHA != x.InputSHA || x.SystemSHA != privateFinalFacts([]byte(input.SystemInstructions)).SHA256 || y.SystemSHA != x.SystemSHA || x.UserSHA != privateFinalFacts(user).SHA256 || y.UserSHA != x.UserSHA || x.SchemaSHA != privateFinalFacts(input.Schema).SHA256 || y.SchemaSHA != x.SchemaSHA ||
			x.Outcome != "completed" || y.Outcome != x.Outcome || !x.CleanLifecycle || !y.Clean || !x.MetadataKnown || !y.Known || x.Input == nil || x.Output == nil || x.Total == nil || x.Version == nil || *x.Version != schemaDiagnosticExpectedVersion ||
			!reflect.DeepEqual(x.Input, y.Input) || !reflect.DeepEqual(x.Output, y.Output) || !reflect.DeepEqual(x.Total, y.Total) || !reflect.DeepEqual(x.Version, y.Version) || *x.Input < 0 || *x.Output < 0 || *x.Total < *x.Input+*x.Output ||
			x.FinalCount != 1 || y.Finals != 1 || x.TerminalCount != 1 || y.Terminals != 1 || x.FinishReason == nil || *x.FinishReason != "STOP" || !reflect.DeepEqual(x.FinishReason, y.Finish) || x.Stage == nil || *x.Stage != transport.ProviderStageStream || y.Stage == nil || *y.Stage != string(*x.Stage) || x.ErrorCategory != "" || y.Error != nil && *y.Error != "" || ret.Error != nil && *ret.Error != "" ||
			y.Elapsed == nil || *y.Elapsed != x.Elapsed || x.Elapsed <= 0 || x.Elapsed > 120000 || y.Finished == nil || y.Admitted.IsZero() || y.Finished.Before(y.Admitted) || !y.Admitted.Before(p.Window.Expires) || y.Finished.After(p.Window.Expires) || x.Elapsed-y.Finished.Sub(y.Admitted).Milliseconds() < 0 || x.Elapsed-y.Finished.Sub(y.Admitted).Milliseconds() > 1 ||
			!x.RetentionOK || !ret.OK || !x.VerificationChecked || !ret.Checked || !ret.Semantic || !x.Verification.Exact || !nativeEqualJSON(x.Verification, ret.Verification) || x.Final == nil || !x.Final.RawRetained || ret.Bytes == nil || *ret.Bytes != x.Final.Bytes || x.Final.Bytes <= 0 || x.Final.Bytes > 8192 || ret.Hash == nil || *ret.Hash != x.Final.SHA256 || !nativeAuditHash.MatchString(x.Final.SHA256) || !ret.Retained || ret.Reason == nil || *ret.Reason != "json_object" || x.Final.Reason != *ret.Reason {
			return errNativeFixtureDenied
		}
		in += *x.Input
		out += *x.Output
		total += *x.Total
	}
	if in != a.KnownInput || out != a.KnownOutput || total != a.KnownTotal {
		return errNativeFixtureDenied
	}
	return nil
}

func readScenarioHighV5QualityHistory() (nativeHistoricalEvidence, error) {
	h, err := readScenarioHighV5History()
	r, a, p, readErr := readHighV5Compatibility()
	if err != nil || readErr != nil || validateHighV5Compatibility(h, r, a, p) != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	for _, x := range a.Attempts {
		h.Usage = append(h.Usage, nativeUsageEvidence{RunID: x.Context, ReservationID: "schema-dialect-native-" + strconv.Itoa(x.Ordinal), ActualInputTokens: x.Input, ActualOutputTokens: x.Output})
	}
	h.NativeAttempts, h.KnownUsagePairs = 283, 257
	fold := strings.Join([]string{h.BaselineSHA256, highV5CompatibilityReportSHA, highV5CompatibilityAuditSHA, highV5CompatibilityPGSHA, highV5CompatibilityRelease, highV5CompatibilitySourceSHA, highV5CompatibilityRuntimeSHA, highV5CompatibilityHardwareSHA}, "\n") + "\n"
	sum := sha256.Sum256([]byte(fold))
	h.BaselineSHA256 = hex.EncodeToString(sum[:])
	h.Artifacts = append(h.Artifacts, nativePriorArtifact{ReportSHA256: highV5CompatibilityAuditSHA, ReleaseRef: highV5CompatibilityRelease, NewAttempts: 4})
	if h.BaselineSHA256 != highV5QualityHistorySHA {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	return h, nil
}

func TestScenarioHighV5QualityHistoryCarriesAll283SpentAttempts(t *testing.T) {
	h, err := readScenarioHighV5QualityHistory()
	if err != nil || h.NativeAttempts != 283 || len(h.Usage) != 283 || len(h.Artifacts) != 21 || h.KnownUsagePairs != 257 || h.UnknownUsagePairs != 26 || h.BaselineSHA256 != highV5QualityHistorySHA || h.Artifacts[20].ReportSHA256 != highV5CompatibilityAuditSHA {
		t.Fatal("actual compatibility accounting denied", err)
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
	if in != 162673 || out != 6484 || unknown != 26 || h.Usage[247].ActualInputTokens != nil || *h.Usage[265].ActualInputTokens != 526 || *h.Usage[278].ActualInputTokens != 509 || highV5QualityHistoricalKnownTotal != 285179+4676 || highV5QualityHistoricalUnknownTotal != 46 {
		t.Fatal("failed spend or historical unknown erased")
	}
	t.Logf("actual_quality_start283_sha256=%s", h.BaselineSHA256)
}

func TestScenarioHighV5QualityHistoryCommittedViewMatchesPureReader(t *testing.T) {
	h, err := readScenarioHighV5QualityHistory()
	var saved nativeHistoricalEvidence
	if err != nil || readPinnedHighHistory(highHistoryPath("cloud-shadow-v5-quality-start-history-20261013.json"), highV5QualityHistoryFileSHA, 58638, &saved) != nil || !reflect.DeepEqual(saved, h) {
		t.Fatal("committed actual quality-start view changed", err)
	}
}

func TestScenarioHighV5QualityHistoryRejectsCompatibilityPromotionOrRefund(t *testing.T) {
	h, _ := readScenarioHighV5History()
	for _, mode := range []string{"pass", "quality", "calls", "raw", "unknown", "cost", "omission", "refund", "pg", "schema", "dialect", "retention", "version", "terminal", "time", "history"} {
		t.Run(mode, func(t *testing.T) {
			r, a, p, err := readHighV5Compatibility()
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "pass":
				a.Compatibility = false
			case "quality":
				a.Quality = true
			case "calls":
				a.Calls = 1
			case "raw":
				a.Raw--
			case "unknown":
				a.UnknownTotal = 1
			case "cost":
				*p.Attempts[0].Input++
			case "omission":
				p.Attempts = p.Attempts[:3]
			case "refund":
				a.Total--
			case "pg":
				p.Attempts[0].Context = p.Attempts[1].Context
			case "schema":
				p.Attempts[0].SchemaSHA = highV5HistorySHA
			case "dialect":
				p.Attempts[1].Dialect = schemaDialectJSON
			case "retention":
				p.Retention[0].Retained = false
			case "version":
				*p.Attempts[0].Version = "unavailable"
			case "terminal":
				p.Attempts[0].Terminals = 2
			case "time":
				*p.Attempts[0].Elapsed++
			case "history":
				p.Window.History = highV4HistorySHA
			}
			if validateHighV5Compatibility(h, r, a, p) == nil {
				t.Fatal("changed compatibility spend accepted")
			}
		})
	}
}
