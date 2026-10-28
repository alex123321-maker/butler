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
