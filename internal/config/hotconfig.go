package config

import (
	"fmt"
	"sync/atomic"
)

type HotUpdateResult struct {
	Applied         bool
	RequiresRestart bool
	Value           string
}

type HotConfig struct {
	state atomic.Pointer[hotConfigState]
}

type hotConfigState struct {
	values map[string]string
}

func NewHotConfig(snapshot Snapshot) *HotConfig {
	hot := &HotConfig{}
	hot.Replace(snapshot)
	return hot
}

func (h *HotConfig) Replace(snapshot Snapshot) {
	values := make(map[string]string)
	for _, key := range snapshot.ListKeys() {
		if key.RequiresRestart {
			continue
		}
		values[key.Key] = key.EffectiveValue
	}
	h.state.Store(&hotConfigState{values: values})
}

func (h *HotConfig) Get(key string) (string, bool) {
	state := h.state.Load()
	if state == nil {
		return "", false
	}
	value, ok := state.values[key]
	return value, ok
}

func (h *HotConfig) Apply(info ConfigKeyInfo, value string) (HotUpdateResult, error) {
	if info.Key == "" {
		return HotUpdateResult{}, fmt.Errorf("config key is required")
	}
	if info.RequiresRestart {
		return HotUpdateResult{Applied: false, RequiresRestart: true, Value: value}, nil
	}

	current := h.state.Load()
	nextValues := make(map[string]string)
	if current != nil {
		for key, currentValue := range current.values {
			nextValues[key] = currentValue
		}
	}
	nextValues[info.Key] = value
	h.state.Store(&hotConfigState{values: nextValues})
	return HotUpdateResult{Applied: true, RequiresRestart: false, Value: value}, nil
}
