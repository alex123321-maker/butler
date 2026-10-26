// The fixed capsule entrypoint waits for the private host's measured-bounds
// barrier. It grants no authority and exposes no general execution interface.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

var errGuard = errors.New("capsule launch unavailable")

type launchPermit struct {
	Version         string    `json:"version"`
	ClaimSHA256     string    `json:"claim_sha256"`
	CandidateSHA256 string    `json:"candidate_sha256"`
	ExpiresAt       time.Time `json:"expires_at"`
}

func digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func validDigest(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == sha256.Size && hex.EncodeToString(raw) == value
}

func permit(raw []byte, now time.Time) (launchPermit, error) {
	var value launchPermit
	if len(raw) == 0 || len(raw) > 512 || json.Unmarshal(raw, &value) != nil {
		return value, errGuard
	}
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(raw, canonical) || value.Version != "butler.maintenance.capsule-launch.v1" || !validDigest(value.ClaimSHA256) || !validDigest(value.CandidateSHA256) || !value.ExpiresAt.After(now) || value.ExpiresAt.After(now.Add(30*time.Second)) {
		return launchPermit{}, errGuard
	}
	return value, nil
}

func readRegular(root *os.Root, name string, limit int64) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > limit {
		return nil, errGuard
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, errGuard
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errGuard
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(raw)) != info.Size() {
		return nil, errGuard
	}
	return raw, nil
}

// The private host atomically publishes control/launch.json only after current
// one-use dispatch and actual kernel isolation checks. No candidate code has run
// before this point. A bare marker is not a signed owner/maintenance permission;
// host confinement, controller checks and measured artifact bindings remain
// mandatory. A read-only root excludes replacing candidate between hash/exec.
func waitForLaunch(ctx context.Context, root *os.Root) (launchPermit, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if ctx.Err() != nil {
			return launchPermit{}, errGuard
		}
		_, err := root.Lstat("control/launch.json")
		if err == nil {
			raw, err := readRegular(root, "control/launch.json", 512)
			if err != nil {
				return launchPermit{}, errGuard
			}
			value, err := permit(raw, time.Now())
			if err != nil {
				return launchPermit{}, errGuard
			}
			candidate, err := readRegular(root, "candidate", 31<<20)
			if err != nil || len(candidate) < 64 || !bytes.Equal(candidate[:4], []byte{0x7f, 'E', 'L', 'F'}) || digest(candidate) != value.CandidateSHA256 || ctx.Err() != nil || !value.ExpiresAt.After(time.Now()) {
				return launchPermit{}, errGuard
			}
			return value, nil
		}
		if !os.IsNotExist(err) {
			return launchPermit{}, errGuard
		}
		select {
		case <-ctx.Done():
			return launchPermit{}, errGuard
		case <-tick.C:
		}
	}
}

func main() {
	if len(os.Args) != 1 {
		os.Exit(2)
	}
	root, err := os.OpenRoot("/")
	if err != nil {
		os.Exit(1)
	}
	_, err = waitForLaunch(context.Background(), root)
	_ = root.Close()
	if err != nil || executeCandidate() != nil {
		os.Exit(1)
	}
}
