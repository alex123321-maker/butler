package benchtrace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type SystemSpan struct {
	Run     string    `json:"run"`
	Stage   string    `json:"stage"`
	Started time.Time `json:"start_utc"`
	Ended   time.Time `json:"end_utc"`
	US      int64     `json:"duration_us"`
}

// Measure captures elapsed time on the Core clock for an enrolled request.
// Keep the reporting failure separate from normal service control flow.
func Measure(ctx context.Context, stage string) func() {
	e, ok := ctx.Value(key{}).(enrollment)
	if !ok {
		return func() {}
	}
	start := time.Now()
	return func() {
		end := time.Now()
		span := SystemSpan{e.run, stage, start.UTC(), end.UTC(), end.Sub(start).Microseconds()}
		f, err := os.OpenFile(filepath.Join(e.dir, "system.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return
		}
		defer f.Close()
		_ = json.NewEncoder(f).Encode(span)
	}
}
