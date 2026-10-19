package api

import (
	"github.com/butler/butler/internal/browsercontrol"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Test-only finite public owner API process for the real UI/Broker/Chromium
// fixture. Production has no environment shortcut for owner proof or authority.
func TestServerBrowserOwnerAPIHarness(t *testing.T) {
	if os.Getenv("BUTLER_SERVER_BROWSER_API_HARNESS") != "1" {
		t.Skip("private subprocess entrypoint")
	}
	client, err := browsercontrol.NewClient(os.Getenv("BUTLER_SERVER_BROWSER_BROKER_ENDPOINT"), strings.Repeat("core-private-", 3))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", os.Getenv("BUTLER_SERVER_BROWSER_OWNER_API_ADDR"))
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: ServerBrowser(NewOwnerControlAuth(strings.Repeat("owner-browser-fixture-", 2), "owner"), client), ReadHeaderTimeout: 3 * time.Second}
	defer server.Close()
	if err = server.Serve(listener); err != nil && err != http.ErrServerClosed {
		t.Fatal(err)
	}
}
