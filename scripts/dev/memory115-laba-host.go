//go:build ignore

// Fixed server-side resource proof. No credential or model access is admitted.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const laba115HostRoot = "/srv/gitea/butler-acceptance/issue115-native-20261028-v1"
const laba115Image = "sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61"
const laba115ProbeName = "issue115-native-host-probe-20261028-v1"

func laba115Docker(ctx context.Context, args ...string) ([]byte, error) {
	fixed := []string{"--host=unix:///var/run/docker.sock", "--config=" + filepath.Join(laba115HostRoot, "probe-v1", "docker-config")}
	cmd := exec.CommandContext(ctx, "/usr/bin/docker", append(fixed, args...)...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=/home/laba"}
	raw, err := cmd.Output()
	if err != nil {
		return nil, errors.New("fixed Docker operation failed")
	}
	return raw, nil
}

type laba115Container struct {
	ID     string `json:"Id"`
	Image  string
	Mounts []struct {
		Type        string
		Source      string
		Destination string
		RW          bool
	}
	NetworkSettings struct {
		Ports map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string
		}
	}
	State struct {
		Running bool
		Pid     int
	}
	Config     struct{ User string }
	HostConfig struct {
		NanoCpus       int64
		Memory         int64
		MemorySwap     int64
		PidsLimit      int64
		ReadonlyRootfs bool
		Privileged     bool
		NetworkMode    string
		PidMode        string
		CapDrop        []string
		CapAdd         []string
		SecurityOpt    []string
	}
}

func laba115Inspect(ctx context.Context, id string) (laba115Container, error) {
	var out laba115Container
	raw, err := laba115Docker(ctx, "inspect", id)
	var rows []laba115Container
	if err != nil || len(raw) > 128*1024 || json.Unmarshal(raw, &rows) != nil || len(rows) != 1 || rows[0].ID != id {
		return out, errors.New("exact private container identity unavailable")
	}
	return rows[0], nil
}

func laba115ResourceConfig(c laba115Container) bool {
	h := c.HostConfig
	return c.Image == laba115Image && c.Config.User == "1000:1000" && h.NanoCpus == 1000000000 && h.Memory == 2147483648 && h.MemorySwap == 2147483648 && h.PidsLimit == 256 && h.ReadonlyRootfs && !h.Privileged && h.NetworkMode == "none" && h.PidMode == "" && len(h.CapAdd) == 0 && len(h.CapDrop) == 1 && strings.EqualFold(h.CapDrop[0], "ALL") && len(h.SecurityOpt) == 1 && h.SecurityOpt[0] == "no-new-privileges"
}

func laba115ProcIdentity(pid int) (string, error) {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	end := strings.LastIndex(string(raw), ")")
	if err != nil || end < 0 {
		return "", errors.New("process identity unavailable")
	}
	fields := strings.Fields(string(raw)[end+1:])
	if len(fields) < 20 {
		return "", errors.New("process identity incomplete")
	}
	return fields[19], nil
}

func laba115Probe() error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dir := filepath.Join(laba115HostRoot, "probe-v1")
	if err := os.MkdirAll(laba115HostRoot, 0700); err != nil {
		return errors.New("private server root unavailable")
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return errors.New("new one-use probe identity required")
	}
	if err := os.Mkdir(filepath.Join(dir, "docker-config"), 0700); err != nil {
		return errors.New("private empty Docker configuration required")
	}
	raw, err := laba115Docker(ctx, "create", "--name", laba115ProbeName, "--network", "none", "--cpus", "1", "--memory", "2g", "--memory-swap", "2g", "--pids-limit", "256", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--log-driver", "none", "--user", "1000:1000", laba115Image, "/bin/sh", "-c", "sleep 30 & wait")
	id := strings.TrimSpace(string(raw))
	if err != nil || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(id) {
		return errors.New("new fixed probe container unavailable")
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_, _ = laba115Docker(cleanup, "rm", "-f", id)
	}()
	if _, err = laba115Docker(ctx, "start", id); err != nil {
		return err
	}
	c, err := laba115Inspect(ctx, id)
	if err != nil || !c.State.Running || c.State.Pid < 1 || !laba115ResourceConfig(c) {
		return errors.New("actual resource configuration differs")
	}
	start, err := laba115ProcIdentity(c.State.Pid)
	if err != nil {
		return err
	}
	raw, err = os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", c.State.Pid))
	cg := strings.TrimSpace(strings.TrimPrefix(string(raw), "0::"))
	if err != nil || !strings.HasPrefix(string(raw), "0::/") || !strings.Contains(cg, id) || strings.Contains(cg, "..") || strings.Contains(cg, "\n") {
		return errors.New("own kernel cgroup identity unavailable")
	}
	root, err := os.OpenRoot("/sys/fs/cgroup")
	if err != nil {
		return err
	}
	defer root.Close()
	rel := strings.TrimPrefix(cg, "/")
	for file, want := range map[string]string{"cpu.max": "100000 100000", "memory.max": "2147483648", "memory.swap.max": "0", "pids.max": "256"} {
		actual, e := root.ReadFile(filepath.Join(rel, file))
		if e != nil || strings.TrimSpace(string(actual)) != want {
			return errors.New("actual kernel resource bound differs")
		}
	}
	raw, err = os.ReadFile(fmt.Sprintf("/proc/%d/status", c.State.Pid))
	if err != nil || !regexp.MustCompile(`(?m)^Uid:\s+1000\s+1000\s+1000\s+1000$`).Match(raw) || !regexp.MustCompile(`(?m)^CapEff:\s+0000000000000000$`).Match(raw) || !regexp.MustCompile(`(?m)^NoNewPrivs:\s+1$`).Match(raw) || !regexp.MustCompile(`(?m)^Seccomp:\s+2$`).Match(raw) {
		return errors.New("actual process security boundary differs")
	}
	top, err := laba115Docker(ctx, "top", id, "-eo", "pid")
	if err != nil || len(strings.Fields(string(top))) < 3 {
		return errors.New("background child process was not observed")
	}
	if _, err = laba115Docker(ctx, "stop", "--timeout", "3", id); err != nil {
		return err
	}
	stopped, err := laba115Inspect(ctx, id)
	if err != nil || stopped.State.Running || stopped.State.Pid != 0 {
		return errors.New("native namespace did not stop")
	}
	if after, e := laba115ProcIdentity(c.State.Pid); e == nil && after == start {
		return errors.New("original init process remains")
	}
	populated, e := root.ReadFile(filepath.Join(rel, "cgroup.events"))
	if e != nil && !os.IsNotExist(e) || e == nil && !strings.Contains("\n"+string(populated), "\npopulated 0\n") {
		return errors.New("whole kernel cgroup is not empty")
	}
	if _, err = laba115Docker(ctx, "rm", id); err != nil {
		return err
	}
	receipt := map[string]any{"version": "issue115-laba-host-probe-v1", "cpu_max": "100000 100000", "memory_max": 2147483648, "swap_max": 0, "pids_max": 256, "actual_kernel_values": true, "background_child_observed": true, "pid_namespace_exited": true, "whole_cgroup_empty_or_removed": true, "native_calls": 0, "real_auth": 0, "native_admission": false}
	encoded, _ := json.MarshalIndent(receipt, "", "  ")
	f, err := os.OpenFile(filepath.Join(dir, "receipt.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, w := f.Write(encoded)
	s := f.Sync()
	closeErr := f.Close()
	if w != nil || s != nil || closeErr != nil {
		return errors.New("probe receipt persistence failed")
	}
	return nil
}

func laba115HostCLI() {
	if len(os.Args) != 2 || os.Args[1] != "probe" {
		fmt.Fprintln(os.Stderr, "only the fixed zero-native probe is implemented; model entry denied")
		os.Exit(2)
	}
	if err := laba115Probe(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("PASS fixed laba kernel resource and child-exit proof; auth=0 native=0")
}
