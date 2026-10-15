package egress

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseClassAllowlists reads an optional operator restriction. An absent class
// has no extra cap; an explicitly empty list denies all destinations.
func ParseClassAllowlists(raw string) (map[string][]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var decoded map[string][]string
	if len(raw) > 16384 || json.Unmarshal([]byte(raw), &decoded) != nil || decoded == nil {
		return nil, fmt.Errorf("network allowlists must be an object of domain arrays")
	}
	result := make(map[string][]string, len(decoded))
	for class, domains := range decoded {
		if (class != "http" && class != "browser" && class != "web") || domains == nil || len(domains) > 128 {
			return nil, fmt.Errorf("invalid network allowlist class or domain array")
		}
		result[class] = []string{}
		for _, domain := range domains {
			domain = strings.ToLower(strings.TrimSpace(domain))
			base := strings.TrimPrefix(domain, "*.")
			if domain == "" || strings.ContainsAny(base, "*/:@?#\\%") {
				return nil, fmt.Errorf("network allowlists require exact domains or *.domain patterns")
			}
			if _, err := (Policy{}).ValidateURL("https://" + base + "/"); err != nil {
				return nil, fmt.Errorf("invalid network allowlist domain")
			}
			result[class] = append(result[class], domain)
		}
	}
	return result, nil
}
