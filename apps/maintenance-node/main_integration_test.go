package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/maintenance"
)

func TestMaintenanceNodeProcessHelper(t *testing.T) {
	if os.Getenv("BUTLER_MAINTENANCE21_NODE_HELPER") != "private-observer-only" {
		return
	}
	if run(context.Background(), os.Getenv("BUTLER_MAINTENANCE21_NODE_CONFIG"), os.Stdin, os.Stdout) != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestMaintenanceNodeActualObservationProcessIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEST_MAINTENANCE_DOCKER") != "private-local-inspection-only" {
		t.Skip("explicit private owned Docker fixture required")
	}
	inspector, err := maintenance.NewDockerInspector("/run/butler21-docker.sock", "butler21-runtime-smoke-v3")
	if err != nil {
		t.Fatal(err)
	}
	physical, err := inspector.Inspect(t.Context())
	if err != nil || physical.State != "stopped" || physical.ArtifactSHA256 != os.Getenv("BUTLER_TEST_MAINTENANCE_DOCKER_IMAGE") {
		t.Fatal("exact actual owned fixture unavailable", err)
	}
	controller, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	checker, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	node, nodeKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	digest := func(value string) string { return maintenance.Digest([]byte(value)) }
	profile := maintenance.Profile{
		Version: "butler.maintenance.profile.v1", OwnerID: "synthetic-node-owner", TargetID: "synthetic-node-target", Revision: 1,
		SourceOriginSHA256: digest("source"), Boundaries: []maintenance.Boundary{{Prefix: "modules/public/", Class: maintenance.Application}},
		CheckerPublicKey: checker, ControllerPublicKey: controller, NodePublicKey: node,
		CheckerSHA256: digest("checker"), FixedCorpusSHA256: digest("fixed"), HiddenCorpusSHA256: digest("hidden"), CriteriaSHA256: digest("criteria"),
		Required:          []maintenance.RequiredCheck{{ID: "fixed", Kind: "regression"}, {ID: "hidden", Kind: "hidden"}, {ID: "state", Kind: "compatibility"}, {ID: "function", Kind: "functional"}},
		StateSchemaSHA256: digest("schema"), ProtocolSHA256: digest("protocol"), RuntimeSHA256: physical.RuntimeSHA256,
		AnchorArtifactSHA256: physical.ArtifactSHA256, AnchorInventorySHA256: digest("inventory"), MaxTrialSeconds: 30, MaxEffectSeconds: 10,
	}
	profileRaw, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	private := t.TempDir()
	write := func(name string, raw []byte) string {
		path := filepath.Join(private, name)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal("private synthetic process fixture unavailable")
		}
		return path
	}
	profileFile := write("profile.json", profileRaw)
	seedFile := write("node.seed", nodeKey.Seed())
	clear(nodeKey)
	configRaw, err := json.Marshal(configuration{DockerSocket: "/run/butler21-docker.sock", Container: "butler21-runtime-smoke-v3", ProfileFile: profileFile, SigningSeedFile: seedFile})
	if err != nil {
		t.Fatal(err)
	}
	configFile := write("node.json", configRaw)
	challenge := maintenance.ObservationChallenge{ProfileSHA256: digest(string(profileRaw)), Epoch: 1, Nonce: digest("fresh independent physical challenge")}
	challengeRaw, err := json.Marshal(challenge)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestMaintenanceNodeProcessHelper$")
	command.Env = append(os.Environ(), "BUTLER_MAINTENANCE21_NODE_HELPER=private-observer-only", "BUTLER_MAINTENANCE21_NODE_CONFIG="+configFile)
	command.Stdin = bytes.NewReader(challengeRaw)
	raw, err := command.Output()
	if err != nil || len(raw) > 16384 {
		t.Fatal("independent actual physical observer process failed")
	}
	var signed maintenance.SignedNodeObservation
	if json.Unmarshal(raw, &signed) != nil {
		t.Fatal("independent observer response malformed")
	}
	observation, err := maintenance.InspectNodeObservation(signed.Raw, signed.Signature, profile, challenge.ProfileSHA256, challenge.Nonce, 1, time.Now())
	if err != nil || observation.ArtifactSHA256 != physical.ArtifactSHA256 || observation.RuntimeSHA256 != physical.RuntimeSHA256 || observation.State != "stopped" || observation.EffectSettled || observation.LastAttemptID != "" {
		t.Fatal("separate process did not sign exact measured physical identity", err)
	}
	t.Log("separate private node process measured actual immutable Docker identity; no Core, deployment, settlement or functional success claimed")
}
