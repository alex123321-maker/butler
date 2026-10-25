package app

import (
	"context"
	"github.com/butler/butler/internal/transport"
	"sync/atomic"
	"testing"
	"time"
)

func TestBehavior76LocalUnknownSourceIteratorIntegration(t *testing.T) {
	for _, kind := range []string{"documents", "working_metadata"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			var global atomic.Bool
			f := newBehavior76LocalFixture(t, ctx, &global)
			f.seed(behavior76DevelopmentEpisode())
			var query string
			if kind == "documents" {
				if _, err := f.pool.Exec(ctx, `ALTER TABLE behavior76_aliases RENAME TO behavior76_aliases_rows;
 CREATE VIEW behavior76_aliases AS SELECT * FROM behavior76_aliases_rows WHERE CASE WHEN kind='document' THEN 1/(length(value)-length(value))>0 ELSE true END`); err != nil {
					t.Fatal(err)
				}
				query = `SELECT value FROM behavior76_aliases WHERE kind='document' ORDER BY key`
			} else {
				if _, err := f.pool.Exec(ctx, `INSERT INTO behavior76_observations(observation) VALUES('{"kind":"public_fault"}');
 ALTER TABLE behavior76_observations RENAME TO behavior76_observation_rows;
 CREATE VIEW behavior76_observations AS SELECT * FROM behavior76_observation_rows WHERE 1/(n-n)>0`); err != nil {
					t.Fatal(err)
				}
				query = `SELECT observation FROM behavior76_observations ORDER BY n`
			}
			// Confirm a real PostgreSQL deferred row-reader failure, rather than a
			// query-admission error or a mock iterator that mirrors the implementation.
			rows, err := f.pool.Query(ctx, query)
			if err != nil {
				t.Fatal("deferred reader fixture failed at query admission", err)
			}
			for rows.Next() {
			}
			rows.Close()
			if rows.Err() == nil {
				t.Fatal("actual deferred PostgreSQL failure absent")
			}
			if kind == "documents" {
				if hits, err := (behavior76Documents{f: f}).Search(ctx, nil, "public", 10); err == nil || len(hits) != 0 {
					t.Fatal("failed row stream became a usable partial source selection", err)
				}
			} else {
				before, err := f.tasks.Get(ctx, f.lookup("task", "A"))
				if err != nil {
					t.Fatal(err)
				}
				if err = f.enrichWorking("A"); err == nil {
					t.Fatal("failed metadata rows were accepted")
				}
				after, err := f.tasks.Get(ctx, before.TaskID)
				if err != nil || after.WorkingRevision != before.WorkingRevision || behavior76JSON(after.Working) != behavior76JSON(before.Working) {
					t.Fatal("partial trusted metadata persisted", err)
				}
			}
			if !global.Load() || f.scope.localStopped() {
				t.Fatal("source observation failure was not GLOBAL")
			}
			called := false
			provider := &behavior76NativeProvider{fixture: f, sealed: &global, scope: f.scope}
			_, err = provider.invoke(ctx, "start", "never-admitted-public-source-run", nil, func(context.Context) (transport.EventStream, error) { called = true; return nil, nil })
			if err == nil || called {
				t.Fatal("source iterator failure admitted next provider callback")
			}
			t.Log("actual PG deferred row failure -> no partial document selection/working write -> GLOBAL before provider callback; native/auth=0")
		})
	}
}
