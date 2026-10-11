package providerauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAntigravityOriginAuditIsPrivateAndReadOnly(t *testing.T) {
	for _, scenario := range []string{"match_expired", "access_rotated", "refresh_rotated", "application_credential_changed", "disconnected", "malformed", "duplicate", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			store := &memorySecretStore{}
			m := NewManager(store)
			m.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Error("origin audit issued a network request")
				return nil, fmt.Errorf("PRIVATE_NETWORK_MARKER")
			})
			reference := importFixture(time.Now().Add(-time.Hour), "private-project")
			var creds antigravityCredentials
			if json.Unmarshal([]byte(reference), &creds) != nil {
				t.Fatal("bad fixture")
			}
			switch scenario {
			case "access_rotated":
				creds.AccessToken = "access-different"
			case "refresh_rotated":
				creds.RefreshToken = "refresh-different"
			case "application_credential_changed":
				creds.ClientSecret = "client-different"
			}
			stored, _ := json.Marshal(creds)
			if scenario != "disconnected" {
				if err := m.ImportAntigravity(ctx, strings.NewReader(string(stored)), "fixture"); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := store.Get(ctx, settingKeyAntigravity)
			switch scenario {
			case "malformed":
				reference = `{"refresh_token":"PRIVATE_INPUT_MARKER"}`
			case "duplicate":
				reference = strings.Replace(reference, `"refresh_token":`, `"REFRESH_TOKEN":"PRIVATE_INPUT_MARKER","refresh_token":`, 1)
			case "oversized":
				reference = strings.Repeat("PRIVATE_INPUT_MARKER", 65536)
			}
			report, err := m.AuditAntigravityOrigin(ctx, strings.NewReader(reference))
			invalid := scenario == "malformed" || scenario == "duplicate" || scenario == "oversized"
			if invalid != (err != nil) {
				t.Fatal("private reference validation", err)
			}
			if !invalid {
				connected := scenario != "disconnected"
				if report.StoredCredentials != connected || report.OAuthApplicationMatches != connected || report.RefreshCredentialMatches != (connected && scenario != "refresh_rotated") || report.OAuthCredentialMatches != (connected && scenario != "application_credential_changed") {
					t.Fatal("origin comparison mismatch")
				}
			}
			after, _ := store.Get(ctx, settingKeyAntigravity)
			if before != after {
				t.Fatal("read-only audit changed stored credentials")
			}
			raw, _ := json.Marshal(report)
			if err != nil {
				raw = append(raw, []byte(err.Error())...)
			}
			for _, private := range []string{"PRIVATE_", "access-old", "access-different", "refresh-private", "client-private", "private-project", antigravityOAuthClientID} {
				if strings.Contains(string(raw), private) {
					t.Fatal("private origin material leaked")
				}
			}
		})
	}
}
