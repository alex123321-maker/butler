package config

import "testing"

func TestOpenAIExplicitLunaProfile(t *testing.T) {
	get := envMap(map[string]string{"BUTLER_POSTGRES_URL": "postgres://fixture@localhost/fixture", "BUTLER_REDIS_URL": "redis://localhost:6379/0", "BUTLER_OPENAI_MODEL": "gpt-6-luna", "BUTLER_OPENAI_REASONING_EFFORT": "high", "BUTLER_OPENAI_RESPONSES_WS_URL": "wss://api.openai.com/v1/responses", "BUTLER_OPENAI_MAXIMUM_INPUT_TOKENS": "65536", "BUTLER_OPENAI_MAXIMUM_OUTPUT_TOKENS": "8192"})
	cfg, _, err := loadOrchestrator(get)
	if err != nil || cfg.OpenAIModel != "gpt-6-luna" || cfg.OpenAIReasoningEffort != "high" || cfg.OpenAIMaximumInputTokens != 65536 || cfg.OpenAIMaximumOutputTokens != 8192 || cfg.OpenAIResponsesWSURL != "wss://api.openai.com/v1/responses" {
		t.Fatal("Luna configuration failed", err)
	}
}
