package app

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/experience"
)

type experienceBoundBase struct{ bound time.Duration }

func (s experienceBoundBase) MemorySourceCheckTimeout() time.Duration          { return s.bound }
func (experienceBoundBase) PinTranscript(context.Context, string, int64) error { return nil }
func (experienceBoundBase) CheckRunSources(context.Context, string) error      { return nil }

func TestExperienceCompoundSourceBound(t *testing.T) {
	for _, tc := range []struct {
		name        string
		store       bool
		inherit     bool
		bound, want time.Duration
	}{
		{name: "catalog_default", want: 2 * time.Second},
		{name: "experience_default", store: true, want: 2 * time.Second},
		{name: "catalog_explicit", inherit: true, bound: 500 * time.Millisecond, want: 500 * time.Millisecond},
		{name: "experience_compound", store: true, inherit: true, bound: 500 * time.Millisecond, want: 2 * time.Second},
		{name: "existing_ceiling", store: true, inherit: true, bound: 2 * time.Second, want: 2 * time.Second},
		{name: "invalid_zero", store: true, inherit: true},
		{name: "invalid_short", store: true, inherit: true, bound: 100 * time.Millisecond, want: 100 * time.Millisecond},
		{name: "invalid_long", store: true, inherit: true, bound: 3 * time.Second, want: 3 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := experienceRunSources{}
			if tc.store {
				s.store = &experience.Store{}
			}
			if tc.inherit {
				s.sources = experienceBoundBase{tc.bound}
			}
			if got := s.MemorySourceCheckTimeout(); got != tc.want {
				t.Fatalf("source bound = %v, want %v", got, tc.want)
			}
		})
	}
}
