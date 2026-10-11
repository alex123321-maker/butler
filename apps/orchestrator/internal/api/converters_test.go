package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/generatedtools"
)

func TestGeneratedConverterRelayRequiresOwnerProofAndRejectsRawCode(t *testing.T) {
	token := strings.Repeat("owner", 8)
	var calls atomic.Int64
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer private.Close()
	c, e := generatedtools.NewClient(private.URL+"/internal/converters", strings.Repeat("core", 10))
	if e != nil {
		t.Fatal(e)
	}
	h := GeneratedConverters(NewOwnerControlAuth(token, "owner"), c)
	req := httptest.NewRequest("GET", "/api/v4/converters", nil)
	req.Header.Set("X-Butler-Actor", "owner")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != 401 || calls.Load() != 0 {
		t.Fatal("caller body/header became owner proof")
	}
	for _, body := range []string{`{"operation":"revoke","digest":"a","generation":2,"code":"secret script"}`, `{"operation":"revoke","operation":"activate"}`, `{"operation":"list"} {}`} {
		req = httptest.NewRequest("POST", "/api/v4/converters", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		res = httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != 400 || calls.Load() != 0 {
			t.Fatal("invalid DATA became control authority", res.Code)
		}
	}
	req = httptest.NewRequest("GET", "/api/v4/converters", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != 200 || calls.Load() != 1 {
		t.Fatal("authenticated owner read unavailable", res.Code)
	}
}
