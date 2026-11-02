package externalauth

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadGoogleWebConfigPinsDownloadedClient(t *testing.T) {
	const callback = "https://rp.example/auth/google/callback"
	base := map[string]any{"client_id": "123-test.apps.googleusercontent.com", "client_secret": "synthetic-client-secret-canary", "project_id": "butler-test", "auth_uri": googleAuthorize, "token_uri": googleToken, "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs", "redirect_uris": []string{callback}}
	for _, mode := range []string{"valid", "download_legacy_auth_uri", "installed", "extra_callback", "wrong_callback", "wrong_auth", "wrong_token", "wrong_certs", "browser_origin", "duplicate_secret", "unknown_field", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			web := make(map[string]any)
			for k, v := range base {
				web[k] = v
			}
			kind := "web"
			switch mode {
			case "download_legacy_auth_uri":
				web["auth_uri"] = "https://accounts.google.com/o/oauth2/auth"
			case "installed":
				kind = "installed"
			case "extra_callback":
				web["redirect_uris"] = []string{callback, "https://other.example/auth/google/callback"}
			case "wrong_callback":
				web["redirect_uris"] = []string{"https://other.example/auth/google/callback"}
			case "wrong_auth":
				web["auth_uri"] = "https://other.example/auth"
			case "wrong_token":
				web["token_uri"] = "https://other.example/token"
			case "wrong_certs":
				web["auth_provider_x509_cert_url"] = "https://other.example/certs"
			case "browser_origin":
				web["javascript_origins"] = []string{"https://rp.example"}
			case "unknown_field":
				web["unsupported"] = true
			}
			raw, err := json.Marshal(map[string]any{kind: web})
			if err != nil {
				t.Fatal("fixture encoding failed")
			}
			if mode == "duplicate_secret" {
				raw = []byte(strings.Replace(string(raw), `"web":{`, `"web":{"client_secret":"duplicate",`, 1))
			}
			if mode == "oversized" {
				raw = append(raw, []byte(strings.Repeat(" ", 16385))...)
			}
			cfg, err := ReadGoogleWebConfig(strings.NewReader(string(raw)), callback)
			if (err == nil) != (mode == "valid" || mode == "download_legacy_auth_uri") {
				t.Fatal("unexpected config verdict")
			}
			if err == nil && (cfg.ClientID != base["client_id"] || cfg.ClientSecret != base["client_secret"] || cfg.RedirectURL != callback) {
				t.Fatal("private config changed")
			}
			if err != nil && (err != ErrProvider || cfg != (GoogleConfig{})) {
				t.Fatal("private diagnostics or partial config exposed")
			}
		})
	}
	for _, r := range []io.Reader{nil, failingReader{}} {
		if _, err := ReadGoogleWebConfig(r, callback); err != ErrProvider {
			t.Fatal("reader failure exposed")
		}
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("private-reader-canary") }
