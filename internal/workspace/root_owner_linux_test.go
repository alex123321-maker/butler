//go:build linux

package workspace

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkspaceRootOwnerRejectsSharedOrReplacedRoot(t *testing.T) {
	root := privateOwnerRoot(t)
	owner, err := LockRoot(root)
	if err != nil {
		t.Fatal("private root ownership unavailable")
	}
	t.Cleanup(func() { _ = owner.Close() })
	if other, err := LockRoot(root); err != ErrDenied {
		if other != nil {
			_ = other.Close()
		}
		t.Fatal("same root admitted concurrent owner")
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatal("root ownership introduced shared lock or temporary files")
	}
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	if owner.Check() != ErrDenied {
		t.Fatal("shared permissions retained root readiness")
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	old := root + "-old"
	if err := os.Rename(root, old); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(old) })
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if owner.Check() != ErrDenied {
		t.Fatal("replacement root borrowed previous inode ownership")
	}
	if other, err := LockRoot(old); err != ErrDenied {
		if other != nil {
			_ = other.Close()
		}
		t.Fatal("renamed original inode lost process ownership")
	}
	if err := owner.Close(); err != nil {
		t.Fatal("root close failed")
	}
	if owner.Check() != ErrDenied || owner.Close() != nil {
		t.Fatal("closed root retained authority or close was not idempotent")
	}
	other, err := LockRoot(old)
	if err != nil {
		t.Fatal("released original root remained locked")
	}
	_ = other.Close()
}

func TestWorkspaceRootOwnerRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "private")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "alias")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if owner, err := LockRoot(link); err != ErrDenied {
		if owner != nil {
			_ = owner.Close()
		}
		t.Fatal("symlink admitted root ownership")
	}
}

// Child processes run this same test entry. Only fixed content-free markers
// cross the pipe; there is no PID file, shared profile or operator credential.
func TestWorkspaceRootOwnerProcesses(t *testing.T) {
	if mode := os.Getenv("BUTLER_TEST_ROOT_OWNER_PROCESS"); mode != "" {
		owner, err := LockRoot(os.Getenv("BUTLER_TEST_ROOT_OWNER_PATH"))
		if mode == "deny" {
			if err != ErrDenied {
				if owner != nil {
					_ = owner.Close()
				}
				t.Fatal("process borrowed active root")
			}
			fmt.Fprintln(os.Stdout, "denied")
			return
		}
		if err != nil {
			t.Fatal("process could not own private root")
		}
		defer owner.Close()
		fmt.Fprintln(os.Stdout, "held")
		var b [1]byte
		_, _ = os.Stdin.Read(b[:])
		return
	}
	ctx, stop := context.WithTimeout(context.Background(), 25*time.Second)
	defer stop()
	type child struct {
		cmd   *exec.Cmd
		input io.WriteCloser
	}
	start := func(root, mode string) child {
		t.Helper()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkspaceRootOwnerProcesses$", "-test.timeout=20s")
		cmd.Env = append(os.Environ(), "BUTLER_TEST_ROOT_OWNER_PROCESS="+mode, "BUTLER_TEST_ROOT_OWNER_PATH="+root)
		input, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal("process stdin unavailable")
		}
		output, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal("process stdout unavailable")
		}
		if err := cmd.Start(); err != nil {
			t.Fatal("root owner process could not start")
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = input.Close() })
		marker := make(chan string, 1)
		go func() { s, _ := bufio.NewReader(output).ReadString('\n'); marker <- s }()
		expect := "held\n"
		if mode == "deny" {
			expect = "denied\n"
		}
		select {
		case got := <-marker:
			if got != expect {
				t.Fatal("root owner process did not confirm expected state")
			}
		case <-ctx.Done():
			t.Fatal("root owner process did not reach bounded start")
		}
		return child{cmd, input}
	}
	root := privateOwnerRoot(t)
	first := start(root, "hold")
	denied := start(root, "deny")
	if err := denied.cmd.Wait(); err != nil {
		t.Fatal("second process did not reject active root")
	}
	independent := start(privateOwnerRoot(t), "hold")
	if err := first.cmd.Process.Kill(); err != nil {
		t.Fatal("could not kill owning process")
	}
	if err := first.cmd.Wait(); err == nil {
		t.Fatal("owning process did not actually die")
	}
	replacement := start(root, "hold")
	for _, p := range []child{replacement, independent} {
		_ = p.input.Close()
		if err := p.cmd.Wait(); err != nil {
			t.Fatal("independent/replacement owner did not exit cleanly")
		}
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatal("crash/restart left lock/temp files")
	}
}

func privateOwnerRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal("could not provision private fixture root")
	}
	return root
}
