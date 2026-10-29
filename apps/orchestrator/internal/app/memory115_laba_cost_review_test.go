package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// Post-exit accounting only. This invokes the original closed-window validators;
// it creates no native history recipe, provider, database or admission authority.
func TestLaba115ClosedCostReview(t *testing.T) {
	profile := os.Getenv("BUTLER_ISSUE115_LABA_COST_REVIEW")
	if profile != "closed-proxy-v2" && profile != "closed-abc-v3" && profile != "closed-abc-v4" && profile != "closed-abc-v5" && profile != "closed-abc-v6" && profile != "guided-full-v1" && profile != "guided-full-v3" && profile != "guided-full-v5" && profile != "guided-full-v6" {
		t.Skip("explicit immutable post-exit server exports required")
	}
	planSHA := "d366ab3b8c10518cf4bfda8ef627e2ea50101847a159a9afbf8ed1a53433d9a7"
	historySHA := "a3a045bd2220eda99e92d85b072b59286aff3aa452e8b6407c02df4a2890d721"
	if profile == "closed-abc-v3" {
		planSHA = "2b17c33e79f9c2b72b71d5fc3363d40c85f67b4aae89c158c9b0a0fecf54eae3"
		historySHA = "b04aec6e8e7e8140532d97d004ded3530f458cfc4ec3323874d52dbe7fa3c595"
	}
	if profile == "closed-abc-v4" {
		planSHA = "18d4ec76d5cbbbf241d7151eb542e7a48d7125c7ea4452ac8c67e0d148d729a2"
		historySHA = "720ac0997a0bfef54b2a15817170abd5f13a8abaa683209975a63fffaf7bd389"
	}
	if profile == "closed-abc-v5" {
		planSHA = "e9a22fdb7f4125e84eef442520f41cae42ce291234343f2eeccd1b129a127497"
		historySHA = "e59ffd65c49ee8401c14059dd909bbafeb8865994c93b7ea8222002b95edfdf9"
	}
	if profile == "closed-abc-v6" {
		planSHA = "8ce5c9aeb47a5837f18a572d45b19e7714be2871122c32ffc8acf2652f65a511"
		historySHA = "a66abae7068998bdde6df13b843a2976b021036a516cdc2733b1c83ba106c490"
	}
	if profile == "guided-full-v1" {
		planSHA = "30acfb4f8ae5260a7e3675822247672d3dde178057d7292d54b0166200ab893c"
		historySHA = "0308c67a29068c468d332cca87de140b9a34bf5bdbd42bbfc002d2ae7735cafb"
	}
	if profile == "guided-full-v3" {
		planSHA = "0d957c32b47ac93fcc6f21826af8d54c645cf15b5347aed72dc98d653dd87ac6"
		historySHA = "5d4b0bdcfd23fa623fce1aca8ec285420d97d4c49b694dd003c9e6e1e9c4da54"
	}
	if profile == "guided-full-v5" {
		planSHA = "9743229d7eea847608f661d299147ef72f12dcd0e0e73840ac5c12be80edff54"
		historySHA = "a84f7e4d6dae31e34042644fb377f4a94bcaf6eb0d8cf62d7de2f7f59869a503"
	}
	if profile == "guided-full-v6" {
		planSHA = "02730ca74dfcb6aad0af512f93d0a68c8575cc2245ad3d7b9dff08e97b0ad9a6"
		historySHA = "a31151b163ed1747519dc83215d9e057b1a142c0223bc14eaf7a0c40fa1b72f9"
	}
	root := "/review"
	read := func(name string, value any) {
		t.Helper()
		path := filepath.Join(root, name)
		hash, err := experienceNativeFileSHA(path)
		if err != nil {
			t.Fatal("review file unavailable", name)
		}
		raw, err := closedExperienceRead(root, name, hash, 64*1024*1024)
		if err != nil || json.Unmarshal(raw, value) != nil {
			t.Fatal("bounded regular review artifact required", name)
		}
	}
	var plan experienceNativeSeal
	read("preseal.json", &plan)
	if hash, err := experienceNativeFileSHA(filepath.Join(root, "preseal.json")); err != nil || hash != planSHA {
		t.Fatal("original fixed preseal differs")
	}
	history, err := loadExperienceNativeHistory("/history/recipe.json", historySHA)
	wantScope, maximumDatabases := 33, 25
	if profile == "guided-full-v1" || profile == "guided-full-v3" || profile == "guided-full-v5" || profile == "guided-full-v6" {
		wantScope, maximumDatabases = 0, 49
		if history.GuidedFull == nil {
			t.Fatal("guided full57 history missing")
		}
	}
	if err != nil || !reflect.DeepEqual(plan.Lineage, history) || plan.ExecutionSlots != wantScope || closedExperienceFinitePlan(plan, history) != nil {
		t.Fatal("exact retained parent and finite33-of57 plan required", err)
	}
	var window struct {
		Slots          []experienceNativeSlot
		Entries        []experienceNativeEntry
		ExecutionSlots int       `json:"execution_slots"`
		Maximum        int       `json:"maximum_entries"`
		Started        time.Time `json:"started_at"`
		Finished       time.Time `json:"finished_at"`
	}
	read("actual/window-final.json", &window)
	maximum := experienceNativeMaximum
	if guided115Expanded(history) {
		maximum = guided115Maximum
	}
	if len(window.Slots) != 57 || window.Maximum != maximum || len(window.Entries) > maximum || window.Started.IsZero() || window.Finished.Before(window.Started) || window.Finished.After(plan.Deadline.Add(15*time.Minute)) {
		t.Fatal("original complete closed window required")
	}
	slots := map[string]experienceNativeSlot{}
	statuses := map[string]int{}
	for i, s := range window.Slots {
		p := plan.Schedule[i]
		if s.Sequence != p.Sequence || s.ID != p.ID || s.Kind != p.Kind || s.CaseID != p.CaseID || s.Arm != p.Arm || s.Repetition != p.Repetition || s.Maximum != p.Maximum || s.Status != "completed" && s.Status != "failed" && s.Status != "not_executed" {
			t.Fatal("original slot identity or status changed")
		}
		slots[s.ID] = s
		statuses[s.Status]++
	}
	var costs struct {
		Quotes    experienceNativeQuotedCosts
		Databases []string `json:"allocated_database_exports"`
		Captured  int      `json:"native_captured_entries"`
		Invoice   string   `json:"invoice_cost"`
	}
	read("actual/unique-quoted-costs.json", &costs)
	if costs.Captured != len(window.Entries) || costs.Invoice != "unknown" || len(costs.Databases) < 1 || len(costs.Databases) > maximumDatabases {
		t.Fatal("complete allocated cost population required")
	}
	byDatabase := map[string]map[string]json.RawMessage{}
	list := []map[string]json.RawMessage{}
	for _, name := range costs.Databases {
		if byDatabase[name] != nil || name != "source-database.json" && !closedExperienceCaseExport(name, slots) {
			t.Fatal("duplicate or foreign cost database")
		}
		var db map[string]json.RawMessage
		read("actual/"+name, &db)
		byDatabase[name] = db
		list = append(list, db)
	}
	if byDatabase["source-database.json"] == nil {
		t.Fatal("source population omitted")
	}
	quotes, err := experienceNativeUniqueQuotes(list, history)
	if err != nil || quotes != costs.Quotes {
		t.Fatal("original immutable quote union failed", err)
	}
	usage, err := closedExperienceProducts(byDatabase, window.Entries, slots)
	if err != nil || usage.ProductAttempts != quotes.ModelRows {
		t.Fatal("original product/native correlation failed", err)
	}
	var journal map[string]json.RawMessage
	read("actual/global-journal-final.json", &journal)
	if err = closedExperienceExecutionScope(plan.ExecutionSlots, window.ExecutionSlots, window.Slots, window.Slots, window.Entries, costs.Databases, journal); err != nil {
		t.Fatal("original shortened scope validation failed", err)
	}
	m := closedExperienceManifest{Preseal: planSHA, Owner: plan.Owner, Previous: historySHA, Day: history.Day, Deadline: plan.Deadline}
	if err = closedExperienceJournal(journal, slots, window.Entries, byDatabase, m, history, quotes); err != nil {
		t.Fatal("original durable journal validation failed", err)
	}
	var domain closedExperienceDomain
	read("actual/domain-claim.json", &domain)
	if !domain.matches(m, history.Deadline) {
		t.Fatal("one-use domain differs")
	}
	result := map[string]any{"version": "butler115.post-exit-accounting.v1", "plan_sha256": planSHA, "history_sha256": historySHA, "new_usage": usage, "inclusive_usage": consolidationV2AddUsage(history.Usage, usage), "quotes": quotes, "mechanical_slot_statuses": statuses, "allocated_database_exports": len(byDatabase), "original_cost_and_journal_validators_passed": true, "quality_qualified": false, "new_native_history_admission": false, "invoice_cost": "unknown"}
	f, err := os.OpenFile("/review-output/cost-review.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("new accounting receipt required")
	}
	writeErr := json.NewEncoder(f).Encode(result)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		t.Fatal("accounting receipt persistence failed")
	}
	t.Logf("POST_EXIT_ACCOUNTING_PASS new_native=%d known=%d unknown=%d new_product=%d product_NULL=%d known_full_tokens=%d; quality remains unqualified", usage.NativeEntries, usage.NativeKnownUsage, usage.NativeUnknownUsage, usage.ProductAttempts, usage.ProductNULL, usage.KnownFullTokenLowerBound)
}
