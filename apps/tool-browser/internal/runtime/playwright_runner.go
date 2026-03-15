package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type PlaywrightRunner struct{}

func NewPlaywrightRunner() *PlaywrightRunner { return &PlaywrightRunner{} }

func (r *PlaywrightRunner) Run(ctx context.Context, req Request) (Result, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return Result{}, fmt.Errorf("encode browser request: %w", err)
	}
	scriptPath, err := browserScriptPath()
	if err != nil {
		return Result{}, err
	}
	cmd := exec.CommandContext(ctx, "node", scriptPath, string(payload))
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

func browserScriptPath() (string, error) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolve browser script path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "scripts", "browser_runtime.mjs")), nil
}
