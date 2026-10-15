package providerauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/config"
)

func TestAntigravityDiagnosticsSemanticValidationTimeoutAndPrivacy(t *testing.T) {
	for _, stage := range []string{"oauth_refresh", "paid_discovery"} {
		for _, kind := range []string{"invalid_json", "invalid_semantics", "client_timeout", "http_rejected", "success"} {
			t.Run(stage+"/"+kind, func(t *testing.T) {
				m := NewManager(&memorySecretStore{})
				expires := time.Now().Add(time.Hour)
				if stage == "oauth_refresh" {
					expires = time.Now().Add(-time.Hour)
				}
				if err := m.ImportAntigravity(context.Background(), strings.NewReader(importFixture(expires, "private-project")), "operator"); err != nil {
					t.Fatal(err)
				}
				m.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
					if kind == "client_timeout" {
						return nil, errors.Join(context.DeadlineExceeded, errors.New("secret-error"))
					}
					if kind == "http_rejected" {
						r := jsonResponse(`{"secret":"private-body"}`)
						r.StatusCode = 403
						return r, nil
					}
					if kind == "invalid_json" {
						return jsonResponse(`private-body`), nil
					}
					if kind == "invalid_semantics" {
						return jsonResponse(`{}`), nil
					}
					if stage == "oauth_refresh" {
						return jsonResponse(`{"access_token":"private-new","refresh_token":"private-rotated","token_type":"Bearer","expires_in":3600}`), nil
					}
					return jsonResponse(`{"cloudaicompanionProject":"private-new","paidTier":{"id":"g1-pro-tier"}}`), nil
				})
				var events []AntigravityDiagnostic
				ctx := WithAntigravityDiagnostics(context.Background(), func(d AntigravityDiagnostic) { events = append(events, d) })
				var err error
				if stage == "oauth_refresh" {
					_, err = m.ResolveAntigravity(ctx)
				} else {
					_, err = m.ResolveAntigravityCloudCode(ctx)
				}
				want := "completed"
				switch kind {
				case "invalid_json", "invalid_semantics":
					want = "invalid_response"
				case "client_timeout":
					want = "deadline"
				case "http_rejected":
					want = "http_rejected"
				}
				found := false
				for _, d := range events {
					if d.Stage == stage && d.Outcome == want {
						found = true
					}
					if kind != "success" && d.Stage == stage && d.Outcome == "completed" {
						t.Fatal("invalid stage reported completed")
					}
				}
				if !found || (err == nil) != (kind == "success") {
					t.Fatal("diagnostic mismatch", events, err)
				}
				raw, _ := json.Marshal(events)
				for _, bad := range []string{"private", "secret", "googleapis", "Bearer"} {
					if strings.Contains(string(raw), bad) {
						t.Fatal("diagnostic leaked content")
					}
				}
				before := len(events)
				observeAntigravity(context.Background(), stage, "completed", 200)
				observeAntigravity(ctx, "arbitrary-private-stage", "completed", 200)
				observeAntigravity(ctx, stage, "private-error", 200)
				if len(events) != before {
					t.Fatal("unscoped or untyped diagnostic accepted")
				}
			})
		}
	}
}

type failRefreshDiagnosticStore struct{ *memorySecretStore }

func (s failRefreshDiagnosticStore) Set(context.Context, config.Setting) (config.Setting, error) {
	return config.Setting{}, errors.New("private-store-error")
}

func TestAntigravityDiagnosticsPersistRotationBeforeDiscoveryAndDenyUncertainRefresh(t *testing.T) {
	for _, fail := range []bool{false, true} {
		store := &memorySecretStore{}
		m := NewManager(store)
		if err := m.ImportAntigravity(context.Background(), strings.NewReader(importFixture(time.Now().Add(-time.Hour), "")), "operator"); err != nil {
			t.Fatal(err)
		}
		if fail {
			m.store = failRefreshDiagnosticStore{store}
		}
		discovery := 0
		m.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == antigravityTokenURL {
				return jsonResponse(`{"access_token":"new-private","refresh_token":"rotated-private","token_type":"Bearer","expires_in":3600}`), nil
			}
			discovery++
			setting, _ := store.Get(context.Background(), settingKeyAntigravity)
			if !strings.Contains(setting.Value, "rotated-private") {
				t.Fatal("discovery preceded durable rotation")
			}
			return nil, context.DeadlineExceeded
		})
		var trace []AntigravityDiagnostic
		ctx := WithAntigravityDiagnostics(context.Background(), func(d AntigravityDiagnostic) { trace = append(trace, d) })
		if _, err := m.ResolveAntigravityCloudCode(ctx); err == nil {
			t.Fatal("failed request accepted")
		}
		if UnsafeAntigravityRefresh(trace) != fail || fail && discovery != 0 || !fail && discovery != 1 {
			t.Fatal("unsafe refresh classification", trace)
		}
	}
	if !UnsafeAntigravityRefresh([]AntigravityDiagnostic{{Stage: "oauth_refresh", Outcome: "started"}}) {
		t.Fatal("unfinished refresh reusable")
	}
}
