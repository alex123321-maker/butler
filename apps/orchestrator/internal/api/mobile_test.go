package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butler/butler/internal/mobile"
)

func TestMobileStrictBoundaryRejectsAuthorityAndDuplicateKeys(t *testing.T) {
	for _, raw := range []string{`{"owner_id":"victim"}`, `{"pairing_code":"a","pairing_code":"b"}`, `{"events":[{"authority":"owner"}]}`, `{} {}`, strings.Repeat("x", mobile.MaxBody+1)} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		var p mobile.Batch
		if mobileDecode(httptest.NewRecorder(), r, &p) == nil {
			t.Fatal("unsafe JSON accepted")
		}
	}
}
func TestMobileOwnerAndDeviceProofsAreDistinct(t *testing.T) {
	s := &MobileServer{Store: &mobile.Store{}, Auth: NewOwnerControlAuth(strings.Repeat("o", 32), "owner")}
	for _, header := range []string{"", "Bearer " + strings.Repeat("d", 64)} {
		r := httptest.NewRequest("GET", "/api/v1/mobile-owner/devices", nil)
		r.Header.Set("Authorization", header)
		w := httptest.NewRecorder()
		s.OwnerHandler().ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal("device token obtained owner surface", w.Code)
		}
	}
	r := httptest.NewRequest("GET", "/api/v1/mobile/status", nil)
	r.Header.Set("Authorization", strings.Repeat("d", 64))
	w := httptest.NewRecorder()
	s.DeviceHandler().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("missing bearer scheme accepted")
	}
}
