package acp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDedicatedProfileIsIdempotentAndDoesNotOverwrite(test *testing.T) {
	profile := filepath.Join(test.TempDir(), "dedicated")
	if PrepareAntigravityProfile(profile) != nil || PrepareAntigravityProfile(profile) != nil {
		test.Fatal("profile preparation failed")
	}
	settings := filepath.Join(profile, ".gemini", "antigravity-cli", "settings.json")
	if err := os.WriteFile(settings, []byte(`{"toolPermission":"always-proceed"}`), 0600); err != nil {
		test.Fatal(err)
	}
	if PrepareAntigravityProfile(profile) == nil || ValidateAntigravityProfile(profile) == nil {
		test.Fatal("unsafe profile silently repaired or accepted")
	}
	actual, _ := os.ReadFile(settings)
	if string(actual) != `{"toolPermission":"always-proceed"}` {
		test.Fatal("existing configuration overwritten")
	}
}

func TestProfileRejectsAmbientCustomizations(test *testing.T) {
	for _, relative := range []string{"hooks.json", "skills.json", "plugins.json", "agents/other/agent.md", "mcp_config.json"} {
		test.Run(relative, func(test *testing.T) {
			profile := filepath.Join(test.TempDir(), "dedicated")
			if PrepareAntigravityProfile(profile) != nil {
				test.Fatal("prepare failed")
			}
			path := filepath.Join(profile, ".gemini", "config", filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				test.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(`{"mcpServers":{"untrusted":{}}}`), 0600); err != nil {
				test.Fatal(err)
			}
			if ValidateAntigravityProfile(profile) == nil {
				test.Fatal("ambient customization accepted")
			}
		})
	}
}

func TestProfileRefusesExistingOrdinaryHome(test *testing.T) {
	profile := test.TempDir()
	if PrepareAntigravityProfile(profile) == nil {
		test.Fatal("existing home accepted")
	}
	entries, err := os.ReadDir(profile)
	if err != nil || len(entries) != 0 {
		test.Fatal("ordinary home modified")
	}
}

func TestDispatchCallbackRunsAfterNegotiationBeforePrompt(test *testing.T) {
	request := fixtureRequest()
	denied := errors.New("authority revoked")
	request.BeforePrompt = func(context.Context) error { return denied }
	result, err := runPeer(test, newFixtureProbe(test, fixtureLimits()), context.Background(), request, func(agent *peer) error {
		if err := agent.handshake("fixture-gemini", "fixture-gemini"); err != nil {
			return err
		}
		_, err := agent.read("session/cancel")
		return err
	})
	if !errors.Is(err, denied) || result != (Result{}) {
		test.Fatalf("result=%+v err=%v", result, err)
	}
}
