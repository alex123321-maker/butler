package config

import "testing"

func TestPublicWebReadsRequireOperatorOptIn(t *testing.T) {
	for _, tc := range []struct {
		value   string
		enabled bool
		invalid bool
	}{
		{"", false, false}, {"false", false, false}, {"true", true, false}, {"yes", false, true},
	} {
		cfg, _, err := loadToolBroker(envMap(map[string]string{"BUTLER_PUBLIC_WEB_READS_ENABLED": tc.value}))
		if (err != nil) != tc.invalid || err == nil && cfg.PublicWebReadsEnabled != tc.enabled {
			t.Fatalf("value=%q enabled=%v error=%v", tc.value, cfg.PublicWebReadsEnabled, err)
		}
	}
	if _, ok := managedSettingSpecByKey("BUTLER_PUBLIC_WEB_READS_ENABLED"); ok {
		t.Fatal("public web consent became a mutable model/UI setting")
	}
}
