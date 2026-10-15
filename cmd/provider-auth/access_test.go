package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAccessProjectionDoesNotInventConsentOrExposeIdentity(t *testing.T) {
	out := projectAccess([]byte("{\"cloudaicompanionProject\":\"PRIVATE_PROJECT\",\"paidTier\":{\"id\":\"g1-pro-tier\",\"name\":\"PRIVATE\",\"hasAcceptedTos\":false},\"ineligibleTiers\":[{\"reasonCode\":\"PRIVATE\",\"reasonMessage\":\"PRIVATE\"},{\"reasonCode\":\"VALIDATION_REQUIRED\"}]}"), 200)
	data, _ := json.Marshal(out)
	if !out.ProjectPresent || !out.PaidTierConfirmed || out.CurrentAcceptedTOS != nil || out.PaidAcceptedTOS == nil || *out.PaidAcceptedTOS || strings.Contains(string(data), "PRIVATE") || out.IneligibleReasons[0] != "UNCLASSIFIED" || out.IneligibleReasons[1] != "VALIDATION_REQUIRED" {
		t.Fatal("unsafe or invented access projection")
	}
}
