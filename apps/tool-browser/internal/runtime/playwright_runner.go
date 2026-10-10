package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

const defaultBrowserScriptPath = "apps/tool-browser/scripts/browser_runtime.mjs"

// PlaywrightRunner owns one helper process. Runtime-only secrets use its
// private stdin, never command arguments or helper stderr.
type PlaywrightRunner struct {
	nodeBinary string
	scriptPath string
	mu         sync.Mutex
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	responses  <-chan helperResponse
}
type helperResponse struct {
	result Result
	err    error
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
func (r *PlaywrightRunner) start() error {
	path, err := resolveBrowserScriptPath(r.scriptPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(r.nodeBinary, path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open browser helper input: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("open browser helper output: %w", err)
	}
	// Playwright diagnostics may contain URLs, values or page content.
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start browser helper: %w", err)
	}
	responses := make(chan helperResponse, 1)
	r.cmd, r.stdin, r.responses = cmd, stdin, responses
	go func() {
		defer close(responses)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 2*1024*1024)
		for scanner.Scan() {
			var result Result
			if err := json.Unmarshal(scanner.Bytes(), &result); err != nil {
				responses <- helperResponse{err: fmt.Errorf("browser helper returned an invalid response")}
				break
			}
			responses <- helperResponse{result: result}
		}
	}()
	return nil
}
func (r *PlaywrightRunner) Run(ctx context.Context, req Request) (Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if r.cmd == nil {
		if req.ToolName != "browser.open" && req.ToolName != "browser.read" {
			return Result{Error: &ResultError{ErrorType: "session_lost", Message: "browser process lost; explicitly open a new session"}}, nil
		}
		if err := r.start(); err != nil {
			return Result{}, err
		}
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return Result{}, fmt.Errorf("encode browser request")
	}
	if _, err = r.stdin.Write(append(payload, '\n')); err != nil {
		r.stop()
		return Result{}, fmt.Errorf("browser process lost; previous sessions are invalid")
	}
	select {
	case response, ok := <-r.responses:
		if !ok || response.err != nil {
			r.stop()
			return Result{}, fmt.Errorf("browser process lost; previous sessions are invalid")
		}
		return response.result, nil
	case <-ctx.Done():
		// The effect may have started: invalidate instead of replaying it.
		r.stop()
		return Result{}, fmt.Errorf("browser execution interrupted; effect outcome unknown and previous sessions invalid")
	}
}
func (r *PlaywrightRunner) stop() {
	if r.cmd != nil {
		_ = r.stdin.Close()
		_ = r.cmd.Process.Kill()
		_ = r.cmd.Wait()
		for range r.responses {
		}
	}
	r.cmd, r.stdin, r.responses = nil, nil, nil
}
func (r *PlaywrightRunner) Close() error { r.mu.Lock(); defer r.mu.Unlock(); r.stop(); return nil }
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
		return "", fmt.Errorf("browser script is not available: %w", err)
	}
	return path, nil
}
