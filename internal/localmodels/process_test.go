package localmodels

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func processFixture(t *testing.T, body string) *Process {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("lab Linux process contract")
	}
	root := t.TempDir()
	binary := filepath.Join(root, "runtime-fixture")
	script := filepath.Join(root, "worker.py")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"+body), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := NewProcess(ProcessConfig{Python: binary, Script: script, ModelRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Unload(context.Background(), "model") })
	return b
}
func TestPinnedMetadataIsBoundedAndContained(t *testing.T) {
	b := processFixture(t, "exit 0\n")
	p := profile()
	p.Model = "google/embeddinggemma-2"
	p.Runtime = "torch-sentence-transformers"
	p.SourceUrl = "https://huggingface.co/" + p.Model
	p.Revision = strings.Repeat("a", 40)
	directory := filepath.Join(b.cfg.ModelRoot, p.Revision)
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(map[string]string{"model": p.Model, "revision": p.Revision, "runtime_version": p.RuntimeVersion})
	path := filepath.Join(directory, "butler-model.json")
	if err := os.WriteFile(path, meta, 0600); err != nil {
		t.Fatal(err)
	}
	if err := b.Verify(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	p.SourceUrl += "?token=credential"
	if err := b.Verify(context.Background(), p); !isFault(err, Unavailable) {
		t.Fatal("untrusted source accepted")
	}
	p.SourceUrl = "https://huggingface.co/" + p.Model
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "metadata")
	if err := os.WriteFile(outside, meta, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if err := b.Verify(context.Background(), p); !isFault(err, Unavailable) {
		t.Fatal("metadata escaped model root")
	}
}
func TestProcessCancellationKillsAndWaitsForOwnedRuntime(t *testing.T) {
	b := processFixture(t, "read init\nprintf '%s\\n' '{\"kind\":\"ready\",\"vram_mib\":0}'\nread job\nwhile :; do :; done\n")
	p := profile()
	p.PreprocessingVersion = "embeddinggemma2-retrieval-v1"
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := b.Infer(ctx, p, request("cancel"), "cpu", 1)
	if err != context.DeadlineExceeded || time.Since(started) > time.Second {
		t.Fatalf("worker did not stop promptly: %v", err)
	}
	resident, err := b.Resident(context.Background())
	if err != nil || len(resident) != 0 || b.cmd != nil {
		t.Fatal("canceled process retained as resident")
	}
}
func TestProcessEnvironmentAndRawErrorAreNotExposed(t *testing.T) {
	t.Setenv("BUTLER_DATABASE_PASSWORD", "secret-should-never-be-forwarded")
	b := processFixture(t, "read init\nif [ -n \"$BUTLER_DATABASE_PASSWORD\" ]; then exit 1; fi\nprintf '%s\\n' '{\"kind\":\"ready\",\"vram_mib\":0}'\nread job\nprintf '%s\\n' '{\"kind\":\"error\",\"code\":\"secret-raw-library-exception\"}'\n")
	p := profile()
	p.PreprocessingVersion = "embeddinggemma2-retrieval-v1"
	_, err := b.Infer(context.Background(), p, request("redact"), "cpu", 1)
	if !isFault(err, Unavailable) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error leaked or ambient environment inherited: %v", err)
	}
}
func TestProcessWarmReuseRequiresSameRevisionAndProfile(t *testing.T) {
	b := processFixture(t, "read init\nprintf '%s\\n' '{\"kind\":\"ready\",\"vram_mib\":0}'\nwhile :; do :; done\n")
	p := profile()
	if err := b.ensure(context.Background(), p, "cpu"); err != nil {
		t.Fatal(err)
	}
	first := b.cmd
	if err := b.ensure(context.Background(), p, "cpu"); err != nil || b.cmd != first {
		t.Fatal("same profile did not reuse the worker")
	}
	p.Revision = "new-revision"
	if err := b.ensure(context.Background(), p, "cpu"); err != nil {
		t.Fatal(err)
	}
	if b.cmd == first || first.ProcessState == nil {
		t.Fatal("old revision worker survived profile change")
	}
}
