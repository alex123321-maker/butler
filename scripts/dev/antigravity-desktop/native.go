package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
)

type syntheticNativeAuth struct{}

func (syntheticNativeAuth) ResolveAntigravity(context.Context) (providerauth.AntigravityAuth, error) {
	return providerauth.AntigravityAuth{AccessToken: "synthetic-access", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func nativeFixture() (map[string]any, error) {
	interfaces, err := net.Interfaces()
	if err != nil || len(interfaces) == 0 {
		return nil, errors.New("network inspection failed")
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback == 0 {
			return nil, errors.New("network none required")
		}
	}
	var mu sync.Mutex
	observations := []map[string]any{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 65537))
		if err != nil || len(body) > 65536 {
			http.Error(w, "fixture bound", 413)
			return
		}
		mu.Lock()
		observations = append(observations, projection(r, body))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"candidates\":[{\"index\":0,\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"DESKTOP_OFFLINE_OK\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":1,\"totalTokenCount\":2}}\n\n")
	}))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	old := http.DefaultTransport
	http.DefaultTransport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "example.com", MinVersion: tls.VersionTLS12}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != "aicode.googleapis.com:443" {
			return nil, errors.New("fixture destination rejected")
		}
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}}
	defer func() { http.DefaultTransport = old }()
	provider, err := antigravity.NewProvider(antigravity.Config{AuthSource: syntheticNativeAuth{}, Timeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := provider.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "desktop-native-fixture", SessionKey: "desktop-native-fixture", ProviderName: antigravity.Name, ModelName: antigravity.DefaultModel}, InputItems: []transport.InputItem{{Role: "user", Content: fixturePrompt}}, StreamingEnabled: true})
	if err != nil {
		return nil, err
	}
	final := false
	for event := range stream {
		if event.TransportError != nil {
			return nil, errors.New("native fixture failed")
		}
		if event.AssistantFinal != nil {
			final = event.AssistantFinal.Content == "DESKTOP_OFFLINE_OK"
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if !final || len(observations) != 1 {
		return nil, errors.New("native fixture incomplete")
	}
	return map[string]any{"actual_go_provider": true, "request": observations[0], "fixture_final_verified": true, "synthetic_usage_excluded_from_real_totals": true, "real_model_calls": 0}, nil
}
