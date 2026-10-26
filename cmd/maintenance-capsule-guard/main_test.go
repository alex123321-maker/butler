package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLaunchPermitStrictAndFinite(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	value := launchPermit{Version: "butler.maintenance.capsule-launch.v2", AdmissionKind: "independent_check", AdmissionSHA256: strings.Repeat("1", 64), CandidateSHA256: strings.Repeat("2", 64), ExpiresAt: now.Add(time.Second)}
	raw, _ := json.Marshal(value)
	if _, err := permit(raw, now); err != nil {
		t.Fatal("finite canonical permit rejected")
	}
	for _, invalid := range [][]byte{
		append(append([]byte(nil), raw...), '\n'),
		[]byte(strings.Replace(string(raw), `"version":`, `"unknown":0,"version":`, 1)),
		[]byte(strings.Replace(string(raw), `"version":`, `"version":"duplicate","version":`, 1)),
		[]byte(strings.Replace(string(raw), value.AdmissionSHA256, strings.Repeat("A", 64), 1)),
		[]byte(strings.Replace(string(raw), "independent_check", "arbitrary", 1)),
		[]byte(strings.Replace(string(raw), ".v2", ".v1", 1)),
		make([]byte, 513),
	} {
		if _, err := permit(invalid, now); err == nil {
			t.Fatal("ambiguous or oversized permit accepted")
		}
	}
	value.AdmissionKind = "maintenance_effect"
	maintenance, _ := json.Marshal(value)
	if _, err := permit(maintenance, now); err != nil {
		t.Fatal("typed effect marker rejected; guard parsing alone grants no authority")
	}
	for _, expiry := range []time.Time{now, now.Add(-time.Second), now.Add(30*time.Second + time.Nanosecond)} {
		value.ExpiresAt = expiry
		raw, _ := json.Marshal(value)
		if _, err := permit(raw, now); err == nil {
			t.Fatal("expired or unbounded permit accepted")
		}
	}
}

func TestLaunchWaitRequiresExactCandidateAndHonorsCancellation(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "control"), 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := waitForLaunch(ctx, root); err == nil || time.Since(started) > time.Second {
		t.Fatal("missing barrier ignored cancellation")
	}
	// This synthetic header exercises only the pre-exec hash barrier, never an
	// executable or a claim of valid built-ELF/physical process acceptance.
	candidate := make([]byte, 64)
	copy(candidate, []byte{0x7f, 'E', 'L', 'F'})
	if err := os.WriteFile(filepath.Join(directory, "candidate"), candidate, 0500); err != nil {
		t.Fatal(err)
	}
	writePermit := func(hash string) {
		t.Helper()
		value := launchPermit{Version: "butler.maintenance.capsule-launch.v2", AdmissionKind: "independent_check", AdmissionSHA256: strings.Repeat("3", 64), CandidateSHA256: hash, ExpiresAt: time.Now().UTC().Add(time.Second)}
		raw, _ := json.Marshal(value)
		if err := os.WriteFile(filepath.Join(directory, "control", "launch.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writePermit(strings.Repeat("4", 64))
	if _, err := waitForLaunch(context.Background(), root); err == nil {
		t.Fatal("wrong candidate hash admitted")
	}
	writePermit(digest(candidate))
	if value, err := waitForLaunch(context.Background(), root); err != nil || value.CandidateSHA256 != digest(candidate) {
		t.Fatal("exact bounded candidate barrier failed")
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := waitForLaunch(cancelled, root); err == nil {
		t.Fatal("cancelled launch admitted")
	}
}
