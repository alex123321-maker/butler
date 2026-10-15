package providerauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const antigravityCloudCodeDiscovery = "https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist"

// ResolveAntigravityCloudCode discovers the subscription project with the same
// refreshed OAuth token used for generation. No onboarding, free-tier selection,
// account rotation, credit purchase or endpoint fallback is performed.
func (m *Manager) ResolveAntigravityCloudCode(ctx context.Context) (AntigravityAuth, error) {
	auth, err := m.ResolveAntigravity(ctx)
	if err != nil {
		return AntigravityAuth{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, antigravityCloudCodeDiscovery, strings.NewReader(`{}`))
	if err != nil {
		return AntigravityAuth{}, fmt.Errorf("Antigravity subscription discovery failed")
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "antigravity/2.22.0 butler/0.1")
	var result struct {
		Project  json.RawMessage `json:"cloudaicompanionProject"`
		PaidTier struct {
			ID string `json:"id"`
		} `json:"paidTier"`
	}
	if err := m.antigravityJSON(req, &result); err != nil {
		return AntigravityAuth{}, err
	}
	if result.PaidTier.ID != "g1-pro-tier" && result.PaidTier.ID != "g1-ultra-tier" {
		observeAntigravity(ctx, "paid_discovery", "invalid_response", 200)
		return AntigravityAuth{}, fmt.Errorf("Antigravity paid subscription was not confirmed")
	}
	var project string
	if json.Unmarshal(result.Project, &project) != nil {
		var object struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(result.Project, &object) != nil {
			observeAntigravity(ctx, "paid_discovery", "invalid_response", 200)
			return AntigravityAuth{}, fmt.Errorf("Antigravity subscription project is unavailable")
		}
		project = object.ID
	}
	if !ValidAntigravityProject(project) {
		observeAntigravity(ctx, "paid_discovery", "invalid_response", 200)
		return AntigravityAuth{}, fmt.Errorf("Antigravity subscription project is unavailable")
	}
	observeAntigravity(ctx, "paid_discovery", "completed", 200)
	auth.ProjectID = project
	return auth, nil
}

// ValidAntigravityProject bounds private routing data before serialization.
func ValidAntigravityProject(project string) bool {
	if len(project) == 0 || len(project) > 256 {
		return false
	}
	for _, r := range project {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
