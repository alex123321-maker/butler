package app

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// One implementation of the five Core/Broker scenarios; only the explicit
// once-only observation scope differs. This has no generic provider routing.
type releaseChainHighEntry struct {
	WindowID                             string
	AcceptanceEnv, PrivateEnv, ImportEnv string
	Baseline, Maximum                    int
	ProviderStages                       bool
	RetainProposals                      bool
	ExtensionHarnessLifetime             time.Duration
	ObserveSourceChecks                  bool
	Policy                               releaseChainHighWindowPolicy
	ReadQuality                          func() (releaseChainV5Prerequisite, error)
	NewWindow                            func(context.Context, *pgxpool.Pool) (*releaseChainV5Window, error)
}

func TestScenarioReleaseChainActualV5HighE2E(t *testing.T) {
	// The old binary and sealed artifacts remain reproducible through its
	// read-only auditor, never another native entry from future source.
	if os.Getenv("BUTLER_SCENARIO_CHAIN_V5_HIGH_ACCEPTANCE") == "1" {
		t.Fatal("V5 native scope is sealed failed; no new entry from current source")
	}
	t.Skip("V5 native scope is sealed failed; no new entry from current source")
}

func TestScenarioReleaseChainActualV6HighE2E(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_CHAIN_V6_HIGH_ACCEPTANCE") == "1" {
		t.Fatal("V6 native scope is sealed failed; no new entry from current source")
	}
	t.Skip("V6 native scope is sealed failed; no new entry from current source")
}

func closedReleaseChainV6Entry(t *testing.T) {
	testReleaseChainHighNative(t, releaseChainHighEntry{
		WindowID:      releaseChainV6WindowID,
		AcceptanceEnv: "BUTLER_SCENARIO_CHAIN_V6_HIGH_ACCEPTANCE", PrivateEnv: "BUTLER_SCENARIO_CHAIN_V6_HIGH_PRIVATE", ImportEnv: "BUTLER_SCENARIO_CHAIN_V6_HIGH_IMPORT_FILE",
		Baseline: 537, Maximum: 78, ProviderStages: true,
		ReadQuality: readReleaseChainV6Prerequisite,
		NewWindow:   newReleaseChainV6Window,
	})
}
