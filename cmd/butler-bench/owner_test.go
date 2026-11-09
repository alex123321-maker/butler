package main

import (
	"context"
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
