//go:build linux

package acp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func fixtureProcess(test *testing.T, mode string, args ...string) ProcessSpec {
	test.Helper()
	path, err := os.Executable()
	if err != nil {
		test.Fatal(err)
	}
	executable, err := os.Open(path)
	if err != nil {
		test.Fatal(err)
	}
	defer executable.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, executable); err != nil {
		test.Fatal(err)
	}
	return ProcessSpec{Executable: path, SHA256: hex.EncodeToString(digest.Sum(nil)), Args: append([]string{"-test.run=^TestACPProcessFixture$", "--", "--acp-fixture", mode}, args...)}
}

func TestProcessStdioAndCleanEnvironment(test *testing.T) {
	test.Setenv("ACP_PARENT_SECRET", "must-not-inherit")
	test.Setenv("GEMINI_API_KEY", "must-not-inherit")
	test.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "must-not-inherit")
	probe := newFixtureProbe(test, fixtureLimits())
	cwdFile := filepath.Join(test.TempDir(), "cwd")
	result, err := probe.RunProcess(context.Background(), fixtureProcess(test, "happy", cwdFile), fixtureRequest())
	if err != nil || result.Text != "synthetic stdio result" || result.UsageState != "unknown" {
		test.Fatalf("result=%+v err=%v", result, err)
	}
	cwd, readErr := os.ReadFile(cwdFile)
	if readErr != nil {
		test.Fatal(readErr)
	}
	if _, statErr := os.Stat(filepath.Dir(string(cwd))); !errors.Is(statErr, os.ErrNotExist) {
		test.Fatal("temporary profile was not removed")
	}
	_, err = probe.RunProcess(context.Background(), ProcessSpec{}, fixtureRequest())
	if !errors.Is(err, ErrConsumed) {
		test.Fatal(err)
	}
}

func TestProcessRejectsBeforeLaunch(test *testing.T) {
	spec := fixtureProcess(test, "happy")
	test.Run("binding", func(test *testing.T) {
		request := fixtureRequest()
		request.Binding.RunID = "stale"
		_, err := newFixtureProbe(test, fixtureLimits()).RunProcess(context.Background(), ProcessSpec{}, request)
		if !errors.Is(err, ErrBinding) {
			test.Fatal(err)
		}
	})
	test.Run("resume", func(test *testing.T) {
		request := fixtureRequest()
		request.ResumeSessionID = "stale"
		_, err := newFixtureProbe(test, fixtureLimits()).RunProcess(context.Background(), ProcessSpec{}, request)
		if !errors.Is(err, ErrResume) {
			test.Fatal(err)
		}
	})
	mutations := map[string]func(*ProcessSpec){
		"wrong-hash":     func(spec *ProcessSpec) { spec.SHA256 = strings.Repeat("00", 32) },
		"invalid-hash":   func(spec *ProcessSpec) { spec.SHA256 = "invalid" },
		"relative-path":  func(spec *ProcessSpec) { spec.Executable = "fixture" },
		"missing-file":   func(spec *ProcessSpec) { spec.Executable = "/missing-acp-fixture" },
		"argument-count": func(spec *ProcessSpec) { spec.Args = make([]string, 33) },
		"argument-bytes": func(spec *ProcessSpec) { spec.Args = []string{strings.Repeat("a", 8193)} },
		"argument-nul":   func(spec *ProcessSpec) { spec.Args = []string{"bad\x00argument"} },
		"non-executable": func(spec *ProcessSpec) { spec.Executable = test.TempDir() },
	}
	fifo := filepath.Join(test.TempDir(), "fifo")
	if err := syscall.Mkfifo(fifo, 0700); err != nil {
		test.Fatal(err)
	}
	mutations["fifo"] = func(spec *ProcessSpec) { spec.Executable = fifo }
	symlink := filepath.Join(test.TempDir(), "symlink")
	if err := os.Symlink(spec.Executable, symlink); err != nil {
		test.Fatal(err)
	}
	mutations["symlink"] = func(spec *ProcessSpec) { spec.Executable = symlink }
	for name, mutate := range mutations {
		test.Run(name, func(test *testing.T) {
			invalid := spec
			mutate(&invalid)
			result, err := newFixtureProbe(test, fixtureLimits()).RunProcess(context.Background(), invalid, fixtureRequest())
			if !errors.Is(err, ErrProcess) || result != (Result{}) {
				test.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestProcessFailureBoundsAndRedaction(test *testing.T) {
	for _, mode := range []string{"stall", "stderr", "stdout", "crash", "partial"} {
		test.Run(mode, func(test *testing.T) {
			limits := fixtureLimits()
			limits.WallTime = 350 * time.Millisecond
			started := time.Now()
			result, err := newFixtureProbe(test, limits).RunProcess(context.Background(), fixtureProcess(test, mode), fixtureRequest())
			if err == nil || result != (Result{}) || strings.Contains(err.Error(), "private-stderr-canary") {
				test.Fatalf("result=%+v err=%v", result, err)
			}
			if mode == "stderr" || mode == "stdout" || mode == "stall" {
				if !errors.Is(err, ErrBudget) {
					test.Fatal(err)
				}
			}
			if time.Since(started) > 2*time.Second {
				test.Fatal("process cleanup exceeded bound")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := newFixtureProbe(test, fixtureLimits()).RunProcess(ctx, fixtureProcess(test, "happy"), fixtureRequest())
	if !errors.Is(err, context.Canceled) || result != (Result{}) {
		test.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestProcessStopsOwnedGroup(test *testing.T) {
	for _, mode := range []string{"descendant-stall", "descendant-exit", "descendant-cancel", "root-moves-group"} {
		test.Run(mode, func(test *testing.T) {
			pidFile := filepath.Join(test.TempDir(), "pids")
			limits := fixtureLimits()
			limits.WallTime = 800 * time.Millisecond
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "descendant-cancel" {
				limits.WallTime = 2 * time.Second
				go func() {
					ticker := time.NewTicker(5 * time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-ctx.Done():
							return
						case <-ticker.C:
							if _, err := os.Stat(pidFile); err == nil {
								cancel()
								return
							}
						}
					}
				}()
			}
			result, err := newFixtureProbe(test, limits).RunProcess(ctx, fixtureProcess(test, mode, pidFile), fixtureRequest())
			if mode == "descendant-exit" && (err != nil || result.Text != "synthetic stdio result") {
				test.Fatalf("result=%+v err=%v", result, err)
			}
			if mode == "descendant-stall" && (!errors.Is(err, ErrBudget) || result != (Result{})) {
				test.Fatalf("result=%+v err=%v", result, err)
			}
			if mode == "root-moves-group" && (!errors.Is(err, ErrBudget) || result != (Result{})) {
				test.Fatalf("result=%+v err=%v", result, err)
			}
			if mode == "descendant-cancel" && (!errors.Is(err, context.Canceled) || result != (Result{})) {
				test.Fatalf("result=%+v err=%v", result, err)
			}
			raw, readErr := os.ReadFile(pidFile)
			if readErr != nil {
				test.Fatal(readErr)
			}
			for index, value := range strings.Fields(string(raw)) {
				pid, parseErr := strconv.Atoi(value)
				if parseErr != nil || pid <= 1 {
					test.Fatal("invalid fixture pid")
				}
				deadline := time.Now().Add(time.Second)
				for {
					status, readErr := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
					gone := errors.Is(readErr, os.ErrNotExist)
					zombie := readErr == nil && strings.HasPrefix(string(status)[strings.LastIndex(string(status), ")")+1:], " Z ")
					if gone || (index > 0 && zombie) {
						break
					}
					if time.Now().After(deadline) {
						test.Fatal("owned fixture process remains running")
					}
					time.Sleep(5 * time.Millisecond)
				}
			}
		})
	}
}

func TestACPProcessFixture(test *testing.T) {
	marker := -1
	for index, argument := range os.Args {
		if argument == "--acp-fixture" {
			marker = index
		}
	}
	if marker < 0 {
		return
	}
	mode := os.Args[marker+1]
	if mode == "root-moves-group" {
		group, err := syscall.Getpgid(os.Getppid())
		if err != nil || syscall.Setpgid(0, group) != nil {
			os.Exit(18)
		}
		if os.WriteFile(os.Args[marker+2], []byte(strconv.Itoa(os.Getpid())), 0600) != nil {
			os.Exit(19)
		}
		for {
			time.Sleep(time.Second)
		}
	}
	if mode == "child" {
		for {
			time.Sleep(time.Second)
		}
	}
	if mode == "crash" {
		_, _ = fmt.Fprint(os.Stderr, "private-stderr-canary")
		os.Exit(7)
	}
	if mode == "stderr" {
		_, _ = fmt.Fprint(os.Stderr, strings.Repeat("private-stderr-canary", 5000))
	}
	if mode == "stdout" {
		_, _ = fmt.Fprint(os.Stdout, strings.Repeat("x", 300*1024))
	}
	if mode == "stall" || mode == "stderr" || mode == "stdout" {
		for {
			time.Sleep(time.Second)
		}
	}
	cwd, err := os.Getwd()
	expectedHome := filepath.Join(filepath.Dir(cwd), "home")
	if mode == "profile" {
		expectedHome = os.Args[marker+2]
	}
	if err != nil || filepath.Base(cwd) != "work" || os.Getenv("HOME") != expectedHome {
		os.Exit(8)
	}
	for _, name := range []string{"ACP_PARENT_SECRET", "GEMINI_API_KEY", "GOOGLE_APPLICATION_CREDENTIALS"} {
		if os.Getenv(name) != "" {
			os.Exit(9)
		}
	}
	if mode == "happy" && len(os.Args) > marker+2 {
		if os.WriteFile(os.Args[marker+2], []byte(cwd), 0600) != nil {
			os.Exit(17)
		}
	}
	if strings.HasPrefix(mode, "descendant-") {
		child := exec.Command("/proc/self/exe", "-test.run=^TestACPProcessFixture$", "--", "--acp-fixture", "child")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if child.Start() != nil {
			os.Exit(10)
		}
		if os.WriteFile(os.Args[marker+2], []byte(fmt.Sprintf("%d %d", os.Getpid(), child.Process.Pid)), 0600) != nil {
			os.Exit(11)
		}
	}
	agent := &peer{decoder: json.NewDecoder(os.Stdin), encoder: json.NewEncoder(os.Stdout), cwd: cwd}
	if agent.handshake("fixture-gemini", "fixture-gemini") != nil {
		os.Exit(12)
	}
	request, err := agent.read("session/prompt")
	if err != nil {
		os.Exit(13)
	}
	if mode == "descendant-stall" || mode == "descendant-cancel" {
		for {
			time.Sleep(time.Second)
		}
	}
	if agent.update("agent-session", "agent_message_chunk", "synthetic stdio result") != nil {
		os.Exit(14)
	}
	if mode == "partial" {
		os.Exit(15)
	}
	if agent.reply(request, map[string]any{"stopReason": "end_turn"}) != nil {
		os.Exit(16)
	}
	os.Exit(0)
}
