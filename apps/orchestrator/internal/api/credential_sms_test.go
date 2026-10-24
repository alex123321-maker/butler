package api

import (
	"github.com/butler/butler/internal/mobile"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCredentialSMSRelayHasNoUnconfiguredDeviceCapability(t *testing.T) {
	s := &MobileServer{Store: &mobile.Store{}}
	r := httptest.NewRequest("POST", "/api/v1/mobile/otp/login", strings.NewReader(`{"event_id":"synthetic-event-1","package_name":"sms.app","sender":"Example","code":"739284","posted_at_epoch_ms":1}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 64))
	r.Header.Set("X-Butler-Device-Id", "phone")
	w := httptest.NewRecorder()
	s.DeviceHandler().ServeHTTP(w, r)
	if w.Code != 401 || strings.Contains(w.Body.String(), "739284") {
		t.Fatal("unconfigured secret ingress accepted or echoed value")
	}
}
