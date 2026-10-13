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
func TestScenarioShadowPreparePins(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_SHADOW_PREPARE") != "1" {
		t.Skip("explicit metadata preparation only")
	}
	private := os.Getenv("BUTLER_SCENARIO_SHADOW_PRIVATE")
	release := os.Getenv("BUTLER_SCENARIO_SHADOW_RELEASE")
	if !filepath.IsAbs(private) || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(release) {
		t.Fatal("invalid preparation scope")
	}
	corpus, err := loadShadowCorpus()
	if err != nil {
		t.Fatal("frozen corpus denied")
	}
	keyFile, ke := acceptance.ReadPrivateFile("/metadata/public-key.hex", 65536)
	approvalFile, ae := acceptance.ReadPrivateFile("/metadata/approval.json", 65536)
	if _, _, err := acceptance.DecodeCloudGoldenArtifacts(corpus, keyFile, approvalFile); err != nil || ke != nil || ae != nil {
		t.Fatal("signed independent golden denied")
	}
	frozen, err := acceptance.ReadPrivateFile("/metadata/frozen-corpus.json", 128*1024)
	fh := sha256.Sum256(frozen)
	actual, actualErr := readShadowCorpusBytes(filepath.Join(shadowRepoRoot(), "testdata", "acceptance", "ru-scenarios-v3.json"))
	if err != nil || actualErr != nil || hex.EncodeToString(fh[:]) != "5941a750b9f053b0eabdda249dd4b257e56c3264cfb31aa4f3950ecc2d98d8a2" || !bytes.Equal(bytes.ReplaceAll(frozen, []byte("\r\n"), []byte("\n")), actual) {
		t.Fatal("raw review CRLF provenance denied")
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
		WeightsStatus: "unavailable", Runtime: "go1.27.2/race; " + os.Getenv("BUTLER_SCENARIO_SHADOW_RUNTIME_IMAGE"), RuntimeSHA256: hex.EncodeToString(rh.Sum(nil)), CodeCommit: release,
		SourceSHA256: source, HardwareSHA256: hex.EncodeToString(hh[:]), CorpusDigest: acceptance.CorpusDigest(corpus), SchemaSHA256: acceptance.CloudSchemaDigest(), PolicySHA256: acceptance.CloudPolicyDigest(), ResourcesSHA256: shadowHash(fixedShadowResources()), WindowID: shadowNativeWindow, Mode: "shadow_observation"}
	if writeShadowJSON(filepath.Join(private, "pin.json"), pin) != nil || writeShadowJSON(filepath.Join(private, "policy.json"), acceptance.FrozenCloudPolicy()) != nil || writeShadowJSON(filepath.Join(private, "resources.json"), fixedShadowResources()) != nil {
		t.Fatal("immutable preparation output denied")
	}
}
