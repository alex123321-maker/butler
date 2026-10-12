package acceptance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
)

const LocalLoadWitnessVersion = "butler-local-load-vocabulary-witness-v2"
const approvedLoadReportSHA = "23fc8f4cab1d105333d3057f136008d7ce0964feba7fbea0bec222c00c90b234"
const approvedLoaderLogSHA = "2303783bdb57a79266815f68908a10b053107acaebd3485badce8bbb7cf0d5ab"
const approvedLoadStartedSHA = "0d8e4dd803b8fbea8eeaa7fd2d463cee366b0c2b362dd5f3211b2d4c9335c38a"
const approvedLoadReviewer = "codex-independent-cloud-critic-20261012"
const globalLoadStarted = "/srv/storage/butler/fault-acceptance-windows/issue33-cached-qwen3-load-20261012/load-started/report-path.txt"
const approvedOriginalLoadDirectory = "/srv/storage/butler/agents/scenario-full-faults/reports/local-model-load-oom-20261012T104346Z-1157147"
const approvedModelLoadingPattern = "I srv    load_model: loading model '/model.gguf'"
const approvedVocabularyPattern = "W load: control-looking token: 128247 '</s>' was not control-type; this is probably a bug in the model. its type will be overridden"

type LocalLoadWitness struct {
	Version                             string `json:"version"`
	OriginalReportSHA256                string `json:"original_report_sha256"`
	OriginalStrictGate                  string `json:"original_strict_gate"`
	OriginalMetadataRead                bool   `json:"original_metadata_read"`
	LoaderLogSHA256                     string `json:"loader_log_sha256"`
	GlobalLoadStartedSHA256             string `json:"global_load_started_sha256"`
	Image                               string `json:"image"`
	WeightsSHA256                       string `json:"weights_sha256"`
	Reviewer                            string `json:"reviewer"`
	ReviewBasis                         string `json:"review_basis"`
	Scope                               string `json:"scope"`
	VocabularyMetadataProcessingReached bool   `json:"vocabulary_metadata_processing_reached"`
	CompleteWeightsLoaded               bool   `json:"complete_weights_loaded"`
	ActualModelLoadAttempts             int    `json:"actual_model_load_attempts"`
	ActualInferenceCalls                int    `json:"actual_inference_calls"`
	ActualNativeCalls                   int    `json:"actual_native_calls"`
	ButlerKernelReceivedOOM             bool   `json:"butler_kernel_received_oom"`
	DeploymentAuthority                 bool   `json:"deployment_authority"`
}

func approvedLocalLoadRules() any {
	return struct{ Version, Report, Log, Started, Reviewer, Loading, Vocabulary string }{LocalLoadWitnessVersion, approvedLoadReportSHA, approvedLoaderLogSHA, approvedLoadStartedSHA, approvedLoadReviewer, approvedModelLoadingPattern, approvedVocabularyPattern}
}

func hashBytes(raw []byte) string { hash := sha256.Sum256(raw); return hex.EncodeToString(hash[:]) }

// The independently reviewed exception is pinned to exactly one immutable
// failed artifact and its public no-input loader log. It never rewrites that
// artifact, pretends to hold a signature, launches another process or gives
// model/deployment authority. New experiments require another reviewed gate.
func ReadApprovedLocalLoadWitness(root, originalPath string) (LocalLoadWitness, error) {
	raw, err := ReadPrivateFile(originalPath, 8192)
	if err != nil {
		return LocalLoadWitness{}, fmt.Errorf("immutable approved load report required")
	}
	log, err := ReadPrivateFile(filepath.Join(filepath.Dir(originalPath), "public-weight-loader.log"), 4096)
	if err != nil {
		return LocalLoadWitness{}, fmt.Errorf("immutable approved public loader log required")
	}
	started, err := ReadPrivateFile(globalLoadStarted, 1024)
	if err != nil {
		return LocalLoadWitness{}, fmt.Errorf("global one-start proof required")
	}
	proof, strictErr := ReadLocalLoadOOM(root, originalPath)
	if strictErr == nil || validateLocalLoadExperiment(root, proof) != nil || proof.ModelMetadataRead {
		return LocalLoadWitness{}, fmt.Errorf("only the exact reviewed strict metadata-witness failure is eligible")
	}
	return verifyApprovedLocalLoadWitness(raw, log, started, proof)
}

func verifyApprovedLocalLoadWitness(raw, log, started []byte, proof LocalLoadOOM) (LocalLoadWitness, error) {
	if hashBytes(raw) != approvedLoadReportSHA || hashBytes(log) != approvedLoaderLogSHA || hashBytes(started) != approvedLoadStartedSHA || string(started) != approvedOriginalLoadDirectory+"\n" {
		return LocalLoadWitness{}, fmt.Errorf("independently approved physical witness hash differs")
	}
	if proof.ModelMetadataRead || !proof.OOMKilled || proof.ExitCode != 137 || proof.Image != localOOMImage || proof.WeightsSHA256 != localOOMWeights || proof.ActualModelLoadAttempts != 1 || proof.ActualInferenceCalls != 0 || proof.ActualNativeCalls != 0 || proof.ButlerKernelReceivedOOM {
		return LocalLoadWitness{}, fmt.Errorf("physical witness scope differs from reviewed before-inference load")
	}
	if !orderedVocabularyWitness(log) {
		return LocalLoadWitness{}, fmt.Errorf("reviewed ordered model/vocabulary metadata witness absent")
	}
	return LocalLoadWitness{Version: LocalLoadWitnessVersion, OriginalReportSHA256: approvedLoadReportSHA, OriginalStrictGate: "FAIL", OriginalMetadataRead: false, LoaderLogSHA256: approvedLoaderLogSHA, GlobalLoadStartedSHA256: approvedLoadStartedSHA, Image: localOOMImage, WeightsSHA256: localOOMWeights, Reviewer: approvedLoadReviewer, ReviewBasis: "independent review plus root-approved fixed hashes and ordered primary-source logger premise; no signature or rebuilt-binary attestation", Scope: "cpu_model_load_vocabulary_metadata_before_inference", VocabularyMetadataProcessingReached: true, ActualModelLoadAttempts: 1}, nil
}

func orderedVocabularyWitness(log []byte) bool {
	loading := bytes.Index(log, []byte(approvedModelLoadingPattern))
	vocabulary := bytes.Index(log, []byte(approvedVocabularyPattern))
	return loading >= 0 && vocabulary > loading && bytes.Count(log, []byte(approvedModelLoadingPattern)) == 1 && bytes.Count(log, []byte(approvedVocabularyPattern)) == 1
}
