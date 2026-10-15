package config

import (
	"fmt"
	"strconv"
)

func validateAntigravityOutputLimit(raw string) error {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > 32768 {
		return fmt.Errorf("must be 0 (provider default) or 1..32768")
	}
	return nil
}
