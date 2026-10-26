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
const behavior76DayStimuliSHA = "0f3654efcbc4795c255425e85acf79678703c6faf21fc4fe962a597b939f163d"
const behavior76DayExpectationsSHA = "db8bc3ecdc1430638ed0e45b280aca48e79ef871d07ac9b4e9bf7e863c35b7a5"

func behavior76ClosedHTTPHistory(ctx context.Context, tx pgx.Tx, root string) ([]byte, error) {
	directory := filepath.Join(root, "native-local-http-v9")
	if err := behavior76CheckRawFiles(directory, "/src20/experiments/behavior76/2026-10-26/native-local-http-v9/raw-inventory.json", "045adc74308ea3faf3024e2b8070cd97aa9ea1470e0bce025354d88b72dfb879", 383); err != nil {
		return nil, err
	}
	manifest, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil || behavior76Hash(manifest) != "bd66d1587f75b32e575faf30592f7c200cb0b47e5f36a193dc08af47970e4a5e" {
		return nil, errors.New("closed v9 manifest changed")
	}
	var exact bool
	if err = tx.QueryRow(ctx, `SELECT closed AND admitted=16 AND manifest=$1::jsonb AND (SELECT count(*)=16 FROM public.behavior76_attempts WHERE window_id='issue76-fixture-day-20261026T092444') FROM public.behavior76_windows WHERE id='issue76-fixture-day-20261026T092444'`, string(manifest)).Scan(&exact); err != nil || !exact {
		return nil, errors.New("closed v9 history changed")
	}
	return manifest, nil
}

func behavior76ClosedModelHistory(ctx context.Context, tx pgx.Tx, root string) ([]byte, error) {
	directory := filepath.Join(root, "native-local-model-v8")
	if err := behavior76CheckRawFiles(directory, "/src20/experiments/behavior76/2026-10-26/native-local-model-v8/raw-inventory.json", "c85f02ef8e3555f1043c4698f48f1bfa0e3af6670fb269fd7a20c3d83c4f2d00", 1253); err != nil {
		return nil, err
	}
	manifest, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil || behavior76Hash(manifest) != "74306fd83b00e36166c185415f6e28435d29a8d7ffdca1a4dd18ccba82817910" {
		return nil, errors.New("closed v8 manifest changed")
	}
	var exact bool
	if err = tx.QueryRow(ctx, `SELECT closed AND admitted=64 AND manifest=$1::jsonb AND (SELECT count(*)=64 FROM public.behavior76_attempts WHERE window_id='issue76-fixture-day-20261026T065844') FROM public.behavior76_windows WHERE id='issue76-fixture-day-20261026T065844'`, string(manifest)).Scan(&exact); err != nil || !exact {
		return nil, errors.New("closed v8 history changed")
	}
	return manifest, nil
}

func behavior76ClosedEmptyHistory(ctx context.Context, tx pgx.Tx, root string) ([]byte, error) {
	directory := filepath.Join(root, "native-day-v7-r2")
	if err := behavior76CheckRawFiles(directory, "/src20/experiments/behavior76/2026-10-26/native-day-v7-r2/raw-inventory.json", "88a4e621879b34d6634384a7b7f70533cd47bd016d6c6451a9e9e1453c5f0938", 71); err != nil {
		return nil, err
	}
	manifest, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil || behavior76Hash(manifest) != "6f7cf255045c456124bc0cbd7ebc62f2c0c644a82ad0d310c2db0759f0e43e34" {
		return nil, errors.New("closed day-v7-r2 manifest changed")
	}
	var exact bool
	if err = tx.QueryRow(ctx, `SELECT closed AND admitted=3 AND manifest=$1::jsonb AND (SELECT count(*)=3 FROM public.behavior76_attempts WHERE window_id='issue76-fixture-day-20261026T050548') FROM public.behavior76_windows WHERE id='issue76-fixture-day-20261026T050548'`, string(manifest)).Scan(&exact); err != nil || !exact {
		return nil, errors.New("closed day-v7-r2 history changed")
	}
	return manifest, nil
}

func behavior76ClosedDayHistory(ctx context.Context, tx pgx.Tx, root string) ([]byte, error) {
	directory := filepath.Join(root, "native-day-v6")
	if err := behavior76CheckRawFiles(directory, "/src20/experiments/behavior76/2026-10-26/native-day-v6/raw-inventory.json", "517f7b215c956ef9b8664622380d2b9fcd7f8282bf5cc3d5f20f892868b1e7d6", 2569); err != nil {
		return nil, err
	}
	manifest, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil || behavior76Hash(manifest) != "7516824e49cb0760b759faf15109eeceef2aee31b08bcc88c06ef62e7692d24c" {
		return nil, errors.New("closed day-v6 manifest changed")
	}
	var exact bool
	if err = tx.QueryRow(ctx, `SELECT closed AND admitted=134 AND manifest=$1::jsonb AND (SELECT count(*)=134 FROM public.behavior76_attempts WHERE window_id='issue76-fixture-day-20261026T012812') FROM public.behavior76_windows WHERE id='issue76-fixture-day-20261026T012812'`, string(manifest)).Scan(&exact); err != nil || !exact {
		return nil, errors.New("closed day-v6 history changed")
	}
	return manifest, nil
}

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
	behavior76CompositionPreflightMode(t, behavior76DayStimuliSHA, true, true, true, true, true)
}
