//go:build ignore

// One-use operator wrapper for the already frozen local #115/#90 collector.
// Runs on this PC's WSL host, never on laba or against production storage.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const local115HostRoot = "/mnt/host/wsl/butler-effort76"
const local115Source = "/private/memory115-source-2a26ceaf"
const local115Plan = "/private/memory115-prepared-20261026-v2/plan/preseal.json"
const local115PlanSHA = "dad3bd40c6cf6d469593b0afb8c98be4c0962e4e32e9ace6ab9020d91c203099"
const local115History = "/private115/local-successor-20261026-v1.json"
const local115HistorySHA = "579aa8c25bcfdba06b1be91f2cb613f2550ee685932c3919e2468bef13f11ec7"
const local115AuthKey = "BUTLER_PROVIDER_AUTH_ANTIGRAVITY"
const local115ControlDSN = "postgres://postgres@127.0.0.1:55451/behavior76_test?sslmode=disable"
const local115TargetDSN = "postgres://postgres@127.0.0.1:55451/memory115_native_test?sslmode=disable"

func local115LegacyArtifactPins() map[string]struct{ path, sha string } {
	return map[string]struct{ path, sha string }{
		"app":            {"/private/memory115-prepared-20261026-v2/bin/app.test", "88f674b66e6171e2457998133643ab4bcdadaa6e78acc370af69508ffb6c8fa0"},
		"broker":         {"/private/memory115-prepared-20261026-v1/bin/tool-broker", "f49e585b57187791e77cb7c39e4b9cab239f5d5a3c751f8d348e88329e180c3d"},
		"broker_control": {"/private/memory115-prepared-20261026-v1/bin/broker-control.test", "1791ba611b4ea9ac45837bac710f1a9a7cbe27ae7d9eed444bf3a0c1e2721b98"},
		"extension":      {"/private/memory115-prepared-20261026-v1/bin/extension.test", "2a409ae52762d3468e5d0693f80b770f455b284a11156e7ada22e9f0b586d106"},
		"http":           {"/private/memory115-prepared-20261026-v1/bin/http.test", "ba5d56a18162da93bcd57c228dcca0f421f479e61fc95aa24c40c71547217ee4"},
	}
}

// Only these compiled operator profiles exist. No CLI path, URL, executable,
// hash or environment override can construct a new profile. The prospective
// skill profile stays unusable until its separately frozen artifacts are bound.
const local115SkillSourceCommit = ""
const local115SkillPlanSHA = ""
const local115SkillHistorySHA = ""
const local115SkillAppSHA = ""
const local115SkillBrokerSHA = ""
const local115SkillBrokerControlSHA = ""
const local115SkillExtensionSHA = ""
const local115SkillHTTPSHA = ""

type local115OperatorProfile struct {
	name, source, sourceCommit, plan, planSHA, history, historySHA string
	database, targetDSN                                            string
	artifacts                                                      map[string]struct{ path, sha string }
}

func local115OperatorProfileByName(name string) (local115OperatorProfile, error) {
	profile := local115OperatorProfile{artifacts: local115LegacyArtifactPins()}
	switch name {
	case "legacy-20261026":
		profile.name, profile.source, profile.sourceCommit = name, local115Source, "2a26ceaf6427eb6727167909a7560c731aff8f9a"
		profile.plan, profile.planSHA, profile.history, profile.historySHA = local115Plan, local115PlanSHA, local115History, local115HistorySHA
		profile.database, profile.targetDSN = "memory115_native_test", local115TargetDSN
	case "skill-preparation-v1":
		profile.name, profile.source, profile.sourceCommit = name, "/private/memory115-source-skillprep-v1", local115SkillSourceCommit
		profile.plan, profile.planSHA = "/private/memory115-prepared-skillprep-v1/plan/preseal.json", local115SkillPlanSHA
		profile.history, profile.historySHA = "/src115/.cache/memory115-skill-successor-recipe-v1/recipe.json", local115SkillHistorySHA
		profile.database, profile.targetDSN = "memory115_skillprep_v1_test", "postgres://postgres@127.0.0.1:55451/memory115_skillprep_v1_test?sslmode=disable"
		profile.artifacts = map[string]struct{ path, sha string }{
			"app":            {"/private/memory115-prepared-skillprep-v1/bin/app.test", local115SkillAppSHA},
			"broker":         {"/private/memory115-prepared-skillprep-v1/bin/tool-broker", local115SkillBrokerSHA},
			"broker_control": {"/private/memory115-prepared-skillprep-v1/bin/broker-control.test", local115SkillBrokerControlSHA},
			"extension":      {"/private/memory115-prepared-skillprep-v1/bin/extension.test", local115SkillExtensionSHA},
			"http":           {"/private/memory115-prepared-skillprep-v1/bin/http.test", local115SkillHTTPSHA},
		}
	default:
		return local115OperatorProfile{}, local115Fail("unknown fixed local operator profile")
	}
	return profile, nil
}

func (profile local115OperatorProfile) bound() bool {
	expected, err := local115OperatorProfileByName(profile.name)
	if err != nil || profile.source != expected.source || profile.sourceCommit != expected.sourceCommit || profile.plan != expected.plan || profile.planSHA != expected.planSHA || profile.history != expected.history || profile.historySHA != expected.historySHA || profile.database != expected.database || profile.targetDSN != expected.targetDSN {
		return false
	}
	sha := regexp.MustCompile(`^[0-9a-f]{64}$`)
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(profile.sourceCommit) || !sha.MatchString(profile.planSHA) || !sha.MatchString(profile.historySHA) || len(profile.artifacts) != 5 {
		return false
	}
	for _, role := range []string{"app", "broker", "broker_control", "extension", "http"} {
		pin := profile.artifacts[role]
		if pin != expected.artifacts[role] || !strings.HasPrefix(pin.path, "/private/") || !sha.MatchString(pin.sha) {
			return false
		}
	}
	return true
}

func local115Fail(code string) error      { return errors.New(code) }
func local115Physical(path string) string { return local115HostRoot + path }

func local115ReadPrivate(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, local115Fail("private regular reference unavailable")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() < 1 || info.Size() > 1024*1024 {
		return nil, local115Fail("private bounded regular reference unavailable")
	}
	raw, err := io.ReadAll(io.LimitReader(f, 1024*1024+1))
	if err != nil || len(raw) != int(info.Size()) {
		return nil, local115Fail("private reference read failed")
	}
	return raw, nil
}

func local115Exclusive(path string, raw []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return local115Fail("one-use private output unavailable")
	}
	_, writeErr := f.Write(raw)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return local115Fail("private output persistence failed")
	}
	return nil
}

func local115JSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return local115Fail("safe receipt encoding failed")
	}
	return local115Exclusive(path, append(raw, '\n'))
}

// This sink validates the existing strict private-import format without
// persisting or activating credentials. PostgreSQL remains authoritative.
type local115ValidationSink struct{}

func (local115ValidationSink) Get(context.Context, string) (config.Setting, error) {
	return config.Setting{}, config.ErrSettingNotFound
}
func (local115ValidationSink) Set(context.Context, config.Setting) (config.Setting, error) {
	return config.Setting{}, nil
}
func (local115ValidationSink) Delete(context.Context, string) error {
	return config.ErrSettingNotFound
}

func local115ValidateCipher(ctx context.Context, key, encrypted string) error {
	if len(encrypted) == 0 || len(encrypted) > 128*1024 {
		return local115Fail("bounded encrypted local auth unavailable")
	}
	plain, err := (config.StaticSecretValueCodec{Key: key}).Decrypt(encrypted, local115AuthKey)
	if err != nil {
		return local115Fail("local auth encryption binding invalid")
	}
	if providerauth.NewManager(local115ValidationSink{}).ImportAntigravity(ctx, strings.NewReader(plain), "issue115-private-format-validation") != nil {
		return local115Fail("local auth strict private format invalid")
	}
	return nil
}

func local115Hash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", local115Fail("pinned artifact unavailable")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 512*1024*1024 {
		return "", local115Fail("pinned artifact shape invalid")
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, 512*1024*1024+1))
	if err != nil || n != info.Size() {
		return "", local115Fail("pinned artifact hash failed")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func local115Environment() []string {
	profile, _ := local115OperatorProfileByName("legacy-20261026")
	return profile.environment()
}

func (profile local115OperatorProfile) environment() []string {
	return []string{"PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin", "GOMAXPROCS=1", "BUTLER_BROKER_E2E_BINARY=" + profile.artifacts["broker"].path, "BUTLER_GUARDED_REQUEST_BROKER_BINARY=" + profile.artifacts["broker_control"].path, "BUTLER_EXTENSION_E2E_BINARY=" + profile.artifacts["extension"].path, "BUTLER_EXPERIENCE_HTTP_BINARY=" + profile.artifacts["http"].path, "BUTLER_EXPERIENCE_NATIVE_HISTORY=" + profile.history, "BUTLER_EXPERIENCE_NATIVE_HISTORY_SHA256=" + profile.historySHA, "BUTLER_EXPERIENCE_NATIVE_PLAN=" + profile.plan, "BUTLER_EXPERIENCE_NATIVE_PLAN_SHA256=" + profile.planSHA}
}

func local115Command(ctx context.Context, env []string, selected, timeout string) *exec.Cmd {
	profile, _ := local115OperatorProfileByName("legacy-20261026")
	return profile.command(ctx, env, selected, timeout)
}

func (profile local115OperatorProfile) command(ctx context.Context, env []string, selected, timeout string) *exec.Cmd {
	args := []string{local115HostRoot, "/bin/sh", "-c", `cd ` + profile.source + `/apps/orchestrator/internal/app && exec ` + profile.artifacts["app"].path + ` -test.v -test.run '` + selected + `' -test.timeout ` + timeout}
	cmd := exec.CommandContext(ctx, "/usr/sbin/chroot", args...)
	// Secret values stay exclusively in the child environment, never argv.
	// Inheritance is replaced by this finite trusted allowlist.
	cmd.Env = env
	return cmd
}

func local115RunLog(cmd *exec.Cmd, path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return local115Fail("private runner log unavailable")
	}
	cmd.Stdout, cmd.Stderr = f, f
	runErr := cmd.Run()
	syncErr := f.Sync()
	closeErr := f.Close()
	if syncErr != nil || closeErr != nil {
		return local115Fail("private runner log persistence failed")
	}
	return runErr
}

// Serializes these local wrappers. The existing76 native collector is also
// excluded by the actual source-window check; this operator never runs them
// concurrently. Retained old running Inputs with expired claims are untouched.
type local115ControlQueries interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func local115ControlIdle(ctx context.Context, conn local115ControlQueries) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var idle bool
	err := conn.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM public.behavior76_windows WHERE NOT closed) AND NOT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND backend_type='client backend' AND pid<>pg_backend_pid())`).Scan(&idle)
	if err != nil || !idle {
		return local115Fail("local source window or credential backend is active or unavailable")
	}
	// The real source retains each fixture's task tables in its own schema;
	// only the provider journal/settings live in public. Inspect catalog names,
	// never model data, and require complete lease/Input pairs in every schema.
	var raw []byte
	if conn.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(jsonb_build_object('schema',nspname,'table',relname) ORDER BY nspname,relname),'[]'::jsonb) FROM (SELECT n.nspname,c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind='r' AND c.relname IN('session_ownership','accepted_inputs') AND n.nspname NOT LIKE 'pg_%' AND n.nspname<>'information_schema' ORDER BY n.nspname,c.relname LIMIT 513) source_tables`).Scan(&raw) != nil || len(raw) > 256*1024 {
		return local115Fail("local source task-store catalog unavailable")
	}
	var tables []struct {
		Schema string `json:"schema"`
		Table  string `json:"table"`
	}
	if json.Unmarshal(raw, &tables) != nil || len(tables) == 0 || len(tables) > 512 {
		return local115Fail("bounded complete source task stores required")
	}
	pairs := make(map[string]int)
	for _, table := range tables {
		column, bit := "", 0
		switch table.Table {
		case "session_ownership":
			column, bit = "expires_at", 1
		case "accepted_inputs":
			column, bit = "claim_expires_at", 2
		default:
			return local115Fail("unknown source task-store relation")
		}
		if table.Schema == "" || len(table.Schema) > 63 || pairs[table.Schema]&bit != 0 {
			return local115Fail("source task-store identity differs")
		}
		pairs[table.Schema] |= bit
		query := "SELECT NOT EXISTS(SELECT 1 FROM " + pgx.Identifier{table.Schema, table.Table}.Sanitize() + " WHERE " + pgx.Identifier{column}.Sanitize() + ">clock_timestamp())"
		if conn.QueryRow(ctx, query).Scan(&idle) != nil || !idle {
			return local115Fail("source task-store live lease or claim remains, or observation unavailable")
		}
	}
	for _, pair := range pairs {
		if pair != 3 {
			return local115Fail("source task-store lease/Input pair incomplete")
		}
	}
	return nil
}

func local115Return(ctx context.Context, conn local115ControlQueries, expected, latest string) error {
	if err := local115ControlIdle(ctx, conn); err != nil {
		return err
	}
	tag, err := conn.Exec(ctx, `UPDATE public.system_settings SET value=$1,updated_by='issue115-local-serialized-handback',updated_at=clock_timestamp() WHERE key=$2 AND is_secret AND value=$3`, latest, local115AuthKey, expected)
	if err != nil || tag.RowsAffected() != 1 {
		return local115Fail("local encrypted auth CAS denied; private material retained")
	}
	return nil
}

func local115WaitDatabaseExit(ctx context.Context, target interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) error {
	for {
		var quiet bool
		if target.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND backend_type='client backend' AND pid<>pg_backend_pid())`).Scan(&quiet) != nil {
			return local115Fail("native database backend exit unavailable; auth handback prohibited")
		}
		if quiet {
			return nil
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return local115Fail("native database backends remain; auth handback prohibited")
		case <-timer.C:
		}
	}
}

func local115Recover(ctx context.Context, target interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, planSHA, expected string, importedReceipt bool) (string, string, error) {
	var schema, table bool
	if target.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='experience_native_v1'),to_regclass('public.issue90_native_windows') IS NOT NULL`).Scan(&schema, &table) != nil {
		return "", "", local115Fail("native domain recovery unavailable")
	}
	if table {
		if !schema {
			return "", "", local115Fail("native claim exists without its owning schema")
		}
		var valid bool
		if target.QueryRow(ctx, `SELECT count(*)=1 AND bool_and(window_id='experience-explicit-v1' AND window_hash=$1 AND schema_name='experience_native_v1' AND owner_id='owner') FROM public.issue90_native_windows`, planSHA).Scan(&valid) != nil || !valid {
			return "", "", local115Fail("native recovery domain identity differs")
		}
	} else if schema {
		return "", "", local115Fail("native schema has no matching claim")
	}
	var settings bool
	if schema && target.QueryRow(ctx, `SELECT to_regclass('experience_native_v1.system_settings') IS NOT NULL`).Scan(&settings) != nil {
		return "", "", local115Fail("native auth table recovery unavailable")
	}
	if settings {
		var latest string
		err := target.QueryRow(ctx, `SELECT value FROM experience_native_v1.system_settings WHERE key=$1 AND is_secret`, local115AuthKey).Scan(&latest)
		if err == nil && latest != "" {
			return latest, "latest_durable_local_setting", nil
		}
		if err != pgx.ErrNoRows {
			return "", "", local115Fail("native auth recovery uncertain")
		}
	}
	if importedReceipt {
		return "", "", local115Fail("provider imported without recoverable encrypted setting")
	}
	// No authenticated provider can exist before the durable import receipt.
	// A matching empty domain may have been allocated before that point.
	if schema {
		var entries bool
		if target.QueryRow(ctx, `SELECT to_regclass('experience_native_v1.experience_native_entries') IS NOT NULL`).Scan(&entries) != nil {
			return "", "", local115Fail("native journal recovery unavailable")
		}
		if entries {
			var n int
			if target.QueryRow(ctx, `SELECT count(*) FROM experience_native_v1.experience_native_entries`).Scan(&n) != nil || n != 0 {
				return "", "", local115Fail("native entries exist without recoverable auth")
			}
		}
	}
	return expected, "unused_original_before_provider_import", nil
}

func local115Cgroup(path string) (*os.File, error) {
	if os.Mkdir(path, 0700) != nil {
		return nil, local115Fail("new private native cgroup unavailable")
	}
	for name, value := range map[string]string{"memory.max": "2147483648", "memory.swap.max": "0", "pids.max": "256", "cpu.max": "100000 100000"} {
		if os.WriteFile(filepath.Join(path, name), []byte(value), 0600) != nil {
			return nil, local115Fail("native cgroup bound unavailable")
		}
		actual, err := os.ReadFile(filepath.Join(path, name))
		if err != nil || strings.TrimSpace(string(actual)) != value {
			return nil, local115Fail("actual native cgroup bound differs")
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, local115Fail("native cgroup descriptor unavailable")
	}
	return f, nil
}

func local115Reaped(path string) bool {
	raw, err := os.ReadFile(filepath.Join(path, "cgroup.events"))
	return err == nil && strings.Contains("\n"+string(raw), "\npopulated 0\n")
}

func local115Execute(ctx context.Context, label string, execute bool) error {
	profile, err := local115OperatorProfileByName("legacy-20261026")
	if err != nil {
		return err
	}
	return profile.execute(ctx, label, execute)
}

func (profile local115OperatorProfile) execute(ctx context.Context, label string, execute bool) error {
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`).MatchString(label) {
		return local115Fail("bounded one-use local label required")
	}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		if os.Getenv(key) != "" {
			return local115Fail("local native profile forbids ambient proxy substitution")
		}
	}
	if !profile.bound() {
		return local115Fail("fixed local operator profile is not fully frozen and bound")
	}
	for path, want := range map[string]string{profile.plan: profile.planSHA, profile.history: profile.historySHA} {
		got, err := local115Hash(local115Physical(path))
		if err != nil || got != want {
			return local115Fail("frozen preseal/history bytes changed")
		}
	}
	for _, pin := range profile.artifacts {
		got, err := local115Hash(local115Physical(pin.path))
		if err != nil || got != pin.sha {
			return local115Fail("frozen native executable bytes changed")
		}
	}
	raw, err := os.ReadFile(local115Physical(profile.plan))
	if err != nil {
		return local115Fail("preseal unavailable")
	}
	var seal struct {
		Deadline       time.Time `json:"deadline"`
		ExecutionSlots int       `json:"execution_slots"`
	}
	if json.Unmarshal(raw, &seal) != nil || seal.ExecutionSlots != 33 || !time.Now().Before(seal.Deadline) {
		return local115Fail("original shortened native window expired or changed")
	}
	inside := "/private/memory115-execution-" + label
	root := local115Physical(inside)
	if os.Mkdir(root, 0700) != nil {
		return local115Fail("new private wrapper identity required")
	}
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	err = local115RunLog(profile.command(checkCtx, append(profile.environment(), "BUTLER_EXPERIENCE_NATIVE_CHECK=1"), "^TestNativeExperienceCheckPreparedWholeComparison$", "3m"), filepath.Join(root, "pre-auth-pins-check.log"))
	cancel()
	if err != nil {
		return local115Fail("original zero-native prepared check failed; no auth clone")
	}
	cgPath := "/sys/fs/cgroup/butler-memory115-" + label
	cg, err := local115Cgroup(cgPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = cg.Close()
		if local115Reaped(cgPath) {
			_ = os.Remove(cgPath) // Only this newly created, empty cgroup.
		}
	}()
	probe := exec.CommandContext(ctx, "/bin/cat", "/proc/self/cgroup")
	probe.Env = []string{"PATH=/usr/bin:/bin"}
	probe.SysProcAttr = &syscall.SysProcAttr{UseCgroupFD: true, CgroupFD: int(cg.Fd()), Setpgid: true}
	observed, probeErr := probe.Output()
	if probeErr != nil || strings.TrimSpace(string(observed)) != "0::/butler-memory115-"+label || !local115Reaped(cgPath) {
		return local115Fail("actual kernel resource containment not proven before auth")
	}
	// Private key/ciphertext access occurs only in explicit execute after the
	// original collector check and actual kernel resource containment proof.
	conn, err := pgx.Connect(ctx, local115ControlDSN)
	if err != nil {
		return local115Fail("local auth source unavailable")
	}
	defer conn.Close(context.Background())
	var locked bool
	if conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended('issue115-local-auth-owner-v1',0))`).Scan(&locked) != nil || !locked {
		return local115Fail("local provider auth owner already active")
	}
	if err = local115ControlIdle(ctx, conn); err != nil {
		return err
	}
	if !execute {
		return local115JSON(filepath.Join(root, "prepared.json"), map[string]any{"result": "ORIGINAL_PRESEAL_RESOURCES_AND_REAL_SOURCE_FENCES_RECHECKED_NO_AUTH_OR_NATIVE", "native_calls": 0, "auth_access": 0, "plan_sha256": profile.planSHA, "actual_cgroup_probe": true, "whole_cgroup_empty": true, "actual_source_window_and_namespace_fences": true, "source_other_credential_backends_idle": true, "cpu_max": "100000 100000", "memory_max": "2147483648", "swap_max": "0", "pids_max": "256"})
	}
	admin, err := pgx.Connect(ctx, "postgres://postgres@127.0.0.1:55451/postgres?sslmode=disable")
	if err != nil {
		return local115Fail("local native database admin unavailable")
	}
	var exists bool
	if admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, profile.database).Scan(&exists) != nil {
		admin.Close(ctx)
		return local115Fail("local native database identity unavailable")
	}
	if !exists {
		_, err = admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{profile.database}.Sanitize())
	}
	admin.Close(ctx)
	if err != nil {
		return local115Fail("local native database creation failed")
	}
	targetConfig, err := pgxpool.ParseConfig(profile.targetDSN)
	if err != nil {
		return local115Fail("local native target configuration unavailable")
	}
	targetConfig.MaxConns = 1
	target, err := pgxpool.NewWithConfig(ctx, targetConfig)
	if err != nil {
		return local115Fail("local native target unavailable")
	}
	defer target.Close()
	var consumed bool
	if target.QueryRow(ctx, `SELECT to_regclass('public.issue90_native_windows') IS NOT NULL OR EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='experience_native_v1')`).Scan(&consumed) != nil || consumed {
		return local115Fail("local native domain consumed or unavailable; never replay")
	}
	target.Close() // No idle operator target backend during native collection.
	var expected string
	if conn.QueryRow(ctx, `SELECT value FROM public.system_settings WHERE key=$1 AND is_secret`, local115AuthKey).Scan(&expected) != nil || expected == "" {
		return local115Fail("private encrypted source auth unavailable")
	}
	for _, name := range []string{"expected-source.value", "auth-import.value"} {
		if local115Exclusive(filepath.Join(root, name), []byte(expected)) != nil {
			return local115Fail("private opaque auth clone failed")
		}
	}
	key, err := local115ReadPrivate(local115Physical("/private/behavior76/settings.key"))
	if err != nil {
		return err
	}
	defer clear(key)
	if len(strings.TrimSpace(string(key))) < 32 {
		return local115Fail("private matching encryption reference invalid")
	}
	if err = local115ValidateCipher(ctx, strings.TrimSpace(string(key)), expected); err != nil {
		return err
	}
	if err = local115JSON(filepath.Join(root, "execution-consumed.json"), map[string]any{"plan_sha256": profile.planSHA, "source": profile.sourceCommit, "native_domain": profile.database + "/experience_native_v1", "original_deadline": seal.Deadline, "new_authority": false}); err != nil {
		return err
	}
	env := append(profile.environment(), "BUTLER_TEST_MEMORY_POSTGRES_URL="+profile.targetDSN, "BUTLER_EXPERIENCE_NATIVE_EXECUTE=1", "BUTLER_EXPERIENCE_NATIVE_APPROVED_SHA256="+profile.planSHA, "BUTLER_EXPERIENCE_NATIVE_OUTPUT="+inside+"/actual", "BUTLER_EXPERIENCE_PROCESS_PROOF="+inside+"/actual/processes", "BUTLER_EXPERIENCE_NATIVE_AUTH_VALUE_FILE="+inside+"/auth-import.value", "BUTLER_EXPERIENCE_NATIVE_AUTH_RETURN_FILE="+inside+"/auth-return.value", config.SettingsEncryptionKeyEnv+"="+strings.TrimSpace(string(key)))
	// Cleanup time grants no model/tool authority: the original collector still
	// clamps every operation to the sealed owner/day deadline.
	nativeCtx, stop := context.WithDeadline(ctx, seal.Deadline.Add(30*time.Second))
	cmd := profile.command(nativeCtx, env, "^TestNativeExperienceWholeComparison$", "2310m")
	cmd.SysProcAttr = &syscall.SysProcAttr{UseCgroupFD: true, CgroupFD: int(cg.Fd()), Setpgid: true}
	nativeErr := local115RunLog(cmd, filepath.Join(root, "runner.log"))
	stop()
	exit := -1
	if cmd.ProcessState != nil {
		exit = cmd.ProcessState.ExitCode()
	}
	forced := false
	if !local115Reaped(cgPath) {
		forced = true
		_ = os.WriteFile(filepath.Join(cgPath, "cgroup.kill"), []byte("1"), 0600)
		until := time.Now().Add(10 * time.Second)
		for !local115Reaped(cgPath) && time.Now().Before(until) {
			time.Sleep(25 * time.Millisecond)
		}
	}
	reaped := (cmd.Process == nil || cmd.ProcessState != nil) && local115Reaped(cgPath)
	if err = local115JSON(filepath.Join(root, "process-exit.json"), map[string]any{"native_exit_status": exit, "cmd_wait_returned": cmd.ProcessState != nil, "whole_cgroup_empty": reaped, "forced_cleanup": forced, "observed_at": time.Now().UTC()}); err != nil {
		return err
	}
	if !reaped {
		return local115Fail("native process group not empty; auth handback prohibited")
	}
	// Only now can the final durable encrypted refresh be recovered. No live
	// collector goroutine, credential-bearing child or process-local snapshot.
	cleanup, done := context.WithTimeout(context.Background(), 45*time.Second)
	defer done()
	recoveryTarget, err := pgxpool.NewWithConfig(cleanup, targetConfig)
	if err != nil {
		return local115Fail("local native recovery connection unavailable")
	}
	defer recoveryTarget.Close()
	quiet, stopQuiet := context.WithTimeout(cleanup, 10*time.Second)
	err = local115WaitDatabaseExit(quiet, recoveryTarget)
	stopQuiet()
	if err != nil {
		return err
	}
	_, receiptErr := os.Stat(filepath.Join(root, "actual/provider-auth-imported.json"))
	if receiptErr != nil && !os.IsNotExist(receiptErr) {
		return local115Fail("native auth import receipt unavailable for recovery")
	}
	latest, source, err := local115Recover(cleanup, recoveryTarget, profile.planSHA, expected, receiptErr == nil)
	if err != nil {
		return err
	}
	if err = local115ValidateCipher(cleanup, strings.TrimSpace(string(key)), latest); err != nil {
		return err
	}
	if err = local115Exclusive(filepath.Join(root, "auth-return.value"), []byte(latest)); err != nil {
		return err
	}
	if err = local115Return(cleanup, conn, expected, latest); err != nil {
		return err
	}
	storePool, err := pgxpool.New(cleanup, local115ControlDSN)
	if err != nil {
		return local115Fail("returned local auth verification unavailable")
	}
	defer storePool.Close()
	m := providerauth.NewManager(config.NewPostgresSettingsStore(storePool, config.WithSettingsCodec(config.StaticSecretValueCodec{Key: strings.TrimSpace(string(key))})))
	if _, err = m.ResolveAntigravityCloudCode(cleanup); err != nil {
		return local115Fail("returned local provider auth discovery failed")
	}
	var final string
	if conn.QueryRow(cleanup, `SELECT value FROM public.system_settings WHERE key=$1 AND is_secret`, local115AuthKey).Scan(&final) != nil || final == "" {
		return local115Fail("fresh local auth snapshot unavailable")
	}
	if err = local115Exclusive(filepath.Join(root, "source-after-native.value"), []byte(final)); err != nil {
		return err
	}
	if err = local115JSON(filepath.Join(root, "local-handback.json"), map[string]any{"version": "butler115-local-provider-handback.v1", "native_exit_status": exit, "collector_started": cmd.Process != nil, "cmd_wait_returned": cmd.ProcessState != nil, "whole_cgroup_empty": reaped, "forced_cleanup": forced, "native_database_backends_gone": true, "opaque_source_cas": true, "source_windows_closed_checked": true, "recovery_source": source, "local_auth_resolved": true, "fresh_local_encrypted_snapshot_saved": true, "lab_main_auth_claim": false, "raw_material_printed": false, "release_qualified": false, "plan_sha256": profile.planSHA}); err != nil {
		return err
	}
	if nativeErr != nil || forced {
		return local115Fail("native cohort failed; exact evidence retained and local auth returned")
	}
	return nil
}

func main() {
	label := flag.String("label", "", "one-use local label")
	execute := flag.Bool("execute", false, "run only the exact prepared native cohort after all gates")
	profileName := flag.String("profile", "legacy-20261026", "fixed compiled local operator profile")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	profile, err := local115OperatorProfileByName(*profileName)
	if err == nil {
		err = profile.execute(ctx, *label, *execute)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Println("Local wrapper stage completed; private receipts retained, no production qualification claimed.")
}
