package projectfixture

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPreparedArtifactIsDurableBoundedAndIdempotent(t *testing.T) {
	directory := t.TempDir()
	s, err := New(directory)
	if err != nil {
		t.Fatal(err)
	}
	p := Proposal{ProjectRevision: Revision(), Changes: []Change{{Path: "README.md", Before: Original, After: Prepared}}}
	body, _ := json.Marshal(p)
	send := func(s *Server, raw []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/project/patches", bytes.NewReader(raw))
		r.Header.Set("Idempotency-Key", "system-key")
		r.Header.Set("Butler-Action-Fingerprint", strings.Repeat("a", 64))
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	first := send(s, body)
	if first.Code != 201 {
		t.Fatal(first.Body.String())
	}
	s.Close()
	s, err = New(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	again := send(s, body)
	if again.Code != 200 || again.Body.String() != first.Body.String() {
		t.Fatal("recipient lost immutable receipt on restart")
	}
	p.Changes[0].Path = "../../private"
	body, _ = json.Marshal(p)
	if got := send(s, body); got.Code != 400 {
		t.Fatal("fixture interpreted caller filesystem path")
	}
	var response struct {
		Artifact string `json:"artifact_id"`
	}
	json.Unmarshal(first.Body.Bytes(), &response)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/project/patches/"+response.Artifact+"/checks", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"applied":false`) || !strings.Contains(w.Body.String(), `"status":"passed"`) {
		t.Fatal("prepared patch checks unavailable")
	}
}
