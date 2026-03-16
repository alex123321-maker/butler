package providerauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/modelprovider"
)

type SecretStore interface {
	Get(context.Context, string) (config.Setting, error)
	Set(context.Context, config.Setting) (config.Setting, error)
	Delete(context.Context, string) error
}

const (
	settingKeyGitHubCopilot = "BUTLER_PROVIDER_AUTH_GITHUB_COPILOT"
	settingKeyOpenAICodex   = "BUTLER_PROVIDER_AUTH_OPENAI_CODEX"
)

var (
	ErrUnknownProvider = errors.New("unknown provider")
	ErrFlowNotFound    = errors.New("provider auth flow not found")
	ErrNotConnected    = errors.New("provider auth is not connected")
)

type githubCopilotCredentials struct {
	GitHubToken      string    `json:"github_token"`
	CopilotToken     string    `json:"copilot_token"`
	BaseURL          string    `json:"base_url"`
	EnterpriseDomain string    `json:"enterprise_domain,omitempty"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type openAICodexCredentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	AccountID    string    `json:"account_id"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func providerSettingKey(provider string) (string, error) {
	switch strings.TrimSpace(provider) {
	case modelprovider.ProviderGitHubCopilot:
		return settingKeyGitHubCopilot, nil
	case modelprovider.ProviderOpenAICodex:
		return settingKeyOpenAICodex, nil
	default:
		return "", ErrUnknownProvider
	}
}

func loadSecretJSON[T any](ctx context.Context, store SecretStore, provider string, target *T) (bool, error) {
	if store == nil {
		return false, fmt.Errorf("provider auth store is not configured")
	}
	key, err := providerSettingKey(provider)
	if err != nil {
		return false, err
	}
	setting, err := store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, config.ErrSettingNotFound) {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal([]byte(setting.Value), target); err != nil {
		return false, fmt.Errorf("decode provider auth setting %s: %w", key, err)
	}
	return true, nil
}

func saveSecretJSON(ctx context.Context, store SecretStore, provider, updatedBy string, value any) error {
	if store == nil {
		return fmt.Errorf("provider auth store is not configured")
	}
	key, err := providerSettingKey(provider)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode provider auth setting %s: %w", key, err)
	}
	_, err = store.Set(ctx, config.Setting{
		Key:       key,
		Value:     string(encoded),
		Component: "orchestrator",
		IsSecret:  true,
		UpdatedBy: updatedBy,
	})
	return err
}

func deleteSecret(ctx context.Context, store SecretStore, provider string) error {
	if store == nil {
		return fmt.Errorf("provider auth store is not configured")
	}
	key, err := providerSettingKey(provider)
	if err != nil {
		return err
	}
	err = store.Delete(ctx, key)
	if errors.Is(err, config.ErrSettingNotFound) {
		return nil
	}
	return err
}
