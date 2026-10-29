package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/jackc/pgx/v5/pgxpool"
)

const experienceNativeCarryPrefix = "issue90-v1-current-day-carry:"

// The source transaction imports the latest inclusive current owner/day charge
// once using normal budget rows. Older-day history remains a separate recursively
// referenced record. The separate global record binds
// the640 NEW-entry journal to that exact receipt. These are finite lab partitions,
// not a production atomic budget transaction across PostgreSQL databases.
func applyExperienceNativeHistory(ctx context.Context, p *experienceNativeProvider, source *pgxpool.Pool, history experienceNativeLineage, sourcePolicy budget.Policy) error {
	expected, err := experienceNativePolicyForHistory(true, history)
	if err != nil || !reflect.DeepEqual(expected, sourcePolicy) || p == nil || p.global == nil || source == nil || validateExperienceNativeLineage(history) != nil {
		return errors.New("validated exact predeclared history/policy and owning databases required")
	}
	var current time.Time
	if source.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&current) != nil || current.UTC().Format("2006-01-02") != history.Day || !history.Deadline.After(current) {
		return errors.New("current owner-day history expired before source import")
	}
	var untouched bool
	if p.global.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM experience_native_windows WHERE window_id=$1 AND owner_id=$2 AND NOT closed AND used=0)`, p.window, p.owner).Scan(&untouched) != nil || !untouched {
		return errors.New("history must precede every provider admission")
	}
	tx, err := source.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `CREATE TABLE experience_native_source_history(singleton BOOLEAN PRIMARY KEY CHECK(singleton),history_sha256 TEXT NOT NULL,metadata JSONB NOT NULL)`); err != nil {
		return errors.New("source history identity already consumed")
	}
	metadata, _ := json.Marshal(history)
	if _, err = tx.Exec(ctx, `INSERT INTO experience_native_source_history VALUES(true,$1,$2)`, history.Hash, metadata); err != nil {
		return errors.New("source history binding failed")
	}
	ledger := budget.NewLedger(source)
	scope, err := ledger.EnsureTx(ctx, tx, budget.ScopeSpec{Kind: "owner_day", OwnerID: p.owner, SubjectID: history.Day, Limits: sourcePolicy.Day, Deadline: history.Deadline})
	if err != nil || scope.Used != (budget.Units{}) {
		return errors.New("source owner-day scope already charged or unavailable")
	}
	reservation, fresh, err := ledger.ReserveTx(ctx, tx, budget.Request{OwnerID: p.owner, OperationID: experienceNativeCarryPrefix + history.Hash, ScopeIDs: []string{scope.ID}, Charge: history.Carry, CostKnown: true})
	if err != nil || !fresh {
		return errors.New("inclusive history carry already consumed or denied")
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	// A failure after this commit is retained as a failed setup, before auth or
	// source execution. No copy is reopened to replace it or refund old history.
	if _, err = p.global.Exec(ctx, `CREATE TABLE IF NOT EXISTS experience_native_history(window_id TEXT PRIMARY KEY REFERENCES experience_native_windows,history_sha256 TEXT NOT NULL,source_carry_reservation_id TEXT NOT NULL,metadata JSONB NOT NULL)`); err != nil {
		return errors.New("independent global history table unavailable")
	}
	tag, err := p.global.Exec(ctx, `INSERT INTO experience_native_history SELECT window_id,$3,$4,$5 FROM experience_native_windows WHERE window_id=$1 AND owner_id=$2 AND NOT closed AND used=0`, p.window, p.owner, history.Hash, reservation.ID, metadata)
	if err != nil || tag.RowsAffected() != 1 {
		return errors.New("independent window/history binding consumed or changed")
	}
	return p.writeNew("history-import.json", map[string]any{"history": history, "source_carry_reservation_id": reservation.ID, "source_scope_id": scope.ID, "new_day_limits": sourcePolicy.Day, "new_native_entry_limit": p.maximum, "carry_is_new_provider_attempt": false, "cross_database_production_authority": false})
}

func TestExperienceNativeHistoryCarryPrecedesSourceAndGlobalAdmissionsIntegration(t *testing.T) {
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	p, err := newExperienceNativeProvider(t.TempDir(), experienceNativeSchedule(experienceWindowFixture()))
	if err != nil {
		t.Fatal(err)
	}
	global := memorytest.Database(t)
	if err = p.installGlobal(ctx, global, experienceDigest(t.Name()+time.Now().String())); err != nil {
		t.Fatal(err)
	}
	databases, err := newExperienceNativeClones(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	defer databases.source.Close()
	now := time.Now().UTC()
	deadline := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	history := experienceNativeLineage{Hash: strings.Repeat("f", 64), Day: now.Format("2006-01-02"), Deadline: deadline, Carry: budget.Units{ModelTurns: 3, Tokens: 3 * 1081344, ActiveMillis: 900000, ProviderInputBytes: 3 * 8388608}, Usage: temporalV5CurrentUsage{ProductAttempts: 3, ProductNULL: 3, ProductWithoutNativeEntry: 3}}
	policy, err := experienceNativePolicyForHistory(true, history)
	if err != nil || applyExperienceNativeHistory(ctx, p, databases.source, history, policy) != nil {
		t.Fatal("current inclusive source/global carry was not imported")
	}
	ledger := budget.NewLedger(databases.source)
	scope, err := ledger.Get(ctx, budget.ScopeID("owner_day", p.owner, history.Day))
	if err != nil || scope.Used != history.Carry || scope.Limits.ModelTurns != 643 || !scope.Deadline.Equal(deadline) {
		t.Fatal("import changed the current owner-day charge/deadline")
	}
	if applyExperienceNativeHistory(ctx, p, databases.source, history, policy) == nil {
		t.Fatal("same source carry was imported twice")
	}
	changed := history
	changed.Hash = strings.Repeat("e", 64)
	if applyExperienceNativeHistory(ctx, p, databases.source, changed, policy) == nil {
		t.Fatal("another hash reopened the source carry identity")
	}
	var bindings, used int
	if global.QueryRow(ctx, `SELECT (SELECT count(*) FROM experience_native_history WHERE window_id=$1),used FROM experience_native_windows WHERE window_id=$1`, p.window).Scan(&bindings, &used) != nil || bindings != 1 || used != 0 {
		t.Fatal("old charges became new provider entries or lost their global receipt")
	}
	if p.export() != nil {
		t.Fatal("closed global history export unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(p.proof, "global-journal-final.json"))
	var exported map[string][]struct {
		History string                  `json:"history_sha256"`
		Data    experienceNativeLineage `json:"metadata"`
	}
	if err != nil || json.Unmarshal(raw, &exported) != nil || len(exported["experience_native_history"]) != 1 || exported["experience_native_history"][0].Data.Usage.ProductNULL != 3 || exported["experience_native_history"][0].Data.Usage.NativeEntries != 0 {
		t.Fatal("zero-native charged NULL predecessor disappeared from global export")
	}
}
