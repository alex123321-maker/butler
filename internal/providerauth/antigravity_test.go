package providerauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/modelprovider"
)

func importFixture(expires time.Time, project string) string {
	data, _ := json.Marshal(antigravityCredentials{AccessToken: "access-old", RefreshToken: "refresh-private", ClientID: antigravityOAuthClientID, ClientSecret: "client-private", ExpiresAt: expires, ProjectID: project})
	return string(data)
}
func TestAntigravityEncryptedImportResolveRefreshAndSafeState(t *testing.T) {
	store := &memorySecretStore{}
	manager := NewManager(store)
	if err := manager.ImportAntigravity(context.Background(), strings.NewReader(importFixture(time.Now().Add(-time.Minute), "")), "operator"); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	manager.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		switch req.URL.String() {
		case antigravityTokenURL:
			if req.ParseForm() != nil || req.PostForm.Get("grant_type") != "refresh_token" || req.PostForm.Get("client_secret") != "client-private" || req.PostForm.Get("refresh_token") != "refresh-private" {
				t.Error("incorrect refresh control-plane request")
			}
			return jsonResponse(`{"access_token":"access-new","refresh_token":"refresh-rotated","token_type":"Bearer","expires_in":3600}`), nil
		case antigravityCodeAssistURL:
			if req.Header.Get("Authorization") != "Bearer access-new" {
				t.Error("old access token reused")
			}
			data, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(data), `"ideType":"ANTIGRAVITY"`) {
				t.Error("metadata missing")
			}
			return jsonResponse(`{"cloudaicompanionProject":"subscription-project","paidTier":{"id":"g1-pro-tier"}}`), nil
		default:
			return nil, fmt.Errorf("unexpected endpoint")
		}
	})
	var group sync.WaitGroup
	for i := 0; i < 16; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			auth, err := manager.ResolveAntigravity(context.Background())
			if err != nil || auth.AccessToken != "access-new" || auth.ProjectID != "subscription-project" {
				t.Error("resolve failed")
			}
		}()
	}
	group.Wait()
	if calls.Load() != 2 {
		t.Fatalf("refresh stampede: %d", calls.Load())
	}
	setting, _ := store.Get(context.Background(), settingKeyAntigravity)
	if !setting.IsSecret || !strings.Contains(setting.Value, "refresh-rotated") {
		t.Fatal("secret not persisted through secret store")
	}
	state, err := manager.State(context.Background(), modelprovider.ProviderAntigravity)
	if err != nil || !state.Connected || state.AuthKind != AuthKindOperatorImport {
		t.Fatal("safe state missing")
	}
	encoded, _ := json.Marshal(state)
	if strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "access-new") || state.AccountHint != "" {
		t.Fatal("state contains credential material")
	}
	if err := manager.Delete(context.Background(), modelprovider.ProviderAntigravity); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ResolveAntigravity(context.Background()); err != ErrNotConnected {
		t.Fatal("disconnected provider resolved")
	}
}

func TestAntigravityImportRejectsMalformedOrUntrustedRecords(t *testing.T) {
	valid := importFixture(time.Now().Add(time.Hour), "project")
	cases := []string{`{}`, strings.TrimSuffix(valid, "}") + `,"id_token":"private"}`, strings.TrimSuffix(valid, "}") + `,"access_token":"duplicate"}`, strings.Replace(valid, antigravityOAuthClientID, "foreign-client", 1), valid + ` {}`, strings.Repeat(" ", 65537)}
	for i, value := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			store := &memorySecretStore{}
			m := NewManager(store)
			if err := m.ImportAntigravity(context.Background(), strings.NewReader(value), "operator"); err == nil {
				t.Fatal("invalid import accepted")
			}
			if len(store.items) > 0 {
				t.Fatal("invalid credentials persisted")
			}
		})
	}
}

func TestAntigravityControlPlaneDeniesRedirectAndMasksFailures(t *testing.T) {
	for _, status := range []int{302, 400, 401, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			m := NewManager(&memorySecretStore{})
			if err := m.ImportAntigravity(context.Background(), strings.NewReader(importFixture(time.Now().Add(-time.Hour), "project")), "operator"); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			m.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return &http.Response{StatusCode: status, Header: http.Header{"Location": {"https://foreign.invalid/steal"}}, Body: io.NopCloser(strings.NewReader(`{"error_description":"refresh-private client-private access-old"}`))}, nil
			})
			_, err := m.ResolveAntigravity(context.Background())
			if err == nil || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "access-old") || calls.Load() != 1 {
				t.Fatal("failure leaked or followed redirect")
			}
		})
	}
}
