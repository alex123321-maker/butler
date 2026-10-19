package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/butler/butler/internal/browsercontrol"
	"google.golang.org/grpc/metadata"
)

func TestOwnerControlFencesAgentAndRequiresFreshObservation(t *testing.T) {
	runner := &stubRunner{result: Result{OK: true}}
	server := NewServer(runner, nil)
	observed := openTest(t, server)
	command := browsercontrol.Command{Operation: "take", SessionID: observed.SessionID, RunID: "run-1", Controller: strings.Repeat("a", 32), Epoch: 1}
	if _, err := server.Control(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	assertCode(t, executeTest(t, server, browserCall("browser.click", "run-1", actionArgs(observed))), "owner_control")
	command.Operation = "release"
	command.Controller = strings.Repeat("b", 32)
	if _, err := server.Control(context.Background(), command); err == nil {
		t.Fatal("foreign controller released owner fence")
	}
	command.Controller = strings.Repeat("a", 32)
	if _, err := server.Control(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	assertCode(t, executeTest(t, server, browserCall("browser.click", "run-1", actionArgs(observed))), "stale_observation")
	command.Operation = "take" // stale epoch cannot take control again
	if _, err := server.Control(context.Background(), command); err == nil {
		t.Fatal("replayed takeover")
	}
	command.Epoch = 2
	if _, err := server.Control(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	command.Operation = "release"
	command.Epoch = 1
	if _, err := server.Control(context.Background(), command); err == nil {
		t.Fatal("old release crossed new fence")
	}
}

func TestPublicOriginsAndCurrentCredentialScope(t *testing.T) {
	runner := &stubRunner{result: Result{OK: true}}
	server := NewServer(runner, nil)
	open := browserCall("browser.open", "run-1", map[string]any{"url": "https://example.com/form"})
	open.Contract.AllowedDomains = nil
	result := executeTest(t, server, open)
	var obs Result
	if result.Status != "completed" {
		t.Fatal(result)
	}
	if err := json.Unmarshal([]byte(result.ResultJson), &obs); err != nil {
		t.Fatal(err)
	}
	args := actionArgs(obs)
	args["url"] = "https://login.example.org/sso"
	nav := browserCall("browser.navigate", "run-1", args)
	nav.Contract.AllowedDomains = nil
	if result = executeTest(t, server, nav); result.Status != "completed" {
		t.Fatal(result)
	}
	if err := json.Unmarshal([]byte(result.ResultJson), &obs); err != nil {
		t.Fatal(err)
	}
	if obs.ScopeOrigin != "https://login.example.org" {
		t.Fatal("initial origin still pinned")
	}
	if validateAuthScope(Request{Cookie: &Cookie{Name: "sid", Domain: "example.com"}}, obs.ScopeOrigin) == nil {
		t.Fatal("previous-origin cookie accepted")
	}
	if err := validateAuthScope(Request{Cookie: &Cookie{Name: "sid", Domain: "login.example.org", Secure: true}}, obs.ScopeOrigin); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerPixelsCannotEnterModelResult(t *testing.T) {
	result := Result{OK: true, OwnerFrame: &browsercontrol.Frame{JPEG: "private pixels", URL: "https://private-owner-url/?code=private-token"}}
	if payload := buildResultJSON("browser.snapshot", result); strings.Contains(payload, "private pixels") || strings.Contains(payload, "private-owner-url") {
		t.Fatal("owner frame leaked to model")
	}
}

func TestOwnerControlRejectsDispatchFromPreviousEpoch(t *testing.T) {
	runner := &stubRunner{result: Result{OK: true}}
	server := NewServer(runner, nil)
	server.RequireDispatchEpoch = true
	observed := openTest(t, server)
	req := browserCall("browser.snapshot", "run-1", map[string]any{"browser_session_id": observed.SessionID})
	command := browsercontrol.Command{Operation: "take", SessionID: observed.SessionID, RunID: "run-1", Controller: strings.Repeat("a", 32), Epoch: 1}
	if _, err := server.Control(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	command.Operation = "release"
	if _, err := server.Control(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	for _, values := range [][]string{nil, {"0"}, {"1"}, {"2", "2"}, {"invalid"}, {"-1"}} {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{"butler-browser-epoch": values})
		resp, err := server.Execute(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		assertCode(t, resp.Result, "stale_control_epoch")
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("butler-browser-epoch", "2"))
	resp, err := server.Execute(ctx, req)
	if err != nil || resp.GetResult().GetStatus() != "completed" {
		t.Fatal("current epoch rejected", err, resp)
	}
}
