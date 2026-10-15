package antigravity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestExplicitTieredCloudCodeProfile(t *testing.T) {
	var calls atomic.Int32
	p, err := NewProvider(Config{Model: DefaultModel, Protocol: "cloudcode", AuthSource: cloudAuthSource{authSource: authSource{legacyProject: "ignored-legacy"}, project: "discovered-project"}})
	if err != nil {
		t.Fatal(err)
	}
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		var wire cloudRequestEnvelope
		if json.NewDecoder(r.Body).Decode(&wire) != nil {
			t.Fatal("invalid wire")
		}
		if r.URL.String() != cloudEndpoint+":streamGenerateContent?alt=sse" ||
			wire.Model != DefaultModel || wire.Project != "discovered-project" ||
			wire.Request.SessionID == "" || wire.RequestID == "" ||
			wire.Request.GenerationConfig.MaxOutputTokens != 8192 ||
			wire.Request.GenerationConfig.ThinkingConfig == nil ||
			wire.Request.GenerationConfig.ThinkingConfig.ThinkingBudget != -1 ||
			wire.Request.GenerationConfig.ResponseMIMEType != "" ||
			len(wire.Request.GenerationConfig.ResponseJSONSchema) != 0 ||
			len(wire.Request.GenerationConfig.ResponseSchema) != 0 {
			t.Fatal("tiered route, model, discovery or generation allowance changed")
		}
		body, _ := json.Marshal(wire)
		if strings.Contains(string(body), "ignored-legacy") || strings.Contains(string(body), "flash-high") || strings.Contains(string(body), "aicode") {
			t.Fatal("profile substitution or legacy routing")
		}
		return response(cloudFrame("{\"text\":\"exact tiered result\"}", "STOP")), nil
	})
	in, out := p.InferenceLimits()
	if in != 1048576 || out != 8192 {
		t.Fatal("admission quote changed")
	}
	stream, err := p.StartRun(context.Background(), request("tiered-cloudcode"))
	if err != nil {
		t.Fatal(err)
	}
	var final bool
	for _, event := range collect(t, stream) {
		if event.TransportError != nil {
			t.Fatal(event.TransportError)
		}
		if event.AssistantFinal != nil {
			final = event.AssistantFinal.Content == "exact tiered result" && event.AssistantFinal.Usage != nil
		}
	}
	if !final || calls.Load() != 1 {
		t.Fatal("missing measured final")
	}
	req := request("untrusted-profile")
	req.TransportOptions = map[string]any{"protocol": "auto"}
	if _, err := p.StartRun(context.Background(), req); err == nil || calls.Load() != 1 {
		t.Fatal("model-selected protocol reached wire")
	}
	req = request("foreign-model")
	req.Context.ModelName = FlashHighModel
	if _, err := p.StartRun(context.Background(), req); err == nil || calls.Load() != 1 {
		t.Fatal("profile changed the configured model")
	}
	// A denial produces one request, without another route/model/admission.
	p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.Host != "daily-cloudcode-pa.googleapis.com" {
			t.Fatal("fallback route")
		}
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader("{\"error\":{\"status\":\"PERMISSION_DENIED\"}}"))}, nil
	})
	stream, err = p.StartRun(context.Background(), request("denied-tiered"))
	if err != nil || findError(collect(t, stream)) == nil || calls.Load() != 2 {
		t.Fatal("denial retried or lost", err)
	}
}

func TestTieredCloudCodeRequiresDiscoveryAndFiniteEnvelope(t *testing.T) {
	for _, mode := range []string{"no-discovery", "invalid-project", "byte-limit"} {
		t.Run(mode, func(t *testing.T) {
			p, err := NewProvider(Config{Protocol: "cloudcode", AuthSource: authSource{}})
			if err != nil {
				t.Fatal(err)
			}
			p.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("denied profile reached generation")
				return nil, nil
			})
			if mode != "no-discovery" {
				project := "discovered-project"
				if mode == "invalid-project" {
					project = "invalid\nproject"
				}
				p.config.AuthSource = cloudAuthSource{project: project}
			}
			ctx := context.Background()
			if mode == "byte-limit" {
				ctx = transport.WithInputByteLimit(ctx, 250)
			}
			stream, err := p.StartRun(ctx, request(mode))
			if err != nil || findError(collect(t, stream)) == nil {
				t.Fatal("invalid admission accepted", err)
			}
		})
	}
	for _, cfg := range []Config{{Protocol: "automatic-fallback"}, {Protocol: "cloudcode", Model: "unknown"}} {
		cfg.AuthSource = authSource{}
		if _, err := NewProvider(cfg); err == nil {
			t.Fatal("invalid profile accepted")
		}
	}
}
