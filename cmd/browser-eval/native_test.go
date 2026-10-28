package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testInput = "input-0123456789abcdef0123456789abcdef"

func TestNativeAttemptsNeverTreatInputCompletionAsTaskSuccess(t *testing.T) {
	for _, status := range []string{"completed", "failed", "waiting_for_user", "blocked"} {
		t.Run(status, func(t *testing.T) {
			var submissions atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer operator-canary-token" {
					t.Error("missing owner proof")
				}
				if r.Method == "POST" {
					submissions.Add(1)
					if r.URL.Path != "/api/v3/communications/messages" || r.Header.Get("X-Butler-Delivery-ID") != "browser-baseline-test" {
						t.Error("wrong ingress")
					}
					var body map[string]string
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body["text"] != "Use the browser" {
						t.Error("prompt changed")
					}
					fmt.Fprintf(w, `{"input_id":%q,"status":"queued"}`, testInput)
				} else {
					fmt.Fprintf(w, `{"input_id":%q,"run_id":"run-123-abc","status":%q,"error_message":"secret-canary"}`, testInput, status)
				}
			}))
			defer s.Close()
			c, err := newNativeClient(s.URL, "operator-canary-token")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			dir := filepath.Join(t.TempDir(), "attempt")
			if err := nativeAttempt(ctx, c, "Use the browser", "browser-baseline-test", dir, time.Millisecond); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "02-observation.json"))
			if err != nil {
				t.Fatal(err)
			}
			var observation nativeObservation
			if err := json.Unmarshal(data, &observation); err != nil {
				t.Fatal(err)
			}
			if observation.Outcome != "INPUT_"+strings.ToUpper(status) || observation.TaskSuccess != "UNVERIFIED" || observation.Effect != "UNKNOWN" || observation.DeliveryStatus != "UNVERIFIED" {
				t.Fatal("completion overclaimed")
			}
			if strings.Contains(string(data), "canary") {
				t.Fatal("private API content exported")
			}
			if err := nativeAttempt(ctx, c, "Use the browser", "browser-baseline-test", dir, time.Millisecond); err == nil || submissions.Load() != 1 {
				t.Fatal("attempt replayed")
			}
		})
	}
}

func TestNativeLostSubmissionIsNotReplayed(t *testing.T) {
	var submissions atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		submissions.Add(1)
		c, _, err := http.NewResponseController(w).Hijack()
		if err == nil {
			_ = c.Close()
		}
	}))
	defer s.Close()
	c, _ := newNativeClient(s.URL, "operator-canary-token")
	dir := filepath.Join(t.TempDir(), "attempt")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := nativeAttempt(ctx, c, "Use browser", "browser-baseline-lost", dir, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "01-submission.json"))
	if !strings.Contains(string(data), "SUBMISSION_UNKNOWN") || submissions.Load() != 1 {
		t.Fatal("unknown submission lost or replayed")
	}
}

func TestNativeOwnerProofDoesNotFollowRedirects(t *testing.T) {
	var followed atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed.Add(1) }))
	defer target.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer s.Close()
	c, _ := newNativeClient(s.URL, "operator-canary-token")
	_, status, err := c.call(context.Background(), "POST", "/api/v3/communications/messages", "browser-baseline-redirect", []byte(`{}`))
	if err == nil || status != 307 || followed.Load() != 0 {
		t.Fatal("owner proof followed redirect")
	}
	for _, endpoint := range []string{"http://example.org", "https://user:password@example.org", "https://example.org/?token=bad", "https://example.org/#token"} {
		if _, err := newNativeClient(endpoint, "operator-canary-token"); err == nil {
			t.Fatal("unsafe endpoint accepted")
		}
	}
}

func TestNativeInvalidReceiptDoesNotExportServerContent(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"input_id":"secret-canary","run_id":"secret-canary"}`)
	}))
	defer s.Close()
	c, _ := newNativeClient(s.URL, "operator-canary-token")
	r, _, err := c.call(context.Background(), "POST", "/api/v3/communications/messages", "browser-baseline-invalid", nil)
	if err == nil || r.InputID != "" || r.RunID != "" {
		t.Fatal("untrusted receipt exported")
	}
}

func TestNativeTimeoutPreservesAttemptWithoutResubmitting(t *testing.T) {
	var submissions atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			submissions.Add(1)
		}
		fmt.Fprintf(w, `{"input_id":%q,"status":"running"}`, testInput)
	}))
	defer s.Close()
	c, _ := newNativeClient(s.URL, "operator-canary-token")
	dir := filepath.Join(t.TempDir(), "attempt")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := nativeAttempt(ctx, c, "Use browser", "browser-baseline-timeout", dir, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "02-observation.json"))
	if !strings.Contains(string(data), "OBSERVATION_TIMEOUT") || submissions.Load() != 1 {
		t.Fatal("timeout lost or resubmitted")
	}
}

func TestNativeResumeObservesSameInputWithoutPosting(t *testing.T) {
	var posts atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			posts.Add(1)
		}
		if r.URL.Path != "/api/v3/inputs/"+testInput {
			t.Error("wrong resumed Input")
		}
		fmt.Fprintf(w, `{"input_id":%q,"status":"completed"}`, testInput)
	}))
	defer s.Close()
	dir := t.TempDir()
	token := filepath.Join(dir, "token")
	if err := os.WriteFile(token, []byte("operator-canary-token"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-endpoint", s.URL, "-owner-token-file", token, "-delivery-id", "browser-baseline-resume", "-observe-input", testInput, "-attempt-dir", filepath.Join(dir, "resume"), "-timeout", "5s"}
	if err := runNative(args); err != nil {
		t.Fatal(err)
	}
	if posts.Load() != 0 {
		t.Fatal("resume submitted new Input")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "resume", "02-observation.json"))
	if !strings.Contains(string(data), "INPUT_COMPLETED") {
		t.Fatal("resume did not observe completion")
	}
}
