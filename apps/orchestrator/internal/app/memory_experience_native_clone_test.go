package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These are controlled-lab database partitions, not cross-database production
// budget authority. Auth and the one global journal live in p.global only.
// Every allocated database is retained, including failed clones.
type experienceNativeClones struct {
	provider *experienceNativeProvider
	template string
	source   *pgxpool.Pool
	frozen   bool
}

func newExperienceNativeClones(ctx context.Context, p *experienceNativeProvider) (*experienceNativeClones, error) {
	if p == nil || p.global == nil || len(p.window) != 64 || strings.Trim(p.window, "0123456789abcdef") != "" || !strings.HasSuffix(p.global.Config().ConnConfig.Database, "_test") {
		return nil, errors.New("isolated persistent native window required")
	}
	c := &experienceNativeClones{provider: p, template: "experience90_" + p.window[:12] + "_source_test"}
	_, err := p.global.Exec(ctx, `CREATE TABLE IF NOT EXISTS experience_native_templates(window_id TEXT PRIMARY KEY REFERENCES experience_native_windows,database_name TEXT UNIQUE NOT NULL,state TEXT NOT NULL CHECK(state IN('preparing','frozen','failed')));
CREATE TABLE IF NOT EXISTS experience_native_case_databases(window_id TEXT NOT NULL,slot_id TEXT NOT NULL,database_name TEXT UNIQUE NOT NULL,state TEXT NOT NULL CHECK(state IN('reserved','created','failed')),PRIMARY KEY(window_id,slot_id),FOREIGN KEY(window_id,slot_id)REFERENCES experience_native_allocations)`)
	if err != nil {
		return nil, err
	}
	// Allocation precedes CREATE. A failure or ambiguous outcome consumes this
	// identity; neither a new process nor a new proof directory can retry it.
	tag, err := p.global.Exec(ctx, `INSERT INTO experience_native_templates(window_id,database_name,state)SELECT window_id,$2,'preparing' FROM experience_native_windows WHERE window_id=$1 AND owner_id=$3 AND NOT closed`, p.window, c.template, p.owner)
	if err != nil || tag.RowsAffected() != 1 {
		return nil, errors.New("source database identity already consumed or unavailable")
	}
	if _, err = p.global.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{c.template}.Sanitize()); err != nil {
		c.failTemplate()
		return nil, errors.New("one-use source database creation failed")
	}
	c.source, err = c.connect(ctx, c.template)
	if err != nil {
		c.failTemplate()
		return nil, err
	}
	paths, err := filepath.Glob("../../../../migrations/*.up.sql")
	if err != nil || len(paths) == 0 {
		c.source.Close()
		c.failTemplate()
		return nil, errors.New("frozen migrations unavailable")
	}
	sort.Strings(paths)
	for _, path := range paths {
		raw, e := os.ReadFile(path)
		if e == nil {
			_, e = c.source.Exec(ctx, string(raw))
		}
		if e != nil {
			c.source.Close()
			c.failTemplate()
			return nil, fmt.Errorf("source migration failed: %s", filepath.Base(path))
		}
	}
	return c, nil
}

func (c *experienceNativeClones) connect(ctx context.Context, database string) (*pgxpool.Pool, error) {
	cfg := c.provider.global.Config()
	cfg.ConnConfig.Database = database
	// An owning global schema and its auth/journal are never copied into the
	// source or selected through an inherited global search_path.
	cfg.ConnConfig.RuntimeParams = map[string]string{"search_path": "public"}
	cfg.MaxConns, cfg.MinConns = 2, 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err == nil {
		err = pool.Ping(ctx)
	}
	if err != nil {
		if pool != nil {
			pool.Close()
		}
		return nil, errors.New("isolated native task database unavailable")
	}
	return pool, nil
}

func (c *experienceNativeClones) failTemplate() {
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	_, _ = c.provider.global.Exec(ctx, `UPDATE experience_native_templates SET state='failed' WHERE window_id=$1 AND state='preparing'`, c.provider.window)
}

// Call only after the actual source/note/skill preparation gates and all source
// Broker/runtime processes have stopped. This mechanism proves immutable reset,
// not the semantic quality or successful completion of those gates.
func (c *experienceNativeClones) freeze(ctx context.Context) error {
	if c.frozen || c.source == nil {
		return errors.New("source template already consumed")
	}
	var current bool
	if c.provider.global.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM experience_native_templates t JOIN experience_native_windows w USING(window_id) WHERE t.window_id=$1 AND t.database_name=$2 AND t.state='preparing' AND NOT w.closed AND w.owner_id=$3)`, c.provider.window, c.template, c.provider.owner).Scan(&current) != nil || !current {
		return errors.New("current one-use source allocation required")
	}
	var private int
	if c.source.QueryRow(ctx, `SELECT count(*) FROM system_settings WHERE is_secret OR key='BUTLER_PROVIDER_AUTH_ANTIGRAVITY'`).Scan(&private) != nil || private != 0 {
		c.failTemplate()
		return errors.New("auth or secret settings cannot enter the task-data template")
	}
	c.source.Close()
	c.source = nil
	if _, err := c.provider.global.Exec(ctx, "ALTER DATABASE "+pgx.Identifier{c.template}.Sanitize()+" WITH ALLOW_CONNECTIONS false IS_TEMPLATE true"); err != nil {
		c.failTemplate()
		return errors.New("source template could not be sealed")
	}
	var connections int
	if c.provider.global.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=$1`, c.template).Scan(&connections) != nil || connections != 0 {
		c.failTemplate()
		return errors.New("source template still has live processes")
	}
	tag, err := c.provider.global.Exec(ctx, `UPDATE experience_native_templates SET state='frozen' WHERE window_id=$1 AND database_name=$2 AND state='preparing'`, c.provider.window, c.template)
	if err != nil || tag.RowsAffected() != 1 {
		return errors.New("source template current allocation changed")
	}
	if err = c.provider.writeNew("database-template.json", map[string]any{"window_id": c.provider.window, "database": c.template, "auth_settings": 0, "connections": 0}); err != nil {
		return err
	}
	c.frozen = true
	return nil
}

func (c *experienceNativeClones) clone(ctx context.Context, index int) (*pgxpool.Pool, error) {
	if !c.frozen || index < 0 || index >= len(c.provider.slots) || c.provider.slots[index].Kind != "control" {
		return nil, errors.New("frozen template and predeclared control slot required")
	}
	slot := c.provider.slots[index]
	name := fmt.Sprintf("experience90_%s_case_%03d_test", c.provider.window[:12], slot.Sequence)
	tag, err := c.provider.global.Exec(ctx, `INSERT INTO experience_native_case_databases(window_id,slot_id,database_name,state)SELECT t.window_id,$2,$3,'reserved' FROM experience_native_templates t JOIN experience_native_windows w USING(window_id) WHERE t.window_id=$1 AND t.state='frozen' AND t.database_name=$4 AND NOT w.closed AND w.owner_id=$5`, c.provider.window, slot.ID, name, c.template, c.provider.owner)
	if err != nil || tag.RowsAffected() != 1 {
		return nil, errors.New("control clone identity consumed or template unavailable")
	}
	if err = c.provider.writeNew(fmt.Sprintf("clone-%03d-reserved.json", slot.Sequence), map[string]any{"slot": slot, "database": name, "template": c.template}); err != nil {
		return nil, err
	}
	if _, err = c.provider.global.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH TEMPLATE "+pgx.Identifier{c.template}.Sanitize()+" ALLOW_CONNECTIONS true IS_TEMPLATE false"); err != nil {
		retained, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_, _ = c.provider.global.Exec(retained, `UPDATE experience_native_case_databases SET state='failed' WHERE window_id=$1 AND slot_id=$2 AND state='reserved'`, c.provider.window, slot.ID)
		return nil, errors.New("one-use control clone creation failed")
	}
	pool, err := c.connect(ctx, name)
	if err != nil {
		return nil, err
	}
	if _, err = c.provider.global.Exec(ctx, `UPDATE experience_native_case_databases SET state='created' WHERE window_id=$1 AND slot_id=$2 AND state='reserved'`, c.provider.window, slot.ID); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func TestExperienceNativeClonesPreserveIsolationAndGlobalJournalIntegration(t *testing.T) {
	global := memorytest.Database(t)
	ctx, stop := context.WithTimeout(context.Background(), 20*time.Second)
	defer stop()
	p, err := newExperienceNativeProvider(t.TempDir(), experienceNativeSchedule(experienceWindowFixture()))
	if err != nil {
		t.Fatal(err)
	}
	window := experienceDigest(t.Name() + time.Now().String())
	if err = p.installGlobal(ctx, global, window); err != nil {
		t.Fatal(err)
	}
	c, err := newExperienceNativeClones(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c.source != nil {
			c.source.Close()
		}
	})
	if _, err = c.clone(ctx, 9); err == nil {
		t.Fatal("unfrozen template admitted a control")
	}
	if _, err = c.source.Exec(ctx, `CREATE TABLE controlled_clone_fixture(value TEXT NOT NULL);INSERT INTO controlled_clone_fixture VALUES('actual frozen database bytes')`); err != nil {
		t.Fatal(err)
	}
	// The shared setting is opaque fixture DATA, never a credential value.
	if _, err = global.Exec(ctx, `CREATE TABLE controlled_shared_auth_fixture(value TEXT NOT NULL);INSERT INTO controlled_shared_auth_fixture VALUES('opaque shared test marker')`); err != nil {
		t.Fatal(err)
	}
	if err = c.freeze(ctx); err != nil {
		t.Fatal(err)
	}
	if err = c.freeze(ctx); err == nil {
		t.Fatal("source template was resealed")
	}
	if _, err = c.clone(ctx, 0); err == nil {
		t.Fatal("original slot borrowed control clone capacity")
	}
	first, err := c.clone(ctx, 9)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err = first.Exec(ctx, `UPDATE controlled_clone_fixture SET value='first clone changed'`); err != nil {
		t.Fatal(err)
	}
	if _, err = c.clone(ctx, 9); err == nil {
		t.Fatal("used clone was reopened")
	}
	second, err := c.clone(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var value string
	if second.QueryRow(ctx, `SELECT value FROM controlled_clone_fixture`).Scan(&value) != nil || value != "actual frozen database bytes" {
		t.Fatal("control clones shared task mutations")
	}
	var leaked bool
	if second.QueryRow(ctx, `SELECT to_regclass('public.experience_native_windows') IS NOT NULL OR to_regclass('public.controlled_shared_auth_fixture') IS NOT NULL OR EXISTS(SELECT 1 FROM system_settings WHERE is_secret OR key='BUTLER_PROVIDER_AUTH_ANTIGRAVITY')`).Scan(&leaked) != nil || leaked {
		t.Fatal("shared auth/global journal leaked into clone")
	}
	// An actually failed CREATE is retained before any data can be populated.
	third := fmt.Sprintf("experience90_%s_case_%03d_test", window[:12], p.slots[11].Sequence)
	if _, err = global.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{third}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = c.clone(ctx, 11); err == nil {
		t.Fatal("existing database was silently reused")
	}
	if _, err = c.clone(ctx, 11); err == nil {
		t.Fatal("failed allocation was retried")
	}
	var state string
	if global.QueryRow(ctx, `SELECT state FROM experience_native_case_databases WHERE window_id=$1 AND slot_id=$2`, window, p.slots[11].ID).Scan(&state) != nil || state != "failed" {
		t.Fatal("failed allocation was not durable")
	}
	if err = p.reserveGlobal(ctx, &experienceNativeEntry{Sequence: 1, Slot: p.slots[9].ID, RunID: "failed-native-fixture", Kind: "start", Request: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	var used, unknown int
	if global.QueryRow(ctx, `SELECT used FROM experience_native_windows WHERE window_id=$1`, window).Scan(&used) != nil || used != 1 || global.QueryRow(ctx, `SELECT count(*) FROM experience_native_entries WHERE window_id=$1 AND usage_json IS NULL`, window).Scan(&unknown) != nil || unknown != 1 {
		t.Fatal("cloning reset global cost or replaced unknown usage")
	}
	if _, err = global.Exec(ctx, `UPDATE experience_native_windows SET closed=TRUE WHERE window_id=$1`, window); err != nil {
		t.Fatal(err)
	}
	if _, err = c.clone(ctx, 12); err == nil {
		t.Fatal("closed window admitted another clone")
	}
}

func TestExperienceNativeTemplateRejectsAuthBeforeFreezeIntegration(t *testing.T) {
	global := memorytest.Database(t)
	ctx, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	p, err := newExperienceNativeProvider(t.TempDir(), experienceNativeSchedule(experienceWindowFixture()))
	if err != nil {
		t.Fatal(err)
	}
	if err = p.installGlobal(ctx, global, experienceDigest(t.Name()+time.Now().String())); err != nil {
		t.Fatal(err)
	}
	c, err := newExperienceNativeClones(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	defer c.source.Close()
	// Opaque marker only, with no real credential import or provider setup.
	if _, err = c.source.Exec(ctx, `INSERT INTO system_settings(key,value,component,is_secret,updated_by)VALUES('BUTLER_PROVIDER_AUTH_ANTIGRAVITY','opaque test marker','fixture',false,'fixture')`); err != nil {
		t.Fatal(err)
	}
	if err = c.freeze(ctx); err == nil || c.frozen {
		t.Fatal("an auth key entered the frozen task-data template")
	}
	var state string
	if global.QueryRow(ctx, `SELECT state FROM experience_native_templates WHERE window_id=$1`, p.window).Scan(&state) != nil || state != "failed" {
		t.Fatal("rejected source preparation was not retained")
	}
	if _, err = c.clone(ctx, 9); err == nil {
		t.Fatal("rejected source template admitted a control clone")
	}
}
