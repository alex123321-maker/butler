package main

import (
	"net/http/httptest"
	"testing"
)

func TestOAuthCallbackChecksStateAndDoesNotReflectCodes(t *testing.T) {
	ch := make(chan string, 1)
	h := callbackHandler("expected", ch)
	for _, query := range []string{"state=bad&code=private-code", "state=expected&state=expected&code=x", "state=expected&code=x&code=y", "state=expected", "state=expected&code=%zz"} {
		r := httptest.NewRequest("GET", "http://localhost/oauth-callback?"+query, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 400 || len(ch) != 0 {
			t.Fatal(w.Code, query)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://localhost/oauth-callback?state=expected&code=private-code", nil))
	if w.Code != 200 || <-ch != "private-code" || w.Body.String() != "Butler local research login received. You may close this tab.\n" {
		t.Fatal("callback failure")
	}
}
