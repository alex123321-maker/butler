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
)

func TestGoalExtensionPermissionMatchesReviewedPackageExactlyIntegration(t *testing.T) {
	pool := extensionDB(t)
	s, p := extensionFixture(t, pool, budget.DefaultPolicy())
	r := readyFixture(t, s, configuredFixture(t, s, p))
	ctx := context.Background()
	expiry := time.Now().Add(time.Hour)
	goal := actionauth.GoalPolicy{ID: "goal", OwnerID: "owner", Kind: "objective", Status: "active", Revision: 1, Purpose: "fixture_inspect", ExpiresAt: expiry}
	m := actionauth.Mandate{ID: "mandate", OwnerID: "owner", ScopeKind: "owner_goal", GoalID: "goal", GoalRevision: 1, Revision: 1, Status: "active", Purpose: goal.Purpose, Tools: []string{"extension.notes.read"}, ResourceScopes: []actionauth.ResourceScope{{Origin: "https://extension.example", PathPrefix: "/notes/item"}}, Effects: []actionauth.Effect{actionauth.EffectRead}, ExpiresAt: expiry, BudgetDeadline: expiry, Privacy: "cloud_allowed", Intervention: []string{"effect_unknown"}, Reporting: "exceptions", Budget: actionauth.MandateBudget{ModelTurns: 10, Tokens: 10000, ActiveMillis: 60000}}
	for _, query := range []string{`INSERT INTO owner_goals(goal_id,owner_id,kind,status,revision,expires_at,goal_data,source_data) VALUES('goal','owner','objective','active',1,$1,$2::jsonb,'{"author":"owner","channel":"fixture"}')`, `INSERT INTO action_mandates(mandate_id,owner_id,task_id,revision,expires_at,mandate_data,scope_kind,goal_id,goal_revision,status) VALUES('mandate','owner','',1,$1,$2::jsonb,'owner_goal','goal',1,'active')`} {
		data, _ := json.Marshal(goal)
		if strings.Contains(query, "action_mandates") {
			data, _ = json.Marshal(m)
		}
		if _, err := pool.Exec(ctx, query, expiry, string(data)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO owner_purpose_policies(owner_id,purpose,revision) VALUES('owner','fixture_inspect',1);INSERT INTO task_authority_bindings(task_id,owner_id,goal_id,goal_revision,mandate_id,mandate_revision,purpose,initiative_reason,observation_ref,policy_revision) VALUES('task','owner','goal',1,'mandate',1,'fixture_inspect','owner-selected fixture','fixture-observation',1)`); err != nil {
		t.Fatal(err)
	}
	c := currentContext()
	c.Authority = &actionauth.Assignment{TaskID: "task", OwnerID: "owner", GoalID: "goal", GoalRevision: 1, MandateID: "mandate", MandateRevision: 1, Purpose: goal.Purpose, PolicyRevision: 1}
	reader := &actionReader{current: c}
	inspector := &countedInspector{}
	journal := actionauth.NewPostgresStore(pool)
	journal.SetContractGuard(s)
	auth := actionauth.NewService(journal, reader, inspector)
	auth.SetCapabilities(s)
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "goal-read", ToolName: "extension.notes.read", ArgsJson: `{"url":"https://extension.example/notes/item/one"}`, AutonomyMode: commonv1.AutonomyMode(3)}
	contract, err := s.GetContract(ctx, call.ToolName)
	if err != nil {
		t.Fatal(err)
	}
	permission := actionauth.ExtensionPermission{Source: p.Manifest.Source.URI, ExtensionID: r.ID, ManifestHash: p.Digest, Tools: []string{call.ToolName}}
	for _, kind := range []string{"missing", "wrong-source", "wrong-manifest", "wrong-tool", "exact"} {
		candidate := m
		candidate.Extensions = []actionauth.ExtensionPermission{permission}
		switch kind {
		case "missing":
			candidate.Extensions = nil
		case "wrong-source":
			candidate.Extensions[0].Source = "https://foreign.invalid/package"
		case "wrong-manifest":
			candidate.Extensions[0].ManifestHash = strings.Repeat("c", 64)
		case "wrong-tool":
			candidate.Extensions[0].Tools = []string{"extension.notes.write"}
		}
		data, _ := json.Marshal(candidate)
		if _, err = pool.Exec(ctx, `UPDATE action_mandates SET mandate_data=$1::jsonb WHERE mandate_id='mandate'`, string(data)); err != nil {
			t.Fatal(err)
		}
		items, e := s.Catalog(ctx, c, "notes", 32)
		if kind != "exact" {
			if e != nil || len(items) != 0 {
				t.Fatal("unreviewed mandate extension reached catalog")
			}
			if _, e = auth.Prepare(ctx, call, contract); e == nil || inspector.calls != 0 {
				t.Fatal("unreviewed mandate inspected credential metadata")
			}
			continue
		}
		if e != nil || len(items) != 1 {
			t.Fatalf("exact goal attachment unavailable: %d %v", len(items), e)
		}
		g := approvedGrant(t, auth, reader, call, contract)
		if _, err = pool.Exec(ctx, `UPDATE action_mandates SET status='revoked',revoked_at=clock_timestamp() WHERE mandate_id='mandate'`); err != nil {
			t.Fatal(err)
		}
		if err = journal.ClaimGrant(ctx, g.ID); err == nil {
			t.Fatal("revoked goal extension dispatched")
		}
	}
}
