package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/butler/butler/internal/config"
)

func TestSettingsServerHandleListGroupsAndMasksSecrets(t *testing.T) {
	server := NewSettingsServer(fakeSettingsManager{list: []config.SettingState{
		{Key: "BUTLER_LOG_LEVEL", Component: "orchestrator", Value: "debug", Source: "db", ValidationStatus: config.ValidationStatusValid},
		{Key: "BUTLER_OPENAI_API_KEY", Component: "orchestrator", Value: "sk-abcdefghijkl1234", Source: "env", IsSecret: true, RequiresRestart: true, ValidationStatus: config.ValidationStatusValid},
	}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()
	server.HandleList().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var payload struct {
		Components []struct {
			Name     string `json:"name"`
			Settings []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"settings"`
		} `json:"components"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Components) != 1 || payload.Components[0].Name != "orchestrator" {
		t.Fatalf("unexpected grouped response: %+v", payload.Components)
	}
	if payload.Components[0].Settings[1].Value != "...1234" {
		t.Fatalf("expected masked API key, got %q", payload.Components[0].Settings[1].Value)
	}
}

func TestSettingsServerHandleUpdateReturnsValidationErrors(t *testing.T) {
	server := NewSettingsServer(fakeSettingsManager{updateErr: config.ErrInvalidSettingValue})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/BUTLER_LOG_LEVEL", strings.NewReader(`{"value":"verbose"}`))
	rr := httptest.NewRecorder()

	server.HandleItem().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestSettingsServerHandleUpdateReturnsRestartMetadata(t *testing.T) {
	server := NewSettingsServer(fakeSettingsManager{update: config.SettingState{Key: "BUTLER_OPENAI_MODEL", Component: "orchestrator", Value: "gpt-4.1-mini", Source: "db", RequiresRestart: true, ValidationStatus: config.ValidationStatusValid}})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/BUTLER_OPENAI_MODEL", strings.NewReader(`{"value":"gpt-4.1-mini"}`))
	rr := httptest.NewRecorder()

	server.HandleItem().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var payload struct {
		Setting struct {
			RequiresRestart bool `json:"requires_restart"`
		} `json:"setting"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Setting.RequiresRestart {
		t.Fatal("expected requires_restart=true")
	}
}

func TestSettingsServerHandleDeleteReturnsResolvedSetting(t *testing.T) {
	server := NewSettingsServer(fakeSettingsManager{deleted: config.SettingState{Key: "BUTLER_LOG_LEVEL", Component: "orchestrator", Value: "info", Source: "default", ValidationStatus: config.ValidationStatusValid}})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/settings/BUTLER_LOG_LEVEL", nil)
	rr := httptest.NewRecorder()

	server.HandleItem().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestSettingsServerHandleRestartReturnsPendingComponents(t *testing.T) {
	manager := &restartAwareManager{fakeSettingsManager: fakeSettingsManager{restarts: []string{"orchestrator", "tool-broker"}}}
	server := NewSettingsServer(manager)

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings/restart", nil)
	getRR := httptest.NewRecorder()
	server.HandleRestart().ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRR.Code)
	}
	var getPayload struct {
		Components []string `json:"components"`
	}
	if err := json.Unmarshal(getRR.Body.Bytes(), &getPayload); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if !reflect.DeepEqual(getPayload.Components, []string{"orchestrator", "tool-broker"}) {
		t.Fatalf("unexpected components: %+v", getPayload.Components)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/settings/restart", nil)
	postRR := httptest.NewRecorder()
	server.HandleRestart().ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", postRR.Code)
	}
	if !manager.cleared {
		t.Fatal("expected pending restart state to be cleared")
	}
}

func TestSettingsServerHandleToolsRegistryReadsAndWritesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tools.json")
	if err := os.WriteFile(path, []byte(`{"tools":[]}`), 0o600); err != nil {
		t.Fatalf("write temp tools file: %v", err)
	}
	manager := &restartAwareManager{fakeSettingsManager: fakeSettingsManager{effective: map[string]string{"BUTLER_TOOL_REGISTRY_PATH": path}}}
	server := NewSettingsServer(manager)

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings/tools-registry", nil)
	getRR := httptest.NewRecorder()
	server.HandleToolsRegistry().ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRR.Code)
	}

	badReq := httptest.NewRequest(http.MethodPut, "/api/v1/settings/tools-registry", strings.NewReader(`{"content":"{"}`))
	badRR := httptest.NewRecorder()
	server.HandleToolsRegistry().ServeHTTP(badRR, badReq)
	if badRR.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", badRR.Code)
	}

	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/settings/tools-registry", strings.NewReader(`{"content":"{\"tools\":[]}"}`))
	putRR := httptest.NewRecorder()
	server.HandleToolsRegistry().ServeHTTP(putRR, putReq)
	if putRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %s", putRR.Code, putRR.Body.String())
	}
	if !reflect.DeepEqual(manager.marked, []string{"tool-broker"}) {
		t.Fatalf("expected tool-broker restart mark, got %+v", manager.marked)
	}
}

type fakeSettingsManager struct {
	list      []config.SettingState
	update    config.SettingState
	deleted   config.SettingState
	effective map[string]string
	restarts  []string
	listErr   error
	updateErr error
	deleteErr error
}

func (f fakeSettingsManager) List(context.Context) ([]config.SettingState, error) {
	return f.list, f.listErr
}

func (f fakeSettingsManager) Update(context.Context, string, string) (config.SettingState, error) {
	if f.updateErr != nil {
		return config.SettingState{}, f.updateErr
	}
	return f.update, nil
}

func (f fakeSettingsManager) Delete(context.Context, string) (config.SettingState, error) {
	if f.deleteErr != nil {
		return config.SettingState{}, f.deleteErr
	}
	return f.deleted, nil
}

func (f fakeSettingsManager) EffectiveValue(_ context.Context, key string) (string, error) {
	if value, ok := f.effective[key]; ok {
		return value, nil
	}
	return "", config.ErrUnknownSetting
}

func (f fakeSettingsManager) PendingRestartComponents() []string {
	return append([]string(nil), f.restarts...)
}

func (f fakeSettingsManager) ClearPendingRestart() {}

func (f fakeSettingsManager) MarkRestartComponent(string) {}

type restartAwareManager struct {
	fakeSettingsManager
	cleared bool
	marked  []string
}

func (m *restartAwareManager) ClearPendingRestart() {
	m.cleared = true
}

func (m *restartAwareManager) MarkRestartComponent(component string) {
	m.marked = append(m.marked, component)
}

var _ SettingsManager = fakeSettingsManager{}
