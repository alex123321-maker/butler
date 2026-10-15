package config

import "fmt"

func validateAntigravityProtocol(v string) error {
	switch v {
	case "auto", "cloudcode":
		return nil
	default:
		return fmt.Errorf("expected auto or cloudcode")
	}
}
