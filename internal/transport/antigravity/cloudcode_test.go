package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
)

type cloudAuthSource struct {
	authSource
	project     string
	unavailable bool
}

func (s cloudAuthSource) ResolveAntigravityCloudCode(ctx context.Context) (providerauth.AntigravityAuth, error) {
	if s.unavailable {
		return providerauth.AntigravityAuth{}, fmt.Errorf("private account detail")
	}
	auth, err := s.ResolveAntigravity(ctx)
	auth.ProjectID = s.project
	return auth, err
}
func cloudFrame(part, finish string) string {
	return "data: {\"response\":" + strings.TrimSpace(strings.TrimPrefix(frame(part, finish), "data: ")) + "}\n\n"
}
func TestFlashHighCloudCodeToolContinuation(t *testing.T) {
	testCloudCodeToolContinuation(t, FlashHighModel, "auto")
}
func TestTieredCloudCodeToolContinuation(t *testing.T) {
	testCloudCodeToolContinuation(t, DefaultModel, "cloudcode")
}
func testCloudCodeToolContinuation(t *testing.T, model, protocol string) {
	var calls atomic.Int32
	var session string
	var previousRequestID string
	p := newFixture(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != cloudEndpoint+":streamGenerateContent?alt=sse" || req.Header.Get("Accept") != "text/event-stream" || req.Header.Get("User-Agent") != "antigravity/2.22.0 butler/0.1" {
			t.Error("wrong High route or headers")
		}
		var wire cloudRequestEnvelope
		if json.NewDecoder(req.Body).Decode(&wire) != nil {
			t.Error("invalid wire JSON")
			return nil, fmt.Errorf("invalid fixture JSON")
		}
		if wire.Model != model || wire.Project != "discovered-project" || wire.UserAgent != "antigravity" || wire.RequestType != "agent" || !strings.HasPrefix(wire.RequestID, "agent-") || wire.RequestID == previousRequestID {
			t.Error("wrong Cloud Code envelope")
		}
		previousRequestID = wire.RequestID
		body := wire.Request
		if calls.Load() == 0 {
			var systemPart, userPart struct {
				Text string `json:"text"`
			}
			if body.SystemInstruction == nil || len(body.SystemInstruction.Parts) != 1 || len(body.Contents) != 1 || len(body.Contents[0].Parts) != 1 {
				t.Error("wrapper added or removed Butler prompt parts")
			} else {
				_ = json.Unmarshal(body.SystemInstruction.Parts[0], &systemPart)
				_ = json.Unmarshal(body.Contents[0].Parts[0], &userPart)
				if systemPart.Text != "Butler owns actions" || userPart.Text != "Read example" {
					t.Error("wrapper modified Butler prompts")
				}
			}
		}
		if body.SessionID == "" || body.GenerationConfig.MaxOutputTokens != 8192 || body.GenerationConfig.ThinkingConfig == nil || body.GenerationConfig.ThinkingConfig.ThinkingBudget != -1 {
			t.Error("wrong High session/thinking/output")
		}
		if calls.Add(1) == 1 {
			session = body.SessionID
			if len(body.Tools) != 1 || len(body.Tools[0].FunctionDeclarations) != 1 || body.Tools[0].FunctionDeclarations[0].Name != "butler_tool_0" || body.SystemInstruction == nil {
				t.Error("Butler context/catalog lost")
			}
			return response(cloudFrame(`{"functionCall":{"id":"high-call","name":"butler_tool_0","args":{"url":"https://example.com"}},"thoughtSignature":"high-signature"}`, "STOP")), nil
		}
		encoded, _ := json.Marshal(body.Contents)
		if body.SessionID != session || !strings.Contains(string(encoded), `"thoughtSignature":"high-signature"`) || !strings.Contains(string(encoded), `"functionResponse":{"id":"high-call"`) {
			t.Error("live High continuation lost session or signature")
		}
		return response(cloudFrame(`{"text":"High complete"}`, "STOP")), nil
	})
	p.config.AuthSource = cloudAuthSource{project: "discovered-project"}
	p.config.Protocol, p.config.Model = protocol, model
	req := request("high-tools")
	req.Context.ModelName = model
	stream, err := p.StartRun(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, stream)
	if err := findError(events); err != nil {
		t.Fatal(err)
	}
	var proposal *transport.ToolCallRequest
	var ref *transport.ProviderSessionRef
	for _, ev := range events {
		if ev.ToolCall != nil {
			proposal = ev.ToolCall
		}
		if ev.ProviderSessionRef != nil {
			ref = ev.ProviderSessionRef
		}
	}
	if proposal == nil || ref == nil {
		t.Fatal("no High tool proposal")
	}
	stream, err = p.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: "high-tools", ProviderSessionRef: ref, ToolCallRef: proposal.ToolCallRef, ToolResultJSON: `{"status":"success","result":{"title":"Example"}}`})
	if err != nil {
		t.Fatal(err)
	}
	final := false
	for _, ev := range collect(t, stream) {
		if ev.TransportError != nil {
			t.Fatal(ev.TransportError)
		}
		if ev.AssistantFinal != nil {
			final = ev.AssistantFinal.Content == "High complete" && ev.AssistantFinal.Usage != nil && ev.AssistantFinal.Usage.InputTokens == 17
		}
	}
	if !final || calls.Load() != 2 {
		t.Fatal("High final/usage failed")
	}
}
func TestFlashHighAuthAndWholeEnvelopeLimitsDenyBeforeGeneration(t *testing.T) {
	for _, mode := range []string{"no-discovery", "discovery-error", "invalid-project", "byte-limit"} {
		t.Run(mode, func(t *testing.T) {
			p := newFixture(t, func(*http.Request) (*http.Response, error) {
				t.Error("generation written after failed admission")
				return response(""), nil
			})
			source := cloudAuthSource{project: "discovered-project", unavailable: mode == "discovery-error"}
			if mode == "invalid-project" {
				source.project = "private\nproject"
			}
			if mode != "no-discovery" {
				p.config.AuthSource = source
			}
			req := request("high-deny")
			req.Context.ModelName = FlashHighModel
			ctx := context.Background()
			if mode == "byte-limit" {
				ctx = transport.WithInputByteLimit(ctx, 250)
			}
			stream, err := p.StartRun(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			failure := findError(collect(t, stream))
			if failure == nil || strings.Contains(failure.Error(), "private account detail") {
				t.Fatal("failure missing or leaks auth detail")
			}
		})
	}
}
func TestCloudCodeSSERejectsWrongEnvelopeAndPartialTools(t *testing.T) {
	for _, wire := range []string{frame(`{"text":"flat"}`, "STOP"), `data: {"response":null}` + "\n\n", `data: {"response":{"error":{"message":"private"}}}` + "\n\n", strings.TrimSuffix(cloudFrame(`{"functionCall":{"name":"x","args":{}}}`, "STOP"), "\n\n"), cloudFrame(`{"functionCall":{"name":"x","args":{}}}`, "MAX_TOKENS"), `data: {"response":{"automaticFunctionCallingHistory":[{}]}}` + "\n\n"} {
		if _, err := readStreamProtocol(strings.NewReader(wire), func(string) bool { return true }, true); err == nil {
			t.Fatal("invalid Cloud Code response accepted")
		}
	}
}
