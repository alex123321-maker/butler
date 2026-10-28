//go:build !linux

package main

func executeCandidate() error { return errGuard }
