package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/singletabcontrol"
)

type capturedTabDecision struct {
	actor owneridentity.Actor
	calls int
	fail  bool
}

func (d *capturedTabDecision) Choices(ctx context.Context, id string) (singletabcontrol.OwnerChoices, error) {
	_, err := d.Select(ctx, id, "")
	return singletabcontrol.OwnerChoices{ApprovalID: id}, err
}

func (d *capturedTabDecision) Select(ctx context.Context, _, _ string) (string, error) {
	d.calls++
	d.actor, _ = owneridentity.FromContext(ctx)
	if d.fail {
		return "", errors.New("PRIVATE_TOKEN_AND_METADATA")
	}
	return "opaque-selected-session", nil
}
func (d *capturedTabDecision) Revoke(ctx context.Context, _ string) error {
	_, err := d.Select(ctx, "", "")
	return err
}

func TestGuardedTabOwnerHTTPRejectsBodyAuthorityAndMasksDenial(t *testing.T) {
	token := strings.Repeat("x", 40)
	id, candidate := strings.Repeat("a", 48), strings.Repeat("b", 48)
	d := &capturedTabDecision{}
	h := GuardedTabOwnerControls(NewOwnerControlAuth(token, "fixed-owner"), d)
	for _, body := range []string{
		`null`, `[]`, `{}`, `{"candidate_token":null}`,
		`{"candidate_token":"` + candidate + `","owner_id":"fixed-owner"}`,
		`{"candidate_token":"` + candidate + `","host":"arbitrary"}`,
		`{"Candidate_Token":"` + candidate + `"}`,
		`{"candidate_token":"` + candidate + `","candidate_token":"` + candidate + `"}`,
		`{"candidate_token":"` + candidate + `"} {}`,
		strings.Repeat(" ", 257),
	} {
		r := httptest.NewRequest(http.MethodPost, guardedTabPrefix+id+"/select", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatal("unsafe body accepted", w.Code, body)
		}
	}
	for _, auth := range []string{"", "Bearer wrong", "Bearer " + token} {
		r := httptest.NewRequest(http.MethodPost, guardedTabPrefix+id+"/select", strings.NewReader(`{"candidate_token":"`+candidate+`"}`))
		r.Header.Set("Authorization", auth)
		r.Header.Set("X-Butler-Actor", "untrusted-owner")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		want := http.StatusUnauthorized
		if auth == "Bearer "+token {
			want = http.StatusOK
		}
		if w.Code != want {
			t.Fatal("owner proof not enforced", w.Code, want)
		}
	}
	if d.calls != 1 || d.actor.OwnerID != "fixed-owner" || d.actor.Channel != "web" {
		t.Fatal("body/header identity reached decision")
	}
	d.fail = true
	r := httptest.NewRequest(http.MethodPost, guardedTabPrefix+id+"/revoke", strings.NewReader(`{}`))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusConflict || strings.Contains(w.Body.String(), "PRIVATE") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private denial escaped", w.Code)
	}
	r = httptest.NewRequest(http.MethodPost, guardedTabPrefix+id+"/select", strings.NewReader(`{"candidate_token":"`+candidate+`"}`))
	r.Header.Add("Authorization", "Bearer "+token)
	r.Header.Add("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized || d.calls != 2 {
		t.Fatal("duplicate authentication accepted")
	}
}
