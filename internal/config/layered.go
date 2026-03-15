package config

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	ConfigSourceEnv     = "env"
	ConfigSourceDB      = "db"
	ConfigSourceDefault = "default"
)

type LayeredResolver struct {
	env      envGetter
	settings map[string]Setting
}

func NewLayeredResolver(get envGetter, settings []Setting) LayeredResolver {
	if get == nil {
		get = os.LookupEnv
	}
	index := make(map[string]Setting, len(settings))
	for _, setting := range settings {
		index[setting.Key] = setting
	}
	return LayeredResolver{env: get, settings: index}
}

func LoadOrchestratorLayered(settings []Setting) (OrchestratorConfig, Snapshot, error) {
	return loadOrchestratorLayered(os.LookupEnv, settings)
}

func loadOrchestratorLayered(get envGetter, settings []Setting) (OrchestratorConfig, Snapshot, error) {
	cfg := OrchestratorConfig{}
	resolver := NewLayeredResolver(get, settings)
	snapshot, err := resolver.Resolve(layeredOrchestratorSpecs(&cfg))
	if conditionalErr := validateOrchestratorConditionalConfig(cfg); conditionalErr != nil {
		if err != nil {
			return cfg, snapshot, fmt.Errorf("%v; %v", err, conditionalErr)
		}
		return cfg, snapshot, conditionalErr
	}
	return cfg, snapshot, err
}

func (r LayeredResolver) Resolve(specs []fieldSpec) (Snapshot, error) {
	return loadSpecsWithResolver(specs, func(spec fieldSpec) (string, string, bool) {
		if value, ok := r.env(spec.key); ok {
			return value, ConfigSourceEnv, true
		}
		setting, ok := r.settings[spec.key]
		if !ok {
			return "", "", false
		}
		return setting.Value, ConfigSourceDB, true
	})
}

func loadSpecsWithResolver(specs []fieldSpec, resolve func(fieldSpec) (string, string, bool)) (Snapshot, error) {
	keys := make([]ConfigKeyInfo, 0, len(specs))
	var problems []string

	for _, spec := range specs {
		value, source, ok := resolve(spec)
		if !ok {
			value = spec.defaultValue
			source = ConfigSourceDefault
		}

		info := ConfigKeyInfo{
			Key:             spec.key,
			Component:       spec.component,
			Type:            spec.typeName,
			Required:        spec.required,
			DefaultValue:    spec.defaultValue,
			Source:          source,
			IsSecret:        spec.isSecret,
			RequiresRestart: spec.requiresRestart,
		}

		if spec.required && value == "" {
			info.ValidationStatus = ValidationStatusMissing
			info.ValidationError = "required value is missing"
			keys = append(keys, finalizeInfo(info, value))
			problems = append(problems, spec.key+": "+info.ValidationError)
			continue
		}

		if value == "" && !spec.required {
			info.ValidationStatus = ValidationStatusValid
			keys = append(keys, finalizeInfo(info, value))
			if spec.assign != nil {
				spec.assign(value)
			}
			continue
		}

		if err := validateValue(spec, value); err != nil {
			info.ValidationStatus = ValidationStatusInvalid
			info.ValidationError = err.Error()
			keys = append(keys, finalizeInfo(info, value))
			problems = append(problems, spec.key+": "+err.Error())
			continue
		}

		info.ValidationStatus = ValidationStatusValid
		keys = append(keys, finalizeInfo(info, value))
		if spec.assign != nil {
			spec.assign(value)
		}
	}

	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Key < keys[j].Key
	})
	if len(problems) > 0 {
		return Snapshot{keys: keys}, fmt.Errorf("config validation failed: %s", strings.Join(problems, "; "))
	}
	return Snapshot{keys: keys}, nil
}

func layeredOrchestratorSpecs(cfg *OrchestratorConfig) []fieldSpec {
	shared := sharedSpecs("orchestrator", &cfg.Shared)
	return append(shared,
		fieldSpec{key: "BUTLER_POSTGRES_URL", component: "orchestrator", typeName: "string", required: true, isSecret: true, requiresRestart: true, validate: validateNonEmptyURL, assign: func(v string) { cfg.PostgresURL = v; cfg.Postgres.URL = v }},
		fieldSpec{key: "BUTLER_REDIS_URL", component: "orchestrator", typeName: "string", required: true, isSecret: true, requiresRestart: true, validate: validateNonEmptyURL, assign: func(v string) { cfg.RedisURL = v; cfg.Redis.URL = v }},
		fieldSpec{key: "BUTLER_SESSION_LEASE_TTL_SECONDS", component: "orchestrator", typeName: "int", required: false, defaultValue: "60", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { cfg.SessionLeaseTTLSeconds = mustParseInt(v) }},
		fieldSpec{key: "BUTLER_POSTGRES_MAX_CONNS", component: "orchestrator", typeName: "int", required: false, defaultValue: "10", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { cfg.Postgres.MaxConns = mustParseInt32(v) }},
		fieldSpec{key: "BUTLER_POSTGRES_MIN_CONNS", component: "orchestrator", typeName: "int", required: false, defaultValue: "2", requiresRestart: true, validate: validateNonNegativeInt, assign: func(v string) { cfg.Postgres.MinConns = mustParseInt32(v) }},
		fieldSpec{key: "BUTLER_POSTGRES_MAX_CONN_LIFETIME_SECONDS", component: "orchestrator", typeName: "int", required: false, defaultValue: "1800", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { cfg.Postgres.MaxConnLifetime = mustParseInt(v) }},
		fieldSpec{key: "BUTLER_POSTGRES_MIGRATIONS_DIR", component: "orchestrator", typeName: "string", required: false, defaultValue: "migrations", requiresRestart: true, validate: validateNonEmpty, assign: func(v string) { cfg.Postgres.MigrationsDir = v }},
		fieldSpec{key: "BUTLER_OPENAI_API_KEY", component: "orchestrator", typeName: "string", required: false, defaultValue: "", isSecret: true, requiresRestart: true, validate: validateOptionalNonEmpty, assign: func(v string) { cfg.OpenAIAPIKey = v }},
		fieldSpec{key: "BUTLER_OPENAI_MODEL", component: "orchestrator", typeName: "string", required: false, defaultValue: "gpt-4o-mini", requiresRestart: true, validate: validateNonEmpty, assign: func(v string) { cfg.OpenAIModel = v }},
		fieldSpec{key: "BUTLER_OPENAI_BASE_URL", component: "orchestrator", typeName: "string", required: false, defaultValue: "https://api.openai.com/v1", requiresRestart: true, validate: validateNonEmptyURL, assign: func(v string) { cfg.OpenAIBaseURL = v }},
		fieldSpec{key: "BUTLER_OPENAI_REALTIME_URL", component: "orchestrator", typeName: "string", required: false, defaultValue: "wss://api.openai.com/v1/realtime", requiresRestart: true, validate: validateNonEmptyURL, assign: func(v string) { cfg.OpenAIRealtimeURL = v }},
		fieldSpec{key: "BUTLER_OPENAI_TRANSPORT_MODE", component: "orchestrator", typeName: "string", required: false, defaultValue: "ws-first", allowedValues: []string{"ws-first", "sse-only"}, requiresRestart: true, assign: func(v string) { cfg.OpenAITransportMode = strings.ToLower(v) }},
		fieldSpec{key: "BUTLER_OPENAI_TIMEOUT_SECONDS", component: "orchestrator", typeName: "int", required: false, defaultValue: "60", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { cfg.OpenAITimeoutSeconds = mustParseInt(v) }},
		fieldSpec{key: "BUTLER_TOOL_BROKER_ADDR", component: "orchestrator", typeName: "string", required: false, defaultValue: "127.0.0.1:10090", requiresRestart: true, validate: validateListenAddr, assign: func(v string) { cfg.ToolBrokerAddr = v }},
		fieldSpec{key: "BUTLER_TELEGRAM_BOT_TOKEN", component: "orchestrator", typeName: "string", required: false, defaultValue: "", isSecret: true, requiresRestart: true, validate: validateOptionalNonEmpty, assign: func(v string) { cfg.TelegramBotToken = v }},
		fieldSpec{key: "BUTLER_TELEGRAM_ALLOWED_CHAT_IDS", component: "orchestrator", typeName: "csv", required: false, defaultValue: "", requiresRestart: true, validate: validateOptionalChatIDList, assign: func(v string) { cfg.TelegramAllowedChatIDs = parseCSV(v) }},
		fieldSpec{key: "BUTLER_TELEGRAM_BASE_URL", component: "orchestrator", typeName: "string", required: false, defaultValue: "https://api.telegram.org", requiresRestart: true, validate: validateNonEmptyURL, assign: func(v string) { cfg.TelegramBaseURL = v }},
		fieldSpec{key: "BUTLER_TELEGRAM_POLL_TIMEOUT_SECONDS", component: "orchestrator", typeName: "int", required: false, defaultValue: "25", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { cfg.TelegramPollTimeout = mustParseInt(v) }},
	)
}
