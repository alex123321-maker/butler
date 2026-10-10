package catalog

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Manifest is an anti-rollback anchor outside database backups. It contains no
// source text, URLs or raw owner identifier. PostgreSQL owns deletion state;
// the anchor prevents accepting a restored database which forgot that state.
type ManifestRecord struct {
	ID        string    `json:"deletion_id"`
	SourceID  string    `json:"source_id"`
	OwnerHash string    `json:"owner_hash"`
	Action    string    `json:"action"`
	Sequence  int64     `json:"sequence"`
	Time      time.Time `json:"time"`
}
type Manifest interface {
	Load(context.Context) ([]ManifestRecord, error)
	Append(context.Context, ManifestRecord) error
}
type FileManifest struct {
	path string
	mu   sync.Mutex
}

func NewFileManifest(path string) *FileManifest { return &FileManifest{path: path} }

// Initialize is operator setup, never an automatic fallback for a missing
// deletion anchor. A missing file during normal startup denies all memory use.
func (f *FileManifest) Initialize() error {
	if !filepath.IsAbs(f.path) {
		return ErrInvalid
	}
	file, e := os.OpenFile(f.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer file.Close()
	if _, e = file.WriteString("butler-memory-deletions-v1\n"); e != nil {
		return e
	}
	if e = file.Sync(); e != nil {
		return e
	}
	// Persist the newly created directory entry as well as the file contents.
	dir, e := os.Open(filepath.Dir(f.path))
	if e != nil {
		return e
	}
	defer dir.Close()
	return dir.Sync()
}
func (f *FileManifest) Load(ctx context.Context) ([]ManifestRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.load(ctx)
}
func (f *FileManifest) load(ctx context.Context) ([]ManifestRecord, error) {
	file, e := os.Open(f.path)
	if e != nil {
		return nil, ErrRestore
	}
	defer file.Close()
	info, e := file.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 64*1024*1024 || info.Size() == 0 {
		return nil, ErrRestore
	}
	var last [1]byte
	if _, e = file.ReadAt(last[:], info.Size()-1); e != nil || last[0] != '\n' {
		return nil, ErrRestore
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 8192)
	if !scanner.Scan() || scanner.Text() != "butler-memory-deletions-v1" {
		return nil, ErrRestore
	}
	var records []ManifestRecord
	seen := map[string]bool{}
	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var r ManifestRecord
		if json.Unmarshal(scanner.Bytes(), &r) != nil || r.Sequence != int64(len(records)+1) || r.ID == "" || r.SourceID == "" || len(r.OwnerHash) != 64 || (r.Action != "delete" && r.Action != "revoke") || seen[r.ID] {
			return nil, ErrRestore
		}
		seen[r.ID] = true
		records = append(records, r)
	}
	if scanner.Err() != nil {
		return nil, ErrRestore
	}
	return records, nil
}
func (f *FileManifest) Append(ctx context.Context, r ManifestRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	records, e := f.load(ctx)
	if e != nil {
		return e
	}
	for _, existing := range records {
		if existing.ID == r.ID {
			if existing.SourceID != r.SourceID || existing.OwnerHash != r.OwnerHash || existing.Action != r.Action {
				return ErrStale
			}
			return nil
		}
	}
	if r.Sequence != int64(len(records)+1) {
		return ErrRestore
	}
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	b = append(b, '\n')
	file, e := os.OpenFile(f.path, os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return e
	}
	defer file.Close()
	n, e := file.Write(b)
	if e != nil {
		return e
	}
	if n != len(b) {
		return io.ErrShortWrite
	}
	return file.Sync()
}
