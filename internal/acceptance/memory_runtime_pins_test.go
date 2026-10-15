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
	pins.ProxyRequired = true
	if ValidateMemoryExecution(e, pins, e.Image) == nil {
		t.Fatal("missing required proxy accepted")
	}
	e.ProxyConfigured = true
	if ValidateMemoryExecution(e, pins, e.Image) == nil {
		t.Fatal("missing local bypass accepted")
	}
	e.LocalBypass = true
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

func TestMemoryV2AuthExecutionProofBeforeCredentials(t *testing.T) {
	pins := MemoryRuntimePins{WindowID: "memory-eval-v2-renewed-test", Renewal: &MemoryRenewal{}}
	e := MemoryExecutionProof{Image: "pinned-image", Path: "/runtime/collector.test", Args: []string{"-test.run", "^TestMemoryEvalNativeAuthPreflight$", "-test.v", "-test.timeout", "100s"}, User: "10001:10001", Memory: 536870912, NanoCpus: 1000000000, PidsLimit: 64, ReadonlyRootfs: true, Networks: []string{"butler-lab_backend", pins.WindowID}, ProxyConfigured: true, LocalBypass: true}
	for _, destination := range []string{"/src", "/runtime", "/metadata", "/prior", "/private", "/auth/native-import.json"} {
		e.Mounts = append(e.Mounts, struct {
			Destination string
			RW          bool
		}{destination, destination == "/private"})
	}
	if err := ValidateMemoryAuthExecution(e, pins, e.Image); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"uid", "image", "network", "proxy", "write_source", "resource", "args", "extra_mount"} {
		bad := e
		bad.Mounts = append([]struct {
			Destination string
			RW          bool
		}(nil), e.Mounts...)
		switch kind {
		case "uid":
			bad.User = "0:0"
		case "image":
			bad.Image = "other"
		case "network":
			bad.Networks = []string{"butler-lab_backend", "other"}
		case "proxy":
			bad.ProxyConfigured = false
		case "write_source":
			bad.Mounts[0].RW = true
		case "resource":
			bad.PidsLimit = 128
		case "args":
			bad.Args = []string{"smoke"}
		case "extra_mount":
			bad.Mounts = append(bad.Mounts, struct {
				Destination string
				RW          bool
			}{"/old-private", false})
		}
		if ValidateMemoryAuthExecution(bad, pins, e.Image) == nil {
			t.Fatal("unsafe auth runtime accepted", kind)
		}
	}
}
