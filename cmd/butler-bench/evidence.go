package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/butler/butler/internal/benchtrace"
	"github.com/jackc/pgx/v5"
)

//go:embed timeline.sql
var timelineSQL string

type interval struct {
	Lane    string          `json:"lane"`
	Ref     string          `json:"ref"`
	Label   string          `json:"label"`
	Started time.Time       `json:"start_utc"`
	Ended   *time.Time      `json:"end_utc,omitempty"`
	US      *int64          `json:"duration_us"`
	Details json.RawMessage `json:"details"`
}

type timing struct {
	Operation     string `json:"operation"`
	Trace         string `json:"trace"`
	DurationUS    *int64 `json:"duration_us"`
	FirstOutputUS *int64 `json:"first_visible_output_us"`
	Issue         string `json:"issue,omitempty"`
}

type durationSample struct {
	Stage string `json:"stage"`
	US    *int64 `json:"duration_us"`
}

// Artifact names in Butler and model output are data. Use the immutable
// artifact ID to name the export; keep titles/source refs in the timeline.
func exportArtifact(dir string, v interval) (string, error) {
	format := str(v.Details, "format")
	content := field(v.Details, "content_json")
	body := []byte(str(v.Details, "content_text"))
	ext := ".txt"
	switch format {
	case "markdown":
		ext = ".md"
	case "json":
		ext = ".json"
		body = content
	case "private_media_v1", "image_data_url":
		var mime string
		var err error
		if format == "private_media_v1" {
			mime = str(content, "mime")
			body, err = base64.StdEncoding.DecodeString(str(content, "data"))
			h := sha256.Sum256(body)
			if err != nil || str(content, "sha256") != hex.EncodeToString(h[:]) {
				return "", errors.New("invalid media digest")
			}
		} else {
			meta, data, ok := strings.Cut(str(content, "image_data_url"), ",")
			mime = strings.TrimSuffix(strings.TrimPrefix(meta, "data:"), ";base64")
			if !ok || meta != "data:"+mime+";base64" || !strings.HasPrefix(mime, "image/") {
				return "", errors.New("invalid image artifact")
			}
			body, err = base64.StdEncoding.DecodeString(data)
			if err != nil {
				return "", err
			}
		}
		ext = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp", "application/pdf": ".pdf", "text/plain": ".txt"}[mime]
		if ext == "" {
			ext = ".bin"
		}
	}
	id := sha256.Sum256([]byte(v.Ref))
	name := "artifact-" + hex.EncodeToString(id[:]) + ext
	return name, os.WriteFile(filepath.Join(dir, name), body, 0600)
}

// Capture completed and unfinished intervals without imputing missing end times.
// The native model duration uses its own monotonic clock. Ledger intervals and
// overlapping tools/states retain their independently reported timestamps.
func (c *liveClient) capture(ctx context.Context, dir, session string, step Step, t *Turn) error {
	if t.Input == "" {
		return nil
	}
	rows, err := c.db.Query(ctx, timelineSQL, t.Input, t.Event, session, step.Task)
	if err != nil {
		return errors.New("timeline_query_unavailable")
	}
	timeline, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (interval, error) {
		var v interval
		err := r.Scan(&v.Lane, &v.Ref, &v.Label, &v.Started, &v.Ended, &v.Details)
		v.Started = v.Started.UTC()
		if v.Ended != nil {
			end := v.Ended.UTC()
			v.Ended = &end
			d := end.Sub(v.Started).Microseconds()
			if d >= 0 {
				v.US = &d
			}
		}
		return v, err
	})
	if err != nil || len(timeline) == 0 {
		return errors.New("timeline_unavailable")
	}
	for _, v := range timeline {
		if bytes.Equal(field(v.Details, "redacted"), []byte("true")) {
			return errors.New("trace_source_redacted")
		}
	}
	if err := writeJSON(filepath.Join(dir, "timeline.json"), timeline); err != nil {
		return errors.New("timeline_write_failed")
	}
	var systemIssue error
	if t.Run != "" {
		raw, err := os.ReadFile(filepath.Join(c.traceDir, t.Event, "system.jsonl"))
		if err == nil {
			if err = os.WriteFile(filepath.Join(dir, "system.jsonl"), raw, 0600); err != nil {
				return errors.New("system_trace_write_failed")
			}
			d := json.NewDecoder(bytes.NewReader(raw))
			for {
				var span benchtrace.SystemSpan
				if err = d.Decode(&span); err == io.EOF {
					break
				}
				if err != nil || span.Run != t.Run || span.Stage == "" || span.Started.IsZero() || span.Ended.Before(span.Started) || span.US < 0 {
					return errors.New("system_trace_incomplete")
				}
				t.Durations = append(t.Durations, durationSample{"core/" + span.Stage, &span.US})
			}
		} else if !os.IsNotExist(err) || t.ModelCalls != nil && *t.ModelCalls > 0 {
			systemIssue = errors.New("system_trace_missing")
		}
	}
	artifacts := map[string]string{}
	var measurements []timing
	var incomplete bool
	for _, v := range timeline {
		if v.Lane == "artifact" {
			name, err := exportArtifact(dir, v)
			if err != nil {
				return errors.New("artifact_export_failed")
			}
			artifacts[v.Ref] = name
		}
		switch v.Lane {
		case "tool", "system_state":
			t.Durations = append(t.Durations, durationSample{v.Lane + "/" + v.Label, v.US})
		case "run", "delivery":
			t.Durations = append(t.Durations, durationSample{v.Lane, v.US})
		}
		if v.Lane != "model_admission" {
			continue
		}
		name := benchtrace.Filename(t.Run, v.Ref)
		row := timing{Operation: v.Ref, Trace: name}
		file := filepath.Join(c.traceDir, t.Event, name)
		// The outbox can commit while the stream goroutine is closing. Read
		// the final bytes before sealing, within a short independent deadline.
		var raw []byte
		var end benchtrace.Record
		for attempt := 0; attempt < 6; attempt++ {
			raw, err = os.ReadFile(file)
			if err == nil {
				end, err = benchtrace.Inspect(bytes.NewReader(raw), t.Run, v.Ref, str(v.Details, "provider"), v.Label)
			}
			if err == nil || attempt == 5 || ctx.Err() != nil {
				break
			}
			select {
			case <-time.After(50 * time.Millisecond):
			case <-ctx.Done():
			}
		}
		if err != nil {
			row.Issue = "missing_or_incomplete_model_trace"
			incomplete = true
		} else {
			d := end.ElapsedUS
			row.DurationUS = &d
			row.FirstOutputUS = end.FirstOutputUS
		}
		if len(raw) > 0 {
			if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
				return errors.New("model_trace_write_failed")
			}
		}
		measurements = append(measurements, row)
		t.Durations = append(t.Durations, durationSample{"model_request", row.DurationUS}, durationSample{"model_first_visible_output", row.FirstOutputUS})
	}
	if err := writeJSON(filepath.Join(dir, "model-timing.json"), measurements); err != nil {
		return errors.New("timing_write_failed")
	}
	if err := writeJSON(filepath.Join(dir, "artifacts.json"), artifacts); err != nil {
		return errors.New("artifact_index_write_failed")
	}
	if incomplete || t.ModelCalls != nil && *t.ModelCalls != len(measurements) {
		return errors.New("model_trace_incomplete")
	}
	return systemIssue
}

type fileDigest struct {
	Name  string `json:"file"`
	Bytes int64  `json:"bytes"`
	SHA   string `json:"sha256"`
}

func fileSum(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}
func inventory(dir string) ([]fileDigest, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var list []fileDigest
	for _, f := range files {
		if f.Name() == "manifest.json" {
			continue
		}
		if !f.Type().IsRegular() {
			return nil, errors.New("unexpected non-regular evidence file")
		}
		hash, size, err := fileSum(filepath.Join(dir, f.Name()))
		if err != nil {
			return nil, err
		}
		list = append(list, fileDigest{f.Name(), size, hash})
	}
	return list, nil
}
func seal(dir string) (string, error) {
	files, err := inventory(dir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "manifest.json")
	if err = writeJSON(path, files); err != nil {
		return "", err
	}
	hash, _, err := fileSum(path)
	return hash, err
}
func verifyFiles(dir, want string) error {
	if !shaRE.MatchString(want) {
		return errors.New("missing evidence manifest digest")
	}
	p := filepath.Join(dir, "manifest.json")
	st, err := os.Lstat(p)
	if err != nil || !st.Mode().IsRegular() {
		return errors.New("missing evidence manifest")
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	if want != hex.EncodeToString(sum[:]) {
		return errors.New("evidence manifest changed")
	}
	var list []fileDigest
	if json.Unmarshal(raw, &list) != nil || len(list) == 0 {
		return errors.New("invalid evidence inventory")
	}
	got, err := inventory(dir)
	if err != nil {
		return err
	}
	if !slices.Equal(got, list) {
		return fmt.Errorf("evidence files changed in %s", filepath.Base(dir))
	}
	return nil
}
