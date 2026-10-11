package config

import (
	"os"
	"strings"

	"github.com/butler/butler/internal/workspaceexec"
)

func workspaceOrchestratorSpecs(c *OrchestratorConfig) []fieldSpec {
	return []fieldSpec{{key: "BUTLER_WORKSPACE_ENABLED", component: "orchestrator", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { c.WorkspaceEnabled = v == "true" }}}
}
func workspaceBrokerSpecs(c *ToolBrokerConfig) []fieldSpec {
	return []fieldSpec{
		{key: "BUTLER_WORKSPACE_ENABLED", component: "tool-broker", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { c.WorkspaceEnabled = v == "true" }},
		{key: "BUTLER_WORKSPACE_PROFILE_ID", component: "tool-broker", typeName: "string", requiresRestart: true, assign: func(v string) { c.WorkspaceProfileID = strings.TrimSpace(v) }},
		{key: "BUTLER_WORKSPACE_RUNTIME_TARGET", component: "tool-broker", typeName: "string", requiresRestart: true, validate: func(v string) error {
			if v != "" && !workspaceexec.ValidTarget(v) {
				return workspaceexec.ErrDenied
			}
			return nil
		}, assign: func(v string) { c.WorkspaceRuntimeTarget = strings.TrimSpace(v) }},
		{key: "BUTLER_WORKSPACE_RUNTIME_TOKEN_FILE", component: "tool-broker", typeName: "string", requiresRestart: true, assign: func(v string) { c.WorkspaceRuntimeTokenFile = strings.TrimSpace(v) }},
	}
}

type ToolWorkspaceConfig struct {
	Shared                                       SharedConfig
	Postgres                                     PostgresConfig
	ProfileID, Root, TokenFile, DeletionManifest string
}

func LoadToolWorkspaceFromEnv() (ToolWorkspaceConfig, Snapshot, error) {
	return loadToolWorkspace(os.LookupEnv)
}
func loadToolWorkspace(get envGetter) (ToolWorkspaceConfig, Snapshot, error) {
	c := ToolWorkspaceConfig{}
	shared := sharedSpecs("tool-workspace", &c.Shared)
	for i := range shared {
		switch shared[i].key {
		case "BUTLER_HTTP_ADDR":
			shared[i].defaultValue = "127.0.0.1:8086"
			shared[i].validate = func(v string) error {
				if !workspaceexec.ValidLoopback(v) {
					return workspaceexec.ErrDenied
				}
				return nil
			}
		case "BUTLER_GRPC_ADDR":
			shared[i].defaultValue = "127.0.0.1:9096"
			shared[i].validate = func(v string) error {
				if !workspaceexec.ValidTarget(v) {
					return workspaceexec.ErrDenied
				}
				return nil
			}
		}
	}
	specs := append(shared,
		fieldSpec{key: "BUTLER_POSTGRES_URL", component: "tool-workspace", typeName: "string", required: true, isSecret: true, requiresRestart: true, validate: validateNonEmptyURL, assign: func(v string) { c.Postgres.URL = v }},
		fieldSpec{key: "BUTLER_POSTGRES_MAX_CONNS", component: "tool-workspace", typeName: "int", defaultValue: "2", requiresRestart: true, validate: validatePositiveInt, assign: func(v string) { c.Postgres.MaxConns = mustParseInt32(v) }},
		fieldSpec{key: "BUTLER_WORKSPACE_PROFILE_ID", component: "tool-workspace", typeName: "string", required: true, requiresRestart: true, validate: validateNonEmpty, assign: func(v string) { c.ProfileID = strings.TrimSpace(v) }},
		fieldSpec{key: "BUTLER_WORKSPACE_ROOT", component: "tool-workspace", typeName: "string", required: true, isSecret: true, requiresRestart: true, validate: validateNonEmpty, assign: func(v string) { c.Root = v }},
		fieldSpec{key: "BUTLER_WORKSPACE_RUNTIME_TOKEN_FILE", component: "tool-workspace", typeName: "string", required: true, requiresRestart: true, validate: validateNonEmpty, assign: func(v string) { c.TokenFile = v }},
		fieldSpec{key: "BUTLER_MEMORY_DELETION_MANIFEST_FILE", component: "tool-workspace", typeName: "string", required: true, requiresRestart: true, validate: validateNonEmpty, assign: func(v string) { c.DeletionManifest = v }},
	)
	snapshot, err := loadSpecs(get, specs)
	return c, snapshot, err
}
