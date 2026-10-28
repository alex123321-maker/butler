//go:build !windows

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/maintenance"
)

// Public operational reproducer only. All key/profile bytes are created here;
// no real key/auth/config, Docker daemon, PostgreSQL or candidate code is used.
func TestInputMustReturnAfterContextDeadline(t *testing.T) {
	private := t.TempDir()
	socket := filepath.Join(private, "synthetic.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	checker, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	controller, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	node, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	digest := func(s string) string { return maintenance.Digest([]byte(s)) }
	profile := maintenance.Profile{
		Version: "butler.maintenance.profile.v1", OwnerID: "synthetic-pipe-owner", TargetID: "synthetic-target", Revision: 1,
		SourceOriginSHA256: digest("source"), Boundaries: []maintenance.Boundary{{Prefix: "modules/public/", Class: maintenance.Application}},
		CheckerPublicKey: checker, ControllerPublicKey: controller, NodePublicKey: node,
		CheckerSHA256: digest("checker"), FixedCorpusSHA256: digest("fixed"), HiddenCorpusSHA256: digest("hidden"), CriteriaSHA256: digest("criteria"),
		Required:          []maintenance.RequiredCheck{{ID: "regression", Kind: "regression"}, {ID: "hidden", Kind: "hidden"}, {ID: "state", Kind: "compatibility"}, {ID: "function", Kind: "functional"}},
		StateSchemaSHA256: digest("schema"), ProtocolSHA256: digest("protocol"), RuntimeSHA256: digest("runtime"),
		AnchorArtifactSHA256: digest("artifact"), AnchorInventorySHA256: digest("inventory"), MaxTrialSeconds: 30, MaxEffectSeconds: 10,
	}
	profileRaw, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := maintenance.ParseProfile(profileRaw); err != nil {
		t.Fatal("synthetic profile invalid", err)
	}
	write := func(name string, body []byte) string {
		p := filepath.Join(private, name)
		if err := os.WriteFile(p, body, 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	pfile := write("synthetic-profile.json", profileRaw)
	sfile := write("synthetic-node.seed", key.Seed())
	clear(key)
	cfg, err := json.Marshal(configuration{DockerSocket: socket, Container: "synthetic-target", ProfileFile: pfile, SigningSeedFile: sfile})
	if err != nil {
		t.Fatal(err)
	}
	cfile := write("synthetic-startup.json", cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	input, heldOpen := io.Pipe()
	defer input.Close()
	defer heldOpen.Close()
	result := make(chan error, 1)
	var output bytes.Buffer
	go func() { result <- run(ctx, cfile, input, &output) }()
	consumed := make(chan error, 1)
	go func() { _, err := heldOpen.Write([]byte("{")); consumed <- err }()
	select {
	case err := <-consumed:
		if err != nil {
			t.Fatal("prefix not consumed", err)
		}
	case err := <-result:
		t.Fatal("fixture rejected before input read", err)
	case <-time.After(2 * time.Second):
		t.Fatal("could not establish blocked reader")
	}
	start := time.Now()
	select {
	case <-result:
		t.Log("context-bounded input returned without EOF")
	case <-time.After(5250 * time.Millisecond):
		if ctx.Err() != context.DeadlineExceeded {
			t.Fatal("context did not expire")
		}
		t.Errorf("REPRO: run still blocked on one stdin byte/no EOF after %s; caller context expired and advertised 5s observation bound elapsed; output_bytes=%d", time.Since(start), output.Len())
		_ = heldOpen.CloseWithError(io.ErrUnexpectedEOF)
		select {
		case <-result:
		case <-time.After(time.Second):
			t.Fatal("probe cleanup failed")
		}
	}
}
