package config

import (
	"fmt"
	"sync"
	"testing"
)

func TestHotConfigLoadsAndSwapsHotValues(t *testing.T) {
	hot := NewHotConfig(Snapshot{keys: []ConfigKeyInfo{
		{Key: "BUTLER_LOG_LEVEL", EffectiveValue: "info", RequiresRestart: false},
		{Key: "BUTLER_HTTP_ADDR", EffectiveValue: ":8080", RequiresRestart: true},
	}})

	value, ok := hot.Get("BUTLER_LOG_LEVEL")
	if !ok || value != "info" {
		t.Fatalf("expected initial hot value, got %q %v", value, ok)
	}
	if _, ok := hot.Get("BUTLER_HTTP_ADDR"); ok {
		t.Fatal("expected cold value to be excluded from hot config")
	}

	result, err := hot.Apply(ConfigKeyInfo{Key: "BUTLER_LOG_LEVEL", RequiresRestart: false}, "debug")
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if !result.Applied || result.RequiresRestart {
		t.Fatalf("unexpected hot update result: %+v", result)
	}
	value, ok = hot.Get("BUTLER_LOG_LEVEL")
	if !ok || value != "debug" {
		t.Fatalf("expected swapped hot value, got %q %v", value, ok)
	}
}

func TestHotConfigMarksColdSettingsAsRestartRequired(t *testing.T) {
	hot := NewHotConfig(Snapshot{})

	result, err := hot.Apply(ConfigKeyInfo{Key: "BUTLER_HTTP_ADDR", RequiresRestart: true}, ":9090")
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Applied {
		t.Fatalf("expected cold update to skip hot apply, got %+v", result)
	}
	if !result.RequiresRestart {
		t.Fatalf("expected restart-required result, got %+v", result)
	}
}

func TestHotConfigConcurrentReadsStaySafeDuringSwap(t *testing.T) {
	hot := NewHotConfig(Snapshot{keys: []ConfigKeyInfo{{Key: "BUTLER_LOG_LEVEL", EffectiveValue: "info", RequiresRestart: false}}})

	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for reader := 0; reader < 4; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				value, ok := hot.Get("BUTLER_LOG_LEVEL")
				if !ok {
					errCh <- fmt.Errorf("missing hot key during read")
					return
				}
				if value != "info" && value != "debug" {
					errCh <- fmt.Errorf("unexpected hot value %q", value)
					return
				}
			}
		}()
	}

	for i := 0; i < 100; i++ {
		if _, err := hot.Apply(ConfigKeyInfo{Key: "BUTLER_LOG_LEVEL", RequiresRestart: false}, "debug"); err != nil {
			t.Fatalf("Apply returned error: %v", err)
		}
		if _, err := hot.Apply(ConfigKeyInfo{Key: "BUTLER_LOG_LEVEL", RequiresRestart: false}, "info"); err != nil {
			t.Fatalf("Apply returned error: %v", err)
		}
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}
