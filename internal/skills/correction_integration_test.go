package skills

import (
	"github.com/butler/butler/internal/actionauth"
	"testing"
	"time"
)

func TestExactDescriptiveCorrectionGrantIntegration(t *testing.T) {
	s, m, _, env, owner := skillFixture(t)
	original := activeFixture(t, s, m, owner)
	expiry := time.Now().Add(time.Hour)
	goal := actionauth.GoalPolicy{ID: "goal", OwnerID: "owner", Kind: "objective", Status: "active", Revision: 1, Purpose: "skill_correction", Title: "Improve descriptive wording", ExpiresAt: expiry}
	mandate := actionauth.Mandate{ID: "mandate", OwnerID: "owner", ScopeKind: "owner_goal", GoalID: "goal", GoalRevision: 1, Revision: 1, Status: "active", Purpose: goal.Purpose, ExpiresAt: expiry}
	if _, err := s.pool.Exec(owner, `INSERT INTO owner_goals(goal_id,owner_id,kind,status,revision,expires_at,goal_data,source_data) VALUES('goal','owner','objective','active',1,$1,$2::jsonb,'{}')`, expiry, encoded(goal)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(owner, `INSERT INTO owner_purpose_policies(owner_id,purpose,revision) VALUES('owner','skill_correction',1)`); err != nil {
		t.Fatal(err)
	}
	// This package fixture seeds the owner service's typed persisted snapshot;
	// production broad mandates are created only by the authenticated goal API.
	if _, err := s.pool.Exec(owner, `INSERT INTO action_mandates(mandate_id,owner_id,task_id,revision,mandate_data,expires_at,scope_kind,goal_id,goal_revision,status) VALUES('mandate','owner','',1,$1::jsonb,$2,'owner_goal','goal',1,'active')`, encoded(mandate), expiry); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(owner, `INSERT INTO task_authority_bindings(task_id,owner_id,goal_id,goal_revision,mandate_id,mandate_revision,purpose,initiative_reason,observation_ref,policy_revision) VALUES('task','owner','goal',1,'mandate',1,'skill_correction','owner requested wording correction','owner:instruction',1)`); err != nil {
		t.Fatal(err)
	}
	env.context.Authority = &actionauth.Assignment{TaskID: "task", OwnerID: "owner", GoalID: "goal", GoalRevision: 1, MandateID: "mandate", MandateRevision: 1, Purpose: goal.Purpose, PolicyRevision: 1, InitiativeReason: "owner requested wording correction", ObservationRef: "owner:instruction"}
	grant := CorrectionGrant{ID: "exact-correction", SkillID: original.ID, ParentDigest: original.Digest, Class: "descriptive_correction_v1", MandateID: "mandate", MandateRevision: 1, Purpose: goal.Purpose, ExpiresAt: expiry}
	if err := s.GrantCorrection(owner, grant); err != nil {
		t.Fatal(err)
	}
	corrected := m
	corrected.Version = "1.0.1"
	corrected.Description = "Read one fixed project check only when applicable"
	promoted, err := s.Propose(owner, "run", packageOf(t, corrected))
	if err != nil || !promoted.Active || promoted.Generation != original.Generation+1 {
		t.Fatal("exact mandated wording correction not promoted", err)
	}
	widened := corrected
	widened.Version = "1.0.2"
	widened.Inputs = []string{"different required input"}
	grant.ID = "new-parent-grant"
	grant.ParentDigest = promoted.Digest
	if err = s.GrantCorrection(owner, grant); err != nil {
		t.Fatal(err)
	}
	candidate, err := s.Propose(owner, "run", packageOf(t, widened))
	if err != nil || candidate.Active || candidate.Status != "tested" {
		t.Fatal("shape widening changed active procedure", err)
	}
	if err = s.RevokeCorrection(owner, grant.ID); err != nil {
		t.Fatal(err)
	}
	next := corrected
	next.Version = "1.0.3"
	next.Description = "Description under revoked correction grant"
	candidate, err = s.Propose(owner, "run", packageOf(t, next))
	if err != nil || candidate.Active {
		t.Fatal("revoked grant promoted candidate", err)
	}
	grant.ID = "expired-correction"
	grant.ExpiresAt = time.Now().Add(time.Second)
	if err = s.GrantCorrection(owner, grant); err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(owner, `UPDATE skill_correction_grants SET expires_at=clock_timestamp()-interval '1 second' WHERE grant_id=$1`, grant.ID); err != nil {
		t.Fatal(err)
	}
	next.Version = "1.0.4"
	candidate, err = s.Propose(owner, "run", packageOf(t, next))
	if err != nil || candidate.Active {
		t.Fatal("expired grant promoted candidate", err)
	}
	if _, err = s.pool.Exec(owner, `UPDATE action_mandates SET status='revoked',revoked_at=clock_timestamp() WHERE mandate_id='mandate'`); err != nil {
		t.Fatal(err)
	}
	next.Version = "1.0.5"
	if _, err = s.Propose(owner, "run", packageOf(t, next)); err == nil {
		t.Fatal("revoked current mandate allowed publication")
	}
}
