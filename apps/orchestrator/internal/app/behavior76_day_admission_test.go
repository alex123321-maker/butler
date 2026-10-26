package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Only fresh independent literal bindings may follow the public source freeze.
const behavior76DayStimuliSHA = "628eb9293577e9a37148367c8ae29ee8f77b2807d9adc5f92128f910fc7cd3ba"
const behavior76DayExpectationsSHA = "d6146f7c01ed7c120d1a0fdc7b435a9a52d07a8f15cda1823b20833e2014d0e9"

func behavior76ClosedStreamHistory(ctx context.Context, tx pgx.Tx, root string) ([]byte, error) {
	directory := filepath.Join(root, "native-stream-v5")
	if err := behavior76CheckRawFiles(directory, "/src20/experiments/behavior76/2026-10-25/native-stream-v5/raw-inventory.json", "0f4acd68d3998e467e95424635fd9e13c7b7a69cf077161b3d6ab8edf7c8c176", 1719); err != nil {
		return nil, err
	}
	manifest, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil || behavior76Hash(manifest) != "ee621c2ba44f688198e3dddad6265d33e7956b920600db76348b0d3d7c656b43" {
		return nil, errors.New("closed stream-v5 manifest changed")
	}
	var exact bool
	if err = tx.QueryRow(ctx, `SELECT closed AND admitted=101 AND manifest=$1::jsonb AND (SELECT count(*)=101 FROM public.behavior76_attempts WHERE window_id='issue76-fixture-stream-20261025T231554') FROM public.behavior76_windows WHERE id='issue76-fixture-stream-20261025T231554'`, string(manifest)).Scan(&exact); err != nil || !exact {
		return nil, errors.New("closed stream-v5 history changed")
	}
	return manifest, nil
}

// Admission is stricter than the unchanged owning budget. It never extends an
// owner-day, model, fixture, authority or whole-window deadline.
func behavior76DayWindow(start time.Time) (deadline, dayEnd time.Time, err error) {
	if start.IsZero() {
		return time.Time{}, time.Time{}, errors.New("original four-hour window needs a known start")
	}
	start = start.UTC()
	deadline = start.Add(4 * time.Hour)
	dayEnd = start.Truncate(24 * time.Hour).Add(24 * time.Hour)
	if deadline.After(dayEnd) {
		return deadline, dayEnd, errors.New("original four-hour window crosses the owning UTC budget day")
	}
	return deadline, dayEnd, nil
}

func TestBehavior76DayWindowAdmission(t *testing.T) {
	for _, test := range []struct {
		start   string
		allowed bool
	}{
		{"2026-10-26T00:01:00Z", true},
		{"2026-10-26T20:00:00Z", true},
		{"2026-10-26T20:00:00.000001Z", false},
		{"2026-10-25T23:15:54Z", false},
		{"2026-10-26T06:15:54+07:00", false},
		{"2026-10-27T02:59:59+07:00", true},
	} {
		t.Run(test.start, func(t *testing.T) {
			start, err := time.Parse(time.RFC3339Nano, test.start)
			if err != nil {
				t.Fatal(err)
			}
			deadline, end, err := behavior76DayWindow(start)
			if (err == nil) != test.allowed || !deadline.Equal(start.Add(4*time.Hour)) || end.Location() != time.UTC || end.Hour() != 0 || end.Minute() != 0 {
				t.Fatal("owning day or original duration changed", deadline, end, err)
			}
		})
	}
	if _, _, err := behavior76DayWindow(time.Time{}); err == nil {
		t.Fatal("unknown start admitted")
	}
}

func TestBehavior76NativeDayWindow(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_NATIVE_ENABLE") != "fixture-day-fit-synthetic-only" {
		t.Skip("separate day-fit admission required")
	}
	behavior76CollectStudy(t, true, true, true)
}

func TestBehavior76DayCorpusSetupPreflight(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT") != "day-setup-only" {
		t.Skip("explicit zero-native day setup required")
	}
	behavior76SetupPreflight(t, behavior76DayStimuliSHA)
}

func TestBehavior76DayCorpusCompositionPreflight(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT") != "day-composition-only" {
		t.Skip("explicit zero-native day composition required")
	}
	behavior76CompositionPreflightMode(t, behavior76DayStimuliSHA, true, true)
}
