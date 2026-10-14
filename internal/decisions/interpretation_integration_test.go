package decisions

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
)

func TestInterpretationCurrentOwnedBoundaryIntegration(t *testing.T) {
	for _, variant := range []string{"complete", "wrong_profile", "partial", "cancelled", "late_source", "wrong_purpose", "wrong_owner"} {
		t.Run(variant, func(t *testing.T) {
			f := newFixture(t)
			p := InterpretationProfiles()[0]
			j, ref := f.interpretationJob(t, "v3-"+variant, `{"context":"Текущая задача: отчёт.","text":"Приостанови выгрузку.","focus":"Приостанови выгрузку."}`, 7*2048)
			b := &fakeBackend{profile: &p, output: interpretationScores([]string{"stop_work"}, "direct", "current_operation")}
			expected := p
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			switch variant {
			case "wrong_profile":
				p.Preprocessing = "unreviewed"
			case "partial":
				b.output.Scores = b.output.Scores[:15]
			case "cancelled":
				b.before = cancel
			case "late_source":
				b.before = func() {
					if _, err := f.catalog.Put(f.owner, catalog.PutSource{ExternalKey: "v3-" + variant, Kind: "document", ScopeType: "user", ScopeID: "owner", ExpectedRevision: 1, Body: "changed", Author: "fixture"}); err != nil {
						t.Fatal(err)
					}
				}
			case "wrong_purpose":
				j.Purpose = "extract"
			case "wrong_owner":
				j.OwnerID = "foreign-owner"
			}
			o, _, err := Interpret(ctx, f.catalog, f.jobs, b, f.target, expected, j, ref)
			if variant == "complete" {
				if err != nil || o.Escalate || b.calls.Load() != 1 {
					t.Fatal(o, err)
				}
				ledger, err := f.budgets.Ledger.Get(context.Background(), budget.ScopeID("auxiliary", "owner", j.ID))
				if err != nil || ledger.Used.Tokens != 7*2048 || ledger.Used.ActiveMillis != int64(b.timeoutMS) || ledger.Used.ActiveMillis > 10000 {
					t.Fatal("wire reservation differs", err, ledger)
				}
			} else if err == nil {
				t.Fatal("invalid observation accepted", variant)
			}
			if (variant == "wrong_profile" || variant == "wrong_purpose" || variant == "wrong_owner") && b.calls.Load() != 0 {
				t.Fatal("invalid profile/job reached IPC")
			}
			var count int
			if err := f.catalog.Pool().QueryRow(context.Background(), `SELECT count(*) FROM memory_claims`).Scan(&count); err != nil || count != 0 {
				t.Fatal("interpretation accepted memory", err)
			}
		})
	}
}
