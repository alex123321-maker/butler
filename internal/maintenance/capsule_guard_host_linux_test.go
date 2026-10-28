//go:build linux

package maintenance

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCapsuleGuardColdIdentityIsNotRuntime(t *testing.T) {
	root := publicDockerInspect()
	state := root["State"].(map[string]any)
	state["Running"], state["Status"], state["Pid"] = false, "created", 0
	root["NetworkSettings"].(map[string]any)["Networks"].(map[string]any)["private"].(map[string]any)["NetworkID"] = ""
	raw, _ := json.Marshal(root)
	identity, pid, err := capsuleOwnedState(raw, strings.Repeat("1", 64), "public-module", strings.Repeat("2", 64))
	if err != nil || pid != 0 || identity.State != "created" || identity.RuntimeSHA256 != "" {
		t.Fatal("cold owned object fabricated a runtime identity", err)
	}
	if _, err = inspectDockerIdentity(raw, "public-module", time.Now()); err == nil {
		t.Fatal("unchanged runtime inspector accepted missing physical network identity")
	}
	if _, _, err = capsuleOwnedState(raw, strings.Repeat("4", 64), "public-module", strings.Repeat("2", 64)); err == nil {
		t.Fatal("foreign object accepted for cleanup")
	}
	delete(state, "Running")
	raw, _ = json.Marshal(root)
	if _, _, err = capsuleOwnedState(raw, strings.Repeat("1", 64), "public-module", strings.Repeat("2", 64)); err == nil {
		t.Fatal("missing stopped-state proof accepted")
	}
}

func TestCapsuleGuardExactEnvironmentAndMountBoundary(t *testing.T) {
	good := "GOMAXPROCS=2\x00HOME=/\x00HOSTNAME=maintenance-capsule\x00PATH=/\x00"
	if !capsuleEnvironment([]byte(good)) {
		t.Fatal("fixed environment rejected")
	}
	for _, value := range []string{good + "EXTRA=value\x00", strings.Replace(good, "PATH=/\x00", "", 1), good + "PATH=/\x00", strings.Replace(good, "HOME=/", "HOME=/private", 1)} {
		if capsuleEnvironment([]byte(value)) {
			t.Fatal("non-exact environment accepted")
		}
	}
	mounts := "1 0 1:1 / / ro,relatime - overlay overlay ro\n2 1 0:2 / /control rw,nosuid,nodev,noexec - tmpfs tmpfs rw\n3 1 0:3 / /sys/fs/cgroup ro,nosuid,nodev,noexec - cgroup2 cgroup ro\n"
	if !capsuleMounts([]byte(mounts)) {
		t.Fatal("declared minimal mount shape rejected")
	}
	for _, value := range []string{strings.Replace(mounts, "/ ro,relatime", "/ rw,relatime", 1), mounts + "4 1 0:4 / /operator rw - ext4 private rw\n", mounts + strings.Split(mounts, "\n")[0] + "\n"} {
		if capsuleMounts([]byte(value)) {
			t.Fatal("foreign mount/writable root/ambiguous shape accepted")
		}
	}
}

func TestCapsuleGuardNeverStartedProofIsSeparateFromCleanup(t *testing.T) {
	base := publicDockerInspect()
	state := base["State"].(map[string]any)
	state["Running"], state["Status"], state["Pid"] = false, "created", 0
	state["StartedAt"], state["FinishedAt"] = time.Time{}, time.Time{}
	base["RestartCount"] = 0
	raw, _ := json.Marshal(base)
	if !capsuleNeverStarted(raw) {
		t.Fatal("explicit never-started state rejected")
	}
	for name, mutate := range map[string]func(map[string]any){
		"exited":          func(v map[string]any) { v["State"].(map[string]any)["Status"] = "exited" },
		"started":         func(v map[string]any) { v["State"].(map[string]any)["StartedAt"] = time.Now().UTC() },
		"finished":        func(v map[string]any) { v["State"].(map[string]any)["FinishedAt"] = time.Now().UTC() },
		"restart":         func(v map[string]any) { v["RestartCount"] = 1 },
		"missing counter": func(v map[string]any) { delete(v, "RestartCount") },
		"missing time":    func(v map[string]any) { delete(v["State"].(map[string]any), "StartedAt") },
	} {
		t.Run(name, func(t *testing.T) {
			var v map[string]any
			json.Unmarshal(raw, &v)
			mutate(v)
			body, _ := json.Marshal(v)
			if capsuleNeverStarted(body) {
				t.Fatal("non-initial state became start permission")
			}
		})
	}
	state["Status"] = "exited"
	state["StartedAt"] = time.Now().UTC()
	raw, _ = json.Marshal(base)
	if _, _, err := capsuleOwnedState(raw, strings.Repeat("1", 64), "public-module", strings.Repeat("2", 64)); err != nil {
		t.Fatal("exact exited object cannot be safely cleaned", err)
	}
}

func capsuleGuardHostFixture(t *testing.T) *capsuleGuardHost {
	t.Helper()
	if os.Getenv("BUTLER_TEST_MAINTENANCE_GUARD_KERNEL") != "private-trusted-guard-only" {
		t.Skip("explicit private guard-only kernel measurement required")
	}
	host, err := newCapsuleGuardHost("/run/butler21-docker.sock", os.Getenv("BUTLER_TEST_MAINTENANCE_GUARD_IMAGE"), os.Getenv("BUTLER_TEST_MAINTENANCE_GUARD_SHA256"), os.Getenv("BUTLER_TEST_MAINTENANCE_CANDIDATE_SHA256"))
	if err != nil {
		t.Fatal("fixed physical artifact configuration unavailable", err)
	}
	return host
}

func TestCapsuleGuardTerminalFaultCannotBecomeQualified(t *testing.T) {
	// Synthetic terminal-state regression, not a reproduced os.Root.Close fault.
	prepared := &capsuleGuardPrepared{controlCloseFault: true}
	w, err := prepared.measure(t.Context())
	if err != errCapsuleGuard || w.ContainerID != "" {
		t.Fatal("terminal fault reached privileged measurement or qualification")
	}
}

func TestCapsuleGuardCloseBudgetIncludesLifecycleWait(t *testing.T) {
	// No container or host client exists: only the bounded lifecycle gate runs.
	prepared := &capsuleGuardPrepared{}
	prepared.mu.Lock()
	started := time.Now()
	err := prepared.close()
	elapsed := time.Since(started)
	prepared.mu.Unlock()
	if err != errCapsuleGuard || prepared.cleanupConfirmed || !prepared.closing.Load() || elapsed < 4*time.Second || elapsed > 7*time.Second {
		t.Fatalf("lifecycle wait escaped original five-second close budget: %s", elapsed)
	}
	started = time.Now()
	if prepared.close() != errCapsuleGuard || time.Since(started) > time.Second {
		t.Fatal("unknown cleanup was retried instead of retaining its first outcome")
	}
	if _, err = prepared.measure(t.Context()); err != errCapsuleGuard {
		t.Fatal("close request was forgotten by a later measurement")
	}
}

type capsuleGuardTrace struct {
	base  http.RoundTripper
	calls int
}

func (trace *capsuleGuardTrace) RoundTrip(request *http.Request) (*http.Response, error) {
	trace.calls++
	return trace.base.RoundTrip(request)
}

func TestCapsuleGuardOwnedKernelIntegration(t *testing.T) {
	host := capsuleGuardHostFixture(t)
	trace := &capsuleGuardTrace{base: host.api.client.Transport}
	host.api.client.Transport = trace
	prepared, err := host.prepare(t.Context())
	if prepared != nil {
		t.Cleanup(func() {
			if err := prepared.close(); err != nil {
				t.Error("owned cleanup unqualified", err)
			}
		})
	}
	if err != nil {
		stage := "constructor"
		diagnostic := []byte("null")
		if prepared != nil {
			stage = prepared.stage
			diagnostic, _ = json.Marshal(prepared.descriptorDiagnostic)
		}
		t.Fatalf("trusted guard preparation unqualified stage=%s descriptors=%s error=%v", stage, diagnostic, err)
	}
	w := prepared.witness
	if w.ContainerID != prepared.id || w.PID < 2 || w.PIDStartTicks == 0 || !w.GuardIsPID1 || !w.MarkerAbsent || w.CandidateExecuted || w.MemoryBytes != 128<<20 || w.PIDs != 32 || w.MaxOpenFiles != 64 || w.CPU != "100000 100000" || w.EffectiveSwapMax != 0 || !validDigest(w.RuntimeSHA256) || !validDigest(w.NamespaceSHA256) || !validDigest(w.MountSHA256) || !validDigest(w.DescriptorSHA256) || w.MeasuredAt.IsZero() {
		t.Fatal("actual guard physical witness incomplete")
	}
	if _, err := prepared.control.Lstat("launch.json"); !os.IsNotExist(err) {
		t.Fatal("a marker was published by a preparation-only operation")
	}
	if err = prepared.close(); err != nil || !prepared.cleanupConfirmed {
		t.Fatal("actual guard kill/join/delete unavailable", err)
	}
	count := trace.calls
	if err = prepared.close(); err != nil || trace.calls != count {
		t.Fatal("closed handle replayed a physical cleanup")
	}
	public, _ := json.Marshal(w)
	t.Logf("actual trusted guard witness=%s; no marker/candidate/model/claim; exact owned container killed, waited and removed", public)
}

func TestCapsuleGuardMeasureCloseSerializedIntegration(t *testing.T) {
	host := capsuleGuardHostFixture(t)
	trace := &capsuleGuardTrace{base: host.api.client.Transport}
	host.api.client.Transport = trace
	prepared, err := host.prepare(t.Context())
	if prepared != nil {
		t.Cleanup(func() {
			if e := prepared.close(); e != nil {
				t.Error("owned cleanup unqualified", e)
			}
		})
	}
	if err != nil {
		t.Fatal("trusted guard preparation unavailable", err)
	}
	start := make(chan struct{})
	measured, closed := make(chan error, 1), make(chan error, 1)
	go func() { <-start; _, e := prepared.measure(t.Context()); measured <- e }()
	go func() { <-start; closed <- prepared.close() }()
	close(start)
	measureErr, closeErr := <-measured, <-closed
	if closeErr != nil || !prepared.cleanupConfirmed {
		t.Fatal("close lost the actual guard join/delete", closeErr)
	}
	if measureErr != nil && measureErr != errCapsuleGuard {
		t.Fatal("unexpected measurement result", measureErr)
	}
	count := trace.calls
	if err := prepared.close(); err != nil || trace.calls != count {
		t.Fatal("serialized closed handle replayed its cleanup")
	}
	t.Logf("actual guard PID%d measure_qualified=%v cleanup_confirmed=true; lifecycle operations serialized, no marker/candidate/claim", prepared.witness.PID, measureErr == nil)
}
