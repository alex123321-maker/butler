//go:build !windows

package maintenance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDockerBuildOutputCannotEscapeWorkerOrPublishIncompleteStatus(t *testing.T) {
	private := t.TempDir()
	output := filepath.Join(private, "worker-output")
	if err := os.Mkdir(output, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(private, "unmounted-operator-fixture")
	if err := os.WriteFile(outside, []byte("synthetic operator-only fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(output)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if done, _, err := buildExit(root); err != nil || done {
		t.Fatal("missing completion became a result", err)
	}
	if err := os.Symlink(outside, filepath.Join(output, "candidate")); err != nil {
		t.Fatal(err)
	}
	if _, err := readBuildOutput(root, "candidate", 4096); err == nil {
		t.Fatal("worker output followed an escape into operator files")
	}
	if _, err := readBuildOutput(root, "../unmounted-operator-fixture", 4096); err == nil {
		t.Fatal("caller selected an arbitrary host file")
	}
	if err := os.WriteFile(filepath.Join(output, "build-exit"), []byte("0"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := buildExit(root); err == nil {
		t.Fatal("incomplete status qualified")
	}
	if err := os.WriteFile(filepath.Join(output, "build-exit"), []byte("0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if done, code, err := buildExit(root); err != nil || !done || code != "0\n" {
		t.Fatal("complete fixed-supervisor result unavailable", err)
	}
}
