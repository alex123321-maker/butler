package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	"github.com/butler/butler/internal/security/egress/egresstest"
)

func TestFormMetadataPrivacyAndBounds(t *testing.T) {
	for _, mode := range []string{"public", "unfrozen", "private", "password", "hidden", "unknown_issue", "contradiction", "duplicate", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			r := Result{NetworkState: "frozen", Forms: []FormDescription{{Fields: []FormFieldDescription{{Name: "quantity", Type: "number", Constraints: &FormFieldConstraints{Required: true, Min: "1", Max: strings.Repeat("x", 65)}, Validation: &FormFieldValidation{Valid: false, Issues: []string{"range_overflow"}}}}}}}
			switch mode {
			case "unfrozen":
				r.NetworkState = "read_only"
			case "password", "hidden":
				r.Forms[0].Fields = append(r.Forms[0].Fields, FormFieldDescription{Name: "private", Type: mode})
			case "unknown_issue":
				r.Forms[0].Fields[0].Validation.Issues = []string{"private-error-message"}
			case "contradiction":
				r.Forms[0].Fields[0].Validation.Valid = true
			case "duplicate":
				r.Forms[0].Fields[0].Validation.Issues = []string{"range_overflow", "range_overflow"}
			case "oversized":
				for range 32 {
					r.Forms[0].Fields = append(r.Forms[0].Fields, FormFieldDescription{Type: "text"})
				}
			}
			sanitizeFormMetadata(&r, mode == "private")
			f := r.Forms[0].Fields[0]
			if (f.Validation != nil) != (mode == "public") {
				t.Fatal("unsafe validation feedback retained")
			}
			if mode == "private" || mode == "password" || mode == "hidden" || mode == "oversized" {
				if f.Constraints != nil {
					t.Fatal("private/oversized form exposed constraints")
				}
			} else if f.Constraints == nil || f.Constraints.Min != "1" || f.Constraints.Max != "" {
				t.Fatal("constraint bounds incorrect")
			}
			if len(r.Forms[0].Fields) > 32 {
				t.Fatal("field bound exceeded")
			}
		})
	}
}

func TestBrowserPublicFormValidationChromium(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_E2E") != "1" {
		t.Skip("actual Chromium required")
	}
	var posts atomic.Int32
	const form = `<form method=post action=/save><input name=title required minlength=2 maxlength=24><input name=quantity type=number min=1 max=5 step=1 required><button>Save</button>%s</form><p id=count>Invalid events: 0</p><script>let invalidEvents=0;document.addEventListener('invalid',()=>{document.querySelector('#count').textContent='Invalid events: '+(++invalidEvents);fetch('/save',{method:'POST'})},true)</script>`
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts.Add(1)
		}
		w.Header().Set("Content-Type", "text/html")
		extra := ""
		switch r.URL.Path {
		case "/password":
			extra = `<input name=password type=password value="password-form-canary">`
		case "/hidden":
			extra = `<input name=csrf type=hidden value="hidden-form-canary">`
		case "/cookie":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "cookie-form-canary"})
		case "/storage":
			extra = `<script>localStorage.setItem('session','storage-form-canary')</script>`
		case "/late-cookie":
			extra = `<script>Object.defineProperty(HTMLInputElement.prototype,'required',{get(){document.cookie='late=late-cookie-form-canary';return true}})</script>`
		}
		fmt.Fprintf(w, form, extra)
	}))
	defer site.Close()
	runner := NewPlaywrightRunnerWithPolicy("node", os.Getenv("BUTLER_BROWSER_E2E_SCRIPT"), egresstest.Policy(site))
	runner.workspaceRoot = filepath.Clean(filepath.Join(filepath.Dir(os.Getenv("BUTLER_BROWSER_E2E_SCRIPT")), "..", "..", ".."))
	server := NewServer(runner, nil)
	defer server.Close()
	invoke := func(tool string, args map[string]any, credentials ...*runtimev1.ResolvedCredential) Result {
		t.Helper()
		req := browserCall(tool, "run-1", args)
		req.Contract.AllowedDomains = nil
		req.ResolvedCredentials = credentials
		if len(credentials) != 0 {
			req.Contract.SupportsCredentialRefs = true
		}
		response, err := server.Execute(context.Background(), req)
		if err != nil || response.GetResult().GetStatus() != "completed" {
			t.Fatal("form observation/preparation failed", tool)
		}
		for _, secret := range []string{"password-form-canary", "hidden-form-canary", "cookie-form-canary", "storage-form-canary", "late-cookie-form-canary"} {
			if strings.Contains(response.Result.ResultJson, secret) {
				t.Fatal("form observation leaked a canary")
			}
		}
		var result Result
		if json.Unmarshal([]byte(response.Result.ResultJson), &result) != nil {
			t.Fatal("invalid result")
		}
		return result
	}
	prepare := func(obs Result, value string) Result {
		args := actionArgs(obs)
		args["selector"], args["value"] = `input[name=quantity]`, value
		return invoke("browser.prepare_fill", args)
	}
	obs := invoke("browser.open", map[string]any{"url": egresstest.Origin + "/public"})
	if len(obs.Forms) != 1 || len(obs.Forms[0].Fields) != 2 {
		t.Fatal("missing form fields")
	}
	field := obs.Forms[0].Fields[1]
	if field.Constraints == nil || !field.Constraints.Required || field.Constraints.Min != "1" || field.Constraints.Max != "5" || field.Validation != nil {
		t.Fatal("public constraints missing or unfrozen validity exposed")
	}
	obs = prepare(obs, "8")
	field = obs.Forms[0].Fields[1]
	if obs.NetworkState != "frozen" || field.Validation == nil || field.Validation.Valid || len(field.Validation.Issues) != 1 || field.Validation.Issues[0] != "range_overflow" || !strings.Contains(obs.Text, "Invalid events: 0") || posts.Load() != 0 {
		t.Fatal("invalid public input not observed without events/requests")
	}
	obs = prepare(obs, "3")
	field = obs.Forms[0].Fields[1]
	if field.Validation == nil || !field.Validation.Valid || len(field.Validation.Issues) != 0 || posts.Load() != 0 {
		t.Fatal("corrected public input not observed locally")
	}
	for _, path := range []string{"/password", "/hidden", "/cookie", "/storage", "/late-cookie"} {
		t.Run(path, func(t *testing.T) {
			private := invoke("browser.open", map[string]any{"url": egresstest.Origin + path})
			private = prepare(private, "8")
			for _, form := range private.Forms {
				for _, field := range form.Fields {
					if field.Constraints != nil || field.Validation != nil {
						t.Fatal("private form/state exposed validity or constraint feedback")
					}
				}
			}
		})
	}
	t.Run("credential_then_plaintext", func(t *testing.T) {
		obs := invoke("browser.open", map[string]any{"url": egresstest.Origin + "/public"})
		args := actionArgs(obs)
		args["selector"] = `input[name=quantity]`
		args["value"] = map[string]string{"type": "credential_ref", "alias": "fixture", "field": "username"}
		obs = invoke("browser.prepare_fill", args, &runtimev1.ResolvedCredential{Alias: "fixture", Field: "username", Value: "9173319"})
		for range 2 {
			for _, form := range obs.Forms {
				for _, field := range form.Fields {
					if field.Constraints != nil || field.Validation != nil {
						t.Fatal("secret-backed value or later plaintext exposed feedback")
					}
				}
			}
			obs = prepare(obs, "3")
		}
	})
	if posts.Load() != 0 {
		t.Fatal("observations or local correction caused a POST")
	}
}
