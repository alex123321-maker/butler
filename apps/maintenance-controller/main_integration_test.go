package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/maintenance"
	"github.com/butler/butler/internal/memory/memorytest"
)

func TestMaintenanceOwnerControlProcessHelper(t *testing.T) {
	if os.Getenv("BUTLER_MAINTENANCE21_PROCESS_HELPER") != "local-owner-only" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	if err := run(ctx, os.Getenv("BUTLER_MAINTENANCE21_PRIVATE_CONFIG")); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestMaintenanceOwnerStopWithoutCoreProcessIntegration(t *testing.T) {
	maintenanceOwnerStopProcess(t, false)
}

func TestMaintenanceNodeCredentialSeparationProcessIntegration(t *testing.T) {
	maintenanceOwnerStopProcess(t, true)
}

func maintenanceOwnerStopProcess(t *testing.T, nodeEnabled bool) {
	t.Helper()
	pool := memorytest.Database(t)
	store := maintenance.Store{Pool: pool}
	controller, key, err := ed25519.GenerateKey(rand.Reader)
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
	digest := func(v string) string { return maintenance.Digest([]byte(v)) }
	profile := maintenance.Profile{Version: "butler.maintenance.profile.v1", OwnerID: "local-controller-owner", TargetID: "local-controller-target", Revision: 1, SourceOriginSHA256: digest("operator source"), Boundaries: []maintenance.Boundary{{Prefix: "modules/local/", Class: maintenance.Application}}, CheckerPublicKey: checker, ControllerPublicKey: controller, NodePublicKey: node, CheckerSHA256: digest("checker"), FixedCorpusSHA256: digest("fixed"), HiddenCorpusSHA256: digest("hidden"), CriteriaSHA256: digest("criteria"), Required: []maintenance.RequiredCheck{{ID: "fixed", Kind: "regression"}, {ID: "hidden", Kind: "hidden"}, {ID: "state", Kind: "compatibility"}, {ID: "function", Kind: "functional"}}, StateSchemaSHA256: digest("schema"), ProtocolSHA256: digest("protocol"), RuntimeSHA256: digest("runtime"), AnchorArtifactSHA256: digest("base artifact"), AnchorInventorySHA256: digest("base inventory"), MaxTrialSeconds: 30, MaxEffectSeconds: 10}
	profileRaw, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.InstallProfile(t.Context(), profileRaw, ""); err != nil {
		t.Fatal(err)
	}
	var databaseNow time.Time
	if err = pool.QueryRow(t.Context(), `SELECT clock_timestamp()`).Scan(&databaseNow); err != nil {
		t.Fatal(err)
	}
	observation := maintenance.NodeObservation{Version: "butler.maintenance.node-observation.v1", OwnerID: profile.OwnerID, TargetID: profile.TargetID, ProfileSHA256: maintenance.Digest(profileRaw), Epoch: 1, Nonce: digest("synthetic bootstrap identity"), State: "running", ArtifactSHA256: profile.AnchorArtifactSHA256, RuntimeSHA256: profile.RuntimeSHA256, ObservedAt: databaseNow}
	body, signature, err := maintenance.SignNodeObservation(observation, nodeKey)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := store.RecordObservation(t.Context(), profile.OwnerID, profile.TargetID, observation.ProfileSHA256, observation.Nonce, 1, body, signature); err != nil || !result.Applied {
		t.Fatal("synthetic identity bootstrap failed", err)
	}
	if target, err := store.Resume(t.Context(), profile.OwnerID, profile.TargetID, observation.ProfileSHA256, 1); err != nil || target.Stopped || target.Epoch != 2 {
		t.Fatal("owner-control precondition must be enabled", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	private := t.TempDir()
	write := func(name string, raw []byte) string {
		path := filepath.Join(private, name)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal("private fixture write failed")
		}
		return path
	}
	token := strings.Repeat("public-synthetic-owner-", 3)
	nodeToken := strings.Repeat("public-synthetic-node-", 3)
	// pgx's original ConnString does not include RuntimeParams changed by the
	// owning memorytest helper. Preserve its actual isolated schema explicitly.
	databaseURL, err := url.Parse(pool.Config().ConnString())
	if err != nil || (databaseURL.Scheme != "postgres" && databaseURL.Scheme != "postgresql") {
		t.Fatal("local process acceptance requires a PostgreSQL URI")
	}
	parameters := databaseURL.Query()
	parameters.Set("search_path", pool.Config().ConnConfig.RuntimeParams["search_path"])
	databaseURL.RawQuery = parameters.Encode()
	config := configuration{Listen: address, DatabaseURLFile: write("database.ref", []byte(databaseURL.String())), SigningSeedFile: write("controller.seed", key.Seed()), OwnerTokenFile: write("owner.token", []byte(token)), ProfileFile: write("profile.json", profileRaw)}
	if nodeEnabled {
		config.NodeTokenFile = write("node.token", []byte(nodeToken))
	}
	configRaw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	configPath := write("control.json", configRaw)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	client := http.Client{Timeout: 2 * time.Second}
	postPath := func(path, auth, body string) (int, []byte, error) {
		request, err := http.NewRequestWithContext(t.Context(), "POST", "http://"+address+path, strings.NewReader(body))
		if err != nil {
			return 0, nil, err
		}
		request.Header.Set("Authorization", "Bearer "+auth)
		response, err := client.Do(request)
		if err != nil {
			return 0, nil, err
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(response.Body, 65537))
		return response.StatusCode, raw, err
	}
	post := func(auth, body string) (int, []byte, error) {
		return postPath("/private/maintenance/owner", auth, body)
	}
	start := func() func() {
		cmd := exec.CommandContext(t.Context(), executable, "-test.run=^TestMaintenanceOwnerControlProcessHelper$")
		cmd.Env = append(os.Environ(), "BUTLER_MAINTENANCE21_PROCESS_HELPER=local-owner-only", "BUTLER_MAINTENANCE21_PRIVATE_CONFIG="+configPath)
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		if err := cmd.Start(); err != nil {
			t.Fatal("separate owner service did not start")
		}
		stop := func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }
		deadline := time.Now().Add(8 * time.Second)
		for {
			status, _, err := post(token, `{"operation":"status"}`)
			if err == nil && status == 200 {
				break
			}
			if time.Now().After(deadline) {
				stop()
				t.Fatal("separate owner service unavailable; private startup output withheld")
			}
			time.Sleep(25 * time.Millisecond)
		}
		return stop
	}
	stop := start()
	if nodeEnabled {
		if status, _, err := post(nodeToken, `{"operation":"status"}`); err != nil || status != http.StatusUnauthorized {
			stop()
			t.Fatal("node credential gained owner controls", status, err)
		}
		request := maintenance.NodeControlRequest{Operation: "dispatch", Raw: []byte(`{}`), Signature: make([]byte, ed25519.SignatureSize)}
		body, _ := json.Marshal(request)
		if status, _, err := postPath(maintenance.NodeControlPath, token, string(body)); err != nil || status != http.StatusUnauthorized {
			stop()
			t.Fatal("owner credential crossed the separate node boundary", status, err)
		}
		if status, _, err := postPath(maintenance.NodeControlPath, nodeToken, string(body)); err != nil || status != http.StatusConflict {
			stop()
			t.Fatal("actual node route failed to reject an unsigned claim", status, err)
		}
		request.Operation = "stop"
		body, _ = json.Marshal(request)
		if status, _, err := postPath(maintenance.NodeControlPath, nodeToken, string(body)); err != nil || status != http.StatusBadRequest {
			stop()
			t.Fatal("node route accepted owner stop", status, err)
		}
	} else {
		if status, _, err := postPath(maintenance.NodeControlPath, token, `{"operation":"status"}`); err != nil || status != http.StatusMethodNotAllowed {
			stop()
			t.Fatal("node route was enabled without separate configuration", status, err)
		}
	}
	if status, _, err := post("wrong-private-token", `{"operation":"stop"}`); err != nil || status != 401 {
		stop()
		t.Fatal("untrusted owner control admitted", status, err)
	}
	if status, _, err := post(token, `{"operation":"stop","operation":"status"}`); err != nil || status != 400 {
		stop()
		t.Fatal("duplicate interpretation admitted", status, err)
	}
	if status, _, err := post(token, `{"operation":"stop"}`); err != nil || status != 200 {
		stop()
		t.Fatal("independent owner stop failed", status, err)
	}
	stop()
	target, err := store.Target(t.Context(), profile.OwnerID, profile.TargetID)
	if err != nil || !target.Stopped || target.Epoch != 3 {
		t.Fatal("separate stop was not durable", err)
	}
	stop = start()
	defer stop()
	status, raw, err := post(token, `{"operation":"status"}`)
	var response maintenance.OwnerControlResponse
	if err != nil || status != 200 || json.Unmarshal(raw, &response) != nil || response.Target == nil || !response.Target.Stopped || response.Target.Epoch != 3 {
		t.Fatal("service restart erased owner stop", status, err)
	}
	t.Logf("actual separate controller process, Core absent, node_route=%t, role boundaries enforced, unauthenticated/ambiguous stop denied, durable stop survives restart; no physical node/deployment/inference proof", nodeEnabled)
}
