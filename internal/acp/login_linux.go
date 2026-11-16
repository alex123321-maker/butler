//go:build linux

package acp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var loginURLPattern = regexp.MustCompile(`https://accounts\.google\.com/[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]+`)

func RunAntigravityLogin(parent context.Context, process AntigravityProcess, codeFile string, publish func(string)) error {
	if process.Validate() != nil || !filepath.IsAbs(codeFile) || publish == nil || CheckRuntimeNamespace() != nil {
		return ErrProcess
	}
	if _, err := os.Lstat(codeFile); !os.IsNotExist(err) {
		return ErrProcess
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	executable, err := openPinnedExecutable(ctx, ProcessSpec{Executable: process.Executable, SHA256: process.SHA256})
	if err != nil {
		return err
	}
	defer executable.Close()
	root, err := os.MkdirTemp("", "butler-acp-login-")
	if err != nil {
		return ErrProcess
	}
	defer os.RemoveAll(root)
	command := exec.CommandContext(ctx, "/proc/self/fd/3", "acp", "--login", "--disable-telemetry", "--include-default-customizations=false")
	command.ExtraFiles = []*os.File{executable}
	command.Dir = root
	command.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + process.ProfileDir, "XDG_CONFIG_HOME=" + filepath.Join(root, "config"), "XDG_CACHE_HOME=" + filepath.Join(root, "cache"), "XDG_DATA_HOME=" + filepath.Join(root, "data"), "TMPDIR=" + root, "NO_COLOR=1", "LANG=C"}
	if process.Proxy {
		command.Env = append(command.Env, "HTTPS_PROXY=http://127.0.0.1:3128", "HTTP_PROXY=http://127.0.0.1:3128", "https_proxy=http://127.0.0.1:3128", "http_proxy=http://127.0.0.1:3128")
	}
	command.WaitDelay = 250 * time.Millisecond
	stdin, err := command.StdinPipe()
	if err != nil {
		return ErrProcess
	}
	defer stdin.Close()
	stdout, err := command.StdoutPipe()
	if err != nil {
		return ErrProcess
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return ErrProcess
	}
	if command.Start() != nil {
		return ErrProcess
	}
	var published atomic.Bool
	var failure atomic.Int32
	var inputFailure atomic.Int32
	var consumed atomic.Bool
	var readers sync.WaitGroup
	readers.Add(2)
	readURL := func(reader io.Reader) {
		defer readers.Done()
		scanner := bufio.NewScanner(io.LimitReader(reader, 64<<10))
		for scanner.Scan() {
			if reason := classifyLoginFailure(scanner.Text()); reason != 0 {
				failure.Store(reason)
			}
			for _, candidate := range loginURLPattern.FindAllString(scanner.Text(), -1) {
				parsed, err := url.Parse(candidate)
				if err == nil && parsed.Host == "accounts.google.com" && parsed.Query().Get("client_id") != "" && parsed.Query().Get("access_token") == "" && parsed.Query().Get("refresh_token") == "" && published.CompareAndSwap(false, true) {
					publish(candidate)
				}
			}
		}
		if scanner.Err() != nil {
			failure.Store(6)
			cancel()
		}
	}
	go readURL(stdout)
	go readURL(stderr)
	inputDone := make(chan struct{})
	go func() {
		defer close(inputDone)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !published.Load() {
					continue
				}
				info, err := os.Lstat(codeFile)
				if os.IsNotExist(err) {
					continue
				}
				if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 8192 {
					inputFailure.Store(1)
					cancel()
					return
				}
				if info.Size() < 10 {
					continue
				}
				raw, err := readProfileFile(codeFile, 8192)
				if err != nil {
					inputFailure.Store(1)
					cancel()
					return
				}
				code, valid := normalizeLoginCode(string(raw))
				_ = os.Remove(codeFile)
				if !valid {
					inputFailure.Store(2)
					cancel()
					return
				}
				consumed.Store(true)
				_, err = io.WriteString(stdin, code+"\n")
				_ = stdin.Close()
				if err != nil {
					inputFailure.Store(3)
					cancel()
				}
				return
			}
		}
	}()
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	readers.Wait()
	err = <-waited
	cancel()
	<-inputDone
	if err != nil {
		exitCode := -1
		if command.ProcessState != nil {
			exitCode = command.ProcessState.ExitCode()
		}
		return fmt.Errorf("%w: %s; input_status=%d; code_consumed=%t; cli_exit=%d", ErrAuthenticationRequired, loginFailureReason(failure.Load()), inputFailure.Load(), consumed.Load(), exitCode)
	}
	return nil
}

func normalizeLoginCode(raw string) (string, bool) {
	code := strings.TrimSpace(raw)
	if strings.HasPrefix(code, "https://") {
		parsed, err := url.Parse(code)
		if err != nil || parsed.Host != "antigravity.google" || parsed.Path != "/oauth-callback" || len(parsed.Query()["code"]) != 1 {
			return "", false
		}
		code = parsed.Query().Get("code")
	}
	return code, len(code) >= 10 && len(code) <= 8192 && !strings.ContainsAny(code, "\r\n\x00 ")
}

func classifyLoginFailure(line string) int32 {
	line = strings.ToLower(line)
	for index, patterns := range [][]string{{"timed out", "timeout", "deadline exceeded"}, {"invalid_grant", "invalid authorization code", "invalid code", "400 bad request", "failed to exchange"}, {"access_denied", "not eligible", "unsupported region", "403 forbidden"}, {"proxyconnect", "connection refused", "network is unreachable", "tls handshake", "no such host", "failed to connect", "bad gateway"}, {"unexpected eof", "end of file"}} {
		for _, pattern := range patterns {
			if strings.Contains(line, pattern) {
				return int32(index + 1)
			}
		}
	}
	return 0
}

func loginFailureReason(code int32) string {
	switch code {
	case 1:
		return "login_timeout"
	case 2:
		return "oauth_code_rejected"
	case 3:
		return "account_access_denied"
	case 4:
		return "login_network_unavailable"
	case 5:
		return "login_input_closed"
	case 6:
		return "login_log_stream_closed_or_limited"
	default:
		return "official_cli_login_failed"
	}
}
