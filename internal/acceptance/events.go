package acceptance

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type TestEvent struct {
	Action  string
	Package string
	Test    string
	Elapsed float64
	Output  string
}

type TestProof struct {
	Name    string  `json:"name"`
	Status  string  `json:"status"`
	Seconds float64 `json:"seconds"`
	Reason  string  `json:"reason,omitempty"`
}

// VerifyEvents accepts go test -json and go tool test2json output. It checks
// top-level required tests, package completion, duplicates and scanner errors.
// Output text is deliberately not copied into reports (may contain user data).
func VerifyEvents(r io.Reader, g Group) ([]TestProof, error) {
	expected := make(map[string]bool, len(g.Tests))
	for _, name := range g.Tests {
		expected[name] = true
	}
	proof := make(map[string]TestProof)
	started := make(map[string]bool)
	packagePassed := false
	packageFailure := ""
	// Read one extra byte: an exact prefix ending in package pass must never
	// conceal additional (or malformed) events beyond the artifact limit.
	limited := &io.LimitedReader{R: r, N: (32 << 20) + 1}
	s := bufio.NewScanner(limited)
	s.Buffer(make([]byte, 4096), 1<<20)
	for s.Scan() {
		var e TestEvent
		if err := json.Unmarshal(s.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("invalid test event: %w", err)
		}
		if e.Package != Module+g.Package {
			return nil, fmt.Errorf("unexpected test package %q", e.Package)
		}
		if packagePassed || packageFailure != "" {
			return nil, fmt.Errorf("event after terminal package result")
		}
		if e.Elapsed < 0 {
			return nil, fmt.Errorf("negative test duration")
		}
		if e.Test == "" {
			if e.Action == "fail" || e.Action == "skip" {
				packageFailure = e.Action
			}
			if e.Action == "pass" {
				if packagePassed {
					return nil, fmt.Errorf("duplicate package result")
				}
				packagePassed = true
			}
			continue
		}
		if !expected[e.Test] {
			continue
		}
		if e.Action == "run" {
			if started[e.Test] {
				return nil, fmt.Errorf("duplicate test run %s", e.Test)
			}
			started[e.Test] = true
		}
		if e.Action == "pass" || e.Action == "fail" || e.Action == "skip" {
			if !started[e.Test] {
				return nil, fmt.Errorf("test result without run: %s", e.Test)
			}
			if _, ok := proof[e.Test]; ok {
				return nil, fmt.Errorf("duplicate test result: %s", e.Test)
			}
			p := TestProof{Name: e.Test, Status: strings.ToUpper(e.Action), Seconds: e.Elapsed}
			if e.Action == "skip" {
				p.Status = "FAIL"
				p.Reason = "required test skipped (often missing integration environment)"
			}
			proof[e.Test] = p
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if limited.N == 0 {
		return nil, fmt.Errorf("test event artifact exceeds 32 MiB")
	}
	out := make([]TestProof, 0, len(g.Tests))
	for _, name := range g.Tests {
		p, ok := proof[name]
		if !ok {
			p = TestProof{Name: name, Status: "FAIL", Reason: "required result missing"}
		}
		out = append(out, p)
	}
	if !packagePassed {
		if packageFailure != "" {
			return out, fmt.Errorf("package %s", packageFailure)
		}
		return out, fmt.Errorf("package completion missing")
	}
	if packageFailure != "" {
		return out, fmt.Errorf("package %s", packageFailure)
	}
	return out, nil
}
