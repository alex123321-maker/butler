//go:build !windows

package maintenance

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNodeObserverSignsOnlyMeasuredPhysicalIdentity(t *testing.T) {
	profile := publicProfile(t)
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	profile.NodePublicKey = public
	profileRaw, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(t.TempDir(), "node.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int64
	var unavailable atomic.Bool
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != http.MethodGet || request.URL.Path != "/containers/public-module/json" || request.URL.RawQuery != "" {
			t.Error("observer selected an arbitrary daemon operation")
			writer.WriteHeader(400)
			return
		}
		if unavailable.Load() {
			writer.WriteHeader(503)
			_, _ = writer.Write([]byte("synthetic private daemon detail"))
			return
		}
		_ = json.NewEncoder(writer).Encode(publicDockerInspect())
	})}
	go server.Serve(listener)
	t.Cleanup(func() { _ = server.Close() })
	node, err := NewNodeObserver(socket, "public-module", profileRaw, key)
	if err != nil {
		t.Fatal(err)
	}
	clear(key) // The node owns a copy of its private startup key.
	challenge := ObservationChallenge{ProfileSHA256: Digest(profileRaw), Epoch: 3, Nonce: strings.Repeat("5", 64)}
	signed, err := node.Observe(t.Context(), challenge)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := InspectNodeObservation(signed.Raw, signed.Signature, profile, challenge.ProfileSHA256, challenge.Nonce, challenge.Epoch, time.Now())
	if err != nil || observation.State != "running" || observation.ArtifactSHA256 != strings.Repeat("2", 64) || !validDigest(observation.RuntimeSHA256) || observation.EffectSettled || observation.LastAttemptID != "" {
		t.Fatal("measured signature was replaced by health or settlement authority", err)
	}
	before := requests.Load()
	wrong := challenge
	wrong.ProfileSHA256 = strings.Repeat("6", 64)
	if _, err := node.Observe(t.Context(), wrong); err == nil || requests.Load() != before {
		t.Fatal("wrong profile reached physical inspection")
	}
	unavailable.Store(true)
	if signed, err := node.Observe(t.Context(), challenge); err == nil || len(signed.Raw) != 0 || strings.Contains(err.Error(), "private daemon") {
		t.Fatal("unavailable measurement was signed or leaked")
	}
}
