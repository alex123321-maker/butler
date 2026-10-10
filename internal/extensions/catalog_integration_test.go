package extensions

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/protobuf/proto"
)

type actionReader struct {
	current  actionauth.ContextSnapshot
	approved actionauth.ApprovalSnapshot
}

func (r *actionReader) GetActionContext(context.Context, string) (actionauth.ContextSnapshot, error) {
	return r.current, nil
}
func (r *actionReader) VerifyActionApproval(context.Context, actionauth.ApprovalRequest) (actionauth.ApprovalSnapshot, error) {
	return r.approved, nil
}

type countedInspector struct{ calls int }

func (i *countedInspector) InspectToolCall(context.Context, *toolbrokerv1.ToolCall) (string, bool, error) {
	i.calls++
	return "metadata", false, nil
}
func currentContext() actionauth.ContextSnapshot {
	return actionauth.ContextSnapshot{RunID: "run", RunState: "tool_pending", SessionKey: "web:owner", OwnerID: "owner", AutonomyMode: 3, TaskID: "task", PinnedTaskRevision: 1, TaskRevision: 1, ConstraintsRevision: 1, TaskStatus: "active", LeaseID: "lease", LeaseEpoch: 1, LeaseExpiresAt: time.Now().Add(time.Hour)}
}
func grantFixtureAccess(t *testing.T, s *Store, r Record) TaskAccess {
	t.Helper()
	a := TaskAccess{TaskID: "task", TaskRevision: 1, ConstraintsRevision: 1, ManifestHash: r.ManifestHash, Generation: r.Generation, Tools: []string{"extension.notes.read", "extension.notes.write"}, Resources: []actionauth.ResourceScope{{Origin: "https://extension.example", PathPrefix: "/notes/item"}}, ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.GrantTask(ownerContext(), r.ID, a); err != nil {
		t.Fatal(err)
	}
	a, err := s.TaskAccess(ownerContext(), "task", r.ID)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func approvedGrant(t *testing.T, s *actionauth.Service, reader *actionReader, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract) actionauth.Grant {
	t.Helper()
	p, err := s.Prepare(context.Background(), call, contract)
	if err != nil {
		t.Fatal(err)
	}
	reader.approved = actionauth.ApprovalSnapshot{ApprovalID: "approval", Status: "approved", Binding: p.Binding, ExpiresAt: time.Now().Add(time.Hour), ResolvedAt: time.Now(), ResolvedVia: "fixture-owner"}
	g, err := s.Authorize(context.Background(), call, contract, p.Binding.ActionID, "approval", "")
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func TestExtensionCatalogAndDispatchUseCurrentPermissionIntegration(t *testing.T) {
	pool := extensionDB(t)
	s, p := extensionFixture(t, pool, budget.DefaultPolicy())
	r := readyFixture(t, s, configuredFixture(t, s, p))
	reader := &actionReader{current: currentContext()}
	inspector := &countedInspector{}
	store := actionauth.NewPostgresStore(pool)
	store.SetContractGuard(s)
	auth := actionauth.NewService(store, reader, inspector)
	auth.SetCapabilities(s)
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "read", ToolName: "extension.notes.read", ArgsJson: `{"url":"https://extension.example/notes/item/one"}`, AutonomyMode: commonv1.AutonomyMode(3)}
	contract, err := s.GetContract(context.Background(), call.ToolName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = auth.Prepare(context.Background(), call, contract); err == nil || inspector.calls != 0 {
		t.Fatal("unattached package reached credential inspection")
	}
	items, err := s.Catalog(context.Background(), reader.current, "", 32)
	if err != nil || len(items) != 0 {
		t.Fatal("unattached package leaked into task catalog")
	}
	a := grantFixtureAccess(t, s, r)
	items, err = s.Catalog(context.Background(), reader.current, "notes read", 32)
	if err != nil || len(items) != 1 || items[0].ToolName != call.ToolName {
		t.Fatalf("scoped lexical catalog unavailable: %d %v", len(items), err)
	}
	changed := proto.Clone(call).(*toolbrokerv1.ToolCall)
	changed.ArgsJson = `{"url":"https://extension.example/notes/other"}`
	if _, err = auth.Prepare(context.Background(), changed, contract); err == nil || inspector.calls != 0 {
		t.Fatal("out-of-task resource reached credential inspection")
	}
	g := approvedGrant(t, auth, reader, call, contract)
	if g.Binding.ExtensionAccessRevision != a.Revision || g.Binding.ExtensionManifestHash != p.Digest || g.Binding.ExtensionAccessExpiresAt == nil || g.ExpiresAt.After(*r.HealthyUntil) {
		t.Fatal("grant omitted exact extension authority")
	}
	if _, err = s.SetEnabled(ownerContext(), r.ID, r.Generation, false); err != nil {
		t.Fatal(err)
	}
	if err = store.ClaimGrant(context.Background(), g.ID); err == nil {
		t.Fatal("disabled extension claimed approved action")
	}
	effect, err := store.GetEffect(context.Background(), g.Binding.ActionID)
	if err != nil || effect.State != actionauth.StateAllowed {
		t.Fatal("denied capability created executing effect")
	}
	var dispatches int
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM action_effect_dispatches`).Scan(&dispatches); err != nil || dispatches != 0 {
		t.Fatal("disabled capability journaled dispatch")
	}
	before := inspector.calls
	if _, err = auth.Prepare(context.Background(), call, contract); err == nil || inspector.calls != before {
		t.Fatal("disabled package inspected credentials")
	}
}
func TestPermissionRevocationAndTaskAmendDenySavedActionIntegration(t *testing.T) {
	for _, mode := range []string{"permission", "task_revision", "health_expiry", "profile_changed"} {
		t.Run(mode, func(t *testing.T) {
			pool := extensionDB(t)
			s, p := extensionFixture(t, pool, budget.DefaultPolicy())
			r := readyFixture(t, s, configuredFixture(t, s, p))
			a := grantFixtureAccess(t, s, r)
			reader := &actionReader{current: currentContext()}
			store := actionauth.NewPostgresStore(pool)
			store.SetContractGuard(s)
			auth := actionauth.NewService(store, reader, nil)
			auth.SetCapabilities(s)
			call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "read", ToolName: "extension.notes.read", ArgsJson: `{"url":"https://extension.example/notes/item/one"}`, AutonomyMode: commonv1.AutonomyMode(3)}
			contract, _ := s.GetContract(context.Background(), call.ToolName)
			g := approvedGrant(t, auth, reader, call, contract)
			switch mode {
			case "permission":
				a.Revoked = true
				if err := s.GrantTask(ownerContext(), r.ID, a); err != nil {
					t.Fatal(err)
				}
			case "task_revision":
				if _, err := pool.Exec(context.Background(), `UPDATE tasks SET revision=2,constraints_revision=2 WHERE task_id='task'`); err != nil {
					t.Fatal(err)
				}
			case "health_expiry":
				if _, err := pool.Exec(context.Background(), `UPDATE extension_installations SET healthy_until=clock_timestamp()-interval '1 second'`); err != nil {
					t.Fatal(err)
				}
			case "profile_changed":
				profile := s.profiles["reference-http"]
				profile.Dependency.SHA256 = strings.Repeat("c", 64)
				s.profiles["reference-http"] = profile
			}
			if err := store.ClaimGrant(context.Background(), g.ID); err == nil {
				t.Fatal("stale capability claimed dispatch")
			}
		})
	}
}
func TestReviewedReceiptIsExactContractAndGenericWriteStaysUnknownIntegration(t *testing.T) {
	pool := extensionDB(t)
	m := fixtureManifest()
	m.Operations[1].Receipt = &Receipt{Protocol: "external_receipt_v1", IdempotencyHeader: "Idempotency-Key", VerificationPath: "/notes/effects/{idempotency_key}"}
	p, err := parseFixture(t, m)
	if err != nil {
		t.Fatal(err)
	}
	s := NewStore(pool, "owner", map[string]Profile{"reference-http": {Target: "runtime:9090", Dependency: p.Manifest.Dependencies[0]}}, budget.NewController(pool, budget.DefaultPolicy()))
	r := readyFixture(t, s, configuredFixture(t, s, p))
	grantFixtureAccess(t, s, r)
	reader := &actionReader{current: currentContext()}
	store := actionauth.NewPostgresStore(pool)
	store.SetContractGuard(s)
	auth := actionauth.NewService(store, reader, nil)
	auth.SetCapabilities(s)
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "write", ToolName: "extension.notes.write", ArgsJson: `{"url":"https://extension.example/notes/item/one","body":"bounded"}`, AutonomyMode: commonv1.AutonomyMode(3)}
	contract, err := s.GetContract(context.Background(), call.ToolName)
	if err != nil {
		t.Fatal(err)
	}
	g := approvedGrant(t, auth, reader, call, contract)
	if err = store.ClaimGrant(context.Background(), g.ID); err != nil {
		t.Fatal(err)
	}
	effect, err := auth.EffectForGrant(context.Background(), g)
	if err != nil || effect.IdempotencyKey == "" {
		t.Fatal("recipient key unavailable")
	}
	body, _ := json.Marshal(map[string]any{"butler_effect_receipt": map[string]string{"idempotency_key": effect.IdempotencyKey, "action_fingerprint": effect.Fingerprint, "effect_state": "effect_confirmed", "artifact_id": "fixture-artifact"}})
	out, _ := json.Marshal(map[string]any{"status_code": 201, "body": string(body)})
	result := &toolbrokerv1.ToolResult{ToolCallId: call.ToolCallId, RunId: call.RunId, ToolName: call.ToolName, Status: "completed", ResultJson: string(out)}
	effect, err = auth.Outcome(context.Background(), g, result, false)
	if err != nil || effect.State != actionauth.StateConfirmed || len(effect.EvidenceRefs) != 1 {
		t.Fatalf("exact recipient proof not recorded: %s %v", effect.State, err)
	}
	// Contract policy is owner-reviewed data; a model cannot turn another tool
	// or unbound package descriptor into an external idempotency guarantee.
	forged := g.Binding
	forged.ExtensionAccessRevision = 0
	if _, err = actionauth.ResolveEffectPolicy(forged, contract); err == nil {
		t.Fatal("unbound extension acquired recipient guarantee")
	}
}
