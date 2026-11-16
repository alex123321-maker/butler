//go:build linux

package acp

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRuntimeNamespaceReapsDetachedChildren(t *testing.T) {
	if os.Getpid() != 1 {
		if CheckRuntimeNamespace() == nil || KillRuntimeChildren() == nil {
			t.Fatal("namespace-wide cleanup enabled outside dedicated PID1")
		}
		return
	}
	if CheckRuntimeNamespace() != nil {
		t.Fatal("Docker fixture does not satisfy isolation requirements")
	}
	command := exec.Command("/proc/self/exe", "-test.run=^TestNamespaceSpawnHelper$")
	command.Env = []string{"BUTLER_ACP_NAMESPACE_HELPER=spawn"}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if command.Start() != nil {
		t.Fatal("helper failed")
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || pid <= 1 {
		t.Fatal("invalid fixture PID")
	}
	if command.Wait() != nil {
		t.Fatal("helper exit failed")
	}
	if err := KillRuntimeChildren(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
		t.Fatal("detached descendant survived or was not reaped")
	}
}

func TestNamespaceSpawnHelper(t *testing.T) {
	if os.Getenv("BUTLER_ACP_NAMESPACE_HELPER") != "spawn" {
		return
	}
	child := exec.Command("/proc/self/exe", "-test.run=^TestNamespaceLingeringHelper$")
	child.Env = []string{"BUTLER_ACP_NAMESPACE_HELPER=linger"}
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if child.Start() != nil {
		os.Exit(2)
	}
	fmt.Println(child.Process.Pid)
	os.Exit(0)
}

func TestNamespaceLingeringHelper(t *testing.T) {
	if os.Getenv("BUTLER_ACP_NAMESPACE_HELPER") != "linger" {
		return
	}
	for {
		time.Sleep(time.Second)
	}
}
