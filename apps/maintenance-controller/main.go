package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/butler/butler/internal/maintenance"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This initial local-only service exposes independent owner controls, never
// an updater, model tool, general shell or Docker API. Private files are read
// at startup and their contents are never printed in errors or request logs.
type configuration struct {
	Listen          string `json:"listen"`
	DatabaseURLFile string `json:"database_url_file"`
	SigningSeedFile string `json:"signing_seed_file"`
	OwnerTokenFile  string `json:"owner_token_file"`
	ProfileFile     string `json:"profile_file"`
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

func run(ctx context.Context, path string) error {
	raw, err := readBounded(path, 8192)
	if err != nil {
		return err
	}
	var config configuration
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&config) != nil || decoder.Decode(new(any)) != io.EOF {
		return maintenance.ErrState
	}
	canonical, err := json.Marshal(config)
	if err != nil || !bytes.Equal(raw, canonical) {
		return maintenance.ErrState
	}
	host, _, err := net.SplitHostPort(config.Listen)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return maintenance.ErrState
	}
	profileRaw, err := readBounded(config.ProfileFile, 1<<20)
	if err != nil {
		return err
	}
	profile, err := maintenance.ParseProfile(profileRaw)
	if err != nil {
		return err
	}
	seed, err := readBounded(config.SigningSeedFile, ed25519.SeedSize)
	if err != nil || len(seed) != ed25519.SeedSize {
		return maintenance.ErrState
	}
	key := ed25519.NewKeyFromSeed(seed)
	if !bytes.Equal(key.Public().(ed25519.PublicKey), profile.ControllerPublicKey) {
		return maintenance.ErrState
	}
	tokenRaw, err := readBounded(config.OwnerTokenFile, 512)
	if err != nil {
		return err
	}
	token := strings.TrimSpace(string(tokenRaw))
	if len(token) < 32 || strings.ContainsAny(token, "\r\n\x00") {
		return maintenance.ErrState
	}
	databaseRaw, err := readBounded(config.DatabaseURLFile, 8192)
	if err != nil {
		return err
	}
	databaseConfig, err := pgxpool.ParseConfig(strings.TrimSpace(string(databaseRaw)))
	if err != nil {
		return maintenance.ErrState
	}
	databaseConfig.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, databaseConfig)
	if err != nil {
		return maintenance.ErrState
	}
	defer pool.Close()
	store := maintenance.Store{Pool: pool}
	startup, cancel := context.WithTimeout(ctx, 5*time.Second)
	target, err := store.Target(startup, profile.OwnerID, profile.TargetID)
	cancel()
	if err != nil || target.ProfileSHA256 != maintenance.Digest(profileRaw) {
		return maintenance.ErrState
	}
	listener, err := net.Listen("tcp", config.Listen)
	if err != nil {
		return maintenance.ErrState
	}
	server := http.Server{Handler: maintenance.NewOwnerControlHandler(store, profile.OwnerID, profile.TargetID, token, key), ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 8 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 8192}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	fmt.Fprintln(os.Stdout, "maintenance owner control listening", listener.Addr().String())
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return maintenance.ErrState
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

func main() {
	config := flag.String("config", "", "private local configuration file")
	enabled := flag.Bool("enable-local-owner-control", false, "explicitly enable local independent owner controls")
	flag.Parse()
	if !*enabled {
		fmt.Fprintln(os.Stdout, "maintenance owner control disabled")
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *config == "" || run(ctx, *config) != nil {
		fmt.Fprintln(os.Stderr, "maintenance owner control unavailable")
		os.Exit(1)
	}
}
