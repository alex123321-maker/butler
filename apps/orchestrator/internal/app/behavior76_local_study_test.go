package app

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// Filled only after public semantics freeze and independent fresh authorship.
// These cannot name any closed corpus, and currently prevent native admission.
const behavior76LocalStimuliSHA = "878f220f31c4e615a56b81cace7be96639a4f4e6dcffee3159b095675ef5d109"
const behavior76LocalExpectationsSHA = "464cb37c9d8bec967baba27fe6b401b6fc41d3d5d855f3ecf5955c04312f4ee0"
const behavior76StreamStimuliSHA = "71ad44ddfadbc18d5cc5c06f5fd4edc043af66584e7f244e7d0b5dac66ddf94b"
const behavior76StreamExpectationsSHA = "84862a7ba12647633c7665e98c9541eb374ced4ad53a2d1fe896f08e6ed494b8"

func TestBehavior76NativeFixtureStream(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_NATIVE_ENABLE") != "fixture-stream-synthetic-only" {
		t.Skip("separate prospective streaming admission required")
	}
	behavior76CollectFreshMode(t, true, true)
}

func TestBehavior76NativeFixtureLocal(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_NATIVE_ENABLE") != "fixture-local-synthetic-only" {
		t.Skip("separate prospective fixture-local admission required")
	}
	behavior76CollectFreshConfigured(t, true)
}

func (f *behavior76Fixture) sealFixtureEvidence(dir string) {
	f.t.Helper()
	if f.authorityTrace != nil {
		trace := f.authorityTrace.snapshot()
		if f.authorityTrace.stream != nil && f.streamQuiescent {
			trace = f.authorityTrace.sealStream()
		} else if f.authorityTrace.stream != nil {
			trace = f.authorityTrace.unsealedStreamEvidence()
		}
		behavior76Write(f.t, filepath.Join(dir, "authority-trace.json"), trace)
	}
	anchor, err := os.ReadFile(f.manifestPath)
	if err != nil {
		f.t.Fatal("exact deletion anchor unavailable")
	}
	file, err := os.OpenFile(filepath.Join(dir, "deletions.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		f.t.Fatal(err)
	}
	_, writeErr := file.Write(anchor)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		f.t.Fatal("exact exported deletion anchor unavailable")
	}
	type item struct {
		Path   string `json:"path"`
		Bytes  int64  `json:"bytes"`
		SHA256 string `json:"sha256"`
	}
	items := []item{}
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fs.ErrInvalid
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		items = append(items, item{Path: filepath.ToSlash(rel), Bytes: int64(len(raw)), SHA256: behavior76Hash(raw)})
		return nil
	})
	if err != nil {
		f.t.Fatal("exact fixture export inventory unavailable", err)
	}
	behavior76Write(f.t, filepath.Join(dir, "sealed-files.json"), map[string]any{"version": "behavior76-quiescent-fixture-files-v1", "files": items})
	if f.authorityTrace != nil && f.authorityTrace.stream != nil {
		trace := f.authorityTrace.snapshot()
		if !f.streamQuiescent || trace.stream == nil || !trace.stream.Sealed || trace.Failed || trace.Dropped != 0 || trace.OpenSpans != 0 || f.researchStop.Load() {
			f.researchStop.Store(true)
			f.t.Error("stream evidence retained but observer/quiescence seal failed globally")
		}
	}
}
