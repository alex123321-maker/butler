package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/butler/butler/internal/benchtrace"
)

func (c Candidate) validEffort() bool {
	if c.Effort == "" {
		return true
	}
	switch c.Effort {
	case "none", "low", "medium", "high", "xhigh", "max":
		return c.Provider == "openai" && c.Model == "gpt-6-luna"
	}
	return false
}

func (c Candidate) matchesProfile(end benchtrace.Record) bool {
	return c.Effort == "" || end.ModelVersion != nil && *end.ModelVersion == c.Model && end.ReasoningEffort == c.Effort
}

// Re-read the adapter's native response identity, so editing a run label
// cannot turn an archived low run into a high run during comparison.
func verifyProfileEvidence(dir string, c Candidate, t Turn) error {
	if c.Effort == "" || t.ModelCalls == nil || *t.ModelCalls == 0 || classify(t.Reason) == "error" && t.Reason != "pass" {
		return nil
	}
	var requests []timing
	raw, err := os.ReadFile(filepath.Join(dir, "model-timing.json"))
	if err != nil || json.Unmarshal(raw, &requests) != nil || len(requests) != *t.ModelCalls {
		return errors.New("missing native profile evidence")
	}
	for _, req := range requests {
		name := benchtrace.Filename(t.Run, req.Operation)
		if req.Trace != name {
			return errors.New("invalid native profile evidence path")
		}
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		end, err := benchtrace.Inspect(f, t.Run, req.Operation, c.Provider, c.Model)
		f.Close()
		if err != nil || !c.matchesProfile(end) {
			return errors.New("archived native profile mismatch")
		}
	}
	return nil
}
