package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/butler/butler/internal/config"
)

type SettingsManager interface {
	List(context.Context) ([]config.SettingState, error)
	Update(context.Context, string, string) (config.SettingState, error)
	Delete(context.Context, string) (config.SettingState, error)
	PendingRestartComponents() []string
	ClearPendingRestart()
}

type SettingsServer struct {
	manager SettingsManager
}

func NewSettingsServer(manager SettingsManager) *SettingsServer {
	return &SettingsServer{manager: manager}
}

func (s *SettingsServer) HandleList() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		settings, err := s.manager.List(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"components": groupSettings(settings)})
	})
}

func (s *SettingsServer) HandleItem() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/settings/"))
		if key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "setting key is required"})
			return
		}

		switch r.Method {
		case http.MethodPut:
			var request struct {
				Value string `json:"value"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body"})
				return
			}
			setting, err := s.manager.Update(r.Context(), key, request.Value)
			if err != nil {
				status := http.StatusInternalServerError
				if errors.Is(err, config.ErrInvalidSettingValue) {
					status = http.StatusBadRequest
				}
				writeJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"setting": toSettingDTO(setting)})
		case http.MethodDelete:
			setting, err := s.manager.Delete(r.Context(), key)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"setting": toSettingDTO(setting)})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func (s *SettingsServer) HandleRestart() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, restartResponse(s.manager.PendingRestartComponents()))
		case http.MethodPost:
			components := s.manager.PendingRestartComponents()
			s.manager.ClearPendingRestart()
			writeJSON(w, http.StatusOK, restartResponse(components))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

type settingsComponentDTO struct {
	Name     string       `json:"name"`
	Settings []settingDTO `json:"settings"`
}

type settingDTO struct {
	Key              string   `json:"key"`
	Component        string   `json:"component"`
	Group            string   `json:"group"`
	Value            string   `json:"value"`
	Source           string   `json:"source"`
	IsSecret         bool     `json:"is_secret"`
	RequiresRestart  bool     `json:"requires_restart"`
	AllowedValues    []string `json:"allowed_values,omitempty"`
	ValidationStatus string   `json:"validation_status"`
	ValidationError  string   `json:"validation_error,omitempty"`
}

func groupSettings(settings []config.SettingState) []settingsComponentDTO {
	groups := map[string][]settingDTO{}
	for _, setting := range settings {
		name := strings.TrimSpace(setting.Group)
		if name == "" {
			name = setting.Component
		}
		groups[name] = append(groups[name], toSettingDTO(setting))
	}
	components := make([]settingsComponentDTO, 0, len(groups))
	for name, items := range groups {
		sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
		components = append(components, settingsComponentDTO{Name: name, Settings: items})
	}
	sort.Slice(components, func(i, j int) bool { return components[i].Name < components[j].Name })
	return components
}

func toSettingDTO(setting config.SettingState) settingDTO {
	value := setting.Value
	if setting.IsSecret {
		value = config.MaskForDisplay(setting.Key, setting.Value)
	}
	return settingDTO{
		Key:              setting.Key,
		Component:        setting.Component,
		Group:            setting.Group,
		Value:            value,
		Source:           setting.Source,
		IsSecret:         setting.IsSecret,
		RequiresRestart:  setting.RequiresRestart,
		AllowedValues:    append([]string(nil), setting.AllowedValues...),
		ValidationStatus: string(setting.ValidationStatus),
		ValidationError:  setting.ValidationError,
	}
}

func restartResponse(components []string) map[string]any {
	command := ""
	if len(components) > 0 {
		command = "docker compose -f deploy/docker-compose.yml restart " + strings.Join(components, " ")
	}
	return map[string]any{
		"components":        components,
		"suggested_command": command,
	}
}
