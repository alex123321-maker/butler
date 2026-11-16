//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/butler/butler/internal/acp"
)

func main() {
	socket := flag.String("socket", "/run/acp/agent.sock", "private runtime socket")
	executable := flag.String("executable", "/usr/local/bin/agy", "pinned official runtime")
	digest := flag.String("sha256", "", "runtime digest")
	profile := flag.String("profile", "/profile/home", "dedicated CLI profile")
	model := flag.String("model", "gemini-3.8-flash", "exact Gemini model")
	egress := flag.Bool("egress", false, "serve fixed Google-only egress instead of inference")
	upstream := flag.String("upstream-proxy", "", "operator HTTP CONNECT route for checked public Google IPs; no credentials")
	egressSocket := flag.String("egress-socket", "", "Google egress relay socket")
	login := flag.Bool("login", false, "operator-only official Google login; no inference")
	codeFile := flag.String("login-code-file", "/tmp/login-code", "private one-use operator code file")
	check := flag.Bool("check", false, "negotiate model without dispatching an inference prompt")
	flag.Parse()
	syscall.Umask(0077)
	if !filepath.IsAbs(*socket) {
		log.Fatal("invalid private socket")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *check {
		ready := errors.New("ready without prompt")
		_, err := acp.RunRemoteAntigravity(ctx, *socket, *model, "preflight", func(context.Context) error { return ready })
		if errors.Is(err, ready) {
			log.Print("ACP model negotiation ready; no inference dispatched")
			return
		}
		log.Fatal(err)
	}
	if !*egress && acp.CheckRuntimeNamespace() != nil {
		log.Fatal("runtime requires non-root private Docker PID1, no privileges and no capabilities")
	}
	if entry, err := os.Lstat(*socket); !*login && err == nil {
		if entry.Mode()&os.ModeSocket == 0 {
			log.Fatal("unsafe socket path")
		}
		connection, err := net.DialTimeout("unix", *socket, 100*time.Millisecond)
		if err == nil {
			connection.Close()
			log.Fatal("runtime socket already active")
		}
		if os.Remove(*socket) != nil {
			log.Fatal("stale socket cleanup failed")
		}
	}
	if *egress {
		listener, err := net.Listen("unix", *socket)
		if err != nil || os.Chmod(*socket, 0600) != nil {
			log.Fatal("private socket unavailable")
		}
		defer listener.Close()
		if acp.ServeGoogleEgressVia(ctx, listener, *upstream) != nil {
			log.Fatal("Google egress stopped")
		}
		return
	}
	process := acp.AntigravityProcess{Executable: *executable, SHA256: *digest, ProfileDir: *profile}
	if acp.PrepareAntigravityProfile(*profile) != nil {
		log.Fatal("managed profile unavailable")
	}
	if *egressSocket != "" {
		if !filepath.IsAbs(*egressSocket) {
			log.Fatal("invalid Google egress socket")
		}
		proxy, err := net.Listen("tcp", "127.0.0.1:3128")
		if err != nil {
			log.Fatal("local proxy unavailable")
		}
		defer proxy.Close()
		go func() {
			if acp.ServeLocalProxy(ctx, proxy, *egressSocket) != nil {
				stop()
			}
		}()
		process.Proxy = true
	}
	if *login {
		err := acp.RunAntigravityLogin(ctx, process, *codeFile, func(url string) {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"event": "login_url", "url": url})
		})
		if cleanupErr := acp.KillRuntimeChildren(); cleanupErr != nil {
			log.Fatal("login namespace cleanup failed")
		}
		if err != nil {
			log.Fatal(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"event": "login_complete"})
		return
	}
	listener, err := net.Listen("unix", *socket)
	if err != nil || os.Chmod(*socket, 0600) != nil {
		log.Fatal("private socket unavailable")
	}
	defer listener.Close()
	if acp.ServeRuntime(ctx, listener, process, *model, acp.KillRuntimeChildren) != nil {
		log.Fatal("ACP runtime stopped")
	}
}
