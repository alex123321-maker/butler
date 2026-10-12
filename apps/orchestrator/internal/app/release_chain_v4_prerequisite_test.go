package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/butler/butler/internal/acceptance"
)

// Remains empty until independent review freezes an actual complete V4 PASS,
// its content-free SQL correlation and this separately reviewed protocol.
// Neither a prospective520 baseline nor an offline fixture may populate it.
const releaseChainV4ReviewedPrerequisiteSHA = ""

type releaseChainV4Prerequisite struct {
	Version          string `json:"version"`
	PriorHistorySHA  string `json:"prior_history_sha256"`
	ReportSHA        string `json:"v4_private_report_sha256"`
	AuditSHA         string `json:"v4_audit_sha256"`
	PGSHA            string `json:"v4_pg_witness_sha256"`
	CorrelationSHA   string `json:"v4_pg_correlation_sha256"`
	Code             string `json:"v4_code_commit"`
	SourceSHA        string `json:"v4_source_sha256"`
	HardwareSHA      string `json:"v4_hardware_sha256"`
	NewAttempts      int    `json:"v4_new_attempts"`
	Baseline         int    `json:"baseline_attempts"`
	RequestedModel   string `json:"requested_model"`
	ObservedVersion  string `json:"observed_model_version"`
	GenerationTokens int    `json:"maximum_generation_tokens"`
}

func readReleaseChainV4Prerequisite() (releaseChainV4Prerequisite, error) {
	var p releaseChainV4Prerequisite
	if !nativeAuditHash.MatchString(releaseChainV4ReviewedPrerequisiteSHA) {
		return p, errNativeFixtureDenied
	}
	root := filepath.Join(shadowRepoRoot(), "deploy", "acceptance")
	raw, err := readNativeArtifact(filepath.Join(root, "release-chain-v4-high-prerequisite-20261013.json"), releaseChainV4ReviewedPrerequisiteSHA)
	if err != nil || acceptance.DecodeStrict(raw, &p, "version", "prior_history_sha256", "v4_private_report_sha256", "v4_audit_sha256", "v4_pg_witness_sha256", "v4_pg_correlation_sha256", "v4_code_commit", "v4_source_sha256", "v4_hardware_sha256", "v4_new_attempts", "baseline_attempts", "requested_model", "observed_model_version", "maximum_generation_tokens") != nil {
		return p, errNativeFixtureDenied
	}
	history, err := readScenarioHighV4History()
	if err != nil {
		return p, errNativeFixtureDenied
	}
	auditRaw, auditErr := readReleaseChainV4Artifact(filepath.Join(root, "cloud-shadow-v4-high-20261013.json"), p.AuditSHA)
	pgRaw, pgErr := readReleaseChainV4Artifact(filepath.Join(root, "cloud-shadow-v4-pg-witness-20261013.json"), p.PGSHA)
	correlationRaw, correlationErr := readNativeArtifact(filepath.Join(root, "cloud-shadow-v4-pg-correlation-20261013.json"), p.CorrelationSHA)
	var audit acceptance.CloudV4WindowAudit
	var witness acceptance.CloudV4PGWitness
	var correlation acceptance.CloudV4PGCorrelation
	if auditErr != nil || pgErr != nil || correlationErr != nil || json.Unmarshal(auditRaw, &audit) != nil ||
		acceptance.DecodeStrict(pgRaw, &witness, "version", "window", "attempts") != nil ||
		acceptance.DecodeStrict(correlationRaw, &correlation, "status", "collector_report_sha256", "pg_witness_sha256", "carry_forward_attempts", "new_attempts", "witness_rows", "exact_journal_match", "quality_passed", "inference_proven", "execution_authorized") != nil ||
		validateReleaseChainV4Prerequisite(p, history, audit, witness, correlation) != nil {
		return p, errNativeFixtureDenied
	}
	return p, nil
}

// Safe SQL and audit rows exceed the historical artifact's32KiB bound. The
// exact reviewed SHA still owns admission; this read grants no provider access.
func readReleaseChainV4Artifact(path, expected string) ([]byte, error) {
	const limit = 8 * 1024 * 1024
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
func validateReleaseChainV4Prerequisite(p releaseChainV4Prerequisite, history nativeHistoricalEvidence, a acceptance.CloudV4WindowAudit, w acceptance.CloudV4PGWitness, c acceptance.CloudV4PGCorrelation) error {
	if p.Version != "butler-chain-high-reviewed-v4-prerequisite-v1" || p.PriorHistorySHA != history.BaselineSHA256 || history.NativeAttempts != 268 || history.KnownUsagePairs != 242 || history.UnknownUsagePairs != 26 ||
		p.NewAttempts != 252 || p.Baseline != 520 || p.RequestedModel != acceptance.CloudRequestedModel || !acceptance.SafeCloudFamilyVersion(p.ObservedVersion) || p.GenerationTokens != 32768 ||
		!regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(p.Code) {
		return errNativeFixtureDenied
	}
	for _, hash := range []string{p.ReportSHA, p.AuditSHA, p.PGSHA, p.CorrelationSHA, p.SourceSHA, p.HardwareSHA} {
		if !nativeAuditHash.MatchString(hash) {
			return errNativeFixtureDenied
		}
	}
	raw, err := readShadowV4CorpusBytes(filepath.Join(shadowRepoRoot(), "testdata", "acceptance", "ru-cloud-fresh-v4.json"))
	var corpus acceptance.CloudV4Corpus
	if err != nil || json.Unmarshal(raw, &corpus) != nil || a.Pin.Mode != "shadow_observation" || a.Pin.Provider != acceptance.CloudProvider ||
		a.Pin.CorpusDigest != acceptance.CloudV4CorpusDigest(corpus) || a.Pin.PolicySHA256 != acceptance.CloudV4PolicyDigest(corpus) || a.Pin.SchemaSHA256 != acceptance.CloudV4SchemaDigest() || a.Pin.ResourcesSHA256 != acceptance.CloudV4ResourcesDigest() {
		return errNativeFixtureDenied
	}
	if a.Status != "CLOUD_V4_WINDOW_AUDIT_PASS" || a.EvidenceKind != "offline_v4_window_audit" || a.CollectorStatus != "passed" || a.OriginalReportSHA256 != p.ReportSHA ||
		a.Pin.EvidenceKind != "native_cloud_observation" || a.Pin.WindowID != acceptance.CloudV4AuditedWindow || a.Pin.CodeCommit != p.Code || a.Pin.SourceSHA256 != p.SourceSHA || a.Pin.HardwareSHA256 != p.HardwareSHA ||
		a.Pin.RequestedModel != p.RequestedModel || a.Pin.ProviderModelVersion != nil || a.HistorySHA256 != p.PriorHistorySHA ||
		a.HistoricalAttempts != 268 || a.HistoricalKnown != 242 || a.HistoricalUnknown != 26 || a.HistoricalArtifacts != 19 || a.HistoricalInput != 155423 || a.HistoricalOutput != 5943 || a.HistoricalTotal != 273072 || a.HistoricalNullTotal != 46 ||
		a.NewAttempts != 252 || a.CarryForwardAttempts != 520 || a.MaximumNew != 252 || a.MaximumTotal != 520 || a.Execution || !a.CorrelationValid ||
		!a.QualityValid || a.Quality.Status != "CLOUD_V4_SHADOW_THRESHOLDS_PASS" || !a.Quality.Complete || !a.Quality.ThresholdsPassed || !a.Quality.Cost.FullTokenCostKnown ||
		!a.ComparisonValid || a.Comparison.Status != "CLOUD_V4_SHADOW_COMPARISON_PASS" || a.Comparison.PairedTasks != 4 || a.Comparison.SingleCorrect != 4 || a.Comparison.MultiCorrect != 4 || !a.Comparison.ThresholdsPassed || !a.Comparison.FullTokenCostKnown ||
		a.ModelFamily != "observed_3.8_flash_family" || a.UnknownVersions != 0 || len(a.Reasons) != 0 || len(a.Attempts) != 252 || a.Cost.Attempts != 252 || !a.Cost.FullTokenCostKnown || a.Cost.UnknownInput != 0 || a.Cost.UnknownOutput != 0 || a.Cost.UnknownTotal != 0 {
		return errNativeFixtureDenied
	}
	if c.Status != "PG_WITNESS_CORRELATION_MATCH" || !c.Matched || c.ReportSHA256 != p.ReportSHA || c.WitnessSHA256 != p.PGSHA || c.TotalAttempts != 520 || c.NewAttempts != 252 || c.Rows != 252 || c.Execution || c.InferenceProven ||
		w.Version != acceptance.CloudV4PGWitnessVersion || w.Window.Attempts != 520 || w.Window.BaselineHash != p.PriorHistorySHA || w.Window.PolicyHash != a.Pin.PolicySHA256 || !w.Window.Expires.Equal(a.Expires) || len(w.Attempts) != 252 {
		return errNativeFixtureDenied
	}
	contexts := map[string]bool{}
	var input, output, total int64
	quality, comparison := 0, 0
	for i, sql := range w.Attempts {
		observed := a.Attempts[i]
		var parent *string
		if sql.Role == acceptance.CloudRoleChildA || sql.Role == acceptance.CloudRoleChildB {
			value := shadowV4ContextID("comparison", sql.CaseID, acceptance.CloudRoleSynthesis)
			parent = &value
		}
		if sql.Ordinal != 269+i || observed.Ordinal != sql.Ordinal || sql.Context != shadowV4ContextID(sql.Phase, sql.CaseID, sql.Role) || contexts[sql.Context] || sql.Context != observed.Context ||
			sql.Phase != observed.Phase || sql.Role != observed.Role || !equalReleaseChainV4Parent(sql.ParentContextID, parent) || !equalReleaseChainV4Parent(observed.ParentContextID, parent) ||
			sql.InputHash != observed.InputSHA256 || !sql.Created.Equal(observed.ChargedAt) || sql.Outcome != "completed" || observed.Outcome != "completed" || sql.OutputHash == nil || !nativeAuditHash.MatchString(*sql.OutputHash) ||
			observed.ModelVersion == nil || *observed.ModelVersion != p.ObservedVersion ||
			!equalReleaseChainV4Cost(sql.InputTokens, observed.InputTokens) || !equalReleaseChainV4Cost(sql.OutputTokens, observed.OutputTokens) || !equalReleaseChainV4Cost(sql.TotalTokens, observed.TotalTokens) {
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
	return nil
}

func equalReleaseChainV4Parent(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
func equalReleaseChainV4Cost(a, b *int64) bool { return a != nil && b != nil && *a >= 0 && *a == *b }

func TestReleaseChainV4OperationalPrerequisiteIsNotApproved(t *testing.T) {
	if releaseChainV4ReviewedPrerequisiteSHA == "" {
		if _, err := readReleaseChainV4Prerequisite(); err == nil {
			t.Fatal("unknown V4 quality bridge opened the prospective520 scope")
		}
	} else if _, err := readReleaseChainV4Prerequisite(); err != nil {
		t.Fatal("separately frozen reviewed prerequisite unavailable")
	}
	if _, err := readChainHighPrerequisite(); err == nil {
		t.Fatal("new V4 protocol reopened the failed old480 prototype")
	}
}

func TestReleaseChainV4OperationalGate(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_CHAIN_V4_HIGH_GATE") != "1" {
		t.Skip("explicit separate V4 operational gate required")
	}
	if _, err := readReleaseChainV4Prerequisite(); err != nil {
		t.Fatal("complete actual V4 PASS bridge has not been independently frozen")
	}
}
