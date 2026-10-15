package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/profile"
)

// Test-only owner decisions, declared before inference. These fixtures exercise
// the real accepted Profile store; they do not claim automatic extraction.
type memorySearchProfile struct {
	SourceID string `json:"source_id"`
	Key      string `json:"key"`
}

func TestMemorySearchProfileFixturesAndProjectionIntegration(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(shadowRepoRoot(), "testdata/acceptance/ru-memory-search-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var suite acceptance.MemorySemanticSuite
	if json.Unmarshal(raw, &suite) != nil || suite.Validate() != nil {
		t.Fatal("invalid prospective comparison corpus")
	}
	c := memoryCollectorFixture(t)
	if err = configureMemorySearchComparison(c, raw, "candidate"); err != nil {
		t.Fatal(err)
	}
	var selected []acceptance.MemorySemanticCase
	for _, cs := range suite.Cases {
		if cs.ID != "search-g02" && cs.ID != "search-g03" && cs.ID != "search-g05" {
			continue
		}
		selected = append(selected, cs)
		if err = c.capture(context.Background(), cs.MemoryCase()); err != nil {
			t.Fatal(cs.ID, err)
		}
	}
	suite.Cases = selected
	inputs, err := semanticFreshInputs(suite, c.captures)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range inputs {
		model, err := acceptance.MemoryModelProjection(i)
		if err != nil || len(model) > 24000 || strings.Contains(string(model), "required_facts") || strings.Contains(string(model), "profile_fixtures") {
			t.Fatal("oversized or rubric-leaking projection", i.CaseID, i.Arm, len(model), err)
		}
	}
	for _, cap := range c.captures {
		if !strings.Contains(cap.Prompt, "418") && !strings.Contains(cap.Prompt, "предпочитаю поезд") && !strings.Contains(cap.Prompt, "13:40") {
			t.Fatal("accepted profile or document lost", cap.CaseID)
		}
	}
	var profiles, chunks int
	if err = c.pool.QueryRow(context.Background(), `SELECT count(*) FROM memory_profile`).Scan(&profiles); err != nil || profiles != 73 {
		t.Fatal("owner profile fixtures missing", profiles, err)
	}
	if err = c.pool.QueryRow(context.Background(), `SELECT count(*) FROM memory_chunks`).Scan(&chunks); err != nil || chunks != 3 {
		t.Fatal("profile incorrectly entered document index", chunks, err)
	}
	if err = c.exportLedger(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(c.private, "ledger.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ledger map[string]json.RawMessage
	if json.Unmarshal(b, &ledger) != nil || len(ledger) != 23 || ledger["memory_profile"] == nil {
		t.Fatal("profile lifecycle not retained")
	}
	// Applicability cannot refill a quota using suppressed, expired, future or
	// source-stale profile data. The independent scope regression covers foreign
	// project grants; these mutations cover the effective-state adapter.
	var cs acceptance.MemoryCase
	for _, candidate := range selected {
		if candidate.ID == "search-g03" {
			cs = candidate.MemoryCase()
		}
	}
	owner, actor := c.owner(cs)
	live, _, err := c.ownedRun(context.Background(), cs, "profile-current-test")
	if err != nil {
		t.Fatal(err)
	}
	adapter := profileStoreAdapter{store: profile.NewStore(c.pool), sources: c.catalog}
	for _, mutation := range []string{"suppressed=TRUE", "expires_at=clock_timestamp()-INTERVAL '1 second'", "effective_from=clock_timestamp()+INTERVAL '1 hour'"} {
		if _, err = c.pool.Exec(actor, `UPDATE memory_profile SET suppressed=FALSE,expires_at=NULL,effective_from=NULL WHERE scope_id=$1 AND key='zz_iney_travel'`, owner); err != nil {
			t.Fatal(err)
		}
		if _, err = c.pool.Exec(actor, `UPDATE memory_profile SET `+mutation+` WHERE scope_id=$1 AND key='zz_iney_travel'`, owner); err != nil {
			t.Fatal(err)
		}
		entries, e := adapter.GetByScope(live, "user", owner)
		if e != nil {
			t.Fatal(e)
		}
		for _, entry := range entries {
			if entry.ProfileKey() == "zz_iney_travel" {
				t.Fatal("withdrawn profile returned", mutation)
			}
		}
	}
	if _, err = c.pool.Exec(actor, `UPDATE memory_profile SET suppressed=FALSE,expires_at=NULL,effective_from=NULL WHERE scope_id=$1 AND key='zz_iney_travel'`, owner); err != nil {
		t.Fatal(err)
	}
	ref := c.sourceRefs[acceptance.MemoryOpaqueCaseID(cs)]["g03p26"]
	if _, err = c.catalog.Correct(actor, catalog.Correction{SourceID: ref.ID, ExpectedRevision: ref.Revision, Body: "Хозяин изменил предпочтение; старое больше не действует."}); err != nil {
		t.Fatal(err)
	}
	entries, e := adapter.GetByScope(live, "user", owner)
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if entry.ProfileKey() == "zz_iney_travel" {
			t.Fatal("source-stale profile returned")
		}
	}
}

func (c *memoryEvalCollector) searchProfileOriginal(caseID, sourceID string) bool {
	for _, p := range c.profileFixtures[caseID] {
		if p.SourceID == sourceID {
			return true
		}
	}
	return false
}

func configureMemorySearchComparison(c *memoryEvalCollector, corpus []byte, variant string) error {
	if variant == "" {
		return nil
	}
	var extra struct {
		ProfileFixtures map[string][]memorySearchProfile `json:"profile_fixtures"`
	}
	if err := json.Unmarshal(corpus, &extra); err != nil {
		return err
	}
	if variant != "baseline" && variant != "candidate" {
		return fmt.Errorf("unknown comparison variant")
	}
	c.searchVariant = variant
	c.profileFixtures = extra.ProfileFixtures
	return nil
}

func (c *memoryEvalCollector) seedSearchProfiles(ctx context.Context, cse acceptance.MemoryCase) error {
	fixtures := c.profileFixtures[cse.ID]
	if len(fixtures) == 0 {
		return nil
	}
	_, owner := c.owner(cse)
	id := acceptance.MemoryOpaqueCaseID(cse)
	begin := time.Now()
	var stored int64
	for _, f := range fixtures {
		ref, ok := c.sourceRefs[id][f.SourceID]
		if !ok || f.Key == "" {
			return fmt.Errorf("profile fixture original missing")
		}
		body := ""
		for _, h := range cse.History {
			if h.ID == f.SourceID {
				body = h.Body
			}
		}
		raw, _ := json.Marshal(body)
		claim, err := c.catalog.Propose(owner, catalog.ClaimRequest{SourceID: ref.ID, SourceRevision: ref.Revision, Key: f.Key, Value: raw, Summary: body, Confidence: 1})
		if err != nil {
			return err
		}
		if _, err = c.catalog.Decide(owner, catalog.Decision{ClaimID: claim.ID}); err != nil {
			return err
		}
		stored += int64(len(raw))
	}
	zero := int64(0)
	elapsed := time.Since(begin).Milliseconds()
	return c.recordExpense(ctx, acceptance.MemoryExpense{ID: id + "/profile-owner-fixtures", CaseID: id, Arm: "retrieval", Stage: "write", Status: "measured", Outcome: "success", EvidenceSHA256: acceptance.MemoryDigest(fixtures), Calls: int64(2 * len(fixtures)), Milliseconds: &elapsed, StorageBytes: &stored, InputTokens: &zero, OutputTokens: &zero, TotalTokens: &zero, Microunits: &zero})
}
