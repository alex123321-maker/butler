package communication_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/butler/butler/internal/memory/memorytest"
)

func migration(t *testing.T, name string) string {
	t.Helper()
	raw, e := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", name))
	if e != nil {
		t.Fatal(e)
	}
	return string(raw)
}

func TestCommunicationOrderedMigrationRollbackIntegration(t *testing.T) {
	pool := memorytest.DatabaseAtVersion(t, 88)
	ctx := context.Background()
	for n := 0; n < 2; n++ {
		for _, name := range []string{"089_communication_core.up.sql", "090_communication_privacy.up.sql", "090_communication_privacy.down.sql", "089_communication_core.down.sql"} {
			if _, e := pool.Exec(ctx, migration(t, name)); e != nil {
				t.Fatal(name, e)
			}
		}
	}
}

func TestCommunicationRollbackRefusesAcceptedHistoryIntegration(t *testing.T) {
	f := setup(t)
	if _, e := f.publish(t, "saved", "communication.message", message); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"090_communication_privacy.down.sql", "089_communication_core.down.sql"} {
		tx, e := f.pool.Begin(f.ctx)
		if e != nil {
			t.Fatal(e)
		}
		_, e = tx.Exec(f.ctx, migration(t, name))
		_ = tx.Rollback(f.ctx)
		if e == nil {
			t.Fatal("rollback erased accepted history", name)
		}
	}
	if f.count(t, "communication_publications") != 1 {
		t.Fatal("accepted history lost")
	}
}
