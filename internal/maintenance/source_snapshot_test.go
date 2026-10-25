package maintenance

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func publicGitSource(t *testing.T) (SourceSnapshotter, func(string, ...string) string) {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal("local Git is required for source snapshot acceptance")
	}
	git, err = filepath.Abs(git)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	workspace := filepath.Join(root, "checker-private")
	for _, path := range []string{repository, workspace} {
		if err = os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	source := SourceSnapshotter{GitBinary: git, RepositoryRoot: repository, WorkspaceRoot: workspace}
	run := func(input string, args ...string) string {
		cmd := source.command(t.Context(), args...)
		cmd.Stdin = strings.NewReader(input)
		output, err := cmd.Output()
		if err != nil {
			t.Fatal("synthetic local Git setup failed")
		}
		return strings.TrimSpace(string(output))
	}
	run("", "init", "--initial-branch=main")
	run("", "config", "user.name", "Synthetic acceptance")
	run("", "config", "user.email", "acceptance@example.invalid")
	run("", "config", "commit.gpgsign", "false")
	return source, run
}

func TestSourceSnapshotUsesWholeCommitInsteadOfDirtyCheckoutOrExportFilters(t *testing.T) {
	source, git := publicGitSource(t)
	if err := os.MkdirAll(filepath.Join(source.RepositoryRoot, "modules/public"), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"modules/public/main.go": []byte("package public\n// committed версия\n"), "README.md": []byte("must remain in measured inventory"), ".gitattributes": []byte("README.md export-ignore\n")}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(source.RepositoryRoot, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("", "add", ".")
	git("", "update-index", "--chmod=+x", "modules/public/main.go")
	git("", "commit", "-m", "fixed synthetic source")
	commit := git("", "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(source.RepositoryRoot, "modules/public/main.go"), []byte("dirty version must not be built"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := source.Snapshot(t.Context(), commit)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := snapshot.Close(); err != nil {
			t.Error("owned snapshot cleanup failed", err)
		}
	})
	if len(snapshot.Files) != len(files) || snapshot.Commit != commit || !validDigest(snapshot.InventorySHA256) {
		t.Fatal("complete immutable commit inventory unavailable")
	}
	for _, file := range snapshot.Files {
		expected, exists := files[file.Path]
		if !exists {
			t.Fatal("untracked/synthetic extra entered source snapshot")
		}
		actual, err := os.ReadFile(filepath.Join(snapshot.Directory(), file.Path))
		if err != nil || !bytes.Equal(actual, expected) || Digest(actual) != file.SHA256 || int64(len(actual)) != file.Bytes {
			t.Fatal("committed blob binding lost", err)
		}
		if file.Path == "modules/public/main.go" && file.Mode != "100755" {
			t.Fatal("actual Git executable mode discarded")
		}
	}
	if _, err = source.Snapshot(t.Context(), "main"); err == nil {
		t.Fatal("moving branch accepted as immutable source identity")
	}
	if _, err = source.Snapshot(t.Context(), git("", "rev-parse", "HEAD^{tree}")); err == nil {
		t.Fatal("uncommitted tree object masqueraded as a commit")
	}
	if err = snapshot.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(source.RepositoryRoot); err != nil {
		t.Fatal("snapshot cleanup touched the author repository")
	}
}

func TestSourceSnapshotRefusesSymlinkBeforePrivateCopy(t *testing.T) {
	source, git := publicGitSource(t)
	blob := git("../../outside-private\n", "hash-object", "-w", "--stdin")
	git("", "update-index", "--add", "--cacheinfo", "120000,"+blob+",link")
	git("", "commit", "-m", "unsupported synthetic link")
	commit := git("", "rev-parse", "HEAD")
	if _, err := source.Snapshot(t.Context(), commit); err == nil {
		t.Fatal("source link accepted for independent build")
	}
	entries, err := os.ReadDir(source.WorkspaceRoot)
	if err != nil || len(entries) != 0 {
		t.Fatal("invalid source left a qualified private snapshot", err)
	}
}

func TestSourceSnapshotCleanupCannotRemoveAnUnownedDirectory(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "unrelated")
	workspace := filepath.Join(root, "private")
	for _, path := range []string{outside, workspace} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := SourceSnapshot{directory: outside, workspace: workspace}
	if err := snapshot.Close(); err == nil {
		t.Fatal("unowned directory accepted for recursive removal")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("unrelated directory changed", err)
	}
}
