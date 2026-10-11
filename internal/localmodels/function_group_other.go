//go:build !linux

package localmodels

import "os/exec"

// Runtime is Linux-only; other builds cannot start an uncontained runner.
func setOwnedGroup(cmd *exec.Cmd) {}
func killOwnedGroup(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
