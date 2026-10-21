package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/memorytest"
)

func TestCommunicationApplicationEvidenceExcludesCredentialSettingsIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	if _, err := pool.Exec(context.Background(), `INSERT INTO system_settings(key,value,component,is_secret,updated_by) VALUES('SYNTHETIC_CREDENTIAL_KEY','SYNTHETIC_PRIVATE_CREDENTIAL_VALUE','orchestrator',TRUE,'fixture')`); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := captureCommunicationApplication(root, pool); err != nil {
		t.Fatal("evidence capture", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "retained-evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) || strings.Contains(string(raw), "SYNTHETIC_CREDENTIAL") || strings.Contains(string(raw), "system_settings") {
		t.Fatal("credential settings entered native evidence")
	}
	var summary map[string]int
	raw, err = os.ReadFile(filepath.Join(root, "summary.json"))
	if err != nil || json.Unmarshal(raw, &summary) != nil || summary["model_attempts"] != 0 {
		t.Fatal("evidence summary", err)
	}
}
