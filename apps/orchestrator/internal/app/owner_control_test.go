package app

import (
	"github.com/butler/butler/internal/config"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnerControlProvisioning(t *testing.T) {
	file := filepath.Join(t.TempDir(), "owner.token")
	token := strings.Repeat("private-fixture-", 3)
	if err := os.WriteFile(file, []byte(token+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	auth, err := ownerControlAuth(config.OrchestratorConfig{OwnerControlTokenFile: file, OwnerControlOwnerID: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/decision", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	if _, ok := auth.Authenticate(r); !ok {
		t.Fatal("file proof not accepted")
	}
	for _, cfg := range []config.OrchestratorConfig{{OwnerControlToken: token}, {OwnerControlOwnerID: "owner"}, {OwnerControlToken: token, OwnerControlTokenFile: file, OwnerControlOwnerID: "owner"}, {OwnerControlTokenFile: filepath.Dir(file), OwnerControlOwnerID: "owner"}} {
		if _, err = ownerControlAuth(cfg); err == nil {
			t.Fatal("invalid owner provisioning accepted")
		}
	}
	auth, err = ownerControlAuth(config.OrchestratorConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := auth.Authenticate(r); ok {
		t.Fatal("absence of authority granted approval")
	}
}
