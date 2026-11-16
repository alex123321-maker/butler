//go:build !linux

package acp

import (
	"context"
	"errors"
	"testing"
)

func TestProcessUnsupportedPlatformFailsClosed(test *testing.T) {
	result, err := newFixtureProbe(test, fixtureLimits()).RunProcess(context.Background(), ProcessSpec{}, fixtureRequest())
	if !errors.Is(err, ErrProcessUnsupported) || result != (Result{}) {
		test.Fatalf("result=%+v err=%v", result, err)
	}
}
