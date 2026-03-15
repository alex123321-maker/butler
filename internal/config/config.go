package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/butler/butler/internal/modelprovider"
)

type ValidationStatus string

const (
	ValidationStatusValid   ValidationStatus = "valid"
	ValidationStatusInvalid ValidationStatus = "invalid"
	ValidationStatusMissing ValidationStatus = "missing"
)

type ConfigKeyInfo struct {
	Key              string
	Component        string
	Type             string
	Required         bool
	DefaultValue     string
	EffectiveValue   string
	Source           string
	IsSecret         bool
	RequiresRestart  bool
	ValidationStatus ValidationStatus
	ValidationError  string
}

type Introspector interface {
	ListKeys() []ConfigKeyInfo
}

type Snapshot struct {
	keys []ConfigKeyInfo
}

func (s Snapshot) ListKeys() []ConfigKeyInfo {
	keys := make([]ConfigKeyInfo, len(s.keys))
	copy(keys, s.keys)
	return keys
}

type SharedConfig struct {
	ServiceName string
	LogLevel    string
	HTTPAddr    string
	GRPCAddr    string
	Environment string
}

type OrchestratorConfig struct {
	Shared                           SharedConfig
	Postgres                         PostgresConfig
	Redis                            RedisConfig
	PostgresURL                      string
	RedisURL                         string
	ModelProvider                    string
	OpenAIAPIKey                     string
	OpenAIModel                      string
	OpenAIBaseURL                    string
	OpenAIRealtimeURL                string
	OpenAITransportMode              string
	OpenAICodexModel                 string
	OpenAICodexBaseURL               string
	GitHubCopilotModel               string
	OpenAITimeoutSeconds             int
	ToolBrokerAddr                   string
	TelegramBotToken                 string
	TelegramAllowedChatIDs           []string
	TelegramBaseURL                  string
	TelegramPollTimeout              int
	SessionLeaseTTLSeconds           int
	MemoryProfileLimit               int
	MemoryEpisodicLimit              int
	MemoryScopeOrder                 []string
	MemoryWorkingTransientTTLSeconds int
}

type ToolBrokerConfig struct {
	Shared        SharedConfig
	Postgres      PostgresConfig
	RegistryPath  string
	DefaultTarget string
}

type ToolBrowserConfig struct {
	Shared           SharedConfig
	NodeBinary       string
	HelperScriptPath string
}

type ToolHTTPConfig struct {
	Shared SharedConfig
}

type ToolDoctorConfig struct {
	Shared           SharedConfig
	Postgres         PostgresConfig
	Redis            RedisConfig
	PostgresURL      string
	RedisURL         string
	OpenAIAPIKey     string
	OpenAIBaseURL    string
	OpenAIModel      string
	ContainerTargets []DoctorContainerTarget
}

type DoctorContainerTarget struct {
	Name string
	URL  string
}

type PostgresConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime int
	MigrationsDir   string
}

type RedisConfig struct {
	URL string
}

type envGetter func(string) (string, bool)

type fieldSpec struct {
	key             string
	component       string
	typeName        string
	required        bool
	defaultValue    string
	isSecret        bool
	requiresRestart bool
	allowedValues   []string
	validate        func(string) error
	assign          func(string)
}

func lookupNonEmptyEnv(get envGetter, key string) (string, bool) {
	if get == nil {
		return "", false
	}
	value, ok := get(key)
	if !ok {
		return "", false
	}
	if strings.TrimSpace(value) == "" {
		return "", false
	}
	return value, true
}

func LoadOrchestratorFromEnv() (OrchestratorConfig, Snapshot, error) {
	return loadOrchestrator(os.LookupEnv)
}

func LoadToolBrokerFromEnv() (ToolBrokerConfig, Snapshot, error) {
	return loadToolBroker(os.LookupEnv)
}

func LoadToolBrowserFromEnv() (ToolBrowserConfig, Snapshot, error) {
	return loadToolBrowser(os.LookupEnv)
}

func LoadToolHTTPFromEnv() (ToolHTTPConfig, Snapshot, error) {
	return loadToolHTTP(os.LookupEnv)
}

func LoadToolDoctorFromEnv() (ToolDoctorConfig, Snapshot, error) {
	return loadToolDoctor(os.LookupEnv)
}

func loadOrchestrator(get envGetter) (OrchestratorConfig, Snapshot, error) {
	cfg := OrchestratorConfig{}
	sharedSpecs := sharedSpecs("orchestrator", &cfg.Shared)
	specs := append(sharedSpecs,
		fieldSpec{key: "BUTLER_POSTGRES_URL", component: "orchestrator", typeName: "string", required: true, isSecret: true, requiresRestart: true, validate: validateNonEmptyURL, assign: func(v string) { cfg.PostgresURL = v; cfg.Postgres.URL = v }},
		fieldSpec{key: "BUTLER_REDIS_URL", component: "orchestrator", typeName: "string", required: true, isSecret: true, requiresRestart: true, validate: validateNonEmptyURL, assign: func(v string) { cfg.RedisURL = v; cfg.Redis.URL = v }},
		fieldSpec{key: "BUTLER_SESSION_LEASE_TTL_SECONDS", component: "orchestrator", typeName: "int", required: false, defaultValue: "60", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { cfg.SessionLeaseTTLSeconds = mustParseInt(v) }},
		fieldSpec{key: "BUTLER_POSTGRES_MAX_CONNS", component: "orchestrator", typeName: "int", required: false, defaultValue: "10", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { cfg.Postgres.MaxConns = mustParseInt32(v) }},
		fieldSpec{key: "BUTLER_POSTGRES_MIN_CONNS", component: "orchestrator", typeName: "int", required: false, defaultValue: "2", requiresRestart: true, validate: validateNonNegativeInt, assign: func(v string) { cfg.Postgres.MinConns = mustParseInt32(v) }},
		fieldSpec{key: "BUTLER_POSTGRES_MAX_CONN_LIFETIME_SECONDS", component: "orchestrator", typeName: "int", required: false, defaultValue: "1800", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { cfg.Postgres.MaxConnLifetime = mustParseInt(v) }},
		fieldSpec{key: "BUTLER_POSTGRES_MIGRATIONS_DIR", component: "orchestrator", typeName: "string", required: false, defaultValue: "migrations", requiresRestart: true, validate: validateNonEmpty, assign: func(v string) { cfg.Postgres.MigrationsDir = v }},
		fieldSpec{key: "BUTLER_MODEL_PROVIDER", component: "orchestrator", typeName: "string", required: false, defaultValue: modelprovider.ProviderOpenAI, allowedValues: modelprovider.SupportedProviders(), requiresRestart: true, assign: func(v string) { cfg.ModelProvider = strings.ToLower(strings.TrimSpace(v)) }},
		fieldSpec{key: "BUTLER_OPENAI_API_KEY", component: "orchestrator", typeName: "string", required: false, defaultValue: "", isSecret: true, requiresRestart: true, validate: validateOptionalNonEmpty, assign: func(v string) { cfg.OpenAIAPIKey = v }},
		fieldSpec{key: "BUTLER_OPENAI_MODEL", component: "orchestrator", typeName: "string", required: false, defaultValue: "gpt-4o-mini", requiresRestart: true, validate: validateNonEmpty, assign: func(v string) { cfg.OpenAIModel = v }},
		fieldSpec{key: "BUTLER_OPENAI_BASE_URL", component: "orchestrator", typeName: "string", required: false, defaultValue: "https://api.openai.com/v1", requiresRestart: true, validate: validateNonEmptyURL, assign: func(v string) { cfg.OpenAIBaseURL = v }},
		fieldSpec{key: "BUTLER_OPENAI_REALTIME_URL", component: "orchestrator", typeName: "string", required: false, defaultValue: "wss://api.openai.com/v1/realtime", requiresRestart: true, validate: validateNonEmptyURL, assign: func(v string) { cfg.OpenAIRealtimeURL = v }},
		fieldSpec{key: "BUTLER_OPENAI_TRANSPORT_MODE", component: "orchestrator", typeName: "string", required: false, defaultValue: "ws-first", allowedValues: []string{"ws-first", "sse-only"}, requiresRestart: true, assign: func(v string) { cfg.OpenAITransportMode = strings.ToLower(v) }},
		fieldSpec{key: "BUTLER_OPENAI_CODEX_MODEL", component: "orchestrator", typeName: "string", required: false, defaultValue: "gpt-5.1-codex", requiresRestart: true, validate: validateNonEmpty, assign: func(v string) { cfg.OpenAICodexModel = v }},
		fieldSpec{key: "BUTLER_OPENAI_CODEX_BASE_URL", component: "orchestrator", typeName: "string", required: false, defaultValue: "https://chatgpt.com/backend-api", requiresRestart: true, validate: validateNonEmptyURL, assign: func(v string) { cfg.OpenAICodexBaseURL = v }},
		fieldSpec{key: "BUTLER_GITHUB_COPILOT_MODEL", component: "orchestrator", typeName: "string", required: false, defaultValue: "gpt-4o", requiresRestart: true, validate: validateNonEmpty, assign: func(v string) { cfg.GitHubCopilotModel = v }},
		fieldSpec{key: "BUTLER_OPENAI_TIMEOUT_SECONDS", component: "orchestrator", typeName: "int", required: false, defaultValue: "60", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { cfg.OpenAITimeoutSeconds = mustParseInt(v) }},
		fieldSpec{key: "BUTLER_TOOL_BROKER_ADDR", component: "orchestrator", typeName: "string", required: false, defaultValue: "127.0.0.1:10090", requiresRestart: true, validate: validateListenAddr, assign: func(v string) { cfg.ToolBrokerAddr = v }},
		fieldSpec{key: "BUTLER_TELEGRAM_BOT_TOKEN", component: "orchestrator", typeName: "string", required: false, defaultValue: "", isSecret: true, requiresRestart: true, validate: validateOptionalNonEmpty, assign: func(v string) { cfg.TelegramBotToken = v }},
		fieldSpec{key: "BUTLER_TELEGRAM_ALLOWED_CHAT_IDS", component: "orchestrator", typeName: "csv", required: false, defaultValue: "", requiresRestart: true, validate: validateOptionalChatIDList, assign: func(v string) { cfg.TelegramAllowedChatIDs = parseCSV(v) }},
		fieldSpec{key: "BUTLER_TELEGRAM_BASE_URL", component: "orchestrator", typeName: "string", required: false, defaultValue: "https://api.telegram.org", requiresRestart: true, validate: validateNonEmptyURL, assign: func(v string) { cfg.TelegramBaseURL = v }},
		fieldSpec{key: "BUTLER_TELEGRAM_POLL_TIMEOUT_SECONDS", component: "orchestrator", typeName: "int", required: false, defaultValue: "25", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { cfg.TelegramPollTimeout = mustParseInt(v) }},
		fieldSpec{key: "BUTLER_MEMORY_PROFILE_LIMIT", component: "orchestrator", typeName: "int", required: false, defaultValue: "20", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { cfg.MemoryProfileLimit = mustParseInt(v) }},
		fieldSpec{key: "BUTLER_MEMORY_EPISODIC_LIMIT", component: "orchestrator", typeName: "int", required: false, defaultValue: "3", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { cfg.MemoryEpisodicLimit = mustParseInt(v) }},
		fieldSpec{key: "BUTLER_MEMORY_SCOPE_ORDER", component: "orchestrator", typeName: "csv", required: false, defaultValue: "session,user,global", requiresRestart: true, validate: validateMemoryScopeOrder, assign: func(v string) { cfg.MemoryScopeOrder = parseCSV(v) }},
		fieldSpec{key: "BUTLER_MEMORY_WORKING_TRANSIENT_TTL_SECONDS", component: "orchestrator", typeName: "int", required: false, defaultValue: "1800", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { cfg.MemoryWorkingTransientTTLSeconds = mustParseInt(v) }},
	)

	snapshot, err := loadSpecs(get, specs)
	if conditionalErr := validateOrchestratorConditionalConfig(cfg); conditionalErr != nil {
		if err != nil {
			return cfg, snapshot, fmt.Errorf("%v; %v", err, conditionalErr)
		}
		return cfg, snapshot, conditionalErr
	}
	return cfg, snapshot, err
}

func loadToolBroker(get envGetter) (ToolBrokerConfig, Snapshot, error) {
	cfg := ToolBrokerConfig{}
	sharedSpecs := sharedSpecs("tool-broker", &cfg.Shared)
	specs := append(sharedSpecs,
		fieldSpec{key: "BUTLER_POSTGRES_URL", component: "tool-broker", typeName: "string", required: false, isSecret: true, requiresRestart: true, validate: validateOptionalURL, assign: func(v string) { cfg.Postgres.URL = v }},
		fieldSpec{key: "BUTLER_POSTGRES_MAX_CONNS", component: "tool-broker", typeName: "int", required: false, defaultValue: "10", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { cfg.Postgres.MaxConns = mustParseInt32(v) }},
		fieldSpec{key: "BUTLER_POSTGRES_MIN_CONNS", component: "tool-broker", typeName: "int", required: false, defaultValue: "2", requiresRestart: true, validate: validateNonNegativeInt, assign: func(v string) { cfg.Postgres.MinConns = mustParseInt32(v) }},
		fieldSpec{key: "BUTLER_POSTGRES_MAX_CONN_LIFETIME_SECONDS", component: "tool-broker", typeName: "int", required: false, defaultValue: "1800", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { cfg.Postgres.MaxConnLifetime = mustParseInt(v) }},
		fieldSpec{key: "BUTLER_POSTGRES_MIGRATIONS_DIR", component: "tool-broker", typeName: "string", required: false, defaultValue: "migrations", requiresRestart: true, validate: validateNonEmpty, assign: func(v string) { cfg.Postgres.MigrationsDir = v }},
		fieldSpec{key: "BUTLER_TOOL_REGISTRY_PATH", component: "tool-broker", typeName: "string", required: false, defaultValue: "configs/tools.json", requiresRestart: true, validate: validateNonEmpty, assign: func(v string) { cfg.RegistryPath = v }},
		fieldSpec{key: "BUTLER_TOOL_DEFAULT_TARGET", component: "tool-broker", typeName: "string", required: false, defaultValue: "local", requiresRestart: true, validate: validateNonEmpty, assign: func(v string) { cfg.DefaultTarget = v }},
	)

	snapshot, err := loadSpecs(get, specs)
	return cfg, snapshot, err
}

func loadToolBrowser(get envGetter) (ToolBrowserConfig, Snapshot, error) {
	cfg := ToolBrowserConfig{}
	specs := append(sharedSpecs("tool-browser", &cfg.Shared),
		fieldSpec{key: "BUTLER_TOOL_BROWSER_NODE_BINARY", component: "tool-browser", typeName: "string", required: false, defaultValue: "node", requiresRestart: true, validate: validateNonEmpty, assign: func(v string) { cfg.NodeBinary = v }},
		fieldSpec{key: "BUTLER_TOOL_BROWSER_SCRIPT_PATH", component: "tool-browser", typeName: "string", required: false, defaultValue: "apps/tool-browser/scripts/browser_runtime.mjs", requiresRestart: true, validate: validateNonEmpty, assign: func(v string) { cfg.HelperScriptPath = v }},
	)
	snapshot, err := loadSpecs(get, specs)
	return cfg, snapshot, err
}

func loadToolHTTP(get envGetter) (ToolHTTPConfig, Snapshot, error) {
	cfg := ToolHTTPConfig{}
	snapshot, err := loadSpecs(get, sharedSpecs("tool-http", &cfg.Shared))
	return cfg, snapshot, err
}

func loadToolDoctor(get envGetter) (ToolDoctorConfig, Snapshot, error) {
	cfg := ToolDoctorConfig{}
	shared := sharedSpecs("tool-doctor", &cfg.Shared)
	specs := append(shared,
		fieldSpec{key: "BUTLER_POSTGRES_URL", component: "tool-doctor", typeName: "string", required: false, defaultValue: "", isSecret: true, requiresRestart: true, validate: validateOptionalURL, assign: func(v string) { cfg.PostgresURL = v; cfg.Postgres.URL = v }},
		fieldSpec{key: "BUTLER_REDIS_URL", component: "tool-doctor", typeName: "string", required: false, defaultValue: "", isSecret: true, requiresRestart: true, validate: validateOptionalURL, assign: func(v string) { cfg.RedisURL = v; cfg.Redis.URL = v }},
		fieldSpec{key: "BUTLER_OPENAI_API_KEY", component: "tool-doctor", typeName: "string", required: false, defaultValue: "", isSecret: true, requiresRestart: true, validate: validateOptionalNonEmpty, assign: func(v string) { cfg.OpenAIAPIKey = v }},
		fieldSpec{key: "BUTLER_OPENAI_BASE_URL", component: "tool-doctor", typeName: "string", required: false, defaultValue: "https://api.openai.com/v1", requiresRestart: true, validate: validateNonEmptyURL, assign: func(v string) { cfg.OpenAIBaseURL = v }},
		fieldSpec{key: "BUTLER_OPENAI_MODEL", component: "tool-doctor", typeName: "string", required: false, defaultValue: "gpt-4o-mini", requiresRestart: true, validate: validateNonEmpty, assign: func(v string) { cfg.OpenAIModel = v }},
		fieldSpec{key: "BUTLER_DOCTOR_CONTAINER_TARGETS", component: "tool-doctor", typeName: "csv", required: false, defaultValue: "", requiresRestart: true, validate: validateOptionalDoctorContainerTargets, assign: func(v string) { cfg.ContainerTargets = parseDoctorContainerTargets(v) }},
		fieldSpec{key: "BUTLER_POSTGRES_MAX_CONNS", component: "tool-doctor", typeName: "int", required: false, defaultValue: "4", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { cfg.Postgres.MaxConns = mustParseInt32(v) }},
		fieldSpec{key: "BUTLER_POSTGRES_MIN_CONNS", component: "tool-doctor", typeName: "int", required: false, defaultValue: "1", requiresRestart: true, validate: validateNonNegativeInt, assign: func(v string) { cfg.Postgres.MinConns = mustParseInt32(v) }},
		fieldSpec{key: "BUTLER_POSTGRES_MAX_CONN_LIFETIME_SECONDS", component: "tool-doctor", typeName: "int", required: false, defaultValue: "30", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { cfg.Postgres.MaxConnLifetime = mustParseInt(v) }},
	)
	snapshot, err := loadSpecs(get, specs)
	return cfg, snapshot, err
}

func sharedSpecs(serviceName string, cfg *SharedConfig) []fieldSpec {
	return []fieldSpec{
		{key: "BUTLER_SERVICE_NAME", component: serviceName, typeName: "string", required: false, defaultValue: serviceName, requiresRestart: true, validate: validateNonEmpty, assign: func(v string) { cfg.ServiceName = v }},
		{key: "BUTLER_LOG_LEVEL", component: serviceName, typeName: "string", required: false, defaultValue: "info", allowedValues: []string{"debug", "info", "warn", "error"}, requiresRestart: false, assign: func(v string) { cfg.LogLevel = strings.ToLower(v) }},
		{key: "BUTLER_HTTP_ADDR", component: serviceName, typeName: "string", required: false, defaultValue: ":8080", requiresRestart: true, validate: validateListenAddr, assign: func(v string) { cfg.HTTPAddr = v }},
		{key: "BUTLER_GRPC_ADDR", component: serviceName, typeName: "string", required: false, defaultValue: ":9090", requiresRestart: true, validate: validateListenAddr, assign: func(v string) { cfg.GRPCAddr = v }},
		{key: "BUTLER_ENVIRONMENT", component: serviceName, typeName: "string", required: false, defaultValue: "development", allowedValues: []string{"development", "test", "production"}, requiresRestart: false, assign: func(v string) { cfg.Environment = strings.ToLower(v) }},
	}
}

func loadSpecs(get envGetter, specs []fieldSpec) (Snapshot, error) {
	keys := make([]ConfigKeyInfo, 0, len(specs))
	var problems []string

	for _, spec := range specs {
		value, ok := lookupNonEmptyEnv(get, spec.key)
		source := "default"
		if ok {
			source = "env"
		} else {
			value = spec.defaultValue
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

		if spec.required && strings.TrimSpace(value) == "" {
			info.ValidationStatus = ValidationStatusMissing
			info.ValidationError = "required value is missing"
			keys = append(keys, finalizeInfo(info, value))
			problems = append(problems, fmt.Sprintf("%s: %s", spec.key, info.ValidationError))
			continue
		}

		if strings.TrimSpace(value) == "" && !spec.required {
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
			problems = append(problems, fmt.Sprintf("%s: %s", spec.key, err.Error()))
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

func finalizeInfo(info ConfigKeyInfo, rawValue string) ConfigKeyInfo {
	if info.IsSecret {
		info.EffectiveValue = maskedPresence(rawValue)
		return info
	}
	info.EffectiveValue = rawValue
	return info
}

func maskedPresence(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "[masked]"
}

func validateValue(spec fieldSpec, value string) error {
	if len(spec.allowedValues) > 0 {
		normalized := strings.ToLower(value)
		for _, allowed := range spec.allowedValues {
			if normalized == allowed {
				goto custom
			}
		}
		return fmt.Errorf("must be one of %s", strings.Join(spec.allowedValues, ", "))
	}

custom:
	if spec.validate != nil {
		return spec.validate(value)
	}
	return nil
}

func validateNonEmpty(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("must not be empty")
	}
	return nil
}

func validateOptionalNonEmpty(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return validateNonEmpty(value)
}

func validateNonEmptyURL(value string) error {
	if err := validateNonEmpty(value); err != nil {
		return err
	}
	if !strings.Contains(value, "://") {
		return fmt.Errorf("must be a URL")
	}
	return nil
}

func validateOptionalURL(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return validateNonEmptyURL(value)
}

func validateOptionalChatIDList(value string) error {
	for _, item := range parseCSV(value) {
		if _, err := strconv.ParseInt(item, 10, 64); err != nil {
			return fmt.Errorf("must be a comma-separated list of Telegram chat ids")
		}
	}
	return nil
}

func validateMemoryScopeOrder(value string) error {
	allowed := map[string]struct{}{"session": {}, "user": {}, "global": {}}
	for _, scope := range parseCSV(value) {
		if _, ok := allowed[strings.ToLower(scope)]; !ok {
			return fmt.Errorf("must contain only session, user, or global")
		}
	}
	return nil
}

func validateOptionalDoctorContainerTargets(value string) error {
	for _, target := range parseDoctorContainerTargets(value) {
		if strings.TrimSpace(target.Name) == "" {
			return fmt.Errorf("doctor container targets require a name")
		}
		if err := validateNonEmptyURL(target.URL); err != nil {
			return fmt.Errorf("doctor container target %q: %w", target.Name, err)
		}
	}
	return nil
}

func validateListenAddr(value string) error {
	if err := validateNonEmpty(value); err != nil {
		return err
	}
	if !strings.Contains(value, ":") {
		return fmt.Errorf("must include host or port separator")
	}
	return nil
}

func parseInt(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("must be an integer")
	}
	return parsed, nil
}

func validatePositiveInt(value string) error {
	parsed, err := parseInt(value)
	if err != nil {
		return err
	}
	if parsed <= 0 {
		return fmt.Errorf("must be greater than zero")
	}
	return nil
}

func validateNonNegativeInt(value string) error {
	parsed, err := parseInt(value)
	if err != nil {
		return err
	}
	if parsed < 0 {
		return fmt.Errorf("must be zero or greater")
	}
	return nil
}

func mustParseInt(value string) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func mustParseInt32(value string) int32 {
	return int32(mustParseInt(value))
}

func parseCSV(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		result = append(result, item)
	}
	return result
}

func parseDoctorContainerTargets(value string) []DoctorContainerTarget {
	items := parseCSV(value)
	if len(items) == 0 {
		return nil
	}
	result := make([]DoctorContainerTarget, 0, len(items))
	for _, item := range items {
		name, url, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		result = append(result, DoctorContainerTarget{Name: strings.TrimSpace(name), URL: strings.TrimSpace(url)})
	}
	return result
}

func validateOrchestratorConditionalConfig(cfg OrchestratorConfig) error {
	if strings.TrimSpace(cfg.TelegramBotToken) != "" && len(cfg.TelegramAllowedChatIDs) == 0 {
		return fmt.Errorf("BUTLER_TELEGRAM_ALLOWED_CHAT_IDS is required when BUTLER_TELEGRAM_BOT_TOKEN is set")
	}
	return nil
}
