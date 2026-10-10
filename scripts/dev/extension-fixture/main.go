// Command extension-fixture serves a disposable synthetic project recipient.
// Its directory is operator configuration, never an extension argument.
package main

import (
	"github.com/butler/butler/internal/extensions/projectfixture"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func main() {
	directory := os.Getenv("BUTLER_EXTENSION_FIXTURE_DATA_DIR")
	if directory == "" {
		slog.Error("fixture data directory required")
		os.Exit(1)
	}
	s, err := projectfixture.New(directory)
	if err != nil {
		slog.Error("fixture state unavailable")
		os.Exit(1)
	}
	defer s.Close()
	addr := os.Getenv("BUTLER_EXTENSION_FIXTURE_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	server := &http.Server{Addr: addr, Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, MaxHeaderBytes: 8192}
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("fixture stopped")
		os.Exit(1)
	}
}
