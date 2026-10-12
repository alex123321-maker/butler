package antigravity

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/transport"
)

func TestFlashHighSequentialAdmissionReceiptsKeepExactEnvelopeAndFinal(t *testing.T) {
	var calls atomic.Int32
	receipts := []delegation.AdmissionReceipt{{TaskID: "task:first", InputID: "input:first", State: "queued", ContractRevision: 1}, {TaskID: "task:second", InputID: "", State: "waiting", ContractRevision: 1}}
	finalJSON := `{"admitted_task_ids":["task:first","task:second"]}`
	args := `{"contract_source":{"source_id":"contract","revision":1,"access_revision":1,"content_hash":"` + strings.Repeat("a", 64) + `"},"source_refs":[],"budget":{"model_turns":1,"tokens":1024,"cost_micros":0,"external_calls":0,"children":0,"active_millis":1000,"egress_bytes":0,"provider_input_bytes":2048},"deadline":"2026-10-12T00:00:00Z","dependencies":[]}`
	p := newFixture(t, func(request *http.Request) (*http.Response, error) {
		var wire cloudRequestEnvelope
		if json.NewDecoder(request.Body).Decode(&wire) != nil || wire.Model != FlashHighModel {
			t.Error("synthetic High request shape invalid")
		}
		turn := int(calls.Add(1))
		observed := []delegation.AdmissionReceipt{}
		for _, content := range wire.Request.Contents {
			for _, part := range content.Parts {
				var decoded struct {
					FunctionResponse *struct {
						Name     string `json:"name"`
						ID       string `json:"id"`
						Response struct {
							Status string                      `json:"status"`
							Result delegation.AdmissionReceipt `json:"result"`
						} `json:"response"`
					} `json:"functionResponse"`
				}
				if json.Unmarshal(part, &decoded) != nil {
					t.Error("synthetic function response invalid")
				}
				if decoded.FunctionResponse != nil {
					response := decoded.FunctionResponse
					id := []string{"native-first", "native-second"}[len(observed)]
					if content.Role != "user" || response.Name != "butler_tool_0" || response.ID != id || response.Response.Status != "completed" {
						t.Error("live receipt lost native call correlation or Broker envelope")
					}
					observed = append(observed, response.Response.Result)
				}
			}
		}
		if turn > 3 || !reflect.DeepEqual(observed, receipts[:turn-1]) {
			t.Error("native continuation omitted, duplicated or changed an actual result envelope")
		}
		serialized, _ := json.Marshal(wire.Request.Contents)
		if turn > 1 && !strings.Contains(string(serialized), `"thoughtSignature":"signature-first"`) || turn > 2 && !strings.Contains(string(serialized), `"thoughtSignature":"signature-second"`) {
			t.Error("receipt continuation lost original native signatures")
		}
		if turn == 1 {
			return response(cloudFrame(`{"text":"First intermediate commentary."},{"functionCall":{"id":"native-first","name":"butler_tool_0","args":`+args+`},"thoughtSignature":"signature-first"}`, "STOP")), nil
		}
		if turn == 2 {
			return response(cloudFrame(`{"text":"Second intermediate commentary."},{"functionCall":{"id":"native-second","name":"butler_tool_0","args":`+args+`},"thoughtSignature":"signature-second"}`, "STOP")), nil
		}
		text, _ := json.Marshal(finalJSON)
		return response(cloudFrame(`{"text":`+string(text)+`}`, "STOP")), nil
	})
	p.config.AuthSource = cloudAuthSource{project: "synthetic-project"}
	req := request("high-receipts")
	req.Context.ModelName = FlashHighModel
	contract := delegation.AdmissionContract()
	req.ToolDefinitions = []transport.ToolDefinition{{Name: contract.ToolName, Description: contract.Description, SchemaJSON: contract.InputSchemaJson}}
	stream, err := p.StartRun(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for turn := 0; turn < 3; turn++ {
		events := collect(t, stream)
		if err := findError(events); err != nil {
			t.Fatal(err)
		}
		var proposal *transport.ToolCallRequest
		var ref *transport.ProviderSessionRef
		var final *transport.AssistantFinal
		for _, event := range events {
			if event.ToolCall != nil {
				proposal = event.ToolCall
			}
			if event.ProviderSessionRef != nil {
				ref = event.ProviderSessionRef
			}
			if event.AssistantFinal != nil {
				final = event.AssistantFinal
			}
		}
		if turn == 2 {
			if proposal != nil || final == nil || final.Content != finalJSON {
				t.Fatal("tool-phase text was combined into the terminal High final")
			}
			break
		}
		if proposal == nil || ref == nil || final != nil || proposal.ToolName != delegation.AdmissionTool {
			t.Fatal("tool turn produced a final or lost its scoped proposal")
		}
		envelope, _ := json.Marshal(struct {
			Status string                      `json:"status"`
			Result delegation.AdmissionReceipt `json:"result"`
		}{"completed", receipts[turn]})
		stream, err = p.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: req.Context.RunID, ProviderSessionRef: ref, ToolCallRef: proposal.ToolCallRef, ToolResultJSON: string(envelope)})
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 3 {
		t.Fatal("wrong synthetic native request count")
	}
}
