// Metadata-only private provider preflight. There is no generation/admission path.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport/antigravity"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// This one-shot control-plane probe has no durable state. Credentials remain
// process-private and disappear at exit; no sealed/native database is opened.
type privateStore struct {
	mu     sync.Mutex
	values map[string]config.Setting
}

func (s *privateStore) Get(_ context.Context, key string) (config.Setting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.values[key]
	if !ok {
		return v, config.ErrSettingNotFound
	}
	return v, nil
}
func (s *privateStore) Set(_ context.Context, v config.Setting) (config.Setting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[v.Key] = v
	return v, nil
}
func (s *privateStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
	return nil
}

type metadataReport struct {
	ModelCalls              int  `json:"model_calls"`
	ProxyConfigured         bool `json:"proxy_configured"`
	CloudDiscovery          bool `json:"cloud_discovery"`
	ValidProject            bool `json:"valid_project"`
	AccessValidThreeMinutes bool `json:"access_valid_three_minutes"`
	CatalogHTTPStatus       int  `json:"catalog_http_status"`
	HighProfilePresent      bool `json:"high_profile_present"`
}

func main() {
	report, ok := probe()
	_ = json.NewEncoder(os.Stdout).Encode(report)
	if !ok {
		os.Exit(1)
	}
}
func probe() (metadataReport, bool) {
	r := metadataReport{}
	if len(os.Args) != 1 || os.Getenv("BUTLER_MEMORY_EVAL_MODEL_METADATA") != "1" || os.Getenv("HTTPS_PROXY") != "http://antigravity-proxy:18080" || os.Getenv("HTTP_PROXY") != "http://antigravity-proxy:18080" {
		return r, false
	}
	r.ProxyConfigured = true
	raw, err := acceptance.ReadPrivateFile("/auth/native-import.json", 64<<10)
	if err != nil {
		return r, false
	}
	defer clear(raw)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	manager := providerauth.NewManager(&privateStore{values: map[string]config.Setting{}})
	if manager.ImportAntigravity(ctx, bytes.NewReader(raw), "isolated-memory-metadata-only") != nil {
		return r, false
	}
	auth, err := manager.ResolveAntigravityCloudCode(ctx)
	if err != nil {
		return r, false
	}
	r.CloudDiscovery = true
	r.ValidProject = providerauth.ValidAntigravityProject(auth.ProjectID)
	r.AccessValidThreeMinutes = auth.ExpiresAt.After(time.Now().Add(3 * time.Minute))
	if !r.ValidProject || !r.AccessValidThreeMinutes {
		return r, false
	}
	body, _ := json.Marshal(map[string]string{"project": auth.ProjectID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://daily-cloudcode-pa.googleapis.com/v1internal:fetchAvailableModels", bytes.NewReader(body))
	if err != nil {
		return r, false
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "antigravity/2.22.0 butler/0.1")
	client := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone(), Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return r, false
	}
	defer resp.Body.Close()
	r.CatalogHTTPStatus = resp.StatusCode
	data, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return r, false
	}
	defer clear(data)
	var catalog struct {
		Models map[string]json.RawMessage `json:"models"`
	}
	if resp.StatusCode != 200 || json.Unmarshal(data, &catalog) != nil {
		return r, false
	}
	_, r.HighProfilePresent = catalog.Models[antigravity.FlashHighModel]
	return r, r.HighProfilePresent
}
