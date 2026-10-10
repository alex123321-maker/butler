package goal_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestBroadMandateRejectsForeignAccountAndExhaustedAllowanceIntegration(t *testing.T) {
	f := setup(t)
	// A new owner-authored mandate has a deliberately smaller immutable budget;
	// changing the earlier grant's limits is forbidden rather than replenished.
	m := f.m
	m.ID = ""
	m.Revision = 0
	m.Budget.ExternalCalls = 1
	r, err := f.manager.PutMandate(f.actor("single-recovery-allowance"), goal.MandateRequest{Mandate: m})
	if err != nil {
		t.Fatal(err)
	}
	f.m, err = actionauth.NewPostgresStore(f.pool).GetMandate(f.ctx, r.SubjectID)
	if err != nil {
		t.Fatal(err)
	}
	worker, _ := processPool(t, f, "limited_dispatch")
	a := newAllowedAction(t, f, worker, false)
	outside := *a.call
	outside.ToolCallId = "foreign-account"
	outside.ArgsJson = fmt.Sprintf(`{"method":"POST","url":%q}`, a.recipient.URL+"/work-account/recover")
	if _, err = a.service.PrepareWithLogicalRef(f.ctx, &outside, a.contract, "foreign-account-slot"); !errors.Is(err, actionauth.ErrOwnerAuthority) {
		t.Fatal("broad service authority escaped into foreign account", err)
	}
	if err = a.store.ClaimGrant(f.ctx, a.grant.ID); err != nil {
		t.Fatal(err)
	}
	a.dispatch(t, f.ctx, false)
	second := *a.call
	second.ToolCallId = "second-recovery"
	prepared, err := a.service.PrepareWithLogicalRef(f.ctx, &second, a.contract, "second-recovery-slot")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := a.service.Authorize(f.ctx, &second, a.contract, prepared.Binding.ActionID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	err = a.store.ClaimGrant(f.ctx, grant.ID)
	var exhausted *budget.ExhaustedError
	if !errors.As(err, &exhausted) || exhausted.Kind != "mandate" || exhausted.Dimension != "external_calls" || a.posts.Load() != 1 {
		t.Fatal("recovery exceeded shared owner allowance", err, a.posts.Load())
	}
	stored, err := a.store.GetGrant(f.ctx, grant.ID)
	if err != nil || stored.State != "authorized" {
		t.Fatal("denied budget spent dispatch", stored, err)
	}
}

func TestExpiredOrConflictingMandateDeniesPreparedDispatchBeforeSweepIntegration(t *testing.T) {
	for _, kind := range []string{"deadline", "purpose_row_deadline", "explicit_deny"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			if kind == "purpose_row_deadline" {
				expiringPurpose(t, f, time.Hour)
			}
			worker, _ := processPool(t, f, "live_policy")
			a := newAllowedAction(t, f, worker, false)
			if kind == "deadline" {
				// Simulate the clock having crossed a saved row deadline; the JSON still
				// contains its earlier future projection and cannot override PG time.
				if _, err := f.pool.Exec(f.ctx, `UPDATE action_mandates SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE mandate_id=$1`, f.m.ID); err != nil {
					t.Fatal(err)
				}
			} else if kind == "purpose_row_deadline" {
				if _, err := f.pool.Exec(f.ctx, `UPDATE owner_goals SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE kind='constraint'`); err != nil {
					t.Fatal(err)
				}
			} else {
				deny := f.m
				deny.ID = ""
				deny.Revision = 0
				deny.DenyTools = []string{"http.request"}
				if _, err := f.manager.PutMandate(f.actor("new-live-deny"), goal.MandateRequest{Mandate: deny}); err != nil {
					t.Fatal(err)
				}
			}
			if err := a.store.ClaimGrant(f.ctx, a.grant.ID); !errors.Is(err, actionauth.ErrOwnerAuthority) {
				t.Fatal("prepared grant ignored current owner restriction", kind, err)
			}
			if a.posts.Load() != 0 {
				t.Fatal("denied authority reached recipient")
			}
			if _, err := a.service.Authorize(f.ctx, a.call, a.contract, a.grant.Binding.ActionID, "", ""); err == nil {
				t.Fatal("live reader reused expired/denied intent")
			}
		})
	}
}

func TestTrustedInitiativeStartsPrepareOnlyAndChildrenCannotWidenIntegration(t *testing.T) {
	f := setup(t)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	var key string
	if err = tx.QueryRow(f.ctx, `SELECT session_key FROM sessions WHERE session_key=$1 FOR SHARE`, f.key).Scan(&key); err != nil {
		t.Fatal(err)
	}
	a, err := f.manager.CheckAdmissionTx(f.ctx, tx, goal.AdmissionRequest{OwnerID: f.owner, GoalID: f.g.ID, MandateID: f.m.ID, Purpose: f.g.Purpose, ObservationRef: "synthetic:repeated-error", InitiativeReason: "prepare a reviewed fix"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	root, err := f.tasks.CreateTx(f.ctx, tx, task.CreateRequest{SessionKey: f.key, OwnerID: f.owner, Goal: "Prepare service fix", CompletionCriteria: []string{"owner review"}, Authority: &a, InitialExecutionPolicy: task.PolicyPrepareOnly})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(f.ctx); err != nil || root.ExecutionPolicy != task.PolicyPrepareOnly {
		t.Fatal(root, err)
	}
	child, err := f.tasks.Create(f.ctx, task.CreateRequest{SessionKey: f.key, OwnerID: f.owner, ParentTaskID: root.TaskID, Goal: "Investigate fix", CompletionCriteria: []string{"reviewed"}})
	if err != nil || child.ExecutionPolicy != task.PolicyPrepareOnly {
		t.Fatal("child widened initial restriction", child, err)
	}
	if _, err = f.tasks.Create(f.ctx, task.CreateRequest{SessionKey: f.key, OwnerID: f.owner, ParentTaskID: root.TaskID, InitialExecutionPolicy: task.PolicyNormal, Goal: "Widen child", CompletionCriteria: []string{"reviewed"}}); !errors.Is(err, task.ErrConflict) {
		t.Fatal("child overrode parent restriction", err)
	}
	id, _ := f.run(t, root, "tool_pending")
	s := actionauth.NewService(actionauth.NewPostgresStore(f.pool), liveContext{f.pool}, nil)
	call := &toolbrokerv1.ToolCall{RunId: id, ToolCallId: "cannot-commit-prepared-fix", ToolName: "http.request", AutonomyMode: 3, ArgsJson: `{"method":"POST","url":"https://example.invalid/test-service/recover"}`}
	contract := &toolbrokerv1.ToolContract{ToolName: "http.request", Status: "enabled", InputSchemaJson: `{"type":"object"}`, ContractRevision: "prepare-only-check"}
	if _, err = s.Prepare(f.ctx, call, contract); err == nil {
		t.Fatal("initial prepare-only task authorized commit")
	}
}

func TestOwnerAuthorityRollbackRefusesSavedHistoryIntegration(t *testing.T) {
	f := setup(t)
	f.create(t, "rollback-task")
	for _, file := range []string{"038_owner_purpose_policies.down.sql", "038_owner_goals_mandates.down.sql"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", file))
		if err != nil {
			t.Fatal(err)
		}
		tx, err := f.pool.Begin(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(f.ctx, string(raw))
		_ = tx.Rollback(f.ctx)
		if err == nil || !strings.Contains(err.Error(), "history exists") {
			t.Fatal("rollback discarded authority evidence", file, err)
		}
	}
	view, err := f.manager.View(f.actor("read-after-refused-rollback"))
	if err != nil || len(view.Goals) != 1 || len(view.Mandates) != 1 || len(view.Obligations) != 1 {
		t.Fatal("refused rollback changed authority", view, err)
	}
}

func TestExplicitOwnerTaskDoesNotNeedSpontaneousInitiativePermissionIntegration(t *testing.T) {
	f := setup(t)
	m := f.m
	m.AllowInitiative = false
	r, err := f.manager.PutMandate(f.actor("disable-spontaneous-work"), goal.MandateRequest{Mandate: m, ExpectedRevision: m.Revision})
	if err != nil {
		t.Fatal(err)
	}
	f.m, err = actionauth.NewPostgresStore(f.pool).GetMandate(f.ctx, r.SubjectID)
	if err != nil {
		t.Fatal(err)
	}
	manual := f.create(t, "explicit-owner-task")
	a, err := actionauth.GetTaskAssignment(f.ctx, f.pool, manual.TaskID)
	if err != nil || a.MandateRevision != f.m.Revision || a.MandateID != f.m.ID {
		t.Fatal("manual task lacked current exact authority", a, err)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.manager.CheckAdmissionTx(f.ctx, tx, goal.AdmissionRequest{OwnerID: f.owner, GoalID: f.g.ID, MandateID: f.m.ID, Purpose: f.g.Purpose, ObservationRef: "synthetic:scheduled-health", InitiativeReason: "spontaneous check"}, time.Now())
	_ = tx.Rollback(f.ctx)
	if !errors.Is(err, actionauth.ErrOwnerAuthority) {
		t.Fatal("scheduler bypassed initiative prohibition", err)
	}
	req := goal.TaskRequest{SessionKey: f.key, GoalID: f.g.ID, MandateID: f.m.ID, Purpose: f.g.Purpose, Goal: "Spoof owner task", CompletionCriteria: []string{"reviewed"}, ObservationRef: "synthetic:input", InitiativeReason: "manual request"}
	if _, err = f.manager.CreateOwnerTask(f.ctx, req); !errors.Is(err, goal.ErrUnauthorized) {
		t.Fatal("manual distinction lacked owner proof", err)
	}
	m = f.m
	m.Status = "revoked"
	if _, err = f.manager.PutMandate(f.actor("revoke-manual-scope"), goal.MandateRequest{Mandate: m, ExpectedRevision: m.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.manager.CreateOwnerTask(f.actor("manual-after-revocation"), req); !errors.Is(err, actionauth.ErrOwnerAuthority) {
		t.Fatal("explicit owner task bypassed revoked scope", err)
	}
}

func TestExactReviewedExtensionAdmissionUsesCurrentOwnerAuthorityIntegration(t *testing.T) {
	f := setup(t)
	reviewed := actionauth.ExtensionPermission{Source: "operator-reviewed:synthetic", ExtensionID: "synthetic-diagnostics", ManifestHash: strings.Repeat("a", 64), Tools: []string{"http.request"}}
	m := f.m
	m.Extensions = []actionauth.ExtensionPermission{reviewed}
	r, err := f.manager.PutMandate(f.actor("allow-reviewed-extension"), goal.MandateRequest{Mandate: m, ExpectedRevision: m.Revision})
	if err != nil {
		t.Fatal(err)
	}
	f.m, err = actionauth.NewPostgresStore(f.pool).GetMandate(f.ctx, r.SubjectID)
	if err != nil {
		t.Fatal(err)
	}
	record := f.create(t, "extension-task")
	check := func(p actionauth.ExtensionPermission) error {
		tx, err := f.pool.Begin(f.ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(f.ctx)
		return f.manager.CheckExtensionAttachmentTx(f.ctx, tx, record.TaskID, f.owner, p, time.Now())
	}
	if err = check(reviewed); err != nil {
		t.Fatal("selected reviewed extension required per-step approval", err)
	}
	a, err := actionauth.GetTaskAssignment(f.ctx, f.pool, record.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	a.TaskID = ""
	prepared, err := f.tasks.Create(f.ctx, task.CreateRequest{SessionKey: f.key, OwnerID: f.owner, Goal: "Prepare extension proposal", CompletionCriteria: []string{"owner review"}, Authority: &a, InitialExecutionPolicy: task.PolicyPrepareOnly})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = f.manager.CheckExtensionAttachmentTx(f.ctx, tx, prepared.TaskID, f.owner, reviewed, time.Now())
	_ = tx.Rollback(f.ctx)
	if err == nil {
		t.Fatal("prepare-only task enabled a reviewed capability")
	}
	changed := reviewed
	changed.ManifestHash = strings.Repeat("b", 64)
	if err = check(changed); !errors.Is(err, actionauth.ErrOwnerAuthority) {
		t.Fatal("unreviewed extension bytes authorized", err)
	}
	changed = reviewed
	changed.Tools = []string{"http.request", "browser.action"}
	if err = check(changed); !errors.Is(err, actionauth.ErrOwnerAuthority) {
		t.Fatal("extension expanded capabilities", err)
	}
	deny := f.m
	deny.ID = ""
	deny.Revision = 0
	deny.DenyTools = []string{"http.request"}
	if _, err = f.manager.PutMandate(f.actor("conflicting-extension-deny"), goal.MandateRequest{Mandate: deny}); err != nil {
		t.Fatal(err)
	}
	if err = check(reviewed); !errors.Is(err, actionauth.ErrOwnerAuthority) {
		t.Fatal("overlapping explicit deny ignored", err)
	}
	revoked := f.m
	revoked.Status = "revoked"
	if _, err = f.manager.PutMandate(f.actor("revoke-extension-authority"), goal.MandateRequest{Mandate: revoked, ExpectedRevision: revoked.Revision}); err != nil {
		t.Fatal(err)
	}
	if err = check(reviewed); !errors.Is(err, actionauth.ErrOwnerAuthority) {
		t.Fatal("revoked extension authority remained valid", err)
	}
}
