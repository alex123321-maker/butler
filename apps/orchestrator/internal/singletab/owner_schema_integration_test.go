package singletab_test

import (
	"context"
	"testing"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/singletabcontrol"
)

func TestOwnerWaitingSelectionSchemaRejectsMissingOrReplacedImmutability(t *testing.T) {
	for _, mutation := range []string{
		`ALTER TABLE guarded_single_tab_requests DISABLE TRIGGER guarded_request_immutable`,
		`DROP TRIGGER guarded_selection_immutable ON guarded_single_tab_selections`,
		`ALTER TABLE guarded_single_tab_choices DISABLE TRIGGER guarded_choice_immutable`,
		`CREATE OR REPLACE FUNCTION guarded_single_tab_immutable() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN RETURN NEW; END'`,
		`ALTER TABLE execution_resource_dispatches DROP COLUMN single_tab_binding`,
	} {
		pool := memorytest.Database(t)
		ctx := context.Background()
		if singletabcontrol.CheckSchema(ctx, pool) != nil {
			t.Fatal("real migration063 rejected")
		}
		if _, err := pool.Exec(ctx, mutation); err != nil {
			t.Fatal(err)
		}
		if singletabcontrol.CheckSchema(ctx, pool) == nil {
			t.Fatal("mutated boundary considered ready", mutation)
		}
	}
}
