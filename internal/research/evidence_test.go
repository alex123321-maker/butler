package research

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/transcript"
)

func TestResearchEvidenceRequiresActualFreshRead(t *testing.T) {
	now := time.Now().UTC()
	for _, subject := range []struct{ url, quote string }{
		{"https://publisher.example/releases/current", "Version 8.3 will be available on 12 November."},
		{"https://venue.example/events/autumn", "The opening has been postponed until 18 November."},
		{"https://operator.example/timetable", "The 10:30 departure is cancelled on Sunday."},
	} {
		t.Run(subject.url, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"requested_url": subject.url, "final_url": subject.url, "status_code": 200, "content_text": "Announcement\n" + subject.quote})
			call := transcript.ToolCall{RunID: "current", ToolName: "web.fetch", Status: "completed", ResultJSON: string(body), FinishedAt: &now}
			pages, research := Pages([]transcript.ToolCall{call}, "current")
			e := Evidence{URL: subject.url, Quote: subject.quote, Role: "primary"}
			answer := "Requested fact. [Source](" + subject.url + ")"
			if !research || Check(answer, []Evidence{e}, pages, now, false) != "" {
				t.Fatal("actual primary reference refused")
			}
			for _, mutate := range []func(*transcript.ToolCall){
				func(c *transcript.ToolCall) { c.RunID = "previous" },
				func(c *transcript.ToolCall) { c.ToolName = "web.search" },
				func(c *transcript.ToolCall) { c.ToolName = "web.extract" },
				func(c *transcript.ToolCall) { c.Status = "failed" },
				func(c *transcript.ToolCall) { old := now.Add(-25 * time.Hour); c.FinishedAt = &old },
				func(c *transcript.ToolCall) { c.FinishedAt = nil },
				func(c *transcript.ToolCall) { c.ResultJSON = `{"status_code":403,"content_text":"denied"}` },
			} {
				c := call
				mutate(&c)
				p, _ := Pages([]transcript.ToolCall{c}, "current")
				if Supports(e, p, now) {
					t.Fatal("non-evidence accepted", c)
				}
			}
			e.Quote = "This exact claim was never present in the page."
			if Check(answer, []Evidence{e}, pages, now, false) == "" {
				t.Fatal("invented quote accepted")
			}
			e.Quote = subject.quote
			e.Role = "secondary"
			if Check(answer, []Evidence{e}, pages, now, false) == "" {
				t.Fatal("secondary-only result represented as verified")
			}
			e.Role = "primary"
			if Check(answer+" https://unread.example/", []Evidence{e}, pages, now, false) == "" {
				t.Fatal("unread citation accepted")
			}
			if Check(strings.Repeat("лишнее ", 300)+answer, []Evidence{e}, pages, now, false) == "" {
				t.Fatal("unrequested long answer accepted")
			}
			if Check(strings.Repeat("detail ", 300)+answer, []Evidence{e}, pages, now, true) != "" {
				t.Fatal("requested detail blocked")
			}
		})
	}
}

func TestResearchBatchPartialFailureAndRedirectFragment(t *testing.T) {
	now := time.Now()
	c := transcript.ToolCall{RunID: "r", ToolName: "web.fetch_batch", Status: "completed", FinishedAt: &now, ResultJSON: `{"items":[{"error":"denied"},{"requested_url":"https://issuer.example/old","final_url":"https://issuer.example/current","status_code":200,"content_text":"A fresh announcement confirms availability."}]}`}
	p, _ := Pages([]transcript.ToolCall{c}, "r")
	if len(p) != 2 || !Supports(Evidence{URL: "https://issuer.example/current#news", Quote: "A fresh announcement confirms availability.", Role: "primary"}, p, now) {
		t.Fatal(p)
	}
}

func TestResearchAlternateReadersDoNotAcceptModelSuppliedExtraction(t *testing.T) {
	now := time.Now()
	quote := "The event has been moved to Monday."
	for _, tc := range []struct {
		name, args, result string
		want               bool
	}{
		{"web.extract", `{"url":"https://issuer.example/"}`, `{"status_code":200,"final_url":"https://issuer.example/","content_text":"The event has been moved to Monday."}`, true},
		{"web.extract", `{"url":"https://issuer.example/","text":"The event has been moved to Monday."}`, `{"status_code":200,"final_url":"https://issuer.example/","content_text":"The event has been moved to Monday."}`, false},
		{"http.request", `{"url":"https://issuer.example/","method":"GET"}`, `{"status_code":200,"body":"<h1>The event has been <em>moved</em> to Monday.</h1>"}`, true},
		{"http.request", `{"url":"https://issuer.example/","method":"POST"}`, `{"status_code":200,"body":"The event has been moved to Monday."}`, false},
		{"browser.snapshot", `{"browser_session_id":"session"}`, `{"ok":true,"final_url":"https://issuer.example/","text":"The event has been moved to Monday."}`, true},
		{"browser.snapshot", `{}`, `{"ok":true,"network_state":"frozen","final_url":"https://issuer.example/","text":"The event has been moved to Monday."}`, false},
	} {
		t.Run(tc.name+tc.args, func(t *testing.T) {
			p, _ := Pages([]transcript.ToolCall{{RunID: "r", ToolName: tc.name, ArgsJSON: tc.args, ResultJSON: tc.result, Status: "completed", FinishedAt: &now}}, "r")
			if Supports(Evidence{URL: "https://issuer.example/", Quote: quote, Role: "primary"}, p, now) != tc.want {
				t.Fatal("reader provenance mismatch", tc, p)
			}
		})
	}
}
