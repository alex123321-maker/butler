package app

import (
	"fmt"
	"os"
	"strings"

	apiservice "github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/internal/config"
)

func ownerControlAuth(cfg config.OrchestratorConfig) (*apiservice.OwnerControlAuth, error) {
	token := cfg.OwnerControlToken
	if cfg.OwnerControlTokenFile != "" {
		if token != "" {
			return nil, fmt.Errorf("configure one owner-control token source")
		}
		info, err := os.Stat(cfg.OwnerControlTokenFile)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
			return nil, fmt.Errorf("owner-control token file unavailable or invalid")
		}
		raw, err := os.ReadFile(cfg.OwnerControlTokenFile)
		if err != nil {
			return nil, fmt.Errorf("owner-control token file unreadable")
		}
		token = strings.TrimSpace(string(raw))
	}
	if token == "" && cfg.OwnerControlOwnerID == "" {
		return apiservice.NewOwnerControlAuth("", ""), nil
	}
	if len(token) < 32 || cfg.OwnerControlOwnerID == "" {
		return nil, fmt.Errorf("owner-control authentication requires a token of at least 32 bytes and an explicit owner ID")
	}
	return apiservice.NewOwnerControlAuth(token, cfg.OwnerControlOwnerID), nil
}
