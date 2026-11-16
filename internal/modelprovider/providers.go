package modelprovider

const (
	ProviderOpenAI         = "openai"
	ProviderOpenAICodex    = "openai-codex"
	ProviderGitHubCopilot  = "github-copilot"
	ProviderAntigravity    = "antigravity"
	ProviderAntigravityACP = "antigravity-acp"
)

func SupportedProviders() []string {
	return []string{ProviderOpenAI, ProviderOpenAICodex, ProviderGitHubCopilot, ProviderAntigravity}
}

func RuntimeProviders() []string {
	return append(SupportedProviders(), ProviderAntigravityACP)
}
