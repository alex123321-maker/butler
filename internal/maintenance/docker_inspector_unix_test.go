//go:build !windows

package maintenance

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDockerInspectorFixedLocalSocket(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "daemon.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	var status atomic.Int64
	status.Store(200)
	var requests atomic.Int64
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != "GET" || r.URL.Path != "/containers/public-module/json" || r.URL.RawQuery != "" {
			t.Error("inspection escaped fixed target")
			w.WriteHeader(500)
			return
		}
		code := int(status.Load())
		if code == 302 {
			w.Header().Set("Location", "http://outside.invalid/private")
		}
		w.WriteHeader(code)
		if code == 200 {
			_ = json.NewEncoder(w).Encode(publicDockerInspect())
		} else {
			_, _ = w.Write([]byte("synthetic-private-daemon-error"))
		}
	})}
	go server.Serve(listener)
	t.Cleanup(func() { _ = server.Close() })
	inspector, err := NewDockerInspector(socket, "public-module")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := inspector.Inspect(context.Background())
	if err != nil || identity.State != "running" || identity.ArtifactSHA256 != strings.Repeat("2", 64) {
		t.Fatal("actual local HTTP inspection unavailable", err)
	}
	for _, code := range []int64{404, 302, 503} {
		status.Store(code)
		before := requests.Load()
		identity, err = inspector.Inspect(context.Background())
		if requests.Load() != before+1 {
			t.Fatal("redirect or extra operation performed")
		}
		if code == 404 {
			if err != nil || identity.State != "missing" || identity.ArtifactSHA256 != "" {
				t.Fatal("missing target was invented", err)
			}
		} else if err == nil || strings.Contains(err.Error(), "synthetic-private") {
			t.Fatal("daemon error was trusted or published", err)
		}
	}
	if _, err = NewDockerInspector(socket, "../other"); err == nil {
		t.Fatal("target path accepted")
	}
	if _, err = NewDockerInspector("tcp://127.0.0.1:2375", "public-module"); err == nil {
		t.Fatal("nonlocal Docker transport accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = inspector.Inspect(ctx); err == nil {
		t.Fatal("cancelled inspection accepted")
	}
}
