package app

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strconv"
	"testing"
)

const highV4DiagnosticSHA = "76d3e1a71681cac0b54e3a54c58978f94185c12e28ead163ee736f2022433adb"
const highV4DiagnosticAuditSHA = "673cb0510621efbc646d8a3ac435a7abb756dd2e4a73d240c79dcc6adbba7eba"
const highV4DiagnosticPGSHA = "eaf05ae023b783795edacd0b8d0c0530554d55d06324a6a71bc7fea4bf6d33dc"
const highV4HistorySHA = "85c6b9e345fe30093a4801bc29b72b098add699e7cf2df524419106e3a75c186"
const highV4DiagnosticRelease = "b7d65b8dbe81bb8b1951c70039869f6f5d66ed3c"

type structuralDiagnosticReport struct {
	Status       string                        `json:"status"`
	Window       string                        `json:"window_id"`
	Baseline     int                           `json:"baseline"`
	Ceiling      int                           `json:"ceiling"`
	Attempts     []structuralDiagnosticAttempt `json:"attempts"`
	IssueReady   bool                          `json:"issue33_ready"`
	QualityClaim bool                          `json:"quality_claim"`
}

type highV4DiagnosticAudit struct {
	Status        string `json:"status"`
	DiagnosticSHA string `json:"diagnostic_sha256"`
	PGSHA         string `json:"pg_witness_sha256"`
	PreviousSHA   string `json:"previous_history_sha256"`
	Rows          int    `json:"exact_correlated_rows"`
	New           int    `json:"new_attempts"`
	Total         int    `json:"carry_forward_attempts"`
	Known         int    `json:"cumulative_known_pairs"`
	Unknown       int    `json:"cumulative_null_pairs"`
	Input         int64  `json:"new_input_tokens"`
	Output        int64  `json:"new_output_tokens"`
	Tokens        int64  `json:"new_total_tokens"`
	Witnesses     int    `json:"public_original_witness_exact_count"`
	Attempts      []struct {
		Ordinal  int    `json:"ordinal"`
		Context  string `json:"context_id"`
		InputSHA string `json:"input_sha256"`
		Bytes    int    `json:"visible_final_bytes"`
		FinalSHA string `json:"visible_final_sha256"`
		Reason   string `json:"encoding_reason"`
		Witness  bool   `json:"public_original_witness_exact"`
		Input    int64  `json:"input_tokens"`
		Output   int64  `json:"output_tokens"`
		Total    int64  `json:"total_tokens"`
		Version  string `json:"observed_model_version"`
	} `json:"attempts"`
	IssueReady bool `json:"issue33_ready"`
	Quality    bool `json:"quality_claim"`
	Inference  bool `json:"inference_proven_by_auditor"`
	OldCause   bool `json:"old_v3_raw_cause_known"`
	Calls      int  `json:"auditor_native_calls"`
}

func readHighV4DiagnosticArtifacts() (structuralDiagnosticReport, highV4DiagnosticAudit, error) {
	var report structuralDiagnosticReport
	var audit highV4DiagnosticAudit
	if readPinnedHighHistory(highHistoryPath("structural-json-high-20261012.json"), highV4DiagnosticSHA, 1706, &report) != nil ||
		readPinnedHighHistory(highHistoryPath("cloud-structural-json-diagnostic-high-20261012.json"), highV4DiagnosticAuditSHA, 2061, &audit) != nil {
		return report, audit, errNativeFixtureDenied
	}
	return report, audit, nil
}

func validateHighV4DiagnosticHistory(h nativeHistoricalEvidence, r structuralDiagnosticReport, a highV4DiagnosticAudit) error {
	if h.NativeAttempts != 266 || len(h.Usage) != 266 || len(h.Artifacts) != 18 || h.BaselineSHA256 != highV3HistorySHA ||
		h.KnownUsagePairs != 240 || h.UnknownUsagePairs != 26 || r.Status != "diagnostic_completed" ||
		r.Window != "issue33-high-structural-json-20261012-baseline266" || r.Baseline != 266 || r.Ceiling != 268 ||
		r.IssueReady || r.QualityClaim || len(r.Attempts) != 2 ||
		a.Status != "STRUCTURAL_DIAGNOSTIC_CORRELATION_MATCH" || a.DiagnosticSHA != highV4DiagnosticSHA || a.PGSHA != highV4DiagnosticPGSHA ||
		a.PreviousSHA != highV3HistorySHA || a.Rows != 2 || a.New != 2 || a.Total != 268 || a.Known != 242 || a.Unknown != 26 ||
		a.Input != 1078 || a.Output != 29 || a.Tokens != 1964 || a.Witnesses != 2 || len(a.Attempts) != 2 ||
		a.IssueReady || a.Quality || a.Inference || a.OldCause || a.Calls != 0 {
		return errNativeFixtureDenied
	}
	for i, observed := range r.Attempts {
		witness := a.Attempts[i]
		if observed.Ordinal != 267+i || observed.Ordinal != witness.Ordinal || observed.Context != witness.Context || observed.InputSHA != witness.InputSHA ||
			observed.Outcome != "completed" || !observed.CleanLifecycle || !observed.MetadataKnown || !observed.RetentionOK ||
			observed.Input == nil || observed.Output == nil || observed.Total == nil || observed.Version == nil || *observed.Version != "gemini-3.8-flash-n" ||
			*observed.Input != witness.Input || *observed.Output != witness.Output || *observed.Total != witness.Total || witness.Version != *observed.Version ||
			observed.Final == nil || !observed.Final.RawRetained || observed.Final.Reason != "json_object" || observed.Final.Bytes != witness.Bytes ||
			observed.Final.SHA256 != witness.FinalSHA || witness.Reason != "json_object" || !witness.Witness {
			return errNativeFixtureDenied
		}
	}
	return nil
}

// Pure forward accounting. Neither diagnostics nor their correlation become
// quality evidence; V3's discarded ordinal266 reason remains UNKNOWN.
func readScenarioHighV4History() (nativeHistoricalEvidence, error) {
	h, err := readScenarioHighV3History()
	r, a, artifactErr := readHighV4DiagnosticArtifacts()
	if err != nil || artifactErr != nil || validateHighV4DiagnosticHistory(h, r, a) != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	for _, attempt := range r.Attempts {
		h.Usage = append(h.Usage, nativeUsageEvidence{RunID: attempt.Context, ReservationID: "structural-native-" + strconv.Itoa(attempt.Ordinal), ActualInputTokens: attempt.Input, ActualOutputTokens: attempt.Output})
	}
	h.NativeAttempts = 268
	h.KnownUsagePairs += 2
	hash := sha256.Sum256([]byte(h.BaselineSHA256 + "\n" + highV4DiagnosticSHA + "\n" + highV4DiagnosticAuditSHA + "\n" + highV4DiagnosticPGSHA + "\n"))
	h.BaselineSHA256 = hex.EncodeToString(hash[:])
	h.Artifacts = append(h.Artifacts, nativePriorArtifact{ReportSHA256: highV4DiagnosticSHA, ReleaseRef: highV4DiagnosticRelease, NewAttempts: 2})
	return h, nil
}

func TestScenarioHighV4HistoryCarriesEvery268SpentAttempt(t *testing.T) {
	h, err := readScenarioHighV4History()
	if err != nil || h.NativeAttempts != 268 || len(h.Usage) != 268 || len(h.Artifacts) != 19 || h.KnownUsagePairs != 242 || h.UnknownUsagePairs != 26 || h.BaselineSHA256 != highV4HistorySHA {
		t.Fatal("diagnostic-inclusive accounting lost", err)
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
	if in != 155423 || out != 5943 || unknown != 26 || h.Usage[247].ActualInputTokens != nil || *h.Usage[265].ActualInputTokens != 526 {
		t.Fatal("unknown history or failed known cost changed")
	}
	prior, err := readScenarioHighV3History()
	if err != nil || prior.NativeAttempts != 266 || prior.BaselineSHA256 != highV3HistorySHA {
		t.Fatal("prior closed reader changed")
	}
}

func TestScenarioHighV4HistoryRejectsMissingArtifact(t *testing.T) {
	var destination any
	if readPinnedHighHistory(filepath.Join(t.TempDir(), "missing"), highV4DiagnosticSHA, 1706, &destination) == nil {
		t.Fatal("missing history accepted")
	}
}

func TestScenarioHighV4HistoryRejectsFalseQualityCostOrCause(t *testing.T) {
	h, err := readScenarioHighV3History()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"quality", "old_cause", "unknown_erased", "known_cost", "omission", "raw", "pg", "reorder"} {
		t.Run(name, func(t *testing.T) {
			r, a, err := readHighV4DiagnosticArtifacts()
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "quality":
				a.Quality = true
			case "old_cause":
				a.OldCause = true
			case "unknown_erased":
				a.Unknown = 0
			case "known_cost":
				a.Attempts[0].Input++
			case "omission":
				r.Attempts = r.Attempts[:1]
			case "raw":
				r.Attempts[0].Final.RawRetained = false
			case "pg":
				a.PGSHA = highV4DiagnosticSHA
			case "reorder":
				a.Attempts[0], a.Attempts[1] = a.Attempts[1], a.Attempts[0]
			}
			if validateHighV4DiagnosticHistory(h, r, a) == nil {
				t.Fatal("false history accepted")
			}
		})
	}
}
