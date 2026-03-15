package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const defaultBrowserScriptPath = "apps/tool-browser/scripts/browser_runtime.mjs"

type PlaywrightRunner struct {
	nodeBinary string
	scriptPath string
}

func NewPlaywrightRunner(nodeBinary, scriptPath string) *PlaywrightRunner {
	if strings.TrimSpace(nodeBinary) == "" {
		nodeBinary = "node"
	}
	if strings.TrimSpace(scriptPath) == "" {
		scriptPath = defaultBrowserScriptPath
	}
	return &PlaywrightRunner{nodeBinary: nodeBinary, scriptPath: scriptPath}
}

func (r *PlaywrightRunner) Run(ctx context.Context, req Request) (Result, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return Result{}, fmt.Errorf("encode browser request: %w", err)
	}
	scriptPath, err := resolveBrowserScriptPath(r.scriptPath)
	if err != nil {
		return Result{}, err
	}
	cmd := exec.CommandContext(ctx, r.nodeBinary, scriptPath, string(payload))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return Result{}, fmt.Errorf("run playwright helper: %w: %s", err, strings.TrimSpace(string(output)))
	}
	var result Result
	if err := json.Unmarshal(output, &result); err != nil {
		return Result{}, fmt.Errorf("decode playwright output: %w", err)
	}
	return result, nil
}

func resolveBrowserScriptPath(configuredPath string) (string, error) {
	path := strings.TrimSpace(configuredPath)
	if path == "" {
		path = defaultBrowserScriptPath
	}
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("resolve browser script path: %w", err)
		}
		path = absPath
	}
	path = filepath.Clean(path)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("browser script path %q is not available: %w", path, err)
	}
	return path, nil
}
