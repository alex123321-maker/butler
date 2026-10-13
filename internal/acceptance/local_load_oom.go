package acceptance

import (
	"fmt"
	"path/filepath"
)

const LocalLoadOOMVersion = "butler-cached-cpu-load-oom-v1"
const localOOMImage = "sha256:b86366bb528bbf7f1424435d165028497a5b69bf6ddb4fa5a87102e2b79f44fb"
const localOOMWeights = "7f4030143c1c477224c5434f8272c662a8b042079a0a584f0a27a1684fe2e1fa"
const localOOMManifest = "6f76d4346c34ba89df6e137fb661b4bb4daf322c1151b6ef0dfda981070a851f"
const localOOMRegistryManifest = "7df6b6e09427a769808717c0a93cadc4ae99ed4eb8bf5ca557c90846becea435"

// This proof describes one physical model LOAD only. It cannot assert a cloud
// RAM measurement, an inference result, or an owning-kernel receipt.
type LocalLoadOOM struct {
	Version                 string `json:"version"`
	RunnerSHA256            string `json:"runner_sha256"`
	Image                   string `json:"image"`
	WeightsSHA256           string `json:"weights_sha256"`
	WeightsBytes            int64  `json:"weights_bytes"`
	ManifestSHA256          string `json:"manifest_sha256"`
	RegistryManifestSHA256  string `json:"registry_manifest_sha256"`
	ActualModelLoadAttempts int    `json:"actual_model_load_attempts"`
	ActualInferenceCalls    int    `json:"actual_inference_calls"`
	ActualNativeCalls       int    `json:"actual_native_calls"`
	ModelMetadataRead       bool   `json:"model_metadata_read"`
	OOMKilled               bool   `json:"oom_killed"`
	ExitCode                int    `json:"exit_code"`
	ElapsedSeconds          int    `json:"elapsed_seconds"`
	MemoryBytes             int64  `json:"memory_bytes"`
	MemorySwapBytes         int64  `json:"memory_swap_bytes"`
	NanoCPUs                int64  `json:"nano_cpus"`
	PidsLimit               int64  `json:"pids_limit"`
	Runtime                 string `json:"runtime"`
	NetworkMode             string `json:"network_mode"`
	User                    string `json:"user"`
	ReadOnlyRoot            bool   `json:"read_only_root"`
	OnlyReadOnlyWeightMount bool   `json:"only_read_only_weight_mount"`
	NoDeviceRequests        bool   `json:"no_device_requests"`
	NoCapabilities          bool   `json:"no_capabilities"`
	NoNewPrivileges         bool   `json:"no_new_privileges"`
	CPUOnly                 bool   `json:"cpu_only"`
	Model                   string `json:"model"`
	Stage                   string `json:"stage"`
	ButlerKernelReceivedOOM bool   `json:"butler_kernel_received_oom"`
}

func ReadLocalLoadOOM(root, path string) (LocalLoadOOM, error) {
	var p LocalLoadOOM
	raw, err := ReadPrivateFile(path, 8192)
	if err != nil {
		return p, fmt.Errorf("private bounded local load proof required")
	}
	if err = DecodeStrict(raw, &p, "version", "runner_sha256", "image", "weights_sha256", "weights_bytes", "manifest_sha256", "registry_manifest_sha256", "actual_model_load_attempts", "actual_inference_calls", "actual_native_calls", "model_metadata_read", "oom_killed", "exit_code", "elapsed_seconds", "memory_bytes", "memory_swap_bytes", "nano_cpus", "pids_limit", "runtime", "network_mode", "user", "read_only_root", "only_read_only_weight_mount", "no_device_requests", "no_capabilities", "no_new_privileges", "cpu_only", "model", "stage", "butler_kernel_received_oom"); err != nil {
		return p, fmt.Errorf("invalid local load proof envelope")
	}
	return p, validateLocalLoadOOM(root, p)
}

func validateLocalLoadOOM(root string, p LocalLoadOOM) error {
	if err := validateLocalLoadExperiment(root, p); err != nil {
		return err
	}
	if !p.ModelMetadataRead {
		return fmt.Errorf("required model metadata witness absent; physical OOM remains failed evidence")
	}
	return nil
}

func validateLocalLoadExperiment(root string, p LocalLoadOOM) error {
	runner, err := FileHash(filepath.Join(root, "scripts", "dev", "local-model-load-oom-lab.sh"))
	if err != nil || p.RunnerSHA256 != runner {
		return fmt.Errorf("local load proof runner bytes differ from current release")
	}
	if p.Version != LocalLoadOOMVersion || p.Image != localOOMImage || p.WeightsSHA256 != localOOMWeights || p.WeightsBytes != 522640096 || p.ManifestSHA256 != localOOMManifest || p.RegistryManifestSHA256 != localOOMRegistryManifest || p.ActualModelLoadAttempts != 1 || p.ActualInferenceCalls != 0 || p.ActualNativeCalls != 0 || !p.OOMKilled || p.ExitCode != 137 || p.ElapsedSeconds < 0 || p.ElapsedSeconds > 90 || p.MemoryBytes != 268435456 || p.MemorySwapBytes != 268435456 || p.NanoCPUs != 1000000000 || p.PidsLimit != 64 || p.Runtime != "runc" || p.NetworkMode != "none" || p.User != "10001:10001" || !p.ReadOnlyRoot || !p.OnlyReadOnlyWeightMount || !p.NoDeviceRequests || !p.NoCapabilities || !p.NoNewPrivileges || !p.CPUOnly || p.Model != "qwen3:0.6b" || p.Stage != "load_before_inference" || p.ButlerKernelReceivedOOM {
		return fmt.Errorf("physical CPU model load OOM or isolation requirement failed")
	}
	return nil
}
