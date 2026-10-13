package app

import (
	"bytes"
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

// The root-owned pure reader validates the actual four-entry producer proof.
// This alias grants no native admission independently of prepared source pins.
const shadowV5CompatibilitySHA = highV5CompatibilityAuditSHA
const shadowV5CompatibilityFile = "schema-dialect-final-audit-20261013.json"

func readShadowV5Prerequisites() (nativeHistoricalEvidence, []byte, error) {
	h, err := readScenarioHighV5QualityHistory()
	if err != nil || h.NativeAttempts != shadowV5NativeBaseline || h.KnownUsagePairs != 257 || h.UnknownUsagePairs != 26 || len(h.Usage) != 283 || len(h.Artifacts) != 21 || !nativeAuditHash.MatchString(h.BaselineSHA256) || !nativeAuditHash.MatchString(shadowV5CompatibilitySHA) || h.Artifacts[20].ReportSHA256 != shadowV5CompatibilitySHA {
		return h, nil, errNativeFixtureDenied
	}
	raw, err := readNativeArtifact(filepath.Join(shadowRepoRoot(), "deploy", "acceptance", shadowV5CompatibilityFile), shadowV5CompatibilitySHA)
	var a struct {
		Status    string `json:"status"`
		Native    bool   `json:"native_compatibility_passed"`
		Rows      int    `json:"exact_correlated_rows"`
		Originals int    `json:"exact_private_originals"`
		Exact     int    `json:"native_original_witness_exact_count"`
		Calls     int    `json:"auditor_native_calls"`
		Ready     bool   `json:"issue33_ready"`
		Quality   bool   `json:"quality_claim"`
	}
	if err != nil || json.Unmarshal(raw, &a) != nil || a.Status != "SCHEMA_DIALECT_COMPATIBILITY_PASS" || !a.Native || a.Rows != 4 || a.Originals != 4 || a.Exact != 2 || a.Calls != 0 || a.Ready || a.Quality {
		return h, nil, errNativeFixtureDenied
	}
	if _, err := readShadowV5PreparedHistoryBytes(h); err != nil {
		return h, nil, errNativeFixtureDenied
	}
	return h, raw, nil
}

func readShadowV5PreparedHistoryBytes(h nativeHistoricalEvidence) ([]byte, error) {
	path := filepath.Join(shadowRepoRoot(), "deploy", "acceptance", "cloud-shadow-v5-quality-start-history-20261013.json")
	stat, err := os.Lstat(path)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > 1<<20 {
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
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	hash := sha256.Sum256(raw)
	var stored nativeHistoricalEvidence
	if err != nil || int64(len(raw)) != stat.Size() || hex.EncodeToString(hash[:]) != highV5QualityHistoryFileSHA || json.Unmarshal(raw, &stored) != nil || !nativeEqualJSON(stored, h) {
		return nil, errNativeFixtureDenied
	}
	return raw, nil
}

func writeShadowV5ExactBytes(path string, raw []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errNativeFixtureDenied
	}
	n, writeErr := f.Write(raw)
	syncErr, closeErr := f.Sync(), f.Close()
	if writeErr != nil || n != len(raw) || syncErr != nil || closeErr != nil {
		return errNativeFixtureDenied
	}
	return nil
}

// Metadata-only preparation cannot import credentials or start inference.
func TestScenarioShadowV5PreparePins(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_SHADOW_V5_PREPARE") != "1" {
		t.Skip("explicit metadata preparation only")
	}
	private := os.Getenv("BUTLER_SCENARIO_SHADOW_V5_PRIVATE")
	release := os.Getenv("BUTLER_SCENARIO_SHADOW_V5_RELEASE")
	if !filepath.IsAbs(private) || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(release) {
		t.Fatal("invalid preparation scope")
	}
	history, compatibility, err := readShadowV5Prerequisites()
	if err != nil {
		t.Fatal("actual producer compatibility and historical baseline denied")
	}
	historyBytes, err := readShadowV5PreparedHistoryBytes(history)
	if err != nil {
		t.Fatal("exact independently prepared history bytes denied")
	}
	corpus, err := loadShadowV5Corpus()
	if err != nil {
		t.Fatal("frozen corpus denied")
	}
	if checkShadowV5NativeSchemas() != nil {
		t.Fatal("frozen actual native schemas denied before private backend")
	}
	keyFile, ke := acceptance.ReadPrivateFile("/metadata/public-key.hex", 65536)
	approvalFile, ae := acceptance.ReadPrivateFile("/metadata/approval.json", 65536)
	if _, _, err := acceptance.DecodeCloudV5GoldenArtifacts(corpus, keyFile, approvalFile); err != nil || ke != nil || ae != nil {
		t.Fatal("signed independent golden denied")
	}
	frozen, err := acceptance.ReadPrivateFile("/metadata/frozen-corpus.json", 512*1024)
	fh := sha256.Sum256(frozen)
	actual, actualErr := readShadowV5CorpusBytes(filepath.Join(shadowRepoRoot(), "testdata", "acceptance", "ru-cloud-fresh-v5.json"))
	if err != nil || actualErr != nil || hex.EncodeToString(fh[:]) != shadowV5RawCorpusSHA || !bytes.Equal(frozen, actual) {
		t.Fatal("new exact independently reviewed corpus denied")
	}
	hardware, err := acceptance.ReadPrivateFile("/metadata/hardware.json", 65536)
	if err != nil {
		t.Fatal("hardware declaration denied")
	}
	hh := sha256.Sum256(hardware)
	binary, err := os.Open("/binaries/orchestrator.test")
	if err != nil {
		t.Fatal("runtime binary denied")
	}
	rh := sha256.New()
	_, copyErr := io.Copy(rh, binary)
	closeErr := binary.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatal("runtime hash denied")
	}
	source, err := acceptance.TreeHash(shadowRepoRoot(), nil)
	if err != nil {
		t.Fatal("source hash denied")
	}
	pin := acceptance.CloudPin{EvidenceKind: "native_cloud_observation", Provider: acceptance.CloudProvider, RequestedModel: acceptance.CloudRequestedModel,
		WeightsStatus: "unavailable", Runtime: "go1.27.2/race; " + os.Getenv("BUTLER_SCENARIO_SHADOW_V5_RUNTIME_IMAGE"), RuntimeSHA256: hex.EncodeToString(rh.Sum(nil)), CodeCommit: release,
		SourceSHA256: source, HardwareSHA256: hex.EncodeToString(hh[:]), CorpusDigest: acceptance.CloudV5CorpusDigest(corpus), SchemaSHA256: acceptance.CloudV5SchemaDigest(), PolicySHA256: acceptance.CloudV5PolicyDigest(corpus), ResourcesSHA256: acceptance.CloudV5ResourcesDigest(), WindowID: shadowV5NativeWindow, Mode: "shadow_observation"}
	if writeShadowJSON(filepath.Join(private, "pin.json"), pin) != nil || writeShadowJSON(filepath.Join(private, "policy.json"), acceptance.FrozenCloudV5Policy(corpus)) != nil || writeShadowJSON(filepath.Join(private, "resources.json"), fixedShadowV5Resources()) != nil || writeShadowV5ExactBytes(filepath.Join(private, "history.json"), historyBytes) != nil || writeShadowV5ExactBytes(filepath.Join(private, "compatibility.json"), compatibility) != nil {
		t.Fatal("immutable preparation output denied")
	}
}
