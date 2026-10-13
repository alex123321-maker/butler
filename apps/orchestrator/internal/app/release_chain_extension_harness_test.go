package app

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Explicit prospective fixture configuration only. Zero preserves the original
// runtime timer and Go test arguments. An override never outlives the owning case.
func releaseChainExtensionHarnessInvocation(ctx context.Context, lifetime, caseTimeout time.Duration) ([]string, []string, error) {
	args := []string{"-test.run=^TestExtensionRuntimeEgressHarness$"}
	if lifetime == 0 {
		return args, nil, nil
	}
	if lifetime < time.Millisecond || !releaseChainFixtureTimeoutAllowed(caseTimeout) || lifetime > caseTimeout || ctx == nil || ctx.Err() != nil {
		return nil, nil, errors.New("invalid fixture harness lifetime")
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, nil, errors.New("bounded owning case required")
	}
	remaining := time.Until(deadline)
	if remaining < lifetime {
		lifetime = remaining
	}
	millis := lifetime.Milliseconds()
	if millis < 1 {
		return nil, nil, errors.New("owning case already expired")
	}
	lifetime = time.Duration(millis) * time.Millisecond
	// A directly launched compiled test binary has no default process timeout.
	// Pin a separate finite bound with one minute for signal/cleanup handling.
	args = append(args, "-test.timeout="+(lifetime+time.Minute).String())
	return args, []string{"BUTLER_EXTENSION_EGRESS_HARNESS_LIFETIME_MS=" + strconv.FormatInt(millis, 10)}, nil
}

func TestReleaseChainV10HarnessInvocationLegacy(t *testing.T) {
	args, env, err := releaseChainExtensionHarnessInvocation(context.Background(), 0, 10*time.Minute)
	if err != nil || len(args) != 1 || args[0] != "-test.run=^TestExtensionRuntimeEgressHarness$" || env != nil {
		t.Fatal("legacy runtime subprocess configuration changed")
	}
	if (releaseChainHighEntry{}).ExtensionHarnessLifetime != 0 || (releaseChainModelConfig{}).ExtensionHarnessLifetime != 0 {
		t.Fatal("legacy entry acquired an implicit runtime override")
	}
}

func TestReleaseChainV10HarnessInvocationBounded(t *testing.T) {
	for _, ownerLifetime := range []time.Duration{40 * time.Minute, 2 * time.Minute} {
		ctx, cancel := context.WithTimeout(context.Background(), ownerLifetime)
		deadline, _ := ctx.Deadline()
		args, env, err := releaseChainExtensionHarnessInvocation(ctx, 40*time.Minute, 40*time.Minute)
		cancel()
		if err != nil || len(args) != 2 || len(env) != 1 {
			t.Fatal("bounded runtime invocation rejected")
		}
		millis, err := strconv.ParseInt(strings.TrimPrefix(env[0], "BUTLER_EXTENSION_EGRESS_HARNESS_LIFETIME_MS="), 10, 64)
		if err != nil || millis < 1 || millis > ownerLifetime.Milliseconds() || millis < ownerLifetime.Milliseconds()-1000 {
			t.Fatal("runtime lifetime was not clamped to owning case")
		}
		processTimeout, err := time.ParseDuration(strings.TrimPrefix(args[1], "-test.timeout="))
		if err != nil || processTimeout != time.Duration(millis)*time.Millisecond+time.Minute || processTimeout > 41*time.Minute || deadline.IsZero() {
			t.Fatal("Go subprocess timeout lost its explicit finite bound")
		}
	}
}

func TestReleaseChainV10HarnessInvocationDenied(t *testing.T) {
	live, cancelLive := context.WithTimeout(context.Background(), 40*time.Minute)
	defer cancelLive()
	cancelled, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	cancel()
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Millisecond))
	defer cancelExpired()
	for _, test := range []struct {
		ctx                   context.Context
		lifetime, caseTimeout time.Duration
	}{
		{live, -time.Second, 40 * time.Minute},
		{live, time.Nanosecond, 40 * time.Minute},
		{live, 41 * time.Minute, 41 * time.Minute},
		{live, 40 * time.Minute, 20 * time.Minute},
		{live, 40 * time.Minute, 0},
		{context.Background(), 40 * time.Minute, 40 * time.Minute},
		{cancelled, 40 * time.Minute, 40 * time.Minute},
		{expired, 40 * time.Minute, 40 * time.Minute},
	} {
		args, env, err := releaseChainExtensionHarnessInvocation(test.ctx, test.lifetime, test.caseTimeout)
		if err == nil || args != nil || env != nil {
			t.Fatal("invalid or inactive runtime invocation accepted")
		}
	}
}
