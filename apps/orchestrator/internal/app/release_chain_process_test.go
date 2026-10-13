package app

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// Restarts the actual owned Broker process at the same private endpoint with
// the exact same tested binary/configuration. It never resets durable state.
func startReleaseChainBroker(t *testing.T, binary, address, httpAddress string, environment ...string) func() error {
	t.Helper()
	var mu sync.Mutex
	var process *exec.Cmd
	var done chan struct{}
	start := func() error {
		process = exec.Command(binary)
		process.Env = append([]string{"PATH=" + os.Getenv("PATH"), "BUTLER_ENVIRONMENT=test", "BUTLER_GRPC_ADDR=" + address, "BUTLER_HTTP_ADDR=" + httpAddress}, environment...)
		// Private process diagnostics are deliberately not a channel artifact.
		if e := process.Start(); e != nil {
			return errors.New("isolated Broker process start failed")
		}
		done = make(chan struct{})
		cmd := process
		finished := done
		go func() { _ = cmd.Wait(); close(finished) }()
		until := time.Now().Add(10 * time.Second)
		for time.Now().Before(until) {
			select {
			case <-done:
				return errors.New("isolated Broker process exited before readiness")
			default:
			}
			conn, e := net.DialTimeout("tcp", address, 100*time.Millisecond)
			if e == nil {
				_ = conn.Close()
				return nil
			}
			time.Sleep(20 * time.Millisecond)
		}
		return errors.New("isolated Broker readiness deadline")
	}
	if e := start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		if process == nil || process.Process == nil {
			return
		}
		_ = process.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = process.Process.Kill()
			<-done
		}
	})
	return func() error {
		mu.Lock()
		defer mu.Unlock()
		if process == nil || process.Process == nil {
			return errors.New("isolated Broker process unavailable")
		}
		if e := process.Process.Kill(); e != nil {
			return errors.New("isolated Broker process kill failed")
		}
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			return errors.New("isolated Broker process exit deadline")
		}
		return start()
	}
}
