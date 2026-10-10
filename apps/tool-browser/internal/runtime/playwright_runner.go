package runtime

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	browserscripts "github.com/butler/butler/apps/tool-browser/scripts"
	"github.com/butler/butler/internal/security/egress"
	"github.com/butler/butler/internal/security/workspace"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const defaultBrowserScriptPath = "apps/tool-browser/scripts/browser_runtime.mjs"

// PlaywrightRunner owns one helper process. Runtime-only secrets use its
// private stdin, never command arguments or helper stderr.
type PlaywrightRunner struct {
	nodeBinary    string
	scriptPath    string
	mu            sync.Mutex
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	responses     <-chan helperResponse
	policy        egress.Policy
	proxies       map[string]*egress.ScopedProxy
	workspaceRoot string
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
	return &PlaywrightRunner{nodeBinary: nodeBinary, scriptPath: scriptPath, proxies: map[string]*egress.ScopedProxy{}}
}

func NewPlaywrightRunnerWithPolicy(nodeBinary, scriptPath string, policy egress.Policy) *PlaywrightRunner {
	runner := NewPlaywrightRunner(nodeBinary, scriptPath)
	runner.policy = policy
	return runner
}
func (r *PlaywrightRunner) start() error {
	root := r.workspaceRoot
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("browser helper unavailable")
		}
	}
	path, err := resolveBrowserScriptPathFrom(root, r.scriptPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(r.nodeBinary, "--input-type=module", "--eval", browserscripts.Runtime)
	cmd.Dir = filepath.Dir(path)
	for _, name := range []string{"PATH", "HOME", "PLAYWRIGHT_BROWSERS_PATH", "NODE_ENV"} {
		if value, ok := os.LookupEnv(name); ok {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
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
	for id, proxy := range r.proxies {
		if proxy.Expired() {
			_ = proxy.Close()
			delete(r.proxies, id)
		}
	}
	if req.ToolName == "browser.open" || req.ToolName == "browser.read" {
		expires := time.Now().Add(30 * time.Second)
		if req.ToolName == "browser.open" {
			var err error
			expires, err = time.Parse(time.RFC3339Nano, req.ExpiresAt)
			if err != nil {
				return Result{}, egress.ErrDenied
			}
		}
		origin := req.ScopeOrigin
		if origin == "" {
			parsed, err := r.policy.ValidateURL(req.URL)
			if err != nil {
				return Result{}, err
			}
			origin = parsed.Scheme + "://" + parsed.Host
		}
		proxy, err := egress.NewScopedProxy(r.policy, origin, expires, egress.MaxRequestBytes)
		if err != nil {
			return Result{}, err
		}
		req.ProxyURL = proxy.URL()
		if req.ToolName == "browser.read" {
			defer proxy.Close()
		} else {
			if _, exists := r.proxies[req.SessionID]; exists {
				_ = proxy.Close()
				return Result{}, egress.ErrDenied
			}
			r.proxies[req.SessionID] = proxy
		}
	}
	if req.ToolName == "browser.close" {
		defer func() {
			if proxy := r.proxies[req.SessionID]; proxy != nil {
				_ = proxy.Close()
				delete(r.proxies, req.SessionID)
			}
		}()
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
	for id, proxy := range r.proxies {
		_ = proxy.Close()
		delete(r.proxies, id)
	}
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
	root, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("browser helper unavailable")
	}
	return resolveBrowserScriptPathFrom(root, configuredPath)
}

func resolveBrowserScriptPathFrom(root, configuredPath string) (string, error) {
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
	name, err := filepath.Rel(root, path)
	if err != nil {
		return "", fmt.Errorf("browser helper unavailable")
	}
	data, err := workspace.ReadRegular(root, filepath.ToSlash(name), 1024*1024)
	if err != nil || sha256.Sum256(data) != sha256.Sum256([]byte(browserscripts.Runtime)) {
		return "", fmt.Errorf("browser helper is outside its workspace or is not the reviewed binary version")
	}
	return path, nil
}
