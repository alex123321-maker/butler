package maintenance

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// SourceSnapshotter is fixed private checker configuration, never a model/tool
// argument. It reads only existing Git objects, without changing the checkout,
// applying export-ignore filters, running author hooks or executing source code.
// Building still requires the independent bounded sandbox; this is not a build.
type SourceSnapshotter struct {
	GitBinary      string
	RepositoryRoot string
	WorkspaceRoot  string
}

type SourceSnapshot struct {
	Commit          string       `json:"commit"`
	Files           []SourceFile `json:"files"`
	InventorySHA256 string       `json:"inventory_sha256"`
	directory       string
	workspace       string
}

// Directory is for the trusted checker sandbox mount, not a public capability.
func (snapshot *SourceSnapshot) Directory() string { return snapshot.directory }

func outside(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Close removes only the unique private snapshot created by this instance.
// Resolve and verify the final absolute target before any recursive removal.
func (snapshot *SourceSnapshot) Close() error {
	if snapshot == nil {
		return nil
	}
	if snapshot.workspace == "" || snapshot.directory == "" || filepath.Dir(snapshot.directory) != snapshot.workspace || !strings.HasPrefix(filepath.Base(snapshot.directory), "maintenance-source-") {
		return ErrInventory
	}
	resolved, err := filepath.EvalSymlinks(snapshot.directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || filepath.Dir(resolved) != snapshot.workspace || outside(snapshot.workspace, resolved) {
		return ErrInventory
	}
	if runtime.GOOS == "windows" {
		if err = filepath.WalkDir(resolved, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() {
				return ErrInventory
			}
			return os.Chmod(path, 0600)
		}); err != nil {
			return err
		}
	}
	return os.RemoveAll(resolved)
}

func gitObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func (source SourceSnapshotter) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, source.GitBinary, append([]string{"--no-pager", "-C", source.RepositoryRoot}, args...)...)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(strings.ToUpper(key), "GIT_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	cmd.Stderr = io.Discard
	return cmd
}

func (source SourceSnapshotter) capture(ctx context.Context, limit int64, args ...string) ([]byte, error) {
	cmd := source.command(ctx, args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, ErrInventory
	}
	if err = cmd.Start(); err != nil {
		return nil, ErrInventory
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	raw, err := io.ReadAll(io.LimitReader(out, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, ErrInventory
	}
	if err = cmd.Wait(); err != nil {
		return nil, ErrInventory
	}
	return raw, nil
}

func (source SourceSnapshotter) tree(ctx context.Context, commit string) ([]byte, error) {
	typeName, err := source.capture(ctx, 32, "cat-file", "-t", commit)
	if err != nil || string(typeName) != "commit\n" {
		return nil, ErrInventory
	}
	return source.capture(ctx, 64<<20, "ls-tree", "-rz", "--full-tree", commit+"^{tree}")
}

// Snapshot binds a complete committed tree, including author-modified tests and
// configuration. Dirty/untracked checkout bytes do not enter that fixed commit.
// Symlinks/submodules and unsafe or ambiguous paths are denied before building.
func (source SourceSnapshotter) Snapshot(ctx context.Context, commit string) (result *SourceSnapshot, err error) {
	if !gitObjectID(commit) || !filepath.IsAbs(source.GitBinary) || !filepath.IsAbs(source.RepositoryRoot) || !filepath.IsAbs(source.WorkspaceRoot) {
		return nil, ErrInventory
	}
	repository, e := filepath.EvalSymlinks(source.RepositoryRoot)
	if e != nil {
		return nil, ErrInventory
	}
	workspace, e := filepath.EvalSymlinks(source.WorkspaceRoot)
	if e != nil || filepath.Dir(workspace) == workspace || !outside(repository, workspace) {
		return nil, ErrInventory
	}
	source.RepositoryRoot = repository
	raw, e := source.tree(ctx, commit)
	if e != nil || len(raw) == 0 || raw[len(raw)-1] != 0 {
		return nil, ErrInventory
	}
	type blob struct{ mode, id, path string }
	blobs := []blob{}
	paths := map[string]string{}
	for _, entry := range bytes.Split(raw[:len(raw)-1], []byte{0}) {
		header, name, ok := bytes.Cut(entry, []byte{'\t'})
		fields := strings.Fields(string(header))
		if !ok || len(fields) != 3 || (fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" || !gitObjectID(fields[2]) || !validSourcePath(string(name)) || len(blobs) >= 100000 {
			return nil, ErrInventory
		}
		// Retain exact case and reject file/directory case aliases portably.
		parts := strings.Split(string(name), "/")
		for i := range parts {
			prefix := strings.Join(parts[:i+1], "/")
			key := strings.ToLower(prefix)
			if previous, exists := paths[key]; exists && previous != prefix {
				return nil, ErrInventory
			}
			paths[key] = prefix
		}
		blobs = append(blobs, blob{fields[0], fields[2], string(name)})
	}
	directory, e := os.MkdirTemp(workspace, "maintenance-source-")
	if e != nil {
		return nil, ErrInventory
	}
	result = &SourceSnapshot{Commit: commit, directory: directory, workspace: workspace}
	root, e := os.OpenRoot(directory)
	if e != nil {
		_ = result.Close()
		return nil, ErrInventory
	}
	completed := false
	defer func() {
		_ = root.Close()
		if !completed {
			_ = result.Close()
			result = nil
		}
	}()
	cmd := source.command(ctx, "cat-file", "--batch")
	stdin, e := cmd.StdinPipe()
	if e != nil {
		return result, ErrInventory
	}
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		return result, ErrInventory
	}
	if e = cmd.Start(); e != nil {
		return result, ErrInventory
	}
	defer func() { _ = stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	reader := bufio.NewReader(stdout)
	var total int64
	for _, blob := range blobs {
		if ctx.Err() != nil {
			return result, ErrInventory
		}
		if _, e = io.WriteString(stdin, blob.id+"\n"); e != nil {
			return result, ErrInventory
		}
		header, e := reader.ReadString('\n')
		fields := strings.Fields(header)
		if e != nil || len(header) > 128 || len(fields) != 3 || fields[0] != blob.id || fields[1] != "blob" {
			return result, ErrInventory
		}
		size, e := strconv.ParseInt(fields[2], 10, 64)
		if e != nil || size < 0 || size > 32<<20 {
			return result, ErrInventory
		}
		total += size
		if total > 1<<30 {
			return result, ErrInventory
		}
		body := make([]byte, int(size))
		if _, e = io.ReadFull(reader, body); e != nil {
			return result, ErrInventory
		}
		ending, e := reader.ReadByte()
		if e != nil || ending != '\n' {
			return result, ErrInventory
		}
		parent := filepath.Dir(blob.path)
		if parent != "." {
			if e = root.MkdirAll(parent, 0700); e != nil {
				return result, ErrInventory
			}
		}
		mode := os.FileMode(0444)
		if blob.mode == "100755" {
			mode = 0555
		}
		file, e := root.OpenFile(blob.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if e != nil {
			return result, ErrInventory
		}
		_, writeErr := file.Write(body)
		syncErr := file.Sync()
		closeErr := file.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil {
			return result, ErrInventory
		}
		result.Files = append(result.Files, SourceFile{Path: blob.path, SHA256: Digest(body), Bytes: size, Mode: blob.mode})
	}
	if e = stdin.Close(); e != nil {
		return result, ErrInventory
	}
	if e = cmd.Wait(); e != nil {
		return result, ErrInventory
	}
	_, inventory, e := sourceInventory(result.Files)
	if e != nil {
		return result, ErrInventory
	}
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	result.InventorySHA256 = inventory
	completed = true
	return result, nil
}
