package acceptance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestApprovedVocabularyWitnessPreservesOldFailAndDoesNotClaimFullWeightsInferenceOrKernel(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "acceptance", "local-load-oom-original-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(filepath.Join("..", "..", "testdata", "acceptance", "local-load-oom-public-log-v1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var proof LocalLoadOOM
	if err = json.Unmarshal(raw, &proof); err != nil {
		t.Fatal(err)
	}
	started := []byte(approvedOriginalLoadDirectory + "\n")
	witness, err := verifyApprovedLocalLoadWitness(raw, log, started, proof)
	if err != nil || witness.OriginalStrictGate != "FAIL" || witness.OriginalMetadataRead || !witness.VocabularyMetadataProcessingReached || witness.CompleteWeightsLoaded || witness.ActualInferenceCalls != 0 || witness.ActualNativeCalls != 0 || witness.ActualModelLoadAttempts != 1 || witness.ButlerKernelReceivedOOM || witness.DeploymentAuthority || witness.Reviewer != approvedLoadReviewer {
		t.Fatal("additive witness changed original failure or inflated scope", witness, err)
	}
	for _, alter := range []func(*LocalLoadOOM){func(p *LocalLoadOOM) { p.OOMKilled = false }, func(p *LocalLoadOOM) { p.ExitCode = 1 }, func(p *LocalLoadOOM) { p.ModelMetadataRead = true }, func(p *LocalLoadOOM) { p.ButlerKernelReceivedOOM = true }, func(p *LocalLoadOOM) { p.ActualInferenceCalls = 1 }, func(p *LocalLoadOOM) { p.ActualModelLoadAttempts = 2 }, func(p *LocalLoadOOM) { p.Image = "different-image" }, func(p *LocalLoadOOM) { p.WeightsSHA256 = "different-weights" }} {
		changed := proof
		alter(&changed)
		if _, err = verifyApprovedLocalLoadWitness(raw, log, started, changed); err == nil {
			t.Fatal("different physical observation accepted")
		}
	}
	for _, changed := range [][]byte{append([]byte{}, append(raw, ' ')...), bytes.Replace(raw, []byte(`"exit_code":137`), []byte(`"exit_code":1`), 1)} {
		if _, err = verifyApprovedLocalLoadWitness(changed, log, started, proof); err == nil {
			t.Fatal("changed immutable original accepted")
		}
	}
	for _, changed := range [][]byte{bytes.Replace(log, []byte("128247"), []byte("128246"), 1), append(append([]byte{}, log...), []byte("different\n")...)} {
		if _, err = verifyApprovedLocalLoadWitness(raw, changed, started, proof); err == nil {
			t.Fatal("unreviewed log accepted")
		}
	}
	if _, err = verifyApprovedLocalLoadWitness(raw, log, []byte("different-root\n"), proof); err == nil {
		t.Fatal("global single-start identity changed")
	}
}

func TestVocabularyWitnessRequiresBothExactOrderedModelSpecificMarkers(t *testing.T) {
	loading := []byte(approvedModelLoadingPattern + "\n")
	vocabulary := []byte(approvedVocabularyPattern + "\n")
	if !orderedVocabularyWitness(append(append([]byte{}, loading...), vocabulary...)) {
		t.Fatal("approved model/vocabulary order denied")
	}
	for _, changed := range [][]byte{loading, vocabulary, append(append([]byte{}, vocabulary...), loading...), append(append(append([]byte{}, loading...), vocabulary...), vocabulary...), bytes.Replace(append(append([]byte{}, loading...), vocabulary...), []byte("/model.gguf"), []byte("/different.gguf"), 1), bytes.Replace(append(append([]byte{}, loading...), vocabulary...), []byte("128247"), []byte("128248"), 1)} {
		if orderedVocabularyWitness(changed) {
			t.Fatal("missing/reversed/duplicate/other-model marker accepted")
		}
	}
}
