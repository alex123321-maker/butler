//go:build linux

package acp

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestManagedProfileProcessUsesDedicatedHome(test *testing.T) {
	profile := filepath.Join(test.TempDir(), "profile")
	if err := PrepareAntigravityProfile(profile); err != nil {
		test.Fatal(err)
	}
	spec := fixtureProcess(test, "profile", profile)
	spec.profileDir = profile
	request := fixtureRequest()
	var dispatched bool
	request.BeforePrompt = func(context.Context) error { dispatched = true; return nil }
	result, err := newFixtureProbe(test, fixtureLimits()).RunProcess(context.Background(), spec, request)
	if err != nil || result.Text != "synthetic stdio result" || !dispatched {
		test.Fatalf("result=%+v err=%v dispatched=%v", result, err, dispatched)
	}
	if ValidateAntigravityProfile(profile) != nil {
		test.Fatal("dedicated profile removed or mutated")
	}
}

func TestManagedProfileRejectsPermissionsAndSpecialFiles(test *testing.T) {
	for _, mode := range []string{"public-home", "public-settings", "symlink", "fifo", "oversize", "project-hooks", "migration-directory"} {
		test.Run(mode, func(test *testing.T) {
			profile := filepath.Join(test.TempDir(), "profile")
			if err := PrepareAntigravityProfile(profile); err != nil {
				test.Fatal(err)
			}
			settings := filepath.Join(profile, ".gemini", "antigravity-cli", "settings.json")
			var err error
			switch mode {
			case "public-home":
				err = os.Chmod(profile, 0755)
			case "public-settings":
				err = os.Chmod(settings, 0644)
			case "symlink", "fifo":
				if err = os.Remove(settings); err != nil {
					test.Fatal(err)
				}
				if mode == "fifo" {
					err = syscall.Mkfifo(settings, 0600)
				} else {
					err = os.Symlink("/etc/passwd", settings)
				}
			case "oversize":
				err = os.WriteFile(settings, make([]byte, 8192), 0600)
			case "project-hooks":
				projects := filepath.Join(profile, ".gemini", "config", "projects")
				if err = os.Mkdir(projects, 0700); err != nil {
					test.Fatal(err)
				}
				err = os.WriteFile(filepath.Join(projects, "default-cli-project.json"), []byte(`{"id":"default-cli-project","projectResources":{"hook":"command"}}`), 0600)
			case "migration-directory":
				err = os.Mkdir(filepath.Join(profile, ".gemini", "config", ".migrated"), 0700)
			}
			if err != nil {
				test.Fatal(err)
			}
			if ValidateAntigravityProfile(profile) == nil {
				test.Fatal("unsafe profile accepted")
			}
		})
	}
}
