package acceptance

import (
	"errors"
	"regexp"
)

// Operator metadata is mounted read-only, outside the candidate's output tree.
// Actual collection checks every identity before opening provider credentials.
type MemoryRuntimePins struct {
	ProxyRequired  bool             `json:"proxy_required,omitempty"`
	Prior          *MemoryPriorPins `json:"prior,omitempty"`
	WindowID       string           `json:"window_id"`
	Release        string           `json:"release"`
	Source         string           `json:"source_sha256"`
	Binary         string           `json:"binary_sha256"`
	Hardware       string           `json:"hardware_sha256"`
	Suite          string           `json:"suite_sha256"`
	Criteria       string           `json:"criteria_sha256"`
	MaxCalls       int64            `json:"maximum_calls"`
	MaxUnits       int64            `json:"maximum_conservative_units"`
	CPUs           int              `json:"cpus"`
	MemoryBytes    int64            `json:"memory_bytes"`
	PIDs           int              `json:"pids"`
	Seconds        int              `json:"window_seconds"`
	MaxEvents      int              `json:"maximum_events_per_entry"`
	MaxReportBytes int64            `json:"maximum_report_bytes"`
}

type MemoryExecutionProof struct {
	ProxyConfigured bool
	LocalBypass     bool
	ID              string
	Image           string
	Path            string
	Args            []string
	User            string
	Memory          int64
	NanoCpus        int64
	PidsLimit       int
	ReadonlyRootfs  bool
	Networks        []string
	Mounts          []struct {
		Destination string
		RW          bool
	}
}

func ValidateMemoryExecution(e MemoryExecutionProof, pins MemoryRuntimePins, image string) error {
	if pins.ProxyRequired && (!e.ProxyConfigured || !e.LocalBypass) {
		return errors.New("required native proxy or local bypass absent")
	}
	if e.Image != image || e.Path != "/runtime/collector.test" || MemoryDigest(e.Args) != MemoryDigest([]string{"-test.run", "^TestMemoryEvalActualCollection$", "-test.v", "-test.timeout", "185m"}) || e.User != "10001:10001" || e.Memory != pins.MemoryBytes || e.NanoCpus != int64(pins.CPUs)*1000000000 || e.PidsLimit != pins.PIDs || !e.ReadonlyRootfs || len(e.Networks) != 3 || !regexp.MustCompile(`^memory-eval-v2-[a-zA-Z0-9_-]+$`).MatchString(pins.WindowID) {
		return errors.New("actual runner resources/command differ")
	}
	networks := map[string]bool{pins.WindowID: true, "butler-lab_backend": true, "butler-lab_local-inference": true}
	for _, n := range e.Networks {
		if !networks[n] {
			return errors.New("actual runner network identity differs")
		}
		delete(networks, n)
	}
	if len(networks) != 0 {
		return errors.New("required runner network absent")
	}
	seen := map[string]bool{}
	for _, m := range e.Mounts {
		if m.Destination == "/private" {
			if !m.RW {
				return errors.New("private output unavailable")
			}
		} else if m.RW {
			return errors.New("writable input mount")
		}
		seen[m.Destination] = true
	}
	for _, required := range []string{"/src", "/runtime", "/metadata", "/review", "/private", "/auth/local-token", "/auth/native-import.json"} {
		if !seen[required] {
			return errors.New("required read-only input absent")
		}
	}
	if pins.Prior != nil && !seen["/prior"] {
		return errors.New("read-only original prefix absent")
	}
	return nil
}
