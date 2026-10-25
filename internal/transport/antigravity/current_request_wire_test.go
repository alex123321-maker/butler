package antigravity

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	securitycontent "github.com/butler/butler/internal/security/content"
)

// The provider sends text, not arbitrary InputItem metadata. Request/history
// identities therefore must survive in the actual native request body.
func TestCurrentRequestAndHistoricalAnswerIdentityReachNativeWire(t *testing.T) {
	for _, phase := range []string{"owner_request", "answered_question"} {
		t.Run(phase, func(t *testing.T) {
			p := newFixture(t, func(req *http.Request) (*http.Response, error) {
				raw, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatal(err)
				}
				for _, want := range []string{"current-run", "input:current-event", phase, "old-run", "interaction_response", "старый ответ", "Спроси заново"} {
					if !strings.Contains(string(raw), want) {
						t.Errorf("native wire lost %q", want)
					}
				}
				return response(frame(`{"text":"done"}`, "STOP")), nil
			})
			q := request("current-run")
			q.InputItems = append(q.InputItems,
				securitycontent.Data(map[string]any{"kind": "historical_message", "run_ref": "old-run", "message_kind": "interaction_response", "content": "старый ответ"}, securitycontent.Origin{Kind: securitycontent.MemoryDerived, SourceRefs: []string{"transcript:old"}, Sensitivity: securitycontent.Private}),
				securitycontent.CurrentOwnerMessage("Спроси заново", "input:current-event"),
				securitycontent.Data(map[string]any{"kind": "current_request", "run_ref": "current-run", "input_event_ref": "input:current-event", "phase": phase}, securitycontent.Origin{Kind: securitycontent.MemoryDerived, SourceRefs: []string{"input:current-event"}, Sensitivity: securitycontent.Private}))
			stream, err := p.StartRun(context.Background(), q)
			if err != nil {
				t.Fatal(err)
			}
			if transportErr := findError(collect(t, stream)); transportErr != nil {
				t.Fatal(transportErr)
			}
		})
	}
}
