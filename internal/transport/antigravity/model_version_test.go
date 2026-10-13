package antigravity

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestObservedModelVersionRemainsOptionalConsistentAndUnaliased(t *testing.T) {
	for _, high := range []bool{false, true} {
		for _, mode := range []string{"missing", "reported", "conflicting_trailer", "invalid", "aliased"} {
			t.Run(map[bool]string{false: "tiered", true: "high"}[high]+"/"+mode, func(t *testing.T) {
				body := frame(`{"text":"done"}`, "STOP")
				if high {
					body = cloudFrame(`{"text":"done"}`, "STOP")
				}
				body = strings.Replace(body, `"modelVersion":"gemini-3.8-flash",`, "", 1)
				version := `"modelVersion":"gemini-3.8-flash-observed",`
				if mode == "invalid" {
					version = `"modelVersion":"untrusted arbitrary prose",`
				}
				if mode == "aliased" {
					version += `"ModelVersion":"gemini-3.8-flash-other",`
				}
				if mode != "missing" {
					body = strings.Replace(body, `"usageMetadata":`, version+`"usageMetadata":`, 1)
				}
				if mode == "conflicting_trailer" {
					trailer := `{"modelVersion":"gemini-3.8-flash-other"}`
					if high {
						trailer = `{"response":` + trailer + `}`
					}
					body += "data: " + trailer + "\n\n"
				}
				p := newFixture(t, func(*http.Request) (*http.Response, error) { return response(body), nil })
				req := request("observed-version")
				if high {
					p.config.AuthSource = cloudAuthSource{project: "discovered-project"}
					req.Context.ModelName = FlashHighModel
				}
				stream, err := p.StartRun(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				events := collect(t, stream)
				failed := mode != "missing" && mode != "reported"
				if (findError(events) != nil) != failed {
					t.Fatal("unexpected version metadata result")
				}
				finals := 0
				for _, event := range events {
					if failed && (event.ProviderModelVersion != nil || event.AssistantFinal != nil || event.ToolCall != nil) {
						t.Fatal("invalid version escaped as a complete observed result")
					}
					if event.AssistantFinal != nil {
						finals++
						if mode == "missing" && event.ProviderModelVersion != nil || mode == "reported" && (event.ProviderModelVersion == nil || *event.ProviderModelVersion != "gemini-3.8-flash-observed") {
							t.Fatal("missing version invented or known version lost")
						}
					}
				}
				if !failed && finals != 1 {
					t.Fatal("complete final missing")
				}
			})
		}
	}
}
