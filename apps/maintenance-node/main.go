package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/butler/butler/internal/maintenance"
)

// Initial one-shot private node observer, default disabled. It has no deployment
// action or general signing API. Input contains only a bounded typed challenge;
// all physical target and key references belong to private startup configuration.
type configuration struct {
	DockerSocket    string `json:"docker_socket"`
	Container       string `json:"container"`
	ProfileFile     string `json:"profile_file"`
	SigningSeedFile string `json:"signing_seed_file"`
}

func readBounded(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, maintenance.ErrState
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > limit {
		return nil, maintenance.ErrState
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, maintenance.ErrState
	}
	return raw, nil
}

func run(ctx context.Context, configPath string, input io.Reader, output io.Writer) error {
	raw, err := readBounded(configPath, 8192)
	if err != nil {
		return maintenance.ErrState
	}
	var config configuration
	if json.Unmarshal(raw, &config) != nil {
		return maintenance.ErrState
	}
	canonical, err := json.Marshal(config)
	if err != nil || !bytes.Equal(canonical, raw) {
		return maintenance.ErrState
	}
	profile, err := readBounded(config.ProfileFile, 1<<20)
	if err != nil {
		return maintenance.ErrState
	}
	seed, err := readBounded(config.SigningSeedFile, ed25519.SeedSize)
	if err != nil || len(seed) != ed25519.SeedSize {
		return maintenance.ErrState
	}
	key := ed25519.NewKeyFromSeed(seed)
	clear(seed)
	defer clear(key)
	observer, err := maintenance.NewNodeObserver(config.DockerSocket, config.Container, profile, key)
	if err != nil {
		return maintenance.ErrState
	}
	challengeRaw, err := io.ReadAll(io.LimitReader(input, 4097))
	if err != nil {
		return maintenance.ErrState
	}
	challenge, err := maintenance.ParseObservationChallenge(challengeRaw)
	if err != nil {
		return maintenance.ErrState
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	observation, err := observer.Observe(ctx, challenge)
	if err != nil {
		return maintenance.ErrState
	}
	// Contains only a signed measured identity, never configuration or seed bytes.
	if json.NewEncoder(output).Encode(observation) != nil {
		return maintenance.ErrState
	}
	return nil
}

func main() {
	enabled := flag.Bool("enable-local-observation", false, "explicitly enable fixed private local identity observation")
	path := flag.String("config", "", "private node configuration file")
	flag.Parse()
	if !*enabled || *path == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "private node observation disabled")
		os.Exit(2)
	}
	if run(context.Background(), *path, os.Stdin, os.Stdout) != nil {
		fmt.Fprintln(os.Stderr, "private node observation unavailable")
		os.Exit(1)
	}
}
