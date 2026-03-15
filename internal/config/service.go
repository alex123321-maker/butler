package config

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

var (
	ErrUnknownSetting      = fmt.Errorf("unknown setting key")
	ErrInvalidSettingValue = fmt.Errorf("invalid setting value")
)

type SettingState struct {
	Key              string
	Component        string
	Group            string
	Value            string
	Source           string
	IsSecret         bool
	RequiresRestart  bool
	AllowedValues    []string
	ValidationStatus ValidationStatus
	ValidationError  string
}

type SettingsService struct {
	store             *PostgresSettingsStore
	hot               *HotConfig
	env               envGetter
	restartComponents map[string]struct{}
	restartMu         sync.Mutex
}

func NewSettingsService(store *PostgresSettingsStore, hot *HotConfig) *SettingsService {
	if hot == nil {
		hot = NewHotConfig(Snapshot{})
	}
	return &SettingsService{store: store, hot: hot, env: os.LookupEnv, restartComponents: make(map[string]struct{})}
}

func (s *SettingsService) List(ctx context.Context) ([]SettingState, error) {
	settings, err := s.store.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	return s.resolveStates(settings, false)
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
	s.markRestartComponent(state)
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
	s.markRestartComponent(state)
	_, _ = s.hot.Apply(ConfigKeyInfo{Key: state.Key, RequiresRestart: state.RequiresRestart}, state.Value)
	return state, nil
}

func (s *SettingsService) EffectiveValue(ctx context.Context, key string) (string, error) {
	state, err := s.stateForKey(ctx, key)
	if err != nil {
		return "", err
	}
	return state.Value, nil
}

func (s *SettingsService) PendingRestartComponents() []string {
	s.restartMu.Lock()
	defer s.restartMu.Unlock()
	items := make([]string, 0, len(s.restartComponents))
	for component := range s.restartComponents {
		items = append(items, component)
	}
	sort.Strings(items)
	return items
}

func (s *SettingsService) ClearPendingRestart() {
	s.restartMu.Lock()
	defer s.restartMu.Unlock()
	s.restartComponents = make(map[string]struct{})
}

func (s *SettingsService) MarkRestartComponent(component string) {
	trimmed := strings.TrimSpace(component)
	if trimmed == "" {
		return
	}
	s.restartMu.Lock()
	defer s.restartMu.Unlock()
	s.restartComponents[trimmed] = struct{}{}
}

func (s *SettingsService) stateForKey(ctx context.Context, key string) (SettingState, error) {
	settings, err := s.store.ListAll(ctx)
	if err != nil {
		return SettingState{}, err
	}
	states, err := s.resolveStates(settings, true)
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

func (s *SettingsService) resolveStates(settings []Setting, includeHidden bool) ([]SettingState, error) {
	resolver := NewLayeredResolver(s.env, settings)
	snapshot, err := resolver.Resolve(managedFieldSpecs())
	if err != nil {
		return nil, err
	}
	settingsByKey := make(map[string]Setting, len(settings))
	for _, setting := range settings {
		settingsByKey[setting.Key] = setting
	}
	states := make([]SettingState, 0, len(snapshot.ListKeys()))
	for _, item := range snapshot.ListKeys() {
		spec, ok := managedSettingSpecByKey(item.Key)
		if !ok || (!includeHidden && !spec.Visible) {
			continue
		}
		state := SettingState{
			Key:              item.Key,
			Component:        item.Component,
			Group:            spec.Group,
			Source:           item.Source,
			IsSecret:         item.IsSecret,
			RequiresRestart:  item.RequiresRestart,
			AllowedValues:    append([]string(nil), spec.Spec.allowedValues...),
			ValidationStatus: item.ValidationStatus,
			ValidationError:  item.ValidationError,
		}
		switch item.Source {
		case ConfigSourceEnv:
			state.Value, _ = s.env(item.Key)
		case ConfigSourceDB:
			state.Value = settingsByKey[item.Key].Value
		default:
			state.Value = spec.Spec.defaultValue
		}
		states = append(states, state)
	}
	sort.Slice(states, func(i, j int) bool {
		left, _ := managedSettingSpecByKey(states[i].Key)
		right, _ := managedSettingSpecByKey(states[j].Key)
		if left.DisplayOrder == right.DisplayOrder {
			return states[i].Key < states[j].Key
		}
		return left.DisplayOrder < right.DisplayOrder
	})
	s.hot.Replace(snapshot)
	return states, nil
}

func settingSpecByKey(key string) (fieldSpec, bool) {
	item, ok := managedSettingSpecByKey(key)
	if ok {
		return item.Spec, true
	}
	return fieldSpec{}, false
}

func (s *SettingsService) markRestartComponent(state SettingState) {
	if !state.RequiresRestart {
		return
	}
	s.MarkRestartComponent(state.Component)
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
