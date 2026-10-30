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
func (r *PlaywrightRunner) start(ctx context.Context) error {
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
	// Load the reviewed bytes through private stdin, keeping the command line
	// below Windows' size limit. No writable helper file or code race is added.
	bootstrap := fmt.Sprintf(`import {readSync} from 'node:fs'; const source=Buffer.alloc(%d); let offset=0; while(offset<source.length){const n=readSync(0,source,offset,source.length-offset,null);if(!n)process.exit(1);offset+=n} const code=source.toString('utf8').replace("from 'playwright'",'from '+JSON.stringify(import.meta.resolve('playwright'))); await import('data:text/javascript;base64,'+Buffer.from(code).toString('base64'))`, len([]byte(browserscripts.Runtime)))
	cmd := exec.Command(r.nodeBinary, "--input-type=module", "--eval", bootstrap)
	cmd.Dir = filepath.Dir(path)
	for _, name := range []string{"PATH", "HOME", "PLAYWRIGHT_BROWSERS_PATH", "NODE_ENV", "SYSTEMROOT", "WINDIR", "TMPDIR", "TMP", "TEMP"} {
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
	loaded := make(chan error, 1)
	go func() { _, writeErr := io.WriteString(stdin, browserscripts.Runtime); loaded <- writeErr }()
	select {
	case err = <-loaded:
	case <-ctx.Done():
		err = ctx.Err()
	}
	if err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("load reviewed browser helper")
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
		policy := r.policy
		// Contract caps and operator caps intersect; neither replaces the other.
		// The helper checks the contract on every request; the proxy retains
		// the independently configured operator policy and public-IP checks.
		proxy, err := egress.NewBrowserProxy(policy, origin, expires, egress.MaxRequestBytes)
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
	if req.ToolName == "browser.prepare_fill" || req.ToolName == "browser.prepare_type" || req.ToolName == "browser.freeze_snapshot" {
		if proxy := r.proxies[req.SessionID]; proxy != nil {
			proxy.Freeze()
		}
	}
	if req.ToolName == "browser.click" || req.ManagedPages && (req.ToolName == "browser.fill" || req.ToolName == "browser.type") {
		if proxy := r.proxies[req.SessionID]; proxy != nil {
			deadline := time.Now().Add(35 * time.Second)
			if owned, ok := ctx.Deadline(); ok && owned.Before(deadline) {
				deadline = owned
			}
			if err := proxy.AllowUntil(deadline); err != nil {
				return Result{}, err
			}
			defer proxy.Freeze()
		}
	}
	if req.OwnerCommand != nil && req.OwnerCommand.Operation == "take" {
		if proxy := r.proxies[req.SessionID]; proxy != nil {
			if err := proxy.AllowUntil(proxy.Deadline()); err != nil {
				return Result{}, err
			}
		}
	}
	if req.OwnerCommand != nil && req.OwnerCommand.Operation == "release" {
		if proxy := r.proxies[req.SessionID]; proxy != nil {
			proxy.Freeze()
		}
	}
	if r.cmd == nil {
		if req.ToolName != "browser.open" && req.ToolName != "browser.read" {
			return Result{Error: &ResultError{ErrorType: "session_lost", Message: "browser process lost; explicitly open a new session"}}, nil
		}
		if err := r.start(ctx); err != nil {
			return Result{}, err
		}
	}
	if req.ToolName == "browser.submit_get_form" || req.ToolName == "browser.follow_link" {
		proxy := r.proxies[req.SessionID]
		if proxy == nil {
			return Result{}, egress.ErrDenied
		}
		// Compile the form while every connection is closed. The model cannot
		// supply either private phase or the one-use helper nonce.
		proxy.Freeze()
		defer proxy.Freeze()
		req.GetFormPhase = "plan"
		plan, err := r.exchange(ctx, req)
		if err != nil || !plan.OK {
			return plan, err
		}
		target, err := r.policy.ValidateURL(plan.GetFormTarget)
		if err != nil || target.User != nil || originOf(plan.GetFormTarget) != originOf(req.ExpectedURL) || len(plan.GetFormNonce) != 36 {
			notDispatched := false
			return Result{ReadRejection: "pre_navigation_v1", NavigationDispatched: &notDispatched, Error: &ResultError{ErrorType: "get_form_denied", Message: "GET form target is outside the permitted public origin"}}, nil
		}
		deadline := time.Now().Add(35 * time.Second)
		if owned, ok := ctx.Deadline(); ok && owned.Before(deadline) {
			deadline = owned
		}
		if err := proxy.AllowUntil(deadline); err != nil {
			return Result{}, err
		}
		req.GetFormPhase, req.GetFormNonce = "navigate", plan.GetFormNonce
		return r.exchange(ctx, req)
	}
	if req.ToolName == "browser.login" {
		return r.login(ctx, req)
	}
	return r.exchange(ctx, req)
}

// exchange is called only with the runner mutex held. Errors invalidate the
// helper generation; no phase or gesture is automatically re-dispatched.
func (r *PlaywrightRunner) exchange(ctx context.Context, req Request) (Result, error) {
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
		if req.OwnerCommand != nil && req.OwnerCommand.Operation == "release" && response.result.OK {
			if proxy := r.proxies[req.SessionID]; proxy != nil {
				if err := proxy.AllowUntil(proxy.Deadline()); err != nil {
					return Result{}, err
				}
			}
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
