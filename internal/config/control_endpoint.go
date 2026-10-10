package config

import (
	"fmt"
	"net/url"
)

// Private service authentication may only be sent to the operator's fixed
// control API, never a database-selected URL, credential-bearing URL or query.
func validateOperatorControlEndpoint(value, endpointPath string) error {
	if value == "" {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || u == nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.Path != endpointPath || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return fmt.Errorf("fixed private control endpoint required")
	}
	return nil
}
