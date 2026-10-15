package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
)

func TestInspectionOnlyExportsConnectionAndExpiry(t *testing.T) {
	now := time.Now()
	expiry := now.Add(-time.Hour)
	state := providerauth.ProviderState{Connected: true, AccountHint: "PRIVATE_ACCOUNT", ExpiresAt: &expiry}
	v := inspectAuth(state, now)
	if !v.Expired || !v.RefreshDue || !v.Connected || !v.ExpiresAt.Equal(expiry) {
		t.Fatal("incorrect expiry projection")
	}
	encoded, err := json.Marshal(v)
	if err != nil || strings.Contains(string(encoded), "PRIVATE") || strings.Contains(string(encoded), "account") {
		t.Fatal("private state leaked")
	}
}

func TestOperatorSmokeRejectsPartialAnswer(t *testing.T) {
	stream := make(chan transport.TransportEvent, 1)
	stream <- transport.NewAssistantFinalEvent("smoke", "antigravity", transport.AssistantFinal{Content: "partial answer", FinishReason: "MAX_TOKENS"})
	close(stream)
	if _, _, err := collect(stream); err == nil {
		t.Fatal("operator smoke certified an incomplete response")
	}
}
