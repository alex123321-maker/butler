package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestBrowserFormValidationChromiumIntegration(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_E2E") != "1" {
		t.Skip("isolated PG and actual Chromium required")
	}
	var posts atomic.Int32
	var accepted atomic.Bool
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.Method == "POST" {
			posts.Add(1)
			accepted.Store(r.ParseForm() == nil && r.PostForm.Get("quantity") == "3")
			fmt.Fprint(w, "Saved")
			return
		}
		fmt.Fprint(w, `<form method=post><input name=quantity type=number min=1 max=5 step=1 required><button id=submit>Save</button></form>`)
	}))
	defer site.Close()
	t.Setenv("BUTLER_RESOURCE_BROWSER_FIXTURE", site.URL)
	f := resourceBroker(t, nil, false)
	id, url, version := browserSafetyOpen(t, f)
	ctx := context.Background()
	for index, value := range []string{"8", "3"} {
		call := f.callRequest(t, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: fmt.Sprintf("validation-%d", index), ToolName: "browser.prepare_fill", AutonomyMode: 3, ArgsJson: fmt.Sprintf(`{"browser_session_id":%q,"expected_url":%q,"observation_version":%d,"selector":"input[name=quantity]","value":%q}`, id, url, version, value)})
		response, err := f.server.ExecuteToolCall(ctx, call)
		var view struct {
			Version int64  `json:"observation_version"`
			Network string `json:"network_state"`
			Forms   []struct {
				Fields []struct {
					Validation *struct {
						Valid  bool     `json:"valid"`
						Issues []string `json:"issues"`
					} `json:"validation"`
				} `json:"fields"`
			} `json:"forms"`
		}
		result := response.GetResult()
		if err != nil || result.GetStatus() != "completed" || result.GetEffectState() != "effect_confirmed" || json.Unmarshal([]byte(result.GetResultJson()), &view) != nil || view.Network != "frozen" || len(view.Forms) != 1 || len(view.Forms[0].Fields) != 1 {
			t.Fatal("ordinary local preparation/metadata failed")
		}
		validation := view.Forms[0].Fields[0].Validation
		if validation == nil || validation.Valid != (value == "3") || (value == "8" && (len(validation.Issues) != 1 || validation.Issues[0] != "range_overflow")) || posts.Load() != 0 {
			t.Fatal("validation state or zero-dispatch boundary incorrect")
		}
		version = view.Version
	}
	click := f.callRequest(t, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "validation-submit", ToolName: "browser.click", AutonomyMode: 3, ArgsJson: fmt.Sprintf(`{"browser_session_id":%q,"expected_url":%q,"observation_version":%d,"selector":"#submit"}`, id, url, version)})
	response, err := f.server.ExecuteToolCall(ctx, click)
	if err != nil || response.GetResult().GetEffectState() != "effect_unknown" || posts.Load() != 1 || !accepted.Load() {
		t.Fatal("corrected form did not submit exactly once with retained uncertainty")
	}
	_, _ = f.server.ExecuteToolCall(ctx, click)
	if posts.Load() != 1 {
		t.Fatal("spent submit grant replayed")
	}
	t.Log("invalid_public_value_observed=true invalid_posts=0 corrected_quantity=3 accepted_posts=1 post_state=effect_unknown no_replay=true")
}
