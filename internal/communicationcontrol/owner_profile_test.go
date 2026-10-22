package communicationcontrol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCommunicationOwnerProfileRequiresExplicitPrivateScope(t *testing.T) {
	dir := t.TempDir()
	p := OwnerProfile{Version: 1, Database: "butler", OwnerID: "owner", BindingID: "owner-tg", WebBindingID: "owner-web", BindingVersion: 1, WebBindingVersion: 1, ExpectedBot: "owned_bot", BotID: 77, ChatID: 42, TokenFile: filepath.Join(dir, "token"), BotAPIProxyFile: filepath.Join(dir, "proxy"), ControlURL: "http://orchestrator:8080/internal/communication", Mode: "active"}
	for name, mutate := range map[string]func(*OwnerProfile){
		"database":           func(p *OwnerProfile) { p.Database = "postgres" },
		"connection_string":  func(p *OwnerProfile) { p.Database = "postgres://private" },
		"numeric_bot":        func(p *OwnerProfile) { p.BotID = 0 },
		"binding_collision":  func(p *OwnerProfile) { p.WebBindingID = p.BindingID },
		"implicit_version":   func(p *OwnerProfile) { p.BindingVersion = 0 },
		"no_proxy":           func(p *OwnerProfile) { p.BotAPIProxyFile = "" },
		"untrusted_endpoint": func(p *OwnerProfile) { p.ControlURL = "http://other/internal/communication" },
		"mode":               func(p *OwnerProfile) { p.Mode = "legacy" },
		"negative_cursor":    func(p *OwnerProfile) { p.InitialUpdateID = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			q := p
			mutate(&q)
			if q.Validate() == nil {
				t.Fatal("invalid owner scope accepted")
			}
		})
	}
	path := filepath.Join(dir, "profile.json")
	raw, _ := json.Marshal(p)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadOwnerProfile(path); err != nil || got != p {
		t.Fatal("private profile unavailable", err)
	}
	q := p
	q.Mode = "drain"
	if q.Validate() != nil {
		t.Fatal("drain denied")
	}
	for _, suffix := range []string{`,"ProviderAuthStore":{}`, `,"ownerDatabase":"other"`, `,"Token":"raw"`} {
		bad := append(append([]byte{}, raw[:len(raw)-1]...), []byte(suffix+"}")...)
		if err := os.WriteFile(path, bad, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadOwnerProfile(path); err == nil {
			t.Fatal("unknown credential/override field accepted")
		}
	}
	if runtime.GOOS != "windows" {
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadOwnerProfile(path); err == nil {
			t.Fatal("public profile accepted")
		}
	}
}
