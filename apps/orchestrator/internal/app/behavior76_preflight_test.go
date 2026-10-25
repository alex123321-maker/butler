package app

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// Actual owning APIs only; no native factory, credentials, model or simulated
// correct answer. The independent checker runs this without revealing inputs.
func TestBehavior76CorpusSetupPreflight(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT") != "1" {
		t.Skip("explicit hidden-control setup preflight required")
	}
	behavior76SetupPreflight(t, behavior76StimuliSHA)
}

func TestBehavior76FreshCorpusSetupPreflight(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT") != "fresh-setup-only" {
		t.Skip("explicit fresh setup preflight required")
	}
	behavior76SetupPreflight(t, behavior76FreshStimuliSHA)
}

func behavior76SetupPreflight(t *testing.T, expectedSHA string) {
	raw, err := os.ReadFile(os.Getenv("BUTLER_BEHAVIOR76_STIMULI"))
	if err != nil || behavior76Hash(raw) != expectedSHA {
		t.Fatal("control commitment mismatch")
	}
	episodes, err := behavior76ReadEpisodes(strings.NewReader(string(raw)))
	if err != nil || len(episodes) != 24 {
		t.Fatal("control interface mismatch", err)
	}
	for n, e := range episodes {
		t.Run(fmt.Sprintf("slot-%02d", n+1), func(t *testing.T) {
			f := newBehavior76Fixture(t)
			f.seed(e)
			for _, step := range e.Steps {
				if _, err := f.apply(step); err != nil {
					t.Fatal("public owning event API failed", step.Event.Type, err)
				}
			}
			_ = f.snapshot()
		})
	}
}
