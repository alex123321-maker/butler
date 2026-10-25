package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Fixed prospective local infrastructure identity. No model, credential source,
// provider, retry, data mutation or timeout adjustment is involved in this probe.
func behavior76LocalResources(ctx context.Context, pool *pgxpool.Pool) (map[string]any, error) {
	if pool == nil || pool.Config().ConnConfig.Host != "127.0.0.1" || pool.Config().ConnConfig.Port != 55451 || pool.Config().ConnConfig.Database != "behavior76_test" {
		return nil, errors.New("exact local control endpoint required")
	}
	var version, maximum, minimum, buffers, fsync, dataDirectory string
	var jit bool
	err := pool.QueryRow(ctx, `SELECT current_setting('server_version'),current_setting('max_wal_size'),current_setting('min_wal_size'),current_setting('shared_buffers'),current_setting('fsync'),pg_jit_available(),current_setting('data_directory')`).Scan(&version, &maximum, &minimum, &buffers, &fsync, &jit, &dataDirectory)
	if err != nil {
		return nil, err
	}
	if version != "17.11" || maximum != "128MB" || minimum != "32MB" || buffers != "32MB" || fsync != "on" || jit || dataDirectory != "/var/lib/postgresql/effort76" {
		return nil, errors.New("prospective local PostgreSQL resource identity changed")
	}
	raw, err := exec.CommandContext(ctx, "/bin/df", "-Pk", dataDirectory).Output()
	if err != nil {
		return nil, errors.New("local disk resource observation unavailable")
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		return nil, errors.New("local disk resource observation malformed")
	}
	fields := strings.Fields(lines[1])
	if len(fields) != 6 || !strings.HasPrefix(fields[0], "/dev/loop") {
		return nil, errors.New("persistent private local disk required")
	}
	capacity, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return nil, err
	}
	available, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return nil, err
	}
	if capacity < 3*1024*1024 || available < 1024*1024 {
		return nil, errors.New("prospective local disk capacity unavailable")
	}
	return map[string]any{"version": "behavior76-persistent-local-resources-v1", "endpoint": "127.0.0.1:55451/behavior76_test", "data_directory": dataDirectory, "server_version": version, "max_wal_size": maximum, "min_wal_size": minimum, "shared_buffers": buffers, "fsync": fsync, "jit_available": jit, "disk_device": fields[0], "disk_capacity_kib": capacity, "disk_available_kib": available, "minimum_available_kib": 1024 * 1024, "authority_total_connections": 10, "authority_reserved_connections": 1, "watchdog_millis": 750}, nil
}

func TestBehavior76LocalUnknownResourceProfileIntegration(t *testing.T) {
	url := os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL")
	if url == "" {
		t.Skip("isolated local PostgreSQL required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil || cfg.ConnConfig.Host != "127.0.0.1" || cfg.ConnConfig.Port != 55451 {
		t.Fatal("isolated local resource probe required")
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	resources, err := behavior76LocalResources(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("prospective persistent private disk/PostgreSQL resource identity observed before any native admission; native/auth=0", behavior76JSON(resources))
}
