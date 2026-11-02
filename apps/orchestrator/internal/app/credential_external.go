package app

import (
	"context"
	"errors"
	"os"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/externalauth"
)

func configureExternalCredentials(ctx context.Context, cfg config.OrchestratorConfig, c *CredentialInteractions, budgets *budget.Controller) error {
	if !cfg.ExternalAuthEnabled {
		return nil
	}
	if c == nil || c.Vault == nil {
		return errors.New("external browser authentication requires the protected credential vault")
	}
	f, e := os.Open(cfg.ExternalAuthClientFile)
	if e != nil {
		return errors.New("external browser authentication private client unavailable")
	}
	defer f.Close()
	client, e := externalauth.ReadGoogleWebConfig(f, cfg.ExternalAuthRPURL+"auth/google/callback")
	if e != nil || c.Vault.ConfigureExternalAuth(ctx, client, cfg.ExternalAuthRPURL, cfg.ExternalAuthRPURL+"auth/google/account", budgets) != nil {
		return errors.New("external browser authentication profile unavailable")
	}
	return nil
}
