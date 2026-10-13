package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"testing"

	"github.com/butler/butler/internal/acceptance"
)

// Actual independent quality/comparison PASS and content-free original/SQL
// correlation. Fresh chain preparation and independent admission remain separate.
const releaseChainV5ReviewedPrerequisiteSHA = "9480184b2f5586d8f9826b8db328c52a845ee812bd48afbf1432ca7f30bfd5d3"
const releaseChainV5ReviewedQualityHistoryArtifactSHA = "1299b074c6b317727495fdb2502456eb18829e4e05df9fae75a15a434215b56d"
const releaseChainV5ReviewedOriginalAuditSHA = "f8248d19ec2a0c222b39d46fbc46c0d0cf4ccc3c1361c89965b597eedbb21cab"
const releaseChainV5QualityCode = "3c4089f35c0bdf33bf9b2c6328ba395c519cecf4"
const releaseChainV5QualitySourceSHA = "61c8e729bb53f69439c3d69e9c0979e62e9a601ab3906a413f9434b0631db374"
const releaseChainV5QualityRuntimeSHA = "4a70bd62a58b8d7162e53b833e850a13b32e1ba0b9c0990df8ebe86d4d1a705c"
const releaseChainV5QualityHardwareSHA = "d3632dff9b14bb6cb710b9f909dfaa8fd7ea6ff313babd28db392ecaa7731c3a"
const releaseChainV5OriginalDiagnosticsSHA = "18c62eb81c6e3595189586a4ad4593e55a9dbe8469f7a34d607d3b44cda28f1a"

type releaseChainV5Prerequisite struct {
	Version                   string `json:"version"`
	PriorHistorySHA           string `json:"prior_history_sha256"`
	ChainHistorySHA           string `json:"chain_history_sha256"`
	QualityHistoryArtifactSHA string `json:"quality_history_artifact_sha256"`
	CompatibilitySHA          string `json:"native_schema_compatibility_sha256"`
	HistoricalTotal           int64  `json:"historical_known_total_tokens"`
	HistoricalNullTotal       int    `json:"historical_unknown_total_attempts"`
	ReportSHA                 string `json:"v5_private_report_sha256"`
	AuditSHA                  string `json:"v5_audit_sha256"`
	PGSHA                     string `json:"v5_pg_witness_sha256"`
	CorrelationSHA            string `json:"v5_pg_correlation_sha256"`
	Code                      string `json:"v5_code_commit"`
	SourceSHA                 string `json:"v5_source_sha256"`
	HardwareSHA               string `json:"v5_hardware_sha256"`
	RuntimeSHA                string `json:"v5_runtime_sha256"`
	NewAttempts               int    `json:"v5_new_attempts"`
	Baseline                  int    `json:"baseline_attempts"`
	RequestedModel            string `json:"requested_model"`
	ObservedVersion           string `json:"observed_model_version"`
	GenerationTokens          int    `json:"maximum_generation_tokens"`
}

type releaseChainV5ActualAudit struct {
	Status           string                          `json:"status"`
	ReportSHA        string                          `json:"report_sha256"`
	PGSHA            string                          `json:"pg_sha256"`
	DiagnosticsSHA   string                          `json:"diagnostics_sha256"`
	DiagnosticRows   int                             `json:"exact_diagnostic_rows"`
	Originals        int                             `json:"exact_private_originals"`
	CorrectOriginals int                             `json:"correct_original_witnesses"`
	Window           acceptance.CloudV5WindowAudit   `json:"window_audit"`
	Correlation      acceptance.CloudV5PGCorrelation `json:"pg_correlation"`
	AuditorCalls     int                             `json:"auditor_native_calls"`
	Ready            bool                            `json:"issue33_ready"`
}

func readReleaseChainV5ActualAudit() (releaseChainV5ActualAudit, error) {
	var f releaseChainV5ActualAudit
	raw, err := readReleaseChainV5Artifact(highHistoryPath("cloud-shadow-v5-final-audit-20261013.json"), releaseChainV5ReviewedOriginalAuditSHA)
	if err != nil || acceptance.DecodeStrict(raw, &f, "status", "report_sha256", "pg_sha256", "diagnostics_sha256", "exact_diagnostic_rows", "exact_private_originals", "correct_original_witnesses", "window_audit", "pg_correlation", "auditor_native_calls", "issue33_ready") != nil {
		return f, errNativeFixtureDenied
	}
	return f, nil
}

// The exact original audit is distinct from independently reserialized nested
// projections. It binds private original retention/SQL correlation to the same
// measured quality/comparison result, without reading any original model bytes.
func validateReleaseChainV5ActualAudit(f releaseChainV5ActualAudit, p releaseChainV5Prerequisite, a acceptance.CloudV5WindowAudit, c acceptance.CloudV5PGCorrelation) error {
	if f.Status != "V5_PRIVATE_FINAL_SQL_CORRELATION_MATCH" || f.ReportSHA != p.ReportSHA || f.PGSHA != p.PGSHA || f.DiagnosticsSHA != releaseChainV5OriginalDiagnosticsSHA || f.DiagnosticRows != 252 || f.Originals != 252 || f.CorrectOriginals != 251 || f.AuditorCalls != 0 || f.Ready || !reflect.DeepEqual(f.Window, a) || !reflect.DeepEqual(f.Correlation, c) {
		return errNativeFixtureDenied
	}
	return nil
}

// Recheck numeric release gates as well as the hash-pinned independent verdict.
// This cannot rescore originals or replace the independent native provenance.
func validateReleaseChainV5QualityGates(a acceptance.CloudV5WindowAudit) error {
	q, comparison := a.Quality, a.Comparison
	if !q.GoldenVerified || !q.Complete || !q.ThresholdsPassed || q.ExecutionAuthorized || q.ObservedModelVersion == nil || *q.ObservedModelVersion != "gemini-3.8-flash-n" || q.UnknownModelVersions != 0 || !reflect.DeepEqual(q.Pin, a.Pin) || len(q.Splits) != 3 || q.Cost.Attempts != 236 || !q.Cost.FullTokenCostKnown || q.Cost.UnknownInput != 0 || q.Cost.UnknownOutput != 0 || q.Cost.UnknownTotal != 0 {
		return errNativeFixtureDenied
	}
	expected := map[string]map[string]int{
		"development": {"retrieval": 24, "extraction": 24, "classification": 24, "commands": 24},
		"tuning":      {"retrieval": 16, "extraction": 16, "classification": 16, "commands": 16},
		"holdout":     {"retrieval": 18, "extraction": 18, "classification": 20, "commands": 20},
	}
	for split, kinds := range expected {
		m, ok := q.Splits[split]
		if !ok || len(m.ByKind) != 4 || !m.ThresholdsPassed || m.Missing != 0 || m.Correct < 0 || m.Correct > m.Cases || m.Correct*100 < m.Cases*90 || m.MissedImportant < 0 || m.MissedImportant > 1 || m.FalseImportant < 0 || m.FalseImportant > 1 || m.CommandMismatches != 0 || m.SchemaErrors != 0 || m.WitnessViolations != 0 || m.UnsafeAuthority != 0 || m.UnsafeToolProposals != 0 || m.FailedAttempts != 0 || m.LatencyP95MS <= 0 || m.LatencyP95MS > 60000 || m.MaximumLatencyMS < m.LatencyP95MS || m.MaximumLatencyMS > 120000 {
			return errNativeFixtureDenied
		}
		correct, cases := 0, 0
		for kind, count := range kinds {
			value, exists := m.ByKind[kind]
			if !exists || value[1] != count || value[0] < 0 || value[0] > count || value[0]*100 < count*85 {
				return errNativeFixtureDenied
			}
			correct, cases = correct+value[0], cases+count
		}
		if correct != m.Correct || cases != m.Cases {
			return errNativeFixtureDenied
		}
	}
	if comparison.PairedTasks != 4 || comparison.SingleCorrect != 4 || comparison.MultiCorrect != 4 || !comparison.ThresholdsPassed || !comparison.FullTokenCostKnown || comparison.ExecutionAuthorized || !comparison.ShadowContextsOnly || comparison.ObservedModelVersion == nil || *comparison.ObservedModelVersion != "gemini-3.8-flash-n" || comparison.UnknownModelVersions != 0 || comparison.Single.Attempts != 4 || comparison.Multi.Attempts != 12 || !comparison.Single.FullTokenCostKnown || !comparison.Multi.FullTokenCostKnown || comparison.Single.UnknownInput != 0 || comparison.Single.UnknownOutput != 0 || comparison.Single.UnknownTotal != 0 || comparison.Multi.UnknownInput != 0 || comparison.Multi.UnknownOutput != 0 || comparison.Multi.UnknownTotal != 0 {
		return errNativeFixtureDenied
	}
	for _, cost := range []acceptance.CloudCost{q.Cost, comparison.Single, comparison.Multi, a.Cost} {
		if cost.KnownInputTokens < 0 || cost.KnownOutputTokens < 0 || cost.KnownTotalTokens < cost.KnownInputTokens+cost.KnownOutputTokens || cost.Milliseconds < 0 || cost.RemoteMemoryStatus != "unavailable" || cost.RemoteMemoryBytes != nil || cost.InvoiceMicrounits != nil {
			return errNativeFixtureDenied
		}
	}
	if comparison.SingleWallMS < 0 || comparison.SingleWallMS > 4*480000 || comparison.MultiWallMS < 0 || comparison.MultiWallMS > 4*480000 || q.Cost.KnownInputTokens+comparison.Single.KnownInputTokens+comparison.Multi.KnownInputTokens != a.Cost.KnownInputTokens || q.Cost.KnownOutputTokens+comparison.Single.KnownOutputTokens+comparison.Multi.KnownOutputTokens != a.Cost.KnownOutputTokens || q.Cost.KnownTotalTokens+comparison.Single.KnownTotalTokens+comparison.Multi.KnownTotalTokens != a.Cost.KnownTotalTokens || q.Cost.Milliseconds+comparison.Single.Milliseconds+comparison.Multi.Milliseconds != a.Cost.Milliseconds {
		return errNativeFixtureDenied
	}
	for _, entry := range a.Attempts {
		if entry.Milliseconds < 0 || entry.Milliseconds > 120000 {
			return errNativeFixtureDenied
		}
	}
	return nil
}

func readReleaseChainV5Prerequisite() (releaseChainV5Prerequisite, error) {
	var p releaseChainV5Prerequisite
	if releaseChainV5Baseline <= 0 || !nativeAuditHash.MatchString(releaseChainV5ReviewedPrerequisiteSHA) || !nativeAuditHash.MatchString(releaseChainV5ReviewedQualityHistoryArtifactSHA) {
		return p, errNativeFixtureDenied
	}
	root := filepath.Join(shadowRepoRoot(), "deploy", "acceptance")
	raw, err := readNativeArtifact(filepath.Join(root, "release-chain-v5-high-prerequisite-20261013.json"), releaseChainV5ReviewedPrerequisiteSHA)
	if err != nil || acceptance.DecodeStrict(raw, &p, "version", "prior_history_sha256", "chain_history_sha256", "quality_history_artifact_sha256", "native_schema_compatibility_sha256", "historical_known_total_tokens", "historical_unknown_total_attempts", "v5_private_report_sha256", "v5_audit_sha256", "v5_pg_witness_sha256", "v5_pg_correlation_sha256", "v5_code_commit", "v5_source_sha256", "v5_hardware_sha256", "v5_runtime_sha256", "v5_new_attempts", "baseline_attempts", "requested_model", "observed_model_version", "maximum_generation_tokens") != nil {
		return p, errNativeFixtureDenied
	}
	history, err := readReleaseChainV5QualityHistory()
	if err != nil {
		return p, errNativeFixtureDenied
	}
	compatibilityRaw, compatibilityErr := readReleaseChainV5Artifact(filepath.Join(root, "schema-dialect-final-audit-20261013.json"), p.CompatibilitySHA)
	var compatibility schemaDiagnosticFinalAudit
	if compatibilityErr != nil || json.Unmarshal(compatibilityRaw, &compatibility) != nil || compatibility.Status != "SCHEMA_DIALECT_COMPATIBILITY_PASS" || !compatibility.Compatibility || compatibility.NativeExact != 2 || compatibility.Rows != 4 || compatibility.Raw != 4 || compatibility.New != history.NativeAttempts-279 || compatibility.Total != history.NativeAttempts || compatibility.Baseline != 279 || compatibility.UnknownInput != 0 || compatibility.UnknownOutput != 0 || compatibility.UnknownTotal != 0 || compatibility.Ready || compatibility.Quality || compatibility.Calls != 0 {
		return p, errNativeFixtureDenied
	}
	auditRaw, auditErr := readReleaseChainV5Artifact(filepath.Join(root, "cloud-shadow-v5-high-20261013.json"), p.AuditSHA)
	pgRaw, pgErr := readReleaseChainV5Artifact(filepath.Join(root, "cloud-shadow-v5-pg-witness-20261013.json"), p.PGSHA)
	correlationRaw, correlationErr := readNativeArtifact(filepath.Join(root, "cloud-shadow-v5-pg-correlation-20261013.json"), p.CorrelationSHA)
	var audit acceptance.CloudV5WindowAudit
	var witness acceptance.CloudV5PGWitness
	var correlation acceptance.CloudV5PGCorrelation
	if auditErr != nil || pgErr != nil || correlationErr != nil || json.Unmarshal(auditRaw, &audit) != nil ||
		acceptance.DecodeStrict(pgRaw, &witness, "version", "window", "attempts") != nil ||
		acceptance.DecodeStrict(correlationRaw, &correlation, "status", "collector_report_sha256", "pg_witness_sha256", "carry_forward_attempts", "new_attempts", "witness_rows", "exact_journal_match", "quality_passed", "inference_proven", "execution_authorized") != nil ||
		validateReleaseChainV5Prerequisite(p, history, audit, witness, correlation) != nil {
		return p, errNativeFixtureDenied
	}
	return p, nil
}

// Safe SQL and audit rows exceed the historical artifact's32KiB bound. The
// exact reviewed SHA still owns admission; this read grants no provider access.
func readReleaseChainV5Artifact(path, expected string) ([]byte, error) {
	const limit = releaseChainV5SafeArtifactOutputMax
	stat, err := os.Lstat(path)
	if err != nil || !nativeAuditHash.MatchString(expected) || !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > limit {
		return nil, errNativeFixtureDenied
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(stat, opened) {
		return nil, errNativeFixtureDenied
	}
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	sum := sha256.Sum256(raw)
	if err != nil || len(raw) > limit || hex.EncodeToString(sum[:]) != expected {
		return nil, errNativeFixtureDenied
	}
	return raw, nil
}

// These are already independently frozen, exact-hash content-free artifacts.
// This checker binds them to each other; it cannot rescore model outputs or
// establish native provenance from an unsigned candidate or SQL equality alone.
func validateReleaseChainV5Prerequisite(p releaseChainV5Prerequisite, history nativeHistoricalEvidence, a acceptance.CloudV5WindowAudit, w acceptance.CloudV5PGWitness, c acceptance.CloudV5PGCorrelation) error {
	actualHistory, historyErr := readScenarioHighV5QualityHistory()
	f, originalErr := readReleaseChainV5ActualAudit()
	if historyErr != nil || !reflect.DeepEqual(history, actualHistory) || originalErr != nil || validateReleaseChainV5ActualAudit(f, p, a, c) != nil || validateReleaseChainV5QualityGates(a) != nil ||
		p.Version != "butler-chain-high-reviewed-v5-prerequisite-v1" || p.PriorHistorySHA != history.BaselineSHA256 || len(history.Artifacts) != 21 || len(history.Usage) != history.NativeAttempts || history.NativeAttempts != 283 || history.KnownUsagePairs != 257 || history.UnknownUsagePairs != 26 ||
		p.QualityHistoryArtifactSHA != releaseChainV5ReviewedQualityHistoryArtifactSHA || p.CompatibilitySHA != history.Artifacts[20].ReportSHA256 || p.HistoricalTotal != 289855 || p.HistoricalNullTotal != 46 ||
		p.NewAttempts != 252 || p.Baseline != releaseChainV5Baseline || p.Baseline != history.NativeAttempts+252 || p.RequestedModel != acceptance.CloudRequestedModel || p.ObservedVersion != "gemini-3.8-flash-n" || p.GenerationTokens != 32768 ||
		!regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(p.Code) || p.Code != releaseChainV5QualityCode || p.SourceSHA != releaseChainV5QualitySourceSHA || p.RuntimeSHA != releaseChainV5QualityRuntimeSHA || p.HardwareSHA != releaseChainV5QualityHardwareSHA {
		return errNativeFixtureDenied
	}
	for _, hash := range []string{p.ReportSHA, p.AuditSHA, p.PGSHA, p.CorrelationSHA, p.SourceSHA, p.HardwareSHA, p.RuntimeSHA, p.CompatibilitySHA, p.QualityHistoryArtifactSHA, p.ChainHistorySHA} {
		if !nativeAuditHash.MatchString(hash) {
			return errNativeFixtureDenied
		}
	}
	file, err := os.Open(filepath.Join(shadowRepoRoot(), acceptance.CloudV5CorpusPath))
	if err != nil {
		return errNativeFixtureDenied
	}
	corpus, err := acceptance.LoadCloudV5Corpus(file)
	_ = file.Close()
	if err != nil || a.Pin.Mode != "shadow_observation" || a.Pin.Provider != acceptance.CloudProvider ||
		a.Pin.CorpusDigest != acceptance.CloudV5CorpusDigest(corpus) || a.Pin.PolicySHA256 != acceptance.CloudV5PolicyDigest(corpus) || a.Pin.SchemaSHA256 != acceptance.CloudV5SchemaDigest() || a.Pin.ResourcesSHA256 != acceptance.CloudV5ResourcesDigest() {
		return errNativeFixtureDenied
	}
	var historicalInput, historicalOutput int64
	knownPairs, unknownPairs := 0, 0
	for _, u := range history.Usage {
		if u.ActualInputTokens == nil || u.ActualOutputTokens == nil {
			unknownPairs++
			continue
		}
		if *u.ActualInputTokens < 0 || *u.ActualOutputTokens < 0 {
			return errNativeFixtureDenied
		}
		knownPairs++
		historicalInput += *u.ActualInputTokens
		historicalOutput += *u.ActualOutputTokens
	}
	if knownPairs != history.KnownUsagePairs || unknownPairs != history.UnknownUsagePairs {
		return errNativeFixtureDenied
	}
	if a.Status != "CLOUD_V5_WINDOW_AUDIT_PASS" || a.EvidenceKind != "offline_v5_window_audit" || a.CollectorStatus != "passed" || a.OriginalReportSHA256 != p.ReportSHA ||
		a.Pin.EvidenceKind != "native_cloud_observation" || a.Pin.WindowID != acceptance.CloudV5WindowID(history.NativeAttempts) || a.Pin.CodeCommit != p.Code || a.Pin.SourceSHA256 != p.SourceSHA || a.Pin.HardwareSHA256 != p.HardwareSHA || a.Pin.RuntimeSHA256 != p.RuntimeSHA ||
		a.Pin.RequestedModel != p.RequestedModel || a.Pin.ProviderModelVersion != nil || a.HistorySHA256 != p.PriorHistorySHA ||
		a.CompatibilitySHA256 != p.CompatibilitySHA || a.HistoricalAttempts != history.NativeAttempts || a.HistoricalKnown != history.KnownUsagePairs || a.HistoricalUnknown != 26 || a.HistoricalArtifacts != len(history.Artifacts) || a.HistoricalInput != historicalInput || a.HistoricalOutput != historicalOutput || a.HistoricalTotal != p.HistoricalTotal || a.HistoricalNullTotal != p.HistoricalNullTotal ||
		a.NewAttempts != 252 || a.CarryForwardAttempts != p.Baseline || a.MaximumNew != 252 || a.MaximumTotal != p.Baseline || a.Execution || !a.CorrelationValid ||
		!a.QualityValid || a.Quality.Status != "CLOUD_V5_SHADOW_THRESHOLDS_PASS" || !a.Quality.Complete || !a.Quality.ThresholdsPassed || !a.Quality.Cost.FullTokenCostKnown ||
		!a.ComparisonValid || a.Comparison.Status != "CLOUD_V5_SHADOW_COMPARISON_PASS" || a.Comparison.PairedTasks != 4 || a.Comparison.SingleCorrect != 4 || a.Comparison.MultiCorrect != 4 || !a.Comparison.ThresholdsPassed || !a.Comparison.FullTokenCostKnown ||
		a.ModelFamily != "observed_3.8_flash_family" || a.UnknownVersions != 0 || len(a.Reasons) != 0 || len(a.Attempts) != 252 || a.Cost.Attempts != 252 || !a.Cost.FullTokenCostKnown || a.Cost.UnknownInput != 0 || a.Cost.UnknownOutput != 0 || a.Cost.UnknownTotal != 0 {
		return errNativeFixtureDenied
	}
	if c.Status != "PG_WITNESS_CORRELATION_MATCH" || !c.Matched || c.ReportSHA256 != p.ReportSHA || c.WitnessSHA256 != p.PGSHA || c.TotalAttempts != p.Baseline || c.NewAttempts != 252 || c.Rows != 252 || c.QualityPassed || c.Execution || c.InferenceProven ||
		w.Version != acceptance.CloudV5PGWitnessVersion || w.Window.Baseline != history.NativeAttempts || w.Window.Attempts != p.Baseline || w.Window.BaselineHash != p.PriorHistorySHA || w.Window.PolicyHash != a.Pin.PolicySHA256 || !w.Window.Expires.Equal(a.Expires) || len(w.Attempts) != 252 {
		return errNativeFixtureDenied
	}
	contexts := map[string]bool{}
	var input, output, total int64
	quality, comparison := 0, 0
	for i, sql := range w.Attempts {
		observed := a.Attempts[i]
		var parent *string
		if sql.Role == acceptance.CloudRoleChildA || sql.Role == acceptance.CloudRoleChildB {
			value := releaseChainV5QualityContext(history.NativeAttempts, "comparison", sql.CaseID, acceptance.CloudRoleSynthesis)
			parent = &value
		}
		if sql.Ordinal != history.NativeAttempts+1+i || observed.Ordinal != sql.Ordinal || sql.Context != releaseChainV5QualityContext(history.NativeAttempts, sql.Phase, sql.CaseID, sql.Role) || contexts[sql.Context] || sql.Context != observed.Context ||
			sql.Phase != observed.Phase || sql.Role != observed.Role || !equalReleaseChainV5Parent(sql.ParentContextID, parent) || !equalReleaseChainV5Parent(observed.ParentContextID, parent) ||
			sql.InputHash != observed.InputSHA256 || !sql.Created.Equal(observed.ChargedAt) || sql.Outcome != "completed" || observed.Outcome != "completed" || sql.OutputHash == nil || !nativeAuditHash.MatchString(*sql.OutputHash) ||
			observed.ModelVersion == nil || *observed.ModelVersion != p.ObservedVersion ||
			!equalReleaseChainV5Cost(sql.InputTokens, observed.InputTokens) || !equalReleaseChainV5Cost(sql.OutputTokens, observed.OutputTokens) || !equalReleaseChainV5Cost(sql.TotalTokens, observed.TotalTokens) {
			return errNativeFixtureDenied
		}
		contexts[sql.Context] = true
		if *sql.InputTokens > 65536 || *sql.OutputTokens > 32768 || *sql.TotalTokens < *sql.InputTokens+*sql.OutputTokens || *sql.TotalTokens > *sql.InputTokens+32768 {
			return errNativeFixtureDenied
		}
		input += *sql.InputTokens
		output += *sql.OutputTokens
		total += *sql.TotalTokens
		if sql.Phase == "quality" && sql.Role == acceptance.CloudRoleSingle {
			quality++
		} else if sql.Phase == "comparison" {
			comparison++
		} else {
			return errNativeFixtureDenied
		}
	}
	if quality != 236 || comparison != 16 || input != a.Cost.KnownInputTokens || output != a.Cost.KnownOutputTokens || total != a.Cost.KnownTotalTokens {
		return errNativeFixtureDenied
	}
	carry := releaseChainV5CarryHistory(p, history, a)
	if p.ChainHistorySHA != carry.BaselineSHA256 || carry.NativeAttempts != p.Baseline || len(carry.Usage) != p.Baseline || carry.UnknownUsagePairs != 26 {
		return errNativeFixtureDenied
	}
	return nil
}

func readReleaseChainV5QualityHistory() (nativeHistoricalEvidence, error) {
	var h nativeHistoricalEvidence
	if !nativeAuditHash.MatchString(releaseChainV5ReviewedQualityHistoryArtifactSHA) {
		return h, errNativeFixtureDenied
	}
	raw, err := readReleaseChainV5Artifact(highHistoryPath("cloud-shadow-v5-quality-start-history-20261013.json"), releaseChainV5ReviewedQualityHistoryArtifactSHA)
	prior, priorErr := readScenarioHighV5History()
	actual, actualErr := readScenarioHighV5QualityHistory()
	if err != nil || priorErr != nil || acceptance.DecodeStrict(raw, &h, "status", "baseline_sha256", "artifacts", "native_attempts", "known_usage_pairs", "unknown_usage_pairs", "usage") != nil || h.Status != "mixed_completed_history" || !nativeAuditHash.MatchString(h.BaselineSHA256) || h.NativeAttempts <= prior.NativeAttempts || h.NativeAttempts > prior.NativeAttempts+4 || len(h.Usage) != h.NativeAttempts || len(h.Artifacts) != len(prior.Artifacts)+1 || !reflect.DeepEqual(h.Usage[:prior.NativeAttempts], prior.Usage) || !reflect.DeepEqual(h.Artifacts[:len(prior.Artifacts)], prior.Artifacts) {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	if actualErr != nil || !reflect.DeepEqual(h, actual) {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	return h, nil
}

func releaseChainV5QualityContext(baseline int, phase, caseID, role string) string {
	return "shadow-" + shadowV4Hash([]string{acceptance.CloudV5WindowID(baseline), phase, caseID, role})[:32]
}

// Append actual audited rows only. Old NULL usage and old artifacts are untouched.
func releaseChainV5CarryHistory(p releaseChainV5Prerequisite, h nativeHistoricalEvidence, a acceptance.CloudV5WindowAudit) nativeHistoricalEvidence {
	h.Usage = append([]nativeUsageEvidence(nil), h.Usage...)
	h.Artifacts = append([]nativePriorArtifact(nil), h.Artifacts...)
	for _, row := range a.Attempts {
		h.Usage = append(h.Usage, nativeUsageEvidence{RunID: row.Context, ReservationID: "shadow-v5-native-" + strconv.Itoa(row.Ordinal), ActualInputTokens: row.InputTokens, ActualOutputTokens: row.OutputTokens})
	}
	h.NativeAttempts += len(a.Attempts)
	h.KnownUsagePairs += len(a.Attempts)
	h.Artifacts = append(h.Artifacts, nativePriorArtifact{ReportSHA256: p.AuditSHA, ReleaseRef: p.Code, NewAttempts: len(a.Attempts)})
	h.BaselineSHA256 = shadowV4Hash([]string{p.PriorHistorySHA, p.QualityHistoryArtifactSHA, p.ReportSHA, p.AuditSHA, p.PGSHA, p.CorrelationSHA, p.SourceSHA, p.HardwareSHA, p.RuntimeSHA})
	return h
}

func equalReleaseChainV5Parent(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
func equalReleaseChainV5Cost(a, b *int64) bool { return a != nil && b != nil && *a >= 0 && *a == *b }

func TestReleaseChainV5OperationalPrerequisiteIsNotApproved(t *testing.T) {
	if releaseChainV5ReviewedPrerequisiteSHA == "" {
		if _, err := readReleaseChainV5Prerequisite(); err == nil {
			t.Fatal("unknown V5 quality bridge opened an unset scope")
		}
	} else if _, err := readReleaseChainV5Prerequisite(); err != nil {
		t.Fatal("separately frozen reviewed prerequisite unavailable")
	}
	if _, err := readChainHighPrerequisite(); err == nil {
		t.Fatal("new V5 protocol reopened the failed old480 prototype")
	}
}

func TestReleaseChainV5OperationalGate(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_CHAIN_V5_HIGH_GATE") != "1" {
		t.Skip("explicit separate V5 operational gate required")
	}
	if _, err := readReleaseChainV5Prerequisite(); err != nil {
		t.Fatal("complete actual V5 PASS bridge has not been independently frozen")
	}
}
