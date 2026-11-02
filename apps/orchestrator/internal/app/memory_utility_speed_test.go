package app

import (
	"fmt"
	"testing"

	"github.com/butler/butler/internal/transport/antigravity"
)

// A new prospective profile. Closed plans use no Profile/Optimization fields
// and retain their exact old limits, prompt, capture and selection behavior.
func utilitySpeedProfile(name string) (provider, model, effort string, err error) {
	switch name {
	case "gemini-medium":
		return "antigravity", antigravity.FlashHighModel, "MEDIUM", nil
	case "gemini-low":
		return "antigravity", antigravity.FlashLowModel, "LOW", nil
	case "luna-max", "luna-high", "luna-low":
		return "openai", "gpt-6-luna", name[5:], nil
	default:
		return "", "", "", fmt.Errorf("unknown prospective speed profile")
	}
}

func (p utilityNativePlan) outputLimit() int64 {
	if p.Optimization == "latency-v1" {
		return 32768
	}
	return 8192
}

func TestMemoryUtilitySpeedProfiles(t *testing.T) {
	for _, name := range []string{"gemini-medium", "gemini-low", "luna-max", "luna-high", "luna-low"} {
		provider, model, effort, err := utilitySpeedProfile(name)
		if err != nil || provider == "" || model == "" || effort == "" {
			t.Fatal(name, err)
		}
	}
	for _, name := range []string{"", "gemini-high", "luna-medium", "luna-low ", "LUNA-LOW"} {
		if _, _, _, err := utilitySpeedProfile(name); err == nil {
			t.Fatal("unfrozen profile")
		}
	}
	if (utilityNativePlan{}).outputLimit() != 8192 || (utilityNativePlan{Optimization: "latency-v1"}).outputLimit() != 32768 {
		t.Fatal("limit drift")
	}
}
