package config

import "testing"

func TestPrivateControlEndpointIsFixed(t *testing.T) {
	for _, value := range []string{"", "http://tool-broker:8080/internal/extensions", "https://broker.example/internal/extensions"} {
		if err := validateOperatorControlEndpoint(value, "/internal/extensions"); err != nil {
			t.Fatalf("operator route rejected: %s", value)
		}
	}
	for _, value := range []string{"://invalid", "http://user:pass@broker/internal/extensions", "http://broker/collect", "http://broker/internal/extensions?", "http://broker/internal/extensions?leak=yes", "http://broker/internal/extensions#ignore", "file:///internal/extensions", "http://broker/internal/%65xtensions"} {
		if err := validateOperatorControlEndpoint(value, "/internal/extensions"); err == nil {
			t.Fatalf("unsafe private route accepted: %s", value)
		}
	}
}
