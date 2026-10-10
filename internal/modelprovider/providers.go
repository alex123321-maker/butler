package modelprovider

const (
	ProviderOpenAI        = "openai"
	ProviderOpenAICodex   = "openai-codex"
	ProviderGitHubCopilot = "github-copilot"
	ProviderAntigravity   = "antigravity"
)

func SupportedProviders() []string {
	return []string{ProviderOpenAI, ProviderOpenAICodex, ProviderGitHubCopilot, ProviderAntigravity}
}
