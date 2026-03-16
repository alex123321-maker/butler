package modelprovider

const (
	ProviderOpenAI        = "openai"
	ProviderOpenAICodex   = "openai-codex"
	ProviderGitHubCopilot = "github-copilot"
)

func SupportedProviders() []string {
	return []string{ProviderOpenAI, ProviderOpenAICodex, ProviderGitHubCopilot}
}
