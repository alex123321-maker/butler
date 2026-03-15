package config

import (
	"context"
	"fmt"
	"os"
	"strings"
)

var (
	ErrUnknownSetting      = fmt.Errorf("unknown setting key")
	ErrInvalidSettingValue = fmt.Errorf("invalid setting value")
)

type SettingState struct {
	Key              string
	Component        string
	Value            string
	Source           string
	IsSecret         bool
	RequiresRestart  bool
	ValidationStatus ValidationStatus
	ValidationError  string
}

type SettingsService struct {
	store *PostgresSettingsStore
	hot   *HotConfig
	env   envGetter
}

func NewSettingsService(store *PostgresSettingsStore, hot *HotConfig) *SettingsService {
	if hot == nil {
		hot = NewHotConfig(Snapshot{})
	}
	return &SettingsService{store: store, hot: hot, env: os.LookupEnv}
}

func (s *SettingsService) List(ctx context.Context) ([]SettingState, error) {
	settings, err := s.store.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	return s.resolveStates(settings)
}

func (s *SettingsService) Update(ctx context.Context, key, value string) (SettingState, error) {
	spec, ok := settingSpecByKey(strings.TrimSpace(key))
	if !ok {
		return SettingState{}, ErrUnknownSetting
	}
	if err := validateSettingValue(spec, value); err != nil {
		return SettingState{}, fmt.Errorf("%w: %v", ErrInvalidSettingValue, err)
	}
	if _, err := s.store.Set(ctx, Setting{Key: spec.key, Value: value, Component: spec.component, IsSecret: spec.isSecret, UpdatedBy: "orchestrator-api"}); err != nil {
		return SettingState{}, err
	}
	state, err := s.stateForKey(ctx, spec.key)
	if err != nil {
		return SettingState{}, err
	}
	_, _ = s.hot.Apply(ConfigKeyInfo{Key: state.Key, RequiresRestart: state.RequiresRestart}, state.Value)
	return state, nil
}

func (s *SettingsService) Delete(ctx context.Context, key string) (SettingState, error) {
	trimmed := strings.TrimSpace(key)
	if err := s.store.Delete(ctx, trimmed); err != nil {
		return SettingState{}, err
	}
	state, err := s.stateForKey(ctx, trimmed)
	if err != nil {
		return SettingState{}, err
	}
	_, _ = s.hot.Apply(ConfigKeyInfo{Key: state.Key, RequiresRestart: state.RequiresRestart}, state.Value)
	return state, nil
}

func (s *SettingsService) stateForKey(ctx context.Context, key string) (SettingState, error) {
	states, err := s.List(ctx)
	if err != nil {
		return SettingState{}, err
	}
	for _, state := range states {
		if state.Key == key {
			return state, nil
		}
	}
	return SettingState{}, ErrUnknownSetting
}

func (s *SettingsService) resolveStates(settings []Setting) ([]SettingState, error) {
	resolver := NewLayeredResolver(s.env, settings)
	snapshot, err := resolver.Resolve(layeredOrchestratorSpecs(&OrchestratorConfig{}))
	if err != nil {
		return nil, err
	}
	settingsByKey := make(map[string]Setting, len(settings))
	for _, setting := range settings {
		settingsByKey[setting.Key] = setting
	}
	states := make([]SettingState, 0, len(snapshot.ListKeys()))
	for _, item := range snapshot.ListKeys() {
		state := SettingState{
			Key:              item.Key,
			Component:        item.Component,
			Source:           item.Source,
			IsSecret:         item.IsSecret,
			RequiresRestart:  item.RequiresRestart,
			ValidationStatus: item.ValidationStatus,
			ValidationError:  item.ValidationError,
		}
		switch item.Source {
		case ConfigSourceEnv:
			state.Value, _ = s.env(item.Key)
		case ConfigSourceDB:
			state.Value = settingsByKey[item.Key].Value
		default:
			if spec, ok := settingSpecByKey(item.Key); ok {
				state.Value = spec.defaultValue
			}
		}
		states = append(states, state)
	}
	s.hot.Replace(snapshot)
	return states, nil
}

func settingSpecByKey(key string) (fieldSpec, bool) {
	for _, spec := range layeredOrchestratorSpecs(&OrchestratorConfig{}) {
		if spec.key == key {
			return spec, true
		}
	}
	return fieldSpec{}, false
}

func validateSettingValue(spec fieldSpec, value string) error {
	if spec.required && strings.TrimSpace(value) == "" {
		return fmt.Errorf("required value is missing")
	}
	if !spec.required && strings.TrimSpace(value) == "" {
		return nil
	}
	return validateValue(spec, value)
}
