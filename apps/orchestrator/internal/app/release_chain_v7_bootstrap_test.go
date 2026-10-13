package app

import (
	"testing"
	"time"
)

func TestReleaseChainV7BootstrapTimeoutGuard(t *testing.T) {
	cfg := releaseChainHighConfigurationFor(releaseChainV7Policy())
	if cfg.FixtureTimeout != 40*time.Minute || !releaseChainFixtureTimeoutAllowed(cfg.FixtureTimeout) {
		t.Fatal("reviewed High profile cannot reach its factory")
	}
	if releaseChainFixtureTimeoutAllowed(41*time.Minute) || releaseChainFixtureTimeoutAllowed(40*time.Minute+time.Nanosecond) || releaseChainFixtureTimeoutAllowed(0) || releaseChainFixtureTimeoutAllowed(-time.Second) {
		t.Fatal("unbounded or nonpositive fixture timeout accepted")
	}
	if !releaseChainFixtureTimeoutAllowed(releaseChainV5Configuration().FixtureTimeout) {
		t.Fatal("retained twenty-minute profile denied")
	}
}
