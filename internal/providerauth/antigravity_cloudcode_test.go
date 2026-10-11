package providerauth

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCloudCodeSubscriptionDiscoveryUsesSameTokenAndIgnoresLegacyProject(t *testing.T) {
	for _, project := range []string{`"discovered-project"`, `{"id":"discovered-project"}`} {
		m := NewManager(&memorySecretStore{})
		if err := m.ImportAntigravity(context.Background(), strings.NewReader(importFixture(time.Now().Add(time.Hour), "legacy-project")), "operator"); err != nil {
			t.Fatal(err)
		}
		m.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != antigravityCloudCodeDiscovery || req.Method != "POST" || req.Header.Get("Authorization") != "Bearer access-old" {
				t.Error("wrong discovery or token")
			}
			return jsonResponse(`{"cloudaicompanionProject":` + project + `,"currentTier":{"id":"free-tier"},"paidTier":{"id":"g1-pro-tier"}}`), nil
		})
		auth, err := m.ResolveAntigravityCloudCode(context.Background())
		if err != nil || auth.ProjectID != "discovered-project" || auth.AccessToken != "access-old" {
			t.Fatal("subscription resolution failed")
		}
		state, err := m.State(context.Background(), "antigravity")
		if err != nil || state.AccountHint != "" {
			t.Fatal("private routing leaked into settings")
		}
	}
}
func TestCloudCodeDiscoveryFailsClosedWithoutPaidTierOrValidProject(t *testing.T) {
	for _, body := range []string{`{}`, `{"cloudaicompanionProject":"private-project","currentTier":{"id":"free-tier"},"allowedTiers":[{"id":"g1-pro-tier"}]}`, `{"cloudaicompanionProject":"private\nproject","paidTier":{"id":"g1-pro-tier"}}`, `{"cloudaicompanionProject":{},"paidTier":{"id":"g1-pro-tier"}}`, `{"cloudaicompanionProject":"private-project","paidTier":{"id":"unknown"}}`} {
		m := NewManager(&memorySecretStore{})
		if err := m.ImportAntigravity(context.Background(), strings.NewReader(importFixture(time.Now().Add(time.Hour), "legacy-project")), "operator"); err != nil {
			t.Fatal(err)
		}
		m.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { return jsonResponse(body), nil })
		_, err := m.ResolveAntigravityCloudCode(context.Background())
		if err == nil || strings.Contains(err.Error(), "private-project") || strings.Contains(err.Error(), "legacy-project") {
			t.Fatal("unverified subscription accepted or leaked")
		}
	}
}
