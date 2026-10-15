package catalog

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/security/content"
)

func TestLifecycleAttackCorpusSelectiveForgetIntegration(t *testing.T) {
	for _, attack := range memorytest.LifecycleAttacks(t) {
		t.Run(attack.ID, func(t *testing.T) {
			s, owner := fixture(t)
			live := seedRun(t, s)
			_, e := s.pool.Exec(owner, `INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('current','web:owner','owner','Read only: do not change the service')`)
			if e != nil {
				t.Fatal(e)
			}
			// Preserve a real bounded current permission as well as denying new
			// permissions. Empty authority tables alone would miss over-deletion.
			mandate := actionauth.Mandate{ID: "read-only", OwnerID: "owner", TaskID: "current", TaskRevision: 1, ConstraintsRevision: 1, Revision: 1, Tools: []string{"http.request"}, Resources: []string{"https://example.com"}, Effects: []actionauth.Effect{actionauth.EffectRead}, ExpiresAt: time.Now().Add(time.Hour)}
			if e = actionauth.ValidateMandate(mandate, time.Now()); e != nil {
				t.Fatal(e)
			}
			mandateJSON, e := json.Marshal(mandate)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.pool.Exec(owner, `INSERT INTO action_mandates(mandate_id,owner_id,task_id,revision,mandate_data,expires_at) VALUES('read-only','owner','current',1,$1,$2)`, mandateJSON, mandate.ExpiresAt); e != nil {
				t.Fatal(e)
			}
			valid := put(t, s, owner, "valid", "Independent valid preference: Russian")
			validClaim := propose(t, s, owner, valid, `"ru"`)
			if _, e = s.Decide(owner, Decision{ClaimID: validClaim.ID}); e != nil {
				t.Fatal(e)
			}
			src, e := s.Put(owner, PutSource{ExternalKey: attack.ID, Kind: "document", ScopeType: "user", ScopeID: "owner", Body: attack.Body, Origin: content.Origin{Kind: content.Kind(attack.OriginKind), SourceRefs: []string{attack.ID}, Sensitivity: content.Private}})
			if e != nil {
				t.Fatal(e)
			}
			parent := src
			var descendants []Source
			// Retention across two derivations must keep authoritative PG edges;
			// textual claims of verification or policy do not grant authority.
			for _, stage := range []string{"retained-summary", "consolidated-lesson"} {
				child, e := s.Put(owner, PutSource{ExternalKey: attack.ID + ":" + stage, Kind: "model_candidate", ScopeType: "user", ScopeID: "owner", Body: attack.Body, Parents: []SourceRef{ref(parent)}, Origin: content.Derive(content.OperatorInstruction, parent.Origin)})
				if e != nil {
					t.Fatal(e)
				}
				if child.Origin.IsInstruction() || child.ScopeID != "owner" {
					t.Fatal("derived data gained authority/scope")
				}
				parent = child
				descendants = append(descendants, child)
			}
			claim := propose(t, s, owner, parent, `"malicious recommendation"`)
			if claim.Status != "candidate" || claim.AcceptedBy != "" {
				t.Fatal("untrusted extraction was accepted")
			}
			if _, e = s.Decide(live, Decision{ClaimID: claim.ID, ExpectedActiveID: validClaim.ID}); !errors.Is(e, ErrDenied) {
				t.Fatal("run promoted DATA without owner proof", e)
			}
			if _, e = s.PublishFragments(owner, ref(parent), []Fragment{{Key: "attack", Title: attack.ID, Text: attack.Body, Locator: json.RawMessage(`{}`)}}); e != nil {
				t.Fatal(e)
			}
			var chunk int64
			if e = s.pool.QueryRow(owner, `SELECT object_id FROM memory_object_sources WHERE source_id=$1 AND memory_type='chunk'`, parent.ID).Scan(&chunk); e != nil {
				t.Fatal(e)
			}
			if _, e = s.Citation(live, chunk); e != nil {
				t.Fatal(e)
			}
			// Forget only the poisoned derivative, preserving its original and
			// unrelated accepted preference. The descendant is also removed.
			if _, e = s.Delete(owner, descendants[0].ID, "delete", 1); e != nil {
				t.Fatal(e)
			}
			restarted := New(s.pool, s.manifest)
			for _, child := range descendants {
				if _, e = restarted.Get(owner, child.ID); !errors.Is(e, ErrDenied) {
					t.Fatal("forgotten derivative readable after restart", e)
				}
			}
			if _, e = restarted.Citation(owner, chunk); e == nil {
				t.Fatal("forgotten citation readable")
			}
			if _, e = restarted.Get(owner, src.ID); e != nil {
				t.Fatal("selective forget removed original", e)
			}
			if _, e = restarted.Get(owner, valid.ID); e != nil {
				t.Fatal("selective forget removed unrelated memory", e)
			}
			var status, goal string
			var mandates, grants, chunks int
			if e = s.pool.QueryRow(owner, `SELECT status FROM memory_claims WHERE claim_id=$1`, validClaim.ID).Scan(&status); e != nil || status != "confirmed" {
				t.Fatal("unrelated preference changed", status, e)
			}
			if e = s.pool.QueryRow(owner, `SELECT goal,(SELECT count(*) FROM action_mandates),(SELECT count(*) FROM action_authorizations),(SELECT count(*) FROM memory_chunks) FROM tasks WHERE task_id='current'`).Scan(&goal, &mandates, &grants, &chunks); e != nil || goal != "Read only: do not change the service" || mandates != 1 || grants != 0 || chunks != 0 {
				t.Fatal("memory changed current authority", goal, mandates, grants, chunks, e)
			}
			var samePermission bool
			if e = s.pool.QueryRow(owner, `SELECT mandate_data=$1::jsonb AND revision=1 AND status='active' AND revoked_at IS NULL FROM action_mandates WHERE mandate_id='read-only'`, mandateJSON).Scan(&samePermission); e != nil || !samePermission {
				t.Fatal("selective forget changed current read permission", e)
			}
			// The untouched original legitimately retains the marker; deleted
			// derivative revision bodies and claim values must be physically empty.
			var remains int
			if e = s.pool.QueryRow(owner, `SELECT (SELECT count(*) FROM memory_source_revisions WHERE source_id=ANY($1) AND body<>'')+(SELECT count(*) FROM memory_claims WHERE source_id=ANY($1) AND (summary<>'' OR value_json<>'{}'::jsonb))`, []string{descendants[0].ID, descendants[1].ID}).Scan(&remains); e != nil || remains != 0 {
				t.Fatal("forgotten derived content retained", remains, e)
			}
		})
	}
}

func TestLifecycleCorrectionFencesEntireDerivativeChainIntegration(t *testing.T) {
	s, owner := fixture(t)
	live := seedRun(t, s)
	src := put(t, s, owner, "correctable", "Outdated instruction: change the service")
	parent := src
	for _, key := range []string{"summary", "lesson"} {
		child, e := s.Put(owner, PutSource{ExternalKey: key, Kind: "model_candidate", ScopeType: "user", ScopeID: "owner", Body: src.Body, Parents: []SourceRef{ref(parent)}})
		if e != nil {
			t.Fatal(e)
		}
		parent = child
	}
	if _, e := s.Get(live, parent.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Correct(owner, Correction{SourceID: src.ID, ExpectedRevision: 1, Body: "Current instruction: read only"}); e != nil {
		t.Fatal(e)
	}
	restarted := New(s.pool, s.manifest)
	if _, e := restarted.Get(owner, parent.ID); !errors.Is(e, ErrStale) {
		t.Fatal("old derivative survived correction", e)
	}
	if e := restarted.CheckRunSources(live, "run"); e == nil {
		t.Fatal("prepared old context released after correction")
	}
	if _, e := restarted.PublishFragments(owner, ref(parent), []Fragment{{Key: "late", Text: parent.Body, Locator: json.RawMessage(`{}`)}}); e == nil {
		t.Fatal("late old derivative republished")
	}
}
