package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTraceReplayRequiresPinnedCollectorAndCompleteCohort(t *testing.T) {
	root := t.TempDir()
	manifest := []byte(`{"scheduled":36,"rows":[]}`)
	if err := os.WriteFile(filepath.Join(root, "public-report.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "new-report.json")
	if err := replayTraceRoot(root, strings.Repeat("a", 64), out); err == nil || err.Error() != "original manifest mismatch" {
		t.Fatal("accepted untrusted collector manifest", err)
	}
	if err := replayTraceRoot(root, bytesSHA(manifest), out); err == nil || err.Error() != "complete workflow cohort required" {
		t.Fatal("accepted missing scheduled cases", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("wrote report for rejected trace")
	}
	if err := run([]string{"-trace-root", root, "-manifest-sha256", bytesSHA(manifest), "-suite", "foreign", "-out", out}); err == nil {
		t.Fatal("mixed live observation and closed trace modes")
	}
}
