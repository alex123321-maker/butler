package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"sync"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/providerauth"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This mode has no generation path or admission. Only two fixed metadata reads
// with private Manager access; fixed safe fields are exported, never raw JSON.
func inspectHigh() error {
	if loadOperatorEnvironment() != nil {
		return denied
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("BUTLER_POSTGRES_URL"))
	if err != nil {
		return denied
	}
	defer pool.Close()
	manager := providerauth.NewManager(config.NewPostgresSettingsStore(pool))
	ref, err := privateRead("/auth/import.json")
	if err != nil {
		return denied
	}
	defer clear(ref)
	origin, err := manager.AuditAntigravityOrigin(ctx, bytes.NewReader(ref))
	if err != nil || !origin.StoredCredentials || !origin.OAuthApplicationMatches || !origin.OAuthCredentialMatches || !origin.RefreshCredentialMatches {
		return denied
	}
	auth, err := manager.ResolveAntigravity(ctx)
	if err != nil {
		return denied
	}
	cloud, cloudErr := manager.ResolveAntigravityCloudCode(ctx)
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"phase": "ACTUAL_MANAGER_HIGH_PREFLIGHT", "real_model_calls": 0, "cloud_auth_resolved": cloudErr == nil, "fixed_diagnostic": highDiscoveryDiagnostic(cloudErr), "access_valid_three_minutes": cloud.ExpiresAt.After(time.Now().Add(3 * time.Minute)), "valid_project": providerauth.ValidAntigravityProject(cloud.ProjectID)})
	client := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone(), Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, host := range []string{"daily-cloudcode-pa.googleapis.com", "cloudcode-pa.googleapis.com"} {
		p := map[string]any{"host": host, "path": "/v1internal:loadCodeAssist", "body_kind": "EMPTY_OBJECT", "real_model_calls": 0, "origin_all_four_match": true}
		req, _ := http.NewRequestWithContext(ctx, "POST", "https://"+host+"/v1internal:loadCodeAssist", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "antigravity/2.22.0 butler/0.1")
		var mu sync.Mutex
		flags := map[string]bool{}
		set := func(k string, v bool) { mu.Lock(); flags[k] = v; mu.Unlock() }
		trace := &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) { set("connection_obtained", true) }, TLSHandshakeDone: func(_ tls.ConnectionState, err error) { set("tls_handshake_completed", err == nil) }, WroteRequest: func(info httptrace.WroteRequestInfo) { set("request_written", info.Err == nil) }, GotFirstResponseByte: func() { set("response_byte_received", true) }}
		start := time.Now()
		resp, err := client.Do(req.WithContext(httptrace.WithClientTrace(req.Context(), trace)))
		if err != nil {
			p["connection_error_class"] = connectionClass(err)
		} else {
			p["http_status"] = resp.StatusCode
			data, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
			resp.Body.Close()
			if err == nil && len(data) <= 1024*1024 {
				var value struct {
					Project  json.RawMessage `json:"cloudaicompanionProject"`
					PaidTier struct {
						ID string `json:"id"`
					} `json:"paidTier"`
				}
				if json.Unmarshal(data, &value) == nil {
					p["paid_tier_present"] = value.PaidTier.ID != ""
					p["google_ai_pro"] = value.PaidTier.ID == "g1-pro-tier"
					p["google_ai_ultra"] = value.PaidTier.ID == "g1-ultra-tier"
					var project string
					kind := "OTHER_OR_MISSING"
					if json.Unmarshal(value.Project, &project) == nil {
						kind = "STRING"
					} else {
						var object struct {
							ID string `json:"id"`
						}
						if json.Unmarshal(value.Project, &object) == nil && object.ID != "" {
							project = object.ID
							kind = "OBJECT_ID"
						}
					}
					p["project_shape"] = kind
					p["valid_project"] = providerauth.ValidAntigravityProject(project)
				}
			}
			clear(data)
		}
		mu.Lock()
		for k, v := range flags {
			p[k] = v
		}
		mu.Unlock()
		p["elapsed_ms"] = time.Since(start).Milliseconds()
		p["access_valid_three_minutes"] = auth.ExpiresAt.After(time.Now().Add(3 * time.Minute))
		_ = json.NewEncoder(os.Stdout).Encode(p)
	}
	return nil
}
