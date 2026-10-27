package app

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/jackc/pgx/v5"
)

// This public driver regression contains no study case, provider or auth input.
// Both cancelled operations must remain errors. A trace must also observe the
// actual rollback return when cached-statement cleanup fails before SQL dispatch.
func TestBehavior76AuthorityTraceCancelledRollbackIntegration(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_PGX_OBSERVER_CONTRACT") != "callback-paired-public-only" {
		t.Skip("explicit research-only observer build contract required")
	}
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	config := pool.Config().ConnConfig.Copy()
	trace := newBehavior76AuthorityTrace()
	config.Tracer = trace
	connection, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(context.Background())
	tx, err := connection.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var value int
	const query = `SELECT $1::integer`
	if err = tx.QueryRow(ctx, query, 7).Scan(&value); err != nil || value != 7 {
		t.Fatal("prepared public query failed", err)
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	queryErr := tx.QueryRow(cancelled, query, 8).Scan(&value)
	closedAfterQuery := connection.IsClosed()
	rollbackErr := tx.Rollback(cancelled)
	if !errors.Is(queryErr, context.Canceled) || !errors.Is(rollbackErr, context.Canceled) {
		t.Fatal("actual cancellation errors absent", behavior76SafeError(queryErr), behavior76SafeError(rollbackErr))
	}
	snapshot := trace.snapshot()
	t.Logf("actual cancelled prepared query and rollback both remain errors; closed_after_query=%t closed_after_rollback=%t observer_failed=%t open_spans=%d dropped=%d operation_errors=%d events=%d; native/auth=0", closedAfterQuery, connection.IsClosed(), snapshot.Failed, snapshot.OpenSpans, snapshot.Dropped, snapshot.OperationFailures, len(snapshot.Events))
	if closedAfterQuery || !connection.IsClosed() {
		t.Fatal("cancelled rollback connection handling changed")
	}
	if snapshot.Failed || snapshot.OpenSpans != 0 || snapshot.Dropped != 0 || snapshot.OperationFailures != 2 {
		t.Fatal("driver did not report every actual operation return")
	}
	metadata := behavior76PGXBuildMetadata(t)
	t.Logf("actual complete isolated module copy verified: files=%v changes=%v; shared cache and root module/sum unchanged", metadata["module_files"], metadata["source_changes"])
}

func behavior76PGXBuildMetadata(t *testing.T) map[string]any {
	t.Helper()
	const directory = "/src20/.cache/behavior76/pgx-observer-build-v1"
	read := func(path string) []byte {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal("exact observer build provenance unavailable")
		}
		return raw
	}
	pinsRaw := read(directory + "/pins.json")
	var pins map[string]string
	if json.Unmarshal(pinsRaw, &pins) != nil || pins["version"] != "behavior76-pgx-observer-build-v1" || pins["original_sha256"] != "076f1d636cd6e2a1b8cadaea24ac60375cd98f1d63034ff796427f49a4408684" || pins["derived_sha256"] != "9fdbb58e77cebad5ff489cec5481b1aac2d5e111e44157605b796f7a7d8078fc" {
		t.Fatal("exact observer callback source pins unavailable")
	}
	for _, check := range []struct{ Path, Pin string }{{"/src20/go.mod", "original_go_mod_sha256"}, {"/src20/go.sum", "original_go_sum_sha256"}, {directory + "/research.mod", "research_mod_sha256"}, {directory + "/research.sum", "original_go_sum_sha256"}, {directory + "/module-files.json", "module_files_sha256"}} {
		if behavior76Hash(read(check.Path)) != pins[check.Pin] {
			t.Fatal("observer build provenance changed")
		}
	}
	var files []struct {
		Path     string `json:"path"`
		Original string `json:"original_sha256"`
		Derived  string `json:"derived_sha256"`
	}
	if json.Unmarshal(read(directory+"/module-files.json"), &files) != nil || len(files) == 0 {
		t.Fatal("full observer module inventory absent")
	}
	expected := map[string]bool{}
	changes := 0
	for _, file := range files {
		if !fs.ValidPath(file.Path) || expected[file.Path] {
			t.Fatal("unsafe observer module inventory")
		}
		expected[file.Path] = true
		if behavior76Hash(read(filepath.Join(directory, "pgx", file.Path))) != file.Derived || behavior76Hash(read(filepath.Join("/module-cache/github.com/jackc/pgx/v5@v5.8.0", file.Path))) != file.Original {
			t.Fatal("actual copied/original observer module bytes differ")
		}
		if file.Original != file.Derived {
			changes++
			if file.Path != "conn.go" || file.Original != pins["original_sha256"] || file.Derived != pins["derived_sha256"] {
				t.Fatal("unexpected research dependency change")
			}
		}
	}
	if changes != 1 {
		t.Fatal("exactly one callback-only source change required")
	}
	err := filepath.WalkDir(directory+"/pgx", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(directory+"/pgx", path)
		if err != nil || !expected[filepath.ToSlash(relative)] {
			return errors.New("unmeasured observer module file")
		}
		return nil
	})
	if err != nil {
		t.Fatal("observer module inventory not complete")
	}
	return map[string]any{"mechanism": "isolated_research_modfile_readonly", "pins": pins, "pins_sha256": behavior76Hash(pinsRaw), "module_files": len(files), "source_changes": changes, "shared_cache_unchanged": true, "production_module_files_unchanged": true}
}
