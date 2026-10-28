//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/providerauth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Binding occurs only after the new server producer/recipe/five artifacts/plan
// are frozen. Empty commitments deny before filesystem, DB or auth access.
const laba115NativeSource = "b1fb478c71c2ced8792126370d68eb90269c9757"
const laba115NativePlanSHA = "18d4ec76d5cbbbf241d7151eb542e7a48d7125c7ea4452ac8c67e0d148d729a2"
const laba115NativeHistorySHA = "720ac0997a0bfef54b2a15817170abd5f13a8abaa683209975a63fffaf7bd389"
const laba115NativeAppSHA = "e348406999e75c5d4fca779e48c1424d0bae69558e9d7a1ce40468c28ab75e36"
const laba115NativeBrokerSHA = "fa1396b6bd9218ab3c25c574e99cd196feeebaabe48faa4d4685d3d96f9cac0c"
const laba115NativeBrokerControlSHA = "4129f9d313759914121de9de1e99bfc1f4108ae1d96229955f7e8d6400f35307"
const laba115NativeExtensionSHA = "40c25e3fa429763f713580eff0b967719b72e66d23eb77a39b4ef81dea4b473b"
const laba115NativeHTTPSHA = "c421446575db51e377546ed5f6c43330982c398496ae71c350bae4075b58ceca"
const laba115ControlDSN = "postgres://postgres@127.0.0.1:55455/behavior76_test?sslmode=disable"
const laba115TargetHostDSN = "postgres://postgres@127.0.0.1:55455/memory115_laba_abc_v5_test?sslmode=disable"
const laba115TargetChildDSN = "postgres://postgres@127.0.0.1:55451/memory115_laba_abc_v5_test?sslmode=disable"
const laba115NativePG = "issue115-native-pg-20261028-v1"
const laba115NativeNetwork = "issue115-native-20261028-v1"
const laba115PGImage = "sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d"

func laba115PGConfig(c laba115Container) bool {
	h := c.HostConfig
	if c.Image != laba115PGImage || !c.State.Running || h.Privileged || !h.ReadonlyRootfs || h.NetworkMode != laba115NativeNetwork || h.Memory != 536870912 || h.MemorySwap != 536870912 || h.NanoCpus != 1000000000 || h.PidsLimit != 128 || len(c.Mounts) != 1 {
		return false
	}
	m := c.Mounts[0]
	ports := c.NetworkSettings.Ports["55451/tcp"]
	for port, bindings := range c.NetworkSettings.Ports {
		if port != "55451/tcp" && (port != "5432/tcp" || len(bindings) != 0) {
			return false
		}
	}
	return m.Type == "bind" && m.Source == filepath.Join(laba115HostRoot, "postgres") && m.Destination == "/var/lib/postgresql/data" && m.RW && len(ports) == 1 && ports[0].HostIP == "127.0.0.1" && ports[0].HostPort == "55455"
}

func laba115OnlyInit(ctx context.Context, id string, pid int) bool {
	top, err := laba115Docker(ctx, "top", id, "-eo", "pid")
	fields := strings.Fields(string(top))
	return err == nil && len(fields) == 2 && fields[1] == fmt.Sprint(pid)
}

// Exercise the exact native capture/stop path with a synthetic leaked child.
// This mode has no source, database, key or provider mounts.
func laba115Containment() error {
	ctx, done := context.WithTimeout(context.Background(), 30*time.Second)
	defer done()
	dir := filepath.Join(laba115HostRoot, "containment-v1")
	if os.Mkdir(dir, 0700) != nil {
		return errors.New("new containment proof identity required")
	}
	raw, err := laba115Docker(ctx, "create", "--name", "issue115-native-containment-20261028-v1", "--network", "none", "--cpus", "1", "--memory", "2g", "--memory-swap", "2g", "--pids-limit", "256", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--log-driver", "none", "--user", "1000:1000", laba115Image, "/bin/sleep", "60")
	id := strings.TrimSpace(string(raw))
	if err != nil || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(id) {
		return errors.New("new containment container unavailable")
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_, _ = laba115Docker(cleanup, "rm", "-f", id)
	}()
	if _, err = laba115Docker(ctx, "start", id); err != nil {
		return err
	}
	p, err := laba115Capture(ctx, id, "none")
	if err != nil {
		return err
	}
	if !laba115OnlyInit(ctx, id, p.PID) {
		return errors.New("unexpected initial child population")
	}
	if _, err = laba115Docker(ctx, "exec", id, "/bin/sh", "-c", "sleep 30 >/dev/null 2>&1 & exit 0"); err != nil {
		return err
	}
	if laba115OnlyInit(ctx, id, p.PID) {
		return errors.New("synthetic leftover child was not detected")
	}
	if err = laba115Stop(ctx, p); err != nil {
		return err
	}
	return local115JSON(filepath.Join(dir, "receipt.json"), map[string]any{"actual_native_capture_and_stop": true, "actual_kernel_bounds": true, "synthetic_leftover_child_detected": true, "whole_cgroup_empty": true, "container_removed": true, "native_calls": 0, "real_auth": 0})
}

func laba115DatabaseIdentity() error {
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	raw, err := laba115Docker(ctx, "inspect", "--format", "{{.Id}}", laba115NativePG)
	id := strings.TrimSpace(string(raw))
	if err != nil || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(id) {
		return errors.New("fixed private database identity unavailable")
	}
	c, err := laba115Inspect(ctx, id)
	if err != nil || !laba115PGConfig(c) {
		return errors.New("actual isolated database boundary differs")
	}
	return local115JSON(filepath.Join(laba115HostRoot, "database-identity-v1.json"), map[string]any{"fixed_image_storage_network_and_loopback": true, "native_calls": 0, "real_auth": 0, "native_admission": false})
}

func laba115NativePins() map[string]string {
	return map[string]string{"prepared-abc-v5/plan/preseal.json": laba115NativePlanSHA, "history-abc-v5/recipe.json": laba115NativeHistorySHA, "prepared-abc-v5/app.test": laba115NativeAppSHA, "prepared-abc-v5/tool-broker": laba115NativeBrokerSHA, "prepared-abc-v5/broker-control.test": laba115NativeBrokerControlSHA, "prepared-abc-v5/extension.test": laba115NativeExtensionSHA, "prepared-abc-v5/http.test": laba115NativeHTTPSHA}
}
func laba115NativeBound() bool {
	return laba115PinsBound(laba115NativeSource, laba115NativePins())
}

func laba115PinsBound(source string, pins map[string]string) bool {
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(source) || len(pins) != 7 {
		return false
	}
	for _, path := range []string{"prepared-abc-v5/plan/preseal.json", "history-abc-v5/recipe.json", "prepared-abc-v5/app.test", "prepared-abc-v5/tool-broker", "prepared-abc-v5/broker-control.test", "prepared-abc-v5/extension.test", "prepared-abc-v5/http.test"} {
		sha := pins[path]
		if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(sha) {
			return false
		}
	}
	return true
}

type laba115Running struct {
	ID     string
	PID    int
	Start  string
	Cgroup string
}

func laba115Capture(ctx context.Context, id, network string) (laba115Running, error) {
	var out laba115Running
	c, err := laba115Inspect(ctx, id)
	normalized := c
	normalized.HostConfig.NetworkMode = "none"
	if err != nil || !c.State.Running || c.State.Pid < 1 || c.HostConfig.NetworkMode != network || !laba115ResourceConfig(normalized) {
		return out, errors.New("fixed native container boundary differs")
	}
	start, err := laba115ProcIdentity(c.State.Pid)
	if err != nil {
		return out, err
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", c.State.Pid))
	cg := strings.TrimSpace(strings.TrimPrefix(string(raw), "0::"))
	if err != nil || !strings.HasPrefix(string(raw), "0::/") || !strings.Contains(cg, id) || strings.Contains(cg, "..") || strings.Contains(cg, "\n") {
		return out, errors.New("own native kernel cgroup unavailable")
	}
	root, err := os.OpenRoot("/sys/fs/cgroup")
	if err != nil {
		return out, err
	}
	defer root.Close()
	for file, want := range map[string]string{"cpu.max": "100000 100000", "memory.max": "2147483648", "memory.swap.max": "0", "pids.max": "256"} {
		actual, e := root.ReadFile(filepath.Join(strings.TrimPrefix(cg, "/"), file))
		if e != nil || strings.TrimSpace(string(actual)) != want {
			return out, errors.New("native actual kernel bound differs")
		}
	}
	raw, err = os.ReadFile(fmt.Sprintf("/proc/%d/status", c.State.Pid))
	if err != nil || !regexp.MustCompile(`(?m)^Uid:\s+1000\s+1000\s+1000\s+1000$`).Match(raw) || !regexp.MustCompile(`(?m)^CapEff:\s+0000000000000000$`).Match(raw) || !regexp.MustCompile(`(?m)^NoNewPrivs:\s+1$`).Match(raw) || !regexp.MustCompile(`(?m)^Seccomp:\s+2$`).Match(raw) {
		return out, errors.New("actual native security boundary differs")
	}
	return laba115Running{ID: id, PID: c.State.Pid, Start: start, Cgroup: cg}, nil
}

func laba115Stop(ctx context.Context, p laba115Running) error {
	if _, err := laba115Docker(ctx, "stop", "--timeout", "3", p.ID); err != nil {
		return err
	}
	stopped, err := laba115Inspect(ctx, p.ID)
	if err != nil || stopped.State.Running || stopped.State.Pid != 0 {
		return errors.New("native PID namespace did not stop")
	}
	if after, e := laba115ProcIdentity(p.PID); e == nil && after == p.Start {
		return errors.New("native init process remains")
	}
	root, err := os.OpenRoot("/sys/fs/cgroup")
	if err != nil {
		return err
	}
	defer root.Close()
	raw, e := root.ReadFile(filepath.Join(strings.TrimPrefix(p.Cgroup, "/"), "cgroup.events"))
	if e != nil && !os.IsNotExist(e) || e == nil && !strings.Contains("\n"+string(raw), "\npopulated 0\n") {
		return errors.New("native whole cgroup not empty")
	}
	_, err = laba115Docker(ctx, "rm", p.ID)
	return err
}

type laba115CommandResult struct {
	Exit         int
	Started      bool
	WaitReturned bool
}

func laba115ExecLog(ctx context.Context, id, path string, envfile string, args ...string) (laba115CommandResult, error) {
	result := laba115CommandResult{Exit: -1}
	fixed := []string{"--host=unix:///var/run/docker.sock", "--config=" + filepath.Join(laba115HostRoot, "probe-v1/docker-config"), "exec"}
	if envfile != "" {
		fixed = append(fixed, "--env-file", envfile)
	}
	fixed = append(fixed, "--workdir", "/source/apps/orchestrator/internal/app", id)
	fixed = append(fixed, args...)
	cmd := exec.CommandContext(ctx, "/usr/bin/docker", fixed...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=/home/laba"}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return result, errors.New("new private server runner log required")
	}
	cmd.Stdout, cmd.Stderr = f, f
	err = cmd.Run()
	result.Started, result.WaitReturned = cmd.Process != nil, cmd.ProcessState != nil
	syncErr := f.Sync()
	closeErr := f.Close()
	if cmd.ProcessState != nil {
		result.Exit = cmd.ProcessState.ExitCode()
	}
	if syncErr != nil || closeErr != nil {
		return result, errors.New("server runner log persistence failed")
	}
	return result, err
}

func laba115ChildEnvironment() []string {
	return []string{"PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin", "GOMAXPROCS=1", "HTTPS_PROXY=http://10.249.116.1:18081", "TMPDIR=/execution/tmp", "BUTLER_BROKER_E2E_BINARY=/prepared/tool-broker", "BUTLER_GUARDED_REQUEST_BROKER_BINARY=/prepared/broker-control.test", "BUTLER_EXTENSION_E2E_BINARY=/prepared/extension.test", "BUTLER_EXPERIENCE_HTTP_BINARY=/prepared/http.test", "BUTLER_EXPERIENCE_NATIVE_HISTORY=/history/recipe.json", "BUTLER_EXPERIENCE_NATIVE_HISTORY_SHA256=" + laba115NativeHistorySHA, "BUTLER_EXPERIENCE_NATIVE_PLAN=/plan/preseal.json", "BUTLER_EXPERIENCE_NATIVE_PLAN_SHA256=" + laba115NativePlanSHA}
}

func laba115Execute(ctx context.Context, native bool) error {
	if !laba115NativeBound() {
		return errors.New("server native profile is not frozen and bound")
	}
	for path, want := range laba115NativePins() {
		got, err := local115Hash(filepath.Join(laba115HostRoot, path))
		if err != nil || got != want {
			return errors.New("server frozen artifact bytes changed")
		}
	}
	raw, err := os.ReadFile(filepath.Join(laba115HostRoot, "prepared-abc-v5/plan/preseal.json"))
	if err != nil {
		return errors.New("server preseal unavailable")
	}
	var seal struct {
		Deadline       time.Time `json:"deadline"`
		ExecutionSlots int       `json:"execution_slots"`
	}
	if json.Unmarshal(raw, &seal) != nil || seal.ExecutionSlots != 33 || !time.Now().Before(seal.Deadline) {
		return errors.New("server original day or33-stage plan invalid")
	}
	caller := ctx
	ctx, setupDone := context.WithDeadline(ctx, seal.Deadline)
	defer setupDone()
	label := "prepare-abc-v5"
	if native {
		label = "native-abc-v5"
	}
	operator := filepath.Join(laba115HostRoot, "operator-"+label)
	if os.Mkdir(operator, 0700) != nil {
		return errors.New("new one-use server operator identity required")
	}
	execution := filepath.Join(operator, "execution")
	if os.Mkdir(execution, 0700) != nil || os.Mkdir(filepath.Join(execution, "tmp"), 0700) != nil {
		return errors.New("private owning execution output unavailable")
	}
	pgRaw, err := laba115Docker(ctx, "inspect", "--format", "{{.Id}}", laba115NativePG)
	pgID := strings.TrimSpace(string(pgRaw))
	if err != nil || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(pgID) {
		return errors.New("fixed isolated native PostgreSQL unavailable")
	}
	pgContainer, err := laba115Inspect(ctx, pgID)
	if err != nil || !laba115PGConfig(pgContainer) {
		return errors.New("actual private server database image, storage or endpoint differs")
	}
	network := "container:" + pgID
	args := []string{"create", "--name", "issue115-" + label + "-20261028-v1", "--network", network, "--cpus", "1", "--memory", "2g", "--memory-swap", "2g", "--pids-limit", "256", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--log-driver", "none", "--user", "1000:1000", "--tmpfs", "/tmp:rw,nosuid,nodev,size=64m"}
	for _, mount := range []string{"type=bind,src=" + laba115HostRoot + "/source-abc-v5,dst=/source,readonly", "type=bind,src=" + laba115HostRoot + "/prepared-abc-v5,dst=/prepared,readonly", "type=bind,src=" + laba115HostRoot + "/history-abc-v5,dst=/history,readonly", "type=bind,src=" + laba115HostRoot + "/prepared-abc-v5/plan,dst=/plan,readonly", "type=bind,src=" + execution + ",dst=/execution"} {
		args = append(args, "--mount", mount)
	}
	for _, entry := range laba115ChildEnvironment() {
		args = append(args, "--env", entry)
	}
	args = append(args, laba115Image, "/bin/sleep", "86400")
	raw, err = laba115Docker(ctx, args...)
	id := strings.TrimSpace(string(raw))
	if err != nil || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(id) {
		return errors.New("fixed server execution container unavailable")
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_, _ = laba115Docker(cleanup, "rm", "-f", id)
	}()
	if _, err = laba115Docker(ctx, "start", id); err != nil {
		return err
	}
	process, err := laba115Capture(ctx, id, network)
	if err != nil {
		return err
	}
	check, stopCheck := context.WithTimeout(ctx, 3*time.Minute)
	_, err = laba115ExecLog(check, id, filepath.Join(operator, "pre-auth-pins-check.log"), "", "/usr/bin/env", "BUTLER_EXPERIENCE_NATIVE_CHECK=1", "/prepared/app.test", "-test.v", "-test.run=^TestNativeExperienceCheckPreparedWholeComparison$", "-test.timeout=3m")
	stopCheck()
	if err != nil {
		return errors.New("server zero-native preseal check failed before auth")
	}
	conn, err := pgx.Connect(ctx, laba115ControlDSN)
	if err != nil {
		return errors.New("private server auth source unavailable")
	}
	defer conn.Close(context.Background())
	var locked bool
	if conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended('issue115-laba-auth-owner-v1',0))`).Scan(&locked) != nil || !locked {
		return errors.New("private server auth owner is active")
	}
	if err = local115ControlIdle(ctx, conn); err != nil {
		return err
	}
	if !native {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if err = laba115Stop(cleanup, process); err != nil {
			return err
		}
		return local115JSON(filepath.Join(operator, "prepared.json"), map[string]any{"result": "SERVER_PRESEAL_KERNEL_AND_REAL_SOURCE_FENCES_PASS", "auth_access": 0, "native_calls": 0, "plan_sha256": laba115NativePlanSHA, "whole_cgroup_empty": true})
	}
	return laba115RunAndRecover(caller, conn, id, process, operator, execution, seal.Deadline)
}

func laba115RunAndRecover(ctx context.Context, source *pgx.Conn, id string, process laba115Running, operator, execution string, deadline time.Time) error {
	caller := ctx
	ctx, setupDone := context.WithDeadline(ctx, deadline)
	defer setupDone()
	admin, err := pgx.Connect(ctx, "postgres://postgres@127.0.0.1:55455/postgres?sslmode=disable")
	if err != nil {
		return errors.New("server native admin unavailable")
	}
	var exists bool
	err = admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname='memory115_laba_abc_v5_test')`).Scan(&exists)
	if err == nil && !exists {
		_, err = admin.Exec(ctx, `CREATE DATABASE memory115_laba_abc_v5_test`)
	}
	admin.Close(ctx)
	if err != nil {
		return errors.New("server native database unavailable")
	}
	target, err := pgxpool.New(ctx, laba115TargetHostDSN)
	if err != nil {
		return errors.New("server native target unavailable")
	}
	var consumed bool
	err = target.QueryRow(ctx, `SELECT to_regclass('public.issue90_native_windows') IS NOT NULL OR EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='experience_native_v1')`).Scan(&consumed)
	target.Close()
	if err != nil || consumed {
		return errors.New("server native domain consumed; never replay")
	}
	var expected string
	if source.QueryRow(ctx, `SELECT value FROM public.system_settings WHERE key=$1 AND is_secret`, local115AuthKey).Scan(&expected) != nil || expected == "" {
		return errors.New("private opaque server source auth unavailable")
	}
	if err = local115Exclusive(filepath.Join(execution, "auth-import.value"), []byte(expected)); err != nil {
		return err
	}
	key, err := local115ReadPrivate(filepath.Join(laba115HostRoot, "auth-source/settings.key"))
	if err != nil {
		return err
	}
	defer clear(key)
	keyText := strings.TrimSpace(string(key))
	if len(keyText) < 32 || strings.ContainsAny(keyText, "\r\n\x00") {
		return errors.New("private server encryption reference invalid")
	}
	if err = local115ValidateCipher(ctx, keyText, expected); err != nil {
		return err
	}
	env := []string{"BUTLER_TEST_MEMORY_POSTGRES_URL=" + laba115TargetChildDSN, "BUTLER_EXPERIENCE_NATIVE_EXECUTE=1", "BUTLER_EXPERIENCE_NATIVE_APPROVED_SHA256=" + laba115NativePlanSHA, "BUTLER_EXPERIENCE_NATIVE_OUTPUT=/execution/actual", "BUTLER_EXPERIENCE_PROCESS_PROOF=/execution/actual/processes", "BUTLER_EXPERIENCE_NATIVE_AUTH_VALUE_FILE=/execution/auth-import.value", "BUTLER_EXPERIENCE_NATIVE_AUTH_RETURN_FILE=/execution/auth-return.value", config.SettingsEncryptionKeyEnv + "=" + keyText}
	envPath := filepath.Join(operator, "native.env") // Host-only; never mounted in the model/tool container.
	if err = local115Exclusive(envPath, []byte(strings.Join(env, "\n")+"\n")); err != nil {
		return err
	}
	defer os.Remove(envPath)
	if ctx.Err() != nil || !time.Now().Before(deadline) {
		return errors.New("server day expired before native entry")
	}
	if err = local115JSON(filepath.Join(operator, "execution-consumed.json"), map[string]any{"plan_sha256": laba115NativePlanSHA, "source": laba115NativeSource, "original_deadline": deadline, "native_domain": "memory115_laba_abc_v5_test/experience_native_v1"}); err != nil {
		return err
	}
	native, stop := context.WithDeadline(caller, deadline.Add(30*time.Second))
	command, nativeErr := laba115ExecLog(native, id, filepath.Join(operator, "runner.log"), envPath, "/prepared/app.test", "-test.v", "-test.run=^TestNativeExperienceWholeComparison$", "-test.timeout=2310m")
	stop()
	cleanup, done := context.WithTimeout(context.Background(), 45*time.Second)
	defer done()
	forced := !laba115OnlyInit(cleanup, id, process.PID)
	if err = laba115Stop(cleanup, process); err != nil {
		return errors.New("whole native namespace exit unproven; no auth handback")
	}
	if err = local115JSON(filepath.Join(operator, "process-exit.json"), map[string]any{"native_exit_status": command.Exit, "docker_exec_started": command.Started, "docker_exec_wait_returned": command.WaitReturned, "forced_cleanup": forced, "whole_cgroup_empty": true, "pid_namespace_gone": true, "container_removed": true}); err != nil {
		return err
	}
	recovery, err := pgxpool.New(cleanup, laba115TargetHostDSN)
	if err != nil {
		return errors.New("server native recovery unavailable")
	}
	defer recovery.Close()
	var sourcePID uint32 = source.PgConn().PID()
	for {
		var quiet bool
		if recovery.QueryRow(cleanup, `SELECT NOT EXISTS(SELECT 1 FROM pg_stat_activity WHERE backend_type='client backend' AND pid<>pg_backend_pid() AND pid<>$1)`, sourcePID).Scan(&quiet) != nil {
			return errors.New("private server backend exit unavailable")
		}
		if quiet {
			break
		}
		select {
		case <-cleanup.Done():
			return errors.New("private native backends remain; no handback")
		case <-time.After(25 * time.Millisecond):
		}
	}
	_, receiptErr := os.Stat(filepath.Join(execution, "actual/provider-auth-imported.json"))
	if receiptErr != nil && !os.IsNotExist(receiptErr) {
		return errors.New("provider import receipt recovery uncertain")
	}
	latest, origin, err := local115Recover(cleanup, recovery, laba115NativePlanSHA, expected, receiptErr == nil)
	if err != nil {
		return err
	}
	if err = local115ValidateCipher(cleanup, keyText, latest); err != nil {
		return err
	}
	if err = local115Exclusive(filepath.Join(operator, "auth-recovered.value"), []byte(latest)); err != nil {
		return err
	}
	if err = local115Return(cleanup, source, expected, latest); err != nil {
		return err
	}
	store, err := pgxpool.New(cleanup, laba115ControlDSN)
	if err != nil {
		return errors.New("server returned auth verification unavailable")
	}
	defer store.Close()
	manager := providerauth.NewManager(config.NewPostgresSettingsStore(store, config.WithSettingsCodec(config.StaticSecretValueCodec{Key: keyText})))
	if _, err = manager.ResolveAntigravityCloudCode(cleanup); err != nil {
		return errors.New("server returned auth discovery failed")
	}
	if err = local115JSON(filepath.Join(operator, "server-handback.json"), map[string]any{"native_exit_status": command.Exit, "forced_cleanup": forced, "whole_cgroup_empty": true, "all_private_credential_backends_joined": true, "opaque_source_cas": true, "recovery_source": origin, "auth_resolved": true, "release_qualified": false, "plan_sha256": laba115NativePlanSHA}); err != nil {
		return err
	}
	if nativeErr != nil || forced {
		return errors.New("server native comparison failed; evidence and refreshed auth retained")
	}
	return nil
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "database-check" {
		if err := laba115DatabaseIdentity(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("PASS actual isolated database identity; auth=0 native=0")
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "probe" {
		laba115HostCLI()
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "containment" {
		if err := laba115Containment(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("PASS actual native capture/stop and leaked-child containment; auth=0 native=0")
		return
	}
	if len(os.Args) != 2 || (os.Args[1] != "prepare" && os.Args[1] != "native") {
		fmt.Fprintln(os.Stderr, "fixed mode probe, prepare or native required")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	if err := laba115Execute(ctx, os.Args[1] == "native"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("PASS fixed server operation; inspect private receipt for actual scope")
}
