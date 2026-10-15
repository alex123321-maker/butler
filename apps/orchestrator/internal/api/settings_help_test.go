package api

import (
	"github.com/butler/butler/internal/config"
	"testing"
)

func TestSettingDTOIncludesCodeOwnedHelp(t *testing.T) {
	dto := toSettingDTO(config.SettingState{Description: "description", ValueHint: "hint", AllowedValues: []string{"one"}})
	if dto.Description != "description" || dto.ValueHint != "hint" || len(dto.AllowedValues) != 1 {
		t.Fatal("help metadata lost")
	}
}
