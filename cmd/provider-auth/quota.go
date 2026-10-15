package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/butler/butler/internal/providerauth"
)

// This is an operator metadata read, not an inference, fallback or quota grant.
type quotaObservation struct {
	Model             string     `json:"model"`
	ObservedAt        time.Time  `json:"observed_at"`
	HTTPStatus        int        `json:"http_status"`
	Advertised        bool       `json:"advertised"`
	RemainingFraction *float64   `json:"remaining_fraction"`
	ResetAt           *time.Time `json:"reset_at,omitempty"`
	MeterAttribution  string     `json:"meter_attribution"`
}

func projectQuota(model string, data []byte, status int) quotaObservation {
	out := quotaObservation{Model: model, ObservedAt: time.Now().UTC(), HTTPStatus: status, MeterAttribution: "reported_model_metadata_not_per_request_accounting"}
	var response struct {
		Models map[string]struct {
			QuotaInfo struct {
				RemainingFraction *float64 `json:"remainingFraction"`
				ResetTime         string   `json:"resetTime"`
			} `json:"quotaInfo"`
		} `json:"models"`
	}
	if status != http.StatusOK || json.Unmarshal(data, &response) != nil {
		return out
	}
	entry, ok := response.Models[model]
	out.Advertised = ok
	if !ok {
		return out
	}
	if f := entry.QuotaInfo.RemainingFraction; f != nil && *f >= 0 && *f <= 1 {
		out.RemainingFraction = f
	}
	if reset, e := time.Parse(time.RFC3339Nano, entry.QuotaInfo.ResetTime); e == nil {
		out.ResetAt = &reset
	}
	return out
}

func observeQuota(ctx context.Context, m *providerauth.Manager, model string) (quotaObservation, error) {
	auth, err := m.ResolveAntigravityCloudCode(ctx)
	if err != nil {
		return quotaObservation{}, fmt.Errorf("Antigravity quota authentication/discovery failed")
	}
	// The private discovered project and bearer never enter output or logs.
	body, _ := json.Marshal(map[string]string{"project": auth.ProjectID})
	defer clear(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://daily-cloudcode-pa.googleapis.com/v1internal:fetchAvailableModels", bytes.NewReader(body))
	if err != nil {
		return quotaObservation{}, fmt.Errorf("quota request failed")
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "antigravity/2.22.0 butler/0.1")
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return quotaObservation{}, fmt.Errorf("quota connection failed")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	defer clear(data)
	if err != nil || len(data) > 2*1024*1024 {
		return quotaObservation{}, fmt.Errorf("quota response invalid")
	}
	return projectQuota(model, data, resp.StatusCode), nil
}
