//go:build linux

package main

import "syscall"

func executeCandidate() error {
	// Replace PID1: no detached child or supervisor success may masquerade as
	// candidate completion. No host environment or author command is forwarded.
	return syscall.Exec("/candidate", []string{"/candidate"}, []string{"GOMAXPROCS=2"})
}
