package providerfactory

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/githubcopilot"
	"github.com/butler/butler/internal/transport/openai"
	"github.com/butler/butler/internal/transport/openaicodex"
)

type Factory struct {
	auth *providerauth.Manager
	log  *slog.Logger
}

type BuildConfig struct {
	ActiveProvider      string
	OpenAIAPIKey        string
	OpenAIModel         string
	OpenAIBaseURL       string
	OpenAIRealtimeURL   string
	OpenAITransportMode string
	OpenAICodexModel    string
	OpenAICodexBaseURL  string
	GitHubCopilotModel  string
	Timeout             time.Duration
}

type Result struct {
	Provider     transport.ModelProvider
	ProviderName string
	ModelName    string
}

func New(auth *providerauth.Manager, log *slog.Logger) *Factory {
	if log == nil {
		log = slog.Default()
	}
	return &Factory{auth: auth, log: logger.WithComponent(log, "model-provider")}
}

func (f *Factory) Build(_ context.Context, cfg BuildConfig) (Result, error) {
	provider := cfg.ActiveProvider
	if provider == "" {
		provider = modelprovider.ProviderOpenAI
	}
	switch provider {
	case modelprovider.ProviderOpenAI:
		instance, err := transport.NewProvider(provider, openai.Config{
			APIKey:        cfg.OpenAIAPIKey,
			Model:         cfg.OpenAIModel,
			BaseURL:       cfg.OpenAIBaseURL,
			RealtimeURL:   cfg.OpenAIRealtimeURL,
			TransportMode: cfg.OpenAITransportMode,
			Timeout:       cfg.Timeout,
			Logger:        logger.WithComponent(f.log, provider),
		})
		if err != nil {
			return Result{}, err
		}
		return Result{Provider: instance, ProviderName: provider, ModelName: cfg.OpenAIModel}, nil
	case modelprovider.ProviderOpenAICodex:
		instance, err := transport.NewProvider(provider, openaicodex.Config{
			Model:      cfg.OpenAICodexModel,
			BaseURL:    cfg.OpenAICodexBaseURL,
			Timeout:    cfg.Timeout,
			Logger:     logger.WithComponent(f.log, provider),
			AuthSource: f.auth,
		})
		if err != nil {
			return Result{}, err
		}
		return Result{Provider: instance, ProviderName: provider, ModelName: cfg.OpenAICodexModel}, nil
	case modelprovider.ProviderGitHubCopilot:
		instance, err := transport.NewProvider(provider, githubcopilot.Config{
			Model:      cfg.GitHubCopilotModel,
			Timeout:    cfg.Timeout,
			Logger:     logger.WithComponent(f.log, provider),
			AuthSource: f.auth,
		})
		if err != nil {
			return Result{}, err
		}
		return Result{Provider: instance, ProviderName: provider, ModelName: cfg.GitHubCopilotModel}, nil
	default:
		return Result{}, fmt.Errorf("unsupported model provider %q", provider)
	}
}
