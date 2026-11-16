//go:build linux

package acp

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"time"
)

func CheckRuntimeNamespace() error {
	if os.Getpid() != 1 || os.Geteuid() == 0 {
		return ErrProcess
	}
	if _, err := os.Stat("/.dockerenv"); err != nil {
		return ErrProcess
	}
	status, err := os.ReadFile("/proc/self/status")
	if err != nil || !strings.Contains(string(status), "CapEff:\t0000000000000000") || !strings.Contains(string(status), "NoNewPrivs:\t1") {
		return ErrProcess
	}
	return nil
}

func KillRuntimeChildren() error {
	if CheckRuntimeNamespace() != nil {
		return ErrProcess
	}
	if err := syscall.Kill(-1, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return ErrProcess
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		var status syscall.WaitStatus
		child, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
		if errors.Is(err, syscall.ECHILD) {
			return nil
		}
		if err != nil && !errors.Is(err, syscall.EINTR) {
			return ErrProcess
		}
		if child == 0 {
			time.Sleep(time.Millisecond)
		}
	}
	return ErrProcess
}
