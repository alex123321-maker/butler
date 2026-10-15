package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/butler/butler/internal/providerauth"
)

// Public Google CLI LoadCodeAssistResponse/GeminiUserTier fields only.
// Messages, URLs, identity, project and credits are never exported.
type accessObservation struct {
	ObservedAt         time.Time `json:"observed_at"`
	HTTPStatus         int       `json:"http_status"`
	ProjectPresent     bool      `json:"project_present"`
	PaidTierConfirmed  bool      `json:"paid_tier_confirmed"`
	CurrentAcceptedTOS *bool     `json:"current_accepted_tos"`
	CurrentOnboarded   *bool     `json:"current_onboarded"`
	PaidAcceptedTOS    *bool     `json:"paid_accepted_tos"`
	PaidOnboarded      *bool     `json:"paid_onboarded"`
	IneligibleReasons  []string  `json:"ineligible_reasons"`
}

func projectAccess(data []byte, status int) accessObservation {
	out := accessObservation{ObservedAt: time.Now().UTC(), HTTPStatus: status}
	var r struct {
		Project json.RawMessage `json:"cloudaicompanionProject"`
		Current struct {
			TOS       *bool `json:"hasAcceptedTos"`
			Onboarded *bool `json:"hasOnboardedPreviously"`
		} `json:"currentTier"`
		Paid struct {
			ID        string `json:"id"`
			TOS       *bool  `json:"hasAcceptedTos"`
			Onboarded *bool  `json:"hasOnboardedPreviously"`
		} `json:"paidTier"`
		Ineligible []struct {
			Reason string `json:"reasonCode"`
		} `json:"ineligibleTiers"`
	}
	if status != 200 || json.Unmarshal(data, &r) != nil {
		return out
	}
	var project string
	_ = json.Unmarshal(r.Project, &project)
	out.ProjectPresent = providerauth.ValidAntigravityProject(project)
	out.PaidTierConfirmed = r.Paid.ID == "g1-pro-tier" || r.Paid.ID == "g1-ultra-tier"
	out.CurrentAcceptedTOS, out.CurrentOnboarded = r.Current.TOS, r.Current.Onboarded
	out.PaidAcceptedTOS, out.PaidOnboarded = r.Paid.TOS, r.Paid.Onboarded
	for _, item := range r.Ineligible {
		reason := "UNCLASSIFIED"
		switch item.Reason {
		case "DASHER_USER", "INELIGIBLE_ACCOUNT", "NON_USER_ACCOUNT", "RESTRICTED_AGE", "RESTRICTED_NETWORK", "UNKNOWN", "UNKNOWN_LOCATION", "UNSUPPORTED_LOCATION", "VALIDATION_REQUIRED":
			reason = item.Reason
		}
		out.IneligibleReasons = append(out.IneligibleReasons, reason)
	}
	return out
}

func observeAccess(ctx context.Context, m *providerauth.Manager) (accessObservation, error) {
	auth, err := m.ResolveAntigravity(ctx)
	if err != nil {
		return accessObservation{}, fmt.Errorf("access metadata authentication failed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist", strings.NewReader("{}"))
	if err != nil {
		return accessObservation{}, fmt.Errorf("access metadata request failed")
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "antigravity/2.22.0 butler/0.1")
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return accessObservation{}, fmt.Errorf("access metadata connection failed")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
	defer clear(data)
	if err != nil || len(data) > 1024*1024 {
		return accessObservation{}, fmt.Errorf("access metadata response invalid")
	}
	return projectAccess(data, resp.StatusCode), nil
}
