package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestQuotaUnknownIsNotZeroAndPrivateFieldsExcluded(t *testing.T) {
	for _, raw := range []string{`{}`, `{"models":{"fixture":{"quotaInfo":{}}}}`, `{"models":{"fixture":{"quotaInfo":{"remainingFraction":-1}}}}`, `{"models":{"fixture":{"quotaInfo":{"remainingFraction":1.2}}}}`} {
		if v := projectQuota("fixture", []byte(raw), 200); v.RemainingFraction != nil {
			t.Fatal("unknown became measured")
		}
	}
	v := projectQuota("fixture", []byte(`{"project":"PRIVATE_PROJECT","account":"PRIVATE_ACCOUNT","models":{"fixture":{"quotaInfo":{"remainingFraction":0,"resetTime":"2026-10-16T00:00:00Z"}}}}`), 200)
	if !v.Advertised || v.RemainingFraction == nil || *v.RemainingFraction != 0 || v.ResetAt == nil {
		t.Fatal("measured zero lost")
	}
	data, _ := json.Marshal(v)
	if strings.Contains(string(data), "PRIVATE") || strings.Contains(string(data), "project") {
		t.Fatal("private metadata leaked")
	}
}
