package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/butler/butler/internal/acceptance"
)

// Metadata-only preparation cannot import credentials or start inference.
func TestScenarioShadowV3PreparePins(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_SHADOW_V3_PREPARE") != "1" {
		t.Skip("explicit metadata preparation only")
	}
	private := os.Getenv("BUTLER_SCENARIO_SHADOW_V3_PRIVATE")
	release := os.Getenv("BUTLER_SCENARIO_SHADOW_V3_RELEASE")
	if !filepath.IsAbs(private) || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(release) {
		t.Fatal("invalid preparation scope")
	}
	corpus, err := loadShadowV3Corpus()
	if err != nil {
		t.Fatal("frozen corpus denied")
	}
	keyFile, ke := acceptance.ReadPrivateFile("/metadata/public-key.hex", 65536)
	approvalFile, ae := acceptance.ReadPrivateFile("/metadata/approval.json", 65536)
	if _, _, err := acceptance.DecodeCloudV3GoldenArtifacts(corpus, keyFile, approvalFile); err != nil || ke != nil || ae != nil {
		t.Fatal("signed independent golden denied")
	}
	frozen, err := acceptance.ReadPrivateFile("/metadata/frozen-corpus.json", 512*1024)
	fh := sha256.Sum256(frozen)
	actual, actualErr := readShadowV3CorpusBytes(filepath.Join(shadowRepoRoot(), "testdata", "acceptance", "ru-cloud-fresh-v3.json"))
	if err != nil || actualErr != nil || hex.EncodeToString(fh[:]) != shadowV3RawCorpusSHA || !bytes.Equal(frozen, actual) {
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
		WeightsStatus: "unavailable", Runtime: "go1.27.2/race; " + os.Getenv("BUTLER_SCENARIO_SHADOW_V3_RUNTIME_IMAGE"), RuntimeSHA256: hex.EncodeToString(rh.Sum(nil)), CodeCommit: release,
		SourceSHA256: source, HardwareSHA256: hex.EncodeToString(hh[:]), CorpusDigest: acceptance.CloudV3CorpusDigest(corpus), SchemaSHA256: acceptance.CloudV3SchemaDigest(), PolicySHA256: acceptance.CloudV3PolicyDigest(corpus), ResourcesSHA256: acceptance.CloudV3ResourcesDigest(), WindowID: shadowV3NativeWindow, Mode: "shadow_observation"}
	if writeShadowJSON(filepath.Join(private, "pin.json"), pin) != nil || writeShadowJSON(filepath.Join(private, "policy.json"), acceptance.FrozenCloudV3Policy(corpus)) != nil || writeShadowJSON(filepath.Join(private, "resources.json"), fixedShadowV3Resources()) != nil {
		t.Fatal("immutable preparation output denied")
	}
}
