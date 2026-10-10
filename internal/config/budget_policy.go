package config

import "github.com/butler/butler/internal/budget"

func validateBudgetPolicy(raw string) error { _, err := budget.ParsePolicy(raw); return err }
