package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	server := NewSettingsServer(fakeSettingsManager{update: config.SettingState{Key: "BUTLER_HTTP_ADDR", Component: "orchestrator", Value: ":9090", Source: "db", RequiresRestart: true, ValidationStatus: config.ValidationStatusValid}})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/BUTLER_HTTP_ADDR", strings.NewReader(`{"value":":9090"}`))
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

type fakeSettingsManager struct {
	list      []config.SettingState
	update    config.SettingState
	deleted   config.SettingState
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

var _ SettingsManager = fakeSettingsManager{}
