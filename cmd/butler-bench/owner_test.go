package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOwnerApprovesOnlyEnrolledRun(t *testing.T) {
	for _, run := range []string{"enrolled", "other"} {
		posts := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer lab" || r.Header.Get("X-Butler-Delivery-ID") == "" {
				t.Error("missing authenticated delivery")
			}
			status := "pending"
			if r.Method == "POST" {
				posts++
				status = "approved"
			}
			fmt.Fprintf(w, `{"approval":{"approval_id":"a","run_id":%q,"session_key":"s","status":%q}}`, run, status)
		}))
		c := liveClient{http: s.Client(), endpoint: s.URL, token: "lab"}
		d := c.approve(context.Background(), snapshot{approvalID: "a", run: "enrolled"}, "s", "event")
		s.Close()
		if d.Confirmed != (run == "enrolled") || posts != map[bool]int{true: 1, false: 0}[run == "enrolled"] {
			t.Fatal(run, d, posts)
		}
	}
}
func TestSeparateFactualReview(t *testing.T) {
	var outcome string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, `{"reviews":[{"review_id":"review","run_id":"enrolled","fingerprint":"fixed","revision":1}]}`)
			return
		}
		var body struct {
			Decision struct {
				Outcome string
				Checked bool `json:"recipient_checked"`
			}
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Decision.Checked {
			t.Error("invented evidence")
			return
		}
		outcome = body.Decision.Outcome
		fmt.Fprint(w, `{"review":{"review_id":"review","run_id":"enrolled","state":"unresolved"}}`)
	}))
	defer s.Close()
	c := liveClient{http: s.Client(), endpoint: s.URL, token: "lab"}
	d := c.reportUnknown(context.Background(), snapshot{reviewID: "review", run: "enrolled"}, "event")
	if !d.Confirmed || outcome != "unknown" {
		t.Fatal(d, outcome)
	}
}
func TestOutcomeEquivalentHTMLAndAcknowledgement(t *testing.T) {
	s := Step{Rule: "two_parses", Witness: "VALUE", Answer: "VALUE", Target: "ID", Allowed: []string{"http.parse_html"}}
	calls := []call{{Name: "http.parse_html", Status: "completed", Args: json.RawMessage(`{"content":"<div><i>ID</i><b class='active'>VALUE</b></div><div><b class='active'>VALUE</b><s>ID</s></div>","selector":"b.active"}`), Result: json.RawMessage(`{"matches":["VALUE","VALUE"]}`)}}
	if grade(s, "VALUE", calls) != "pass" || grade(s, "WRONG", calls) == "pass" {
		t.Fatal("combined evidence")
	}
	calls[0].Args = json.RawMessage(`{"content":"VALUE VALUE","selector":"b.active"}`)
	if grade(s, "VALUE", calls) == "pass" {
		t.Fatal("missing original sources")
	}
	if grade(Step{Answer: "ПРИНЯТО"}, "ПРИНЯТО.", nil) != "pass" || grade(Step{Answer: "110"}, "110.", nil) == "pass" {
		t.Fatal("ack scope")
	}
}
