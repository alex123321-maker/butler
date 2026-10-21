package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
)

func TestCommunicationTelegramCurrentExportPolicyIntegration(t *testing.T) {
	for _, scenario := range []string{"private-allowed", "private-denied", "wrong-recipient", "binding-revoked", "source-revoked"} {
		t.Run(scenario, func(t *testing.T) {
			f := newDelegationGuardFixtureWithManifest(t, filepath.Join(t.TempDir(), "deletions.log"))
			run := "telegram-export-run"
			ctx := f.leasedWorker(t, f.parent, run)
			ref := f.source(t, "private-export", "user", f.parent.OwnerID, "Private synthetic owner DATA", nil)
			tx, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.sources.AccessTx(ctx, tx, []catalog.SourceRef{{ID: ref.ID, Revision: ref.Revision, AccessRevision: ref.AccessRevision, Hash: ref.Hash}}, "memory_read"); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			b := communication.Binding{ID: "test-bot", Version: 1, Channel: "telegram", Recipient: "telegram:42", Enabled: true, Capabilities: []string{"message"}}
			core := &communication.Store{Pool: f.pool, Inputs: input.NewStore(f.pool)}
			actor := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: b.Channel, SessionKey: b.Recipient, DeliveryID: "operator-bind"})
			if err = core.Bind(actor, b, 0); err != nil {
				t.Fatal(err)
			}
			policy := telegramExportPolicy{owner: f.parent.OwnerID, binding: b, allowPrivate: scenario != "private-denied", catalog: f.sources, sourceGuard: f.sources.CheckRunSourcesTx}
			if scenario == "wrong-recipient" {
				b.Recipient = "telegram:other"
			}
			if scenario == "binding-revoked" {
				if _, err = f.pool.Exec(f.ctx, `UPDATE communication_bindings SET enabled=FALSE WHERE binding_id=$1`, b.ID); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "source-revoked" {
				if _, err = f.sources.Delete(f.owner, ref.ID, "revoke", ref.AccessRevision); err != nil {
					t.Fatal(err)
				}
			}
			// Delivery has no execution lease. Current Catalog checks must also
			// succeed/fail correctly from its ordinary background context.
			tx, err = f.pool.Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			err = policy.check(context.Background(), tx, run, b, communicationcontrol.Args{})
			if (err == nil) != (scenario == "private-allowed") {
				t.Fatal("export policy boundary", scenario, err)
			}
		})
	}
}
