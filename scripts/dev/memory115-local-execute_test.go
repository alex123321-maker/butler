//go:build ignore

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/butler/butler/internal/config"
)

func TestLocal115OperatorEnvironmentNeverPlacesKeyInArgv(t *testing.T) {
	const sentinel = "SYNTHETIC_TEST_KEY_NOT_A_CREDENTIAL"
	t.Setenv("UNRELATED_SYNTHETIC_PARENT_SECRET", sentinel)
	env := append(local115Environment(), "BUTLER_SETTINGS_ENCRYPTION_KEY="+sentinel)
	cmd := local115Command(context.Background(), env, "^TestNativeExperienceWholeComparison$", "2310m")
	if strings.Contains(strings.Join(cmd.Args, " "), sentinel) || strings.Contains(strings.Join(cmd.Args, " "), "BUTLER_SETTINGS_ENCRYPTION_KEY") {
		t.Fatal("key appeared in child argv")
	}
	if !strings.Contains(strings.Join(cmd.Env, "\n"), "BUTLER_SETTINGS_ENCRYPTION_KEY="+sentinel) || strings.Contains(strings.Join(cmd.Env, "\n"), "UNRELATED_SYNTHETIC_PARENT_SECRET") {
		t.Fatal("explicit child environment was lost or broadened")
	}
	for _, entry := range cmd.Env {
		if strings.Contains(strings.ToUpper(entry), "PROXY=") {
			t.Fatal("ambient route inherited")
		}
	}
}

func TestLocal115CipherMustMatchKeyAndStrictPrivateFormat(t *testing.T) {
	const key = "SYNTHETIC_OPERATOR_TEST_KEY_ONLY_123456789"
	const good = `{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","client_id":"1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com","client_secret":"synthetic-client","expires_at":"2026-10-26T00:00:00Z","project_id":"synthetic-project"}`
	for _, tc := range []struct {
		name, plain, encryptionKey, associated string
		want                                   bool
	}{
		{"valid", good, key, local115AuthKey, true},
		{"wrong_key", good, "different-synthetic-key", local115AuthKey, false},
		{"wrong_setting", good, key, "foreign-setting", false},
		{"arbitrary_JSON", `{}`, key, local115AuthKey, false},
		{"duplicate", strings.Replace(good, `"access_token":`, `"refresh_token":"duplicate","access_token":`, 1), key, local115AuthKey, false},
		{"unknown_field", strings.Replace(good, `"access_token":`, `"unrecognized":true,"access_token":`, 1), key, local115AuthKey, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encrypted, err := config.EncryptWithKey(tc.encryptionKey, tc.plain, tc.associated)
			if err != nil {
				t.Fatal("synthetic encryption failed")
			}
			err = local115ValidateCipher(t.Context(), key, encrypted)
			if (err == nil) != tc.want || (err != nil && strings.Contains(err.Error(), "synthetic-access")) {
				t.Fatal("private validation boundary failed", tc.name)
			}
		})
	}
	if local115ValidateCipher(t.Context(), key, strings.Repeat("x", 128*1024+1)) == nil {
		t.Fatal("unbounded private ciphertext accepted")
	}
}

func TestLocal115PrivateReferenceAndOneUseWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "synthetic.value")
	if err := local115Exclusive(path, []byte("synthetic encrypted test fixture")); err != nil {
		t.Fatal(err)
	}
	if err := local115Exclusive(path, []byte("replacement")); err == nil {
		t.Fatal("private one-use value overwritten")
	}
	value, err := local115ReadPrivate(path)
	if err != nil || string(value) != "synthetic encrypted test fixture" {
		t.Fatal("private exact bytes unavailable", err)
	}
	if os.Chmod(path, 0644) != nil {
		t.Fatal("synthetic chmod failed")
	}
	if _, err = local115ReadPrivate(path); err == nil {
		t.Fatal("weak permissions accepted")
	}
	if os.Chmod(path, 0600) != nil {
		t.Fatal("synthetic chmod failed")
	}
	link := filepath.Join(dir, "link")
	if os.Symlink(path, link) != nil {
		t.Fatal("synthetic symlink creation failed")
	}
	if _, err = local115ReadPrivate(link); err == nil {
		t.Fatal("symlink followed")
	}
	pipe := filepath.Join(dir, "pipe")
	if syscall.Mkfifo(pipe, 0600) != nil {
		t.Fatal("synthetic FIFO failed")
	}
	if _, err = local115ReadPrivate(pipe); err == nil {
		t.Fatal("nonregular private reference accepted")
	}
	large := filepath.Join(dir, "oversize")
	if os.WriteFile(large, make([]byte, 1024*1024+1), 0600) != nil {
		t.Fatal("synthetic oversized fixture failed")
	}
	if _, err = local115ReadPrivate(large); err == nil {
		t.Fatal("oversized private reference accepted")
	}
}

func TestLocal115NoSecretOrNativeBeforeStaticGuards(t *testing.T) {
	if err := local115Execute(context.Background(), "../foreign", true); err == nil {
		t.Fatal("path escape accepted")
	}
	t.Setenv("HTTPS_PROXY", "http://synthetic.invalid")
	if err := local115Execute(context.Background(), "public-rejected", true); err == nil || err.Error() != "local native profile forbids ambient proxy substitution" {
		t.Fatal("ambient proxy reached prepared/auth/native stage", err)
	}
}

func TestLocal115FixedProfilesKeepLegacyAndDenyUnboundOrSubstitutedSuccessor(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(key, "")
	}
	legacy, err := local115OperatorProfileByName("legacy-20261026")
	if err != nil || !legacy.bound() || legacy.plan != local115Plan || legacy.history != local115History || legacy.source != local115Source || legacy.targetDSN != local115TargetDSN || legacy.database != "memory115_native_test" || strings.Join(legacy.environment(), "\n") != strings.Join(local115Environment(), "\n") {
		t.Fatal("legacy fixed operator binding changed", err)
	}
	successor, err := local115OperatorProfileByName("skill-preparation-v1")
	if err != nil || successor.targetDSN != "postgres://postgres@127.0.0.1:55451/memory115_skillprep_v1_test?sslmode=disable" || successor.database != "memory115_skillprep_v1_test" || successor.source == legacy.source || successor.plan == legacy.plan || successor.history == legacy.history || successor.artifacts["app"].path == legacy.artifacts["app"].path {
		t.Fatal("prospective operator reuses a closed identity", err)
	}
	for _, role := range []string{"broker", "broker_control", "extension", "http"} {
		if successor.artifacts[role].path == legacy.artifacts[role].path || !strings.HasPrefix(successor.artifacts[role].path, "/private/memory115-prepared-skillprep-v1/bin/") {
			t.Fatal("new source cohort reuses a closed executable path")
		}
	}
	for _, name := range []string{"", "../foreign", "skill-preparation-v2", "postgres://foreign"} {
		if _, err := local115OperatorProfileByName(name); err == nil {
			t.Fatal("unknown operator profile accepted")
		}
	}
	for name, mutate := range map[string]func(*local115OperatorProfile){
		"unbound_plan":    func(p *local115OperatorProfile) { p.planSHA = "" },
		"unbound_history": func(p *local115OperatorProfile) { p.historySHA = "" },
		"unbound_app":     func(p *local115OperatorProfile) { pin := p.artifacts["app"]; pin.sha = ""; p.artifacts["app"] = pin },
		"different_database": func(p *local115OperatorProfile) {
			p.database, p.targetDSN = "foreign_test", "postgres://postgres@127.0.0.1:55451/foreign_test?sslmode=disable"
		},
		"query_host_override": func(p *local115OperatorProfile) { p.targetDSN += "&host=synthetic.invalid" },
		"foreign_source":      func(p *local115OperatorProfile) { p.source = "/private/foreign" },
		"foreign_app": func(p *local115OperatorProfile) {
			pin := p.artifacts["app"]
			pin.path = "/private/foreign"
			p.artifacts["app"] = pin
		},
	} {
		for _, base := range []string{"legacy-20261026", "skill-preparation-v1"} {
			t.Run(base+"/"+name, func(t *testing.T) {
				profile, _ := local115OperatorProfileByName(base)
				mutate(&profile)
				if err := profile.execute(t.Context(), "synthetic-denied-before-io", true); err == nil || err.Error() != "fixed local operator profile is not fully frozen and bound" {
					t.Fatal("unbound/substituted profile reached filesystem, PostgreSQL or auth", err)
				}
			})
		}
	}
	changed, _ := local115OperatorProfileByName("legacy-20261026")
	delete(changed.artifacts, "app")
	again, _ := local115OperatorProfileByName("legacy-20261026")
	if len(again.artifacts) != 5 || !again.bound() {
		t.Fatal("profile maps share mutable process-global pins")
	}
}

func TestLocal115ResourceReceiptNeedsActualKernelPopulationProof(t *testing.T) {
	dir := t.TempDir()
	if local115Reaped(dir) {
		t.Fatal("missing kernel proof accepted")
	}
	if os.WriteFile(filepath.Join(dir, "cgroup.events"), []byte("populated 1\nfrozen 0\n"), 0600) != nil {
		t.Fatal("synthetic resource fixture failed")
	}
	if local115Reaped(dir) {
		t.Fatal("live process group accepted")
	}
}
