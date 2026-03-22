package app

import "testing"

func TestResolveDispatchMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		rolloutMode  string
		explicitMode string
		want         string
	}{
		{name: "native_only always browser bridge", rolloutMode: "native_only", explicitMode: "orchestrator_relay", want: "browser_bridge"},
		{name: "dual honors explicit relay", rolloutMode: "dual", explicitMode: "orchestrator_relay", want: "orchestrator_relay"},
		{name: "dual defaults browser bridge", rolloutMode: "dual", explicitMode: "browser_bridge", want: "browser_bridge"},
		{name: "remote preferred always relay", rolloutMode: "remote_preferred", explicitMode: "browser_bridge", want: "orchestrator_relay"},
		{name: "unknown rollout defaults browser bridge", rolloutMode: "unknown", explicitMode: "orchestrator_relay", want: "browser_bridge"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := resolveDispatchMode(tc.rolloutMode, tc.explicitMode)
			if got != tc.want {
				t.Fatalf("resolveDispatchMode(%q, %q) = %q, want %q", tc.rolloutMode, tc.explicitMode, got, tc.want)
			}
		})
	}
}
