package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDeletionAnchorRejectsSymlinkReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "anchor")
	m := NewFileManifest(path)
	if e := m.Initialize(); e != nil {
		t.Fatal(e)
	}
	record := ManifestRecord{ID: "deletion", SourceID: "source", OwnerHash: hash("owner"), Sequence: 1, Action: "delete", Time: time.Now()}
	if e := m.Append(context.Background(), record); e != nil {
		t.Fatal(e)
	}
	original := path + ".original"
	if e := os.Rename(path, original); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(original, path); e != nil {
		t.Fatal(e)
	}
	if _, e := m.Load(context.Background()); !errors.Is(e, ErrRestore) {
		t.Fatal("symlink read", e)
	}
	record.ID = "next"
	record.Sequence = 2
	if e := m.Append(context.Background(), record); !errors.Is(e, ErrRestore) {
		t.Fatal("symlink append", e)
	}
	if e := m.Initialize(); e == nil {
		t.Fatal("symlink replaced by initialization")
	}
	rows, e := NewFileManifest(original).Load(context.Background())
	if e != nil || len(rows) != 1 {
		t.Fatal("target modified", rows, e)
	}
}
