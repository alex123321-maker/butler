package antigravity

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

//go:embed settings.json
var isolationSettings []byte

// validateProfile reads only non-secret policy configuration. Credential files
// and the CLI keyring remain opaque to Butler.
func validateProfile(profile string) error {
	actualHome, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(profile) || filepath.Clean(actualHome) != filepath.Clean(profile) {
		return protocolError("Antigravity must run under its dedicated private profile")
	}
	info, err := os.Lstat(profile)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return protocolError("Antigravity profile must be a private directory")
	}
	resolved, err := filepath.EvalSymlinks(profile)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(profile) {
		return protocolError("Antigravity profile cannot use filesystem redirects")
	}
	settings, err := os.ReadFile(filepath.Join(profile, ".gemini", "antigravity-cli", "settings.json"))
	if err != nil {
		return protocolError("Antigravity profile is missing its restricted policy")
	}
	var policy struct {
		ToolPermission        string `json:"toolPermission"`
		EnableTerminalSandbox bool   `json:"enableTerminalSandbox"`
		Permissions           struct {
			Allow []string `json:"allow"`
			Ask   []string `json:"ask"`
			Deny  []string `json:"deny"`
		} `json:"permissions"`
		ArtifactReviewPolicy    string   `json:"artifactReviewPolicy"`
		AllowNonWorkspaceAccess bool     `json:"allowNonWorkspaceAccess"`
		UseG1Credits            bool     `json:"useG1Credits"`
		EnableTelemetry         bool     `json:"enableTelemetry"`
		TrustedWorkspaces       []string `json:"trustedWorkspaces"`
		Model                   string   `json:"model"`
	}
	decoder := json.NewDecoder(bytes.NewReader(settings))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&policy) != nil || policy.ToolPermission != "strict" || !policy.EnableTerminalSandbox || len(policy.Permissions.Allow) != 0 || len(policy.Permissions.Ask) != 0 || policy.AllowNonWorkspaceAccess || policy.UseG1Credits || policy.EnableTelemetry || (policy.ArtifactReviewPolicy != "" && policy.ArtifactReviewPolicy != "asks-for-review") {
		return protocolError("Antigravity profile policy differs from the required model-only policy")
	}
	// Interactive login records the empty read-only /work mount as trusted.
	// Runtime turns use a different fresh workspace. Never accept arbitrary
	// host directories, a wildcard, or the profile itself as trusted workspaces.
	for _, workspace := range policy.TrustedWorkspaces {
		if workspace != "/work" {
			return protocolError("Antigravity profile contains an unsupported trusted workspace")
		}
	}
	// CLI 1.3.3 canonicalizes the settings file by omitting empty lists and
	// default-valued optional fields. Require the actual execution restrictions,
	// rather than byte equality with the initial JSON/IDE-oriented settings.
	for _, required := range []string{"command(*)", "unsandboxed(*)", "read_file(*)", "write_file(*)", "read_url(*)", "execute_url(*)", "mcp(*)"} {
		present := false
		for _, denied := range policy.Permissions.Deny {
			if denied == required {
				present = true
				break
			}
		}
		if !present {
			return protocolError("Antigravity profile is missing a required execution deny rule")
		}
	}
	configDir := filepath.Join(profile, ".gemini", "config")
	if entries, err := os.ReadDir(configDir); err == nil {
		for _, entry := range entries {
			if entry.Name() != ".migrated" && entry.Name() != "mcp_config.json" && entry.Name() != "projects" {
				return protocolError("Antigravity profile contains unsupported global customizations")
			}
		}
	} else if !os.IsNotExist(err) {
		return protocolError("cannot verify Antigravity global customization policy")
	}
	if data, err := os.ReadFile(filepath.Join(configDir, "mcp_config.json")); err == nil && strings.TrimSpace(string(data)) != "" {
		var mcp struct {
			Servers map[string]json.RawMessage `json:"mcpServers"`
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&mcp) != nil || len(mcp.Servers) != 0 {
			return protocolError("Antigravity profile must not configure MCP servers")
		}
	} else if err != nil && !os.IsNotExist(err) {
		return protocolError("cannot verify Antigravity MCP configuration")
	}
	for _, directory := range []string{
		filepath.Join(profile, ".gemini", "antigravity-cli", "plugins"),
		filepath.Join(profile, ".gemini", "antigravity-cli", "hooks"),
	} {
		entries, err := os.ReadDir(directory)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || len(entries) != 0 {
			return protocolError("Antigravity profile contains unsupported global customizations")
		}
	}
	return nil
}
