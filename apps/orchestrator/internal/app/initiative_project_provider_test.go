package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/extensions/projectfixture"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/butler/butler/internal/transport"
)

// Fixed component transport, never generation acceptance. Every returned tool
// result and artifact ID must still come from the actual Broker/recipient.
type initiativeProjectScriptedProvider struct {
	mu       sync.Mutex
	run      string
	phase    int
	artifact string
	entries  atomic.Int32
}

func (*initiativeProjectScriptedProvider) Name() string                    { return "fixture" }
func (*initiativeProjectScriptedProvider) InferenceLimits() (int64, int64) { return 1024, 8192 }
func (*initiativeProjectScriptedProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{SupportsStreaming: true, SupportsToolCalls: true, SupportsCancel: true, SupportsUsageMetadata: true}, nil
}
func initiativeProjectStream(events ...transport.TransportEvent) transport.EventStream {
	out := make(chan transport.TransportEvent, len(events))
	for _, event := range events {
		out <- event
	}
	close(out)
	return out
}
func (p *initiativeProjectScriptedProvider) proposal(run, call, tool string, args any) transport.EventStream {
	raw, _ := json.Marshal(args)
	return initiativeProjectStream(transport.NewToolCallRequestedEvent(run, p.Name(), transport.ToolCallRequest{ToolCallRef: call, ToolName: tool, ArgsJSON: string(raw), SequenceNo: p.phase}))
}
func (p *initiativeProjectScriptedProvider) StartRun(_ context.Context, request transport.StartRunRequest) (transport.EventStream, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.phase != 0 || request.Context.ProviderSessionRef != nil {
		return nil, errors.New("unexpected fixture model restart")
	}
	names := map[string]bool{}
	for _, definition := range request.ToolDefinitions {
		names[definition.Name] = true
	}
	if len(names) != 3 || !names["extension.project.inspect"] || !names["extension.project.prepare-patch"] || !names["extension.project.check"] {
		return nil, errors.New("exact initiative tools unavailable")
	}
	p.entries.Add(1)
	p.phase, p.run = 1, request.Context.RunID
	return p.proposal(p.run, "initiative-model-inspect", "extension.project.inspect", map[string]any{"url": egresstest.Origin + "/project"}), nil
}
func initiativeProjectToolBody(raw string) (map[string]any, error) {
	var envelope struct {
		Body   string          `json:"body"`
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal([]byte(raw), &envelope) != nil {
		return nil, errors.New("invalid project tool envelope")
	}
	if envelope.Body == "" && len(envelope.Result) != 0 {
		if json.Unmarshal(envelope.Result, &envelope) != nil {
			return nil, errors.New("invalid project tool result")
		}
	}
	var body map[string]any
	if json.Unmarshal([]byte(envelope.Body), &body) != nil {
		return nil, errors.New("actual project body unavailable")
	}
	return body, nil
}
func (p *initiativeProjectScriptedProvider) SubmitToolResult(_ context.Context, request transport.SubmitToolResultRequest) (transport.EventStream, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if request.RunID != p.run {
		return nil, errors.New("foreign project model run")
	}
	body, err := initiativeProjectToolBody(request.ToolResultJSON)
	if err != nil {
		return nil, err
	}
	p.entries.Add(1)
	switch p.phase {
	case 1:
		if request.ToolCallRef != "initiative-model-inspect" || body["checks_passed"] != false || body["project_revision"] != projectfixture.Revision() {
			return nil, errors.New("actual failed project inspection missing")
		}
		p.phase = 2
		return p.proposal(p.run, "initiative-model-patch", "extension.project.prepare-patch", map[string]any{"url": egresstest.Origin + "/project/patches", "body": projectfixture.Proposal{ProjectRevision: projectfixture.Revision(), Changes: []projectfixture.Change{{Path: "README.md", Before: projectfixture.Original, After: projectfixture.Prepared}}}}), nil
	case 2:
		artifact, ok := body["artifact_id"].(string)
		if request.ToolCallRef != "initiative-model-patch" || !ok || len(artifact) != 64 {
			return nil, errors.New("actual recipient patch receipt missing")
		}
		p.artifact, p.phase = artifact, 3
		return p.proposal(p.run, "initiative-model-check", "extension.project.check", map[string]any{"url": egresstest.Origin + "/project/patches/" + artifact + "/checks"}), nil
	case 3:
		if request.ToolCallRef != "initiative-model-check" || body["artifact_id"] != p.artifact || body["checks_passed"] != true || body["applied"] != false {
			return nil, errors.New("actual stored patch checks missing")
		}
		p.phase = 4
		final, _ := json.Marshal(map[string]any{"artifact_id": p.artifact, "checks_passed": true, "applied": false})
		return initiativeProjectStream(transport.NewAssistantFinalEvent(p.run, p.Name(), transport.AssistantFinal{Content: string(final), FinishReason: "stop"}), transport.NewTerminalEvent(p.run, transport.EventTypeRunCompleted, p.Name())), nil
	default:
		return nil, errors.New("unexpected extra initiative model turn")
	}
}
func (*initiativeProjectScriptedProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, errors.New("fixture continuation unavailable")
}
func (*initiativeProjectScriptedProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}

// Final projection is an owner-declared fixture protocol, not a verifier. The
// artifact/checks below are independently checked before owner verification.
func initiativeProjectFinal(raw, artifact string) error {
	if len(raw) > 4096 {
		return errors.New("initiative final too large")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return errors.New("initiative final object required")
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := key.(string)
		if !ok || name != "artifact_id" && name != "checks_passed" && name != "applied" {
			return errors.New("unknown initiative final field")
		}
		if _, exists := fields[name]; exists {
			return errors.New("duplicate initiative final field")
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return errors.New("invalid initiative final field")
		}
		fields[name] = value
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim('}') || decoder.Decode(new(any)) != io.EOF || len(fields) != 3 {
		return errors.New("exact initiative final fields required")
	}
	var id string
	var checked, applied bool
	if json.Unmarshal(fields["artifact_id"], &id) != nil || json.Unmarshal(fields["checks_passed"], &checked) != nil || json.Unmarshal(fields["applied"], &applied) != nil || string(fields["checks_passed"]) == "null" || string(fields["applied"]) == "null" || id != artifact || !checked || applied {
		return errors.New("initiative final disagrees with actual artifact/checks")
	}
	return nil
}

func TestInitiativeProjectFinalProjection(t *testing.T) {
	good := `{"artifact_id":"artifact","checks_passed":true,"applied":false}`
	if initiativeProjectFinal(good, "artifact") != nil {
		t.Fatal("valid exact projection denied")
	}
	for _, bad := range []string{"```json\n" + good + "\n```", strings.Replace(good, `"artifact"`, `"other"`, 1), strings.Replace(good, `true`, `false`, 1), strings.Replace(good, `false`, `true`, 1), strings.Replace(good, `"applied":false`, `"applied":null`, 1), strings.TrimSuffix(good, "}") + `,"checks_passed":true}`, strings.Replace(good, `checks_passed`, `Checks_passed`, 1), good + good} {
		if initiativeProjectFinal(bad, "artifact") == nil {
			t.Fatal("invalid projection accepted")
		}
	}
}
