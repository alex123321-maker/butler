package acceptance

import "testing"

func TestMemoryV2ExecutionNetworkProof(t *testing.T) {
	pins := MemoryRuntimePins{WindowID: "memory-eval-v2-network-fixture", CPUs: 2, MemoryBytes: 1610612736, PIDs: 128}
	e := MemoryExecutionProof{Image: "pinned-image", Path: "/runtime/collector.test", Args: []string{"-test.run", "^TestMemoryEvalActualCollection$", "-test.v", "-test.timeout", "185m"}, User: "10001:10001", Memory: pins.MemoryBytes, NanoCpus: 2000000000, PidsLimit: pins.PIDs, ReadonlyRootfs: true, Networks: []string{pins.WindowID, "butler-lab_backend", "butler-lab_local-inference"}}
	for _, destination := range []string{"/src", "/runtime", "/metadata", "/review", "/private", "/auth/local-token", "/auth/native-import.json"} {
		e.Mounts = append(e.Mounts, struct {
			Destination string
			RW          bool
		}{destination, destination == "/private"})
	}
	if err := ValidateMemoryExecution(e, pins, e.Image); err != nil {
		t.Fatal(err)
	}
	for _, networks := range [][]string{
		{pins.WindowID, "butler-lab_backend"},
		{pins.WindowID, "butler-lab_backend", "different-local-network"},
		{pins.WindowID, "butler-lab_backend", "butler-lab_backend"},
	} {
		bad := e
		bad.Networks = networks
		if ValidateMemoryExecution(bad, pins, e.Image) == nil {
			t.Fatal("missing, substituted or duplicate network accepted", networks)
		}
	}
}
