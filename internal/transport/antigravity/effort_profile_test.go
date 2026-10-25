package antigravity

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestLowUsesExactPaidCloudProfileWithoutFallback(t *testing.T) {
	calls := 0
	p := newFixture(t, func(r *http.Request) (*http.Response, error) {
		calls++
		var w cloudRequestEnvelope
		if json.NewDecoder(r.Body).Decode(&w) != nil || w.Model != FlashLowModel || w.Project != "research-project" || w.Request.GenerationConfig.ThinkingConfig == nil || w.Request.GenerationConfig.ThinkingConfig.ThinkingBudget != -1 || w.Request.GenerationConfig.MaxOutputTokens != 8192 || r.URL.String() != cloudEndpoint+":streamGenerateContent?alt=sse" {
			t.Fatal("wrong Low wire route")
		}
		return response(cloudFrame(`{"text":"ready"}`, "STOP")), nil
	})
	p.config.AuthSource = cloudAuthSource{project: "research-project"}
	r := request("low-profile")
	r.Context.ModelName = FlashLowModel
	r.ToolDefinitions = nil
	s, e := p.StartRun(context.Background(), r)
	if e != nil {
		t.Fatal(e)
	}
	for _, ev := range collect(t, s) {
		if ev.TransportError != nil {
			t.Fatal(ev.TransportError)
		}
	}
	if calls != 1 {
		t.Fatal(calls)
	}
	_, _ = p.CancelRun(context.Background(), transport.CancelRunRequest{RunID: r.Context.RunID})
}

func TestThinkingUsageIsOptionalExactAndConsistent(t *testing.T) {
	base := strings.Replace(frame(`{"text":"ok"}`, "STOP"), `"totalTokenCount":31`, `"totalTokenCount":36,"thoughtsTokenCount":5`, 1)
	for _, tc := range []struct {
		name, wire string
		bad        bool
		want       *int
	}{
		{"present", base, false, intPtr(5)},
		{"absent", frame(`{"text":"ok"}`, "STOP"), false, nil},
		{"zero", strings.Replace(base, `"thoughtsTokenCount":5`, `"thoughtsTokenCount":0`, 1), false, intPtr(0)},
		{"negative", strings.Replace(base, `"thoughtsTokenCount":5`, `"thoughtsTokenCount":-1`, 1), true, nil},
		{"over_total", strings.Replace(base, `"thoughtsTokenCount":5`, `"thoughtsTokenCount":6`, 1), true, nil},
		{"alias", strings.Replace(base, `"thoughtsTokenCount":5`, `"ThoughtsTokenCount":5`, 1), true, nil},
		{"duplicate", strings.Replace(base, `"thoughtsTokenCount":5`, `"thoughtsTokenCount":5,"thoughtsTokenCount":5`, 1), true, nil},
		{"consistent_trailer", base + "data: {\"usageMetadata\":{\"promptTokenCount\":17,\"candidatesTokenCount\":14,\"totalTokenCount\":36,\"thoughtsTokenCount\":5}}\n\n", false, intPtr(5)},
		{"conflicting_trailer", base + "data: {\"usageMetadata\":{\"promptTokenCount\":17,\"candidatesTokenCount\":14,\"totalTokenCount\":36,\"thoughtsTokenCount\":4}}\n\n", true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, e := readStream(strings.NewReader(tc.wire), func(string) bool { return true })
			if (e != nil) != tc.bad {
				t.Fatalf("err=%v", e)
			}
			if tc.bad {
				return
			}
			if a.Usage == nil || (a.Usage.ThinkingTokens == nil) != (tc.want == nil) || tc.want != nil && *a.Usage.ThinkingTokens != *tc.want {
				t.Fatalf("usage=%+v", a.Usage)
			}
		})
	}
}

func intPtr(v int) *int { return &v }

func TestExplicitThinkingLevelCannotBeOverriddenByRequest(t *testing.T) {
	if _, err := NewProvider(Config{Model: FlashLowModel, MaximumOutputTokens: 32768, AuthSource: authSource{}}); err == nil {
		t.Fatal("unqualified Low output allowance accepted")
	}
	for _, cfg := range []Config{{Model: FlashLowModel, ThinkingLevel: "HIGH"}, {Model: FlashHighModel, ThinkingLevel: "LOW"}, {Model: DefaultModel, ThinkingLevel: "LOW"}} {
		cfg.AuthSource = authSource{}
		if _, e := NewProvider(cfg); e == nil {
			t.Fatal("mismatched thinking profile accepted")
		}
	}
	for _, profile := range []struct{ model, level string }{{FlashLowModel, "LOW"}, {FlashHighModel, "HIGH"}} {
		p, e := NewProvider(Config{Model: profile.model, ThinkingLevel: profile.level, AuthSource: cloudAuthSource{project: "research-project"}})
		if e != nil {
			t.Fatal(e)
		}
		calls := 0
		p.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			var wire cloudRequestEnvelope
			if json.NewDecoder(r.Body).Decode(&wire) != nil || wire.Request.GenerationConfig.ThinkingConfig.ThinkingLevel != profile.level || wire.Request.GenerationConfig.ThinkingConfig.ThinkingBudget != 0 {
				t.Fatal("explicit level wire mismatch")
			}
			return response(cloudFrame(`{"text":"ready"}`, "STOP")), nil
		})
		r := request(profile.level)
		r.Context.ModelName = profile.model
		r.ToolDefinitions = nil
		s, e := p.StartRun(context.Background(), r)
		if e != nil {
			t.Fatal(e)
		}
		if failure := findError(collect(t, s)); failure != nil {
			t.Fatal(failure)
		}
		r.Context.RunID += "-mismatch"
		r.Context.ModelName = DefaultModel
		if _, e = p.StartRun(context.Background(), r); e == nil {
			t.Fatal("request replaced the profile")
		}
		if calls != 1 {
			t.Fatal(calls)
		}
		_, _ = p.CancelRun(context.Background(), transport.CancelRunRequest{RunID: profile.level})
	}
}
