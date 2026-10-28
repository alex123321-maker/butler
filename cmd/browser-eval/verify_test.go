package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublicControlVerificationChecksExactProtectedPublication(t *testing.T) {
	ref := "publication:" + strings.Repeat("a", 64)
	dir := t.TempDir()
	token := filepath.Join(dir, "token")
	if err := os.WriteFile(token, []byte("operator-canary-token"), 0600); err != nil {
		t.Fatal(err)
	}
	valid := true
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/v3/communications/"+ref || r.Header.Get("Authorization") != "Bearer operator-canary-token" {
			t.Error("wrong owner verification")
		}
		actual := ref
		if !valid {
			actual = "publication:" + strings.Repeat("b", 64)
		}
		fmt.Fprintf(w, `{"publication_ref":%q,"available":true,"payload":{"text":"Example Domain https://example.com для документации. secret-canary"}}`, actual)
	}))
	defer s.Close()
	out := filepath.Join(dir, "verified.json")
	args := []string{"-endpoint", s.URL, "-owner-token-file", token, "-publication-ref", ref, "-output", out}
	if err := verifyPublicControl(args); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	var report map[string]any
	_ = json.Unmarshal(data, &report)
	for _, key := range []string{"title_present", "link_present", "documentation_purpose_present"} {
		if report[key] != true {
			t.Fatalf("missing criterion %s", key)
		}
	}
	if strings.Contains(string(data), "canary") {
		t.Fatal("private text exported")
	}
	if err := verifyPublicControl(args); err == nil {
		t.Fatal("verification overwritten")
	}
	valid = false
	args[len(args)-1] = filepath.Join(dir, "wrong.json")
	if err := verifyPublicControl(args); err == nil {
		t.Fatal("wrong publication accepted")
	}
}

func TestFixtureVerificationRejectsWrongAnswerDuplicateEffectAndSecret(t *testing.T) {
	ref := "publication:" + strings.Repeat("c", 64)
	for _, tc := range []struct {
		name, scenario, text string
		kinds                []string
		qty                  int
		match                bool
	}{
		{"read", "read", "expected-public", []string{"read"}, 0, true},
		{"wrong answer", "read", "other", []string{"read"}, 0, false},
		{"secret in answer", "read", "expected-public password-canary", []string{"read"}, 0, false},
		{"form", "form", "saved", []string{"form_submission"}, 3, true},
		{"wrong quantity", "form", "saved", []string{"form_submission"}, 2, false},
		{"duplicate submit", "form", "saved", []string{"form_submission", "form_submission"}, 3, false},
		{"login proof absent", "login", "expected-private", []string{"account_read"}, 0, false},
		{"login", "login", "expected-private", []string{"login", "account_read"}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			token := filepath.Join(dir, "token")
			cfg := filepath.Join(dir, "private.json")
			journal := filepath.Join(dir, "recipient.jsonl")
			out := filepath.Join(dir, "report.json")
			if err := os.WriteFile(token, []byte("operator-canary-token"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := exclusiveJSON(cfg, fixtureConfig{"username-canary", "password-canary", "expected-public", "expected-record", "expected-private"}); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(journal)
			if err != nil {
				t.Fatal(err)
			}
			for i, kind := range tc.kinds {
				if err := json.NewEncoder(f).Encode(receipt{Sequence: i + 1, At: time.Now(), Kind: kind, Accepted: true, Quantity: tc.qty}); err != nil {
					t.Fatal(err)
				}
			}
			_ = f.Close()
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"publication_ref": ref, "available": true, "payload": map[string]string{"text": tc.text}})
			}))
			defer s.Close()
			args := []string{"-endpoint", s.URL, "-owner-token-file", token, "-publication-ref", ref, "-scenario", tc.scenario, "-config", cfg, "-journal", journal, "-output", out}
			if err := verifyFixture(args); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(out)
			var report map[string]any
			_ = json.Unmarshal(data, &report)
			if report["recipient_and_answer_criteria_match"] != tc.match {
				t.Fatal("fixture verification overclaimed or missed result")
			}
			if strings.Contains(string(data), "password-canary") || strings.Contains(string(data), "expected-public") {
				t.Fatal("private input/answer exported")
			}
		})
	}
}
