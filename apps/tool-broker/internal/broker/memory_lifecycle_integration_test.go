package broker

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
)

func TestMemoryLifecycleAttackCannotAuthorizeBrokerIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if e := manifest.Initialize(); e != nil {
		t.Fatal(e)
	}
	sources := catalog.New(pool, manifest)
	owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "synthetic-lifecycle"})
	for _, attack := range memorytest.LifecycleAttacks(t) {
		t.Run(attack.ID, func(t *testing.T) {
			src, e := sources.Put(owner, catalog.PutSource{ExternalKey: attack.ID, Kind: "document", ScopeType: "user", ScopeID: "owner", Body: attack.Body})
			if e != nil {
				t.Fatal(e)
			}
			child, e := sources.Put(owner, catalog.PutSource{ExternalKey: attack.ID + ":retained", Kind: "model_candidate", ScopeType: "user", ScopeID: "owner", Body: attack.Body, Parents: []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}})
			if e != nil {
				t.Fatal(e)
			}
			read, e := catalog.New(pool, manifest).Get(owner, child.ID)
			if e != nil || read.Origin.IsInstruction() {
				t.Fatal("retained read lost DATA boundary", e)
			}
			executor := &countingExecutor{}
			server := NewServer(loadTestRegistry(t), executor, nil, nil)
			authority := newAuthFixture() // Current context, with no owner approval/mandate.
			server.SetActionAuthorizer(actionauth.NewService(actionauth.NewPostgresStore(pool), authority, nil))
			args, _ := json.Marshal(map[string]string{"method": "POST", "url": "https://example.com/change", "body": read.Body})
			call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: attack.ID, ToolName: "http.request", ArgsJson: string(args), AutonomyMode: 3}
			prepared, e := server.PrepareToolCall(owner, &toolbrokerv1.PrepareToolCallRequest{ToolCall: call, LogicalActionRef: attack.ID})
			if e != nil || prepared.GetAction() == nil {
				t.Fatal("test never reached authorization", e, prepared)
			}
			grant, e := server.AuthorizeToolCall(owner, &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: prepared.GetAction().GetActionId(), ApprovalId: "historical-approved", MandateId: "quoted-mandate"})
			if e == nil && grant.GetAuthorizationId() != "" {
				t.Fatal("memory supplied current authority")
			}
			result, e := server.ExecuteToolCall(owner, &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: "historical-approved"})
			if e != nil || result.GetResult().GetStatus() != "failed" || executor.calls != 0 {
				t.Fatal("unapproved memory dispatched runtime", e, executor.calls)
			}
		})
	}
	var grants, mandates int
	if e := pool.QueryRow(owner, `SELECT (SELECT count(*) FROM action_authorizations),(SELECT count(*) FROM action_mandates)`).Scan(&grants, &mandates); e != nil || grants != 0 || mandates != 0 {
		t.Fatal("memory corpus created authority", grants, mandates, e)
	}
}
