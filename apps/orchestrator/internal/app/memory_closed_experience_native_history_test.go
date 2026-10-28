package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/transport"
)

// The legacy entry retains its lab-main handback. Shared validation independently
// recomputes immutable costs; the explicit local entry has a separate handback.
// Neither entry grants provider authority or quality acceptance.
type closedExperienceManifest struct {
	Format             string                                          `json:"format"`
	Owner              string                                          `json:"owner"`
	Day                string                                          `json:"day"`
	Deadline           time.Time                                       `json:"deadline"`
	Preseal            string                                          `json:"preseal_sha256"`
	Previous           string                                          `json:"previous_history_sha256"`
	Denominator        int                                             `json:"denominator"`
	Maximum            int                                             `json:"maximum_new_entries"`
	Complete           bool                                            `json:"cost_population_complete"`
	Carry              budget.Units                                    `json:"inclusive_carry"`
	Usage              struct{ New, Inclusive temporalV5CurrentUsage } `json:"usage"`
	References         []consolidationHistoryRef                       `json:"references"`
	Qualified          *bool                                           `json:"release_qualified"`
	Review             *bool                                           `json:"independent_quality_review_required"`
	Invoice            string                                          `json:"invoice_cost"`
	DiscoveryInference *bool                                           `json:"control_plane_handback_is_inference"`
}

type closedExperienceDomain struct {
	Window   string `json:"window_id"`
	Hash     string `json:"window_hash"`
	Schema   string `json:"schema_name"`
	Owner    string `json:"owner_id"`
	History  string `json:"history_sha256"`
	Day      string
	Deadline time.Time
}

func (d closedExperienceDomain) matches(m closedExperienceManifest, deadline time.Time) bool {
	return d.Window == experienceNativeWindow && d.Hash == m.Preseal && d.Schema == experienceNativeSchema && d.Owner == m.Owner && d.History == m.Previous && d.Day == m.Day && d.Deadline.Equal(deadline)
}

// Every component is checked with Lstat. A lexical path check alone would let
// an intermediate symlink escape this retained, private evidence directory.
func closedExperienceRead(root, name, expected string, limit int64) ([]byte, error) {
	if name == "" || strings.ContainsAny(name, "\\:") || filepath.IsAbs(name) || filepath.ToSlash(filepath.Clean(name)) != name || name == "." || strings.HasPrefix(name, "../") || !experienceNativeSHA(expected) {
		return nil, errors.New("bounded relative immutable closure reference required")
	}
	path := root
	parts := strings.Split(name, "/")
	for i, part := range parts {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || i < len(parts)-1 && !info.IsDir() || i == len(parts)-1 && (!info.Mode().IsRegular() || info.Size() > limit) {
			return nil, errors.New("regular bounded closure artifact required")
		}
	}
	hash, err := experienceNativeFileSHA(path)
	if err != nil || hash != expected {
		return nil, errors.New("closure artifact digest differs")
	}
	if limit == 0 {
		return nil, nil
	}
	return os.ReadFile(path)
}

func loadClosedExperienceNativeHistory(path, expected string) (experienceNativeLineage, error) {
	return loadClosedExperienceNativeHistoryProfile(path, expected, false)
}

// Local handback is a separate explicit profile. The original lab consumer
// retains all of its original handback requirements and cannot fall back here.
func loadClosedExperienceNativeHistoryProfile(path, expected string, local bool) (experienceNativeLineage, error) {
	return loadClosedExperienceNativeHistoryProfiles(path, expected, local, false)
}

func loadClosedExperienceNativeHistoryProfiles(path, expected string, local, laba bool) (experienceNativeLineage, error) {
	var out experienceNativeLineage
	root := filepath.Dir(path)
	raw, err := closedExperienceRead(root, filepath.Base(path), expected, 4*1024*1024)
	var m closedExperienceManifest
	var labaProfile laba115ClosureProfile
	format := "butler.experience.closed-owner-leaf.v1"
	if local {
		format = local115NativeClosedFormat
	}
	if laba {
		labaProfile, err = laba115Closure(expected)
		if err != nil {
			return out, err
		}
		format = labaProfile.Format
	}
	if err != nil || json.Unmarshal(raw, &m) != nil || m.Format != format || m.Owner != experienceNativeOwner || m.Denominator != 57 || m.Maximum != 640 || !m.Complete || m.Qualified == nil || *m.Qualified || m.Review == nil || !*m.Review || m.DiscoveryInference == nil || *m.DiscoveryInference || m.Invoice != "unknown" || !experienceNativeSHA(m.Preseal) || !experienceNativeSHA(m.Previous) {
		return out, errors.New("complete unqualified closed experience leaf required")
	}
	if local && (m.Preseal != local115NativeClosedPreseal || m.Previous != local115NativeClosedPrevious) {
		return out, errors.New("exact original failed local cohort and predecessor required")
	}
	if laba && (m.Preseal != labaProfile.Plan || m.Previous != labaProfile.Previous) {
		return out, errors.New("exact failed server cohort and predecessor required")
	}
	refs := map[string]string{}
	for _, ref := range m.References {
		if refs[ref.Path] != "" {
			return out, errors.New("duplicate closed reference")
		}
		if _, err = closedExperienceRead(root, ref.Path, ref.SHA256, 64*1024*1024); err != nil {
			return out, err
		}
		refs[ref.Path] = ref.SHA256
	}
	read := func(name string, value any) error {
		raw, err := closedExperienceRead(root, name, refs[name], 64*1024*1024)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, value)
	}
	if refs["plan/preseal.json"] != m.Preseal || refs["history/history.json"] != m.Previous {
		return out, errors.New("closure preseal or recursive parent differs")
	}
	if local && (refs["sealed-local-raw-inventory.json"] != local115NativeClosedInventory || refs["sealed-local-raw-evidence.zip"] != local115NativeClosedArchive || refs["closed-handback.json"] != local115NativeClosedHandback) {
		return out, errors.New("original local raw seal and separate local handback required")
	}
	previous, err := loadExperienceNativeHistory(filepath.Join(root, "history", "history.json"), m.Previous)
	if err != nil {
		return out, err
	}
	var plan experienceNativeSeal
	var schedule struct {
		Format  string
		Owner   string `json:"owner_id"`
		Maximum int    `json:"maximum_entries"`
		Slots   []experienceNativeSlot
	}
	if read("plan/preseal.json", &plan) != nil || read("plan/schedule.json", &schedule) != nil || plan.Format != "butler.experience.native-preseal.v1" || schedule.Format != "experience90-window-v1" || schedule.Owner != m.Owner || schedule.Maximum != 640 || plan.Owner != m.Owner || !reflect.DeepEqual(plan.Lineage, previous) || !plan.Deadline.Equal(m.Deadline) || m.Day != previous.Day || m.Deadline.After(previous.Deadline) || !reflect.DeepEqual(schedule.Slots, plan.Schedule) {
		return out, errors.New("closed cohort differs from exact original preseal and parent")
	}
	corpus, err := experienceNativeCorpusFiles()
	if err != nil || !reflect.DeepEqual(plan.Schedule, experienceNativeSchedule(corpus)) || plan.Originals != corpus.OriginalsSHA256 || plan.Controls != corpus.ControlsSHA256 {
		return out, errors.New("closed full57 original schedule changed")
	}
	if err = closedExperienceFinitePlan(plan, previous); err != nil {
		return out, err
	}
	var inventory struct {
		Format string
		Files  []consolidationHistoryRef
	}
	if read("private-artifacts.json", &inventory) != nil || inventory.Format != "butler.experience.private-artifacts.v1" || len(inventory.Files) == 0 || len(inventory.Files) > 8192 {
		return out, errors.New("complete bounded private inventory required")
	}
	files := map[string]string{}
	var total int64
	for _, ref := range inventory.Files {
		if !strings.HasPrefix(ref.Path, "actual/") || files[ref.Path] != "" {
			return out, errors.New("duplicate or foreign private artifact")
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(ref.Path)))
		if err != nil {
			return out, errors.New("private artifact unavailable")
		}
		total += info.Size()
		if total > 2*1024*1024*1024 {
			return out, errors.New("private evidence exceeds bounded population")
		}
		if _, err = closedExperienceRead(root, ref.Path, ref.SHA256, 512*1024*1024); err != nil {
			return out, err
		}
		files[ref.Path] = ref.SHA256
	}
	count := 0
	if err = filepath.WalkDir(filepath.Join(root, "actual"), func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || files[filepath.ToSlash(rel)] == "" || d.Type()&os.ModeSymlink != 0 {
			return errors.New("unlisted or linked private evidence")
		}
		count++
		return nil
	}); err != nil || count != len(files) {
		return out, errors.New("private inventory is incomplete")
	}
	if local && validateLocal115ClosedRawPopulation(root, files) != nil {
		return out, errors.New("local raw population differs from its original157-file seal")
	}
	if laba && validateLaba115ClosedRawPopulation(root, files, labaProfile) != nil {
		return out, errors.New("server raw population differs from its original seal")
	}
	for name, hash := range files {
		if old := refs[name]; old != "" && old != hash {
			return out, errors.New("private reference conflicts")
		}
		refs[name] = hash
	}
	var window struct {
		Slots          []experienceNativeSlot
		Entries        []experienceNativeEntry
		ExecutionSlots int       `json:"execution_slots,omitempty"`
		Maximum        int       `json:"maximum_entries"`
		Active         int       `json:"active_slot"`
		Started        time.Time `json:"started_at"`
		Finished       time.Time `json:"finished_at"`
	}
	var outcomes struct {
		Slots       []experienceNativeSlot
		Denominator int
		Present     bool `json:"original_final_window_present"`
	}
	var costs struct {
		Quotes    experienceNativeQuotedCosts
		Databases []string `json:"allocated_database_exports"`
		Captured  int      `json:"native_captured_entries"`
		Invoice   string   `json:"invoice_cost"`
	}
	var journal map[string]json.RawMessage
	if read("actual/window-final.json", &window) != nil || read("all-original-outcomes.json", &outcomes) != nil || read("actual/unique-quoted-costs.json", &costs) != nil || read("actual/global-journal-final.json", &journal) != nil || window.Maximum != 640 || window.Active < -1 || window.Active >= 57 || len(window.Slots) != 57 || len(window.Entries) > 640 || window.Started.IsZero() || window.Finished.Before(window.Started) || window.Finished.After(m.Deadline.Add(15*time.Minute)) || outcomes.Denominator != 57 || !outcomes.Present || len(outcomes.Slots) != 57 || costs.Captured != len(window.Entries) || costs.Invoice != "unknown" {
		return out, errors.New("original sealed window, costs and full denominator required")
	}
	slots := map[string]experienceNativeSlot{}
	for i, slot := range window.Slots {
		original, outcome := plan.Schedule[i], outcomes.Slots[i]
		identity := func(s experienceNativeSlot) string {
			return fmt.Sprintf("%d|%s|%s|%s|%s|%d", s.Sequence, s.ID, s.Kind, s.CaseID, s.Arm, s.Repetition)
		}
		if identity(slot) != identity(original) || identity(outcome) != identity(original) || slot.Maximum != original.Maximum || slot.Status != outcome.Status || slot.Status != "completed" && slot.Status != "failed" && slot.Status != "not_executed" || slot.Status == "not_executed" && (slot.Started != nil || slot.Finished != nil) || slot.Status != "not_executed" && (slot.Started == nil || slot.Finished == nil || slot.Finished.Before(*slot.Started)) {
			return out, errors.New("original slot identity or sealed outcome changed")
		}
		slots[slot.ID] = slot
	}
	if err = closedExperienceExecutionScope(plan.ExecutionSlots, window.ExecutionSlots, window.Slots, outcomes.Slots, window.Entries, costs.Databases, journal); err != nil {
		return out, err
	}
	databases := []map[string]json.RawMessage{}
	byDatabase := map[string]map[string]json.RawMessage{}
	for _, name := range costs.Databases {
		if byDatabase[name] != nil || name != "source-database.json" && !closedExperienceCaseExport(name, slots) {
			return out, errors.New("foreign or duplicate cost database")
		}
		var db map[string]json.RawMessage
		if read("actual/"+name, &db) != nil {
			return out, errors.New("allocated immutable cost database unavailable")
		}
		byDatabase[name] = db
		databases = append(databases, db)
	}
	if byDatabase["source-database.json"] == nil {
		return out, errors.New("source cost database omitted")
	}
	quotes, err := experienceNativeUniqueQuotes(databases, previous)
	if err != nil || quotes != costs.Quotes || quotes.Inclusive != m.Carry {
		return out, errors.New("unique immutable quote union differs from closed carry")
	}
	usage, err := closedExperienceProducts(byDatabase, window.Entries, slots)
	if err != nil || usage.ProductAttempts != quotes.ModelRows || usage != m.Usage.New || consolidationV2AddUsage(previous.Usage, usage) != m.Usage.Inclusive || m.Carry.ModelTurns != int64(m.Usage.Inclusive.ProductAttempts) {
		return out, errors.New("independent native/product usage differs from closed totals")
	}
	if err = closedExperienceJournal(journal, slots, window.Entries, byDatabase, m, previous, quotes); err != nil {
		return out, err
	}
	var domain closedExperienceDomain
	if read("actual/domain-claim.json", &domain) != nil || !domain.matches(m, previous.Deadline) {
		return out, errors.New("one-use domain receipt differs")
	}
	if laba {
		var handback laba115ClosedHandback
		var exit laba115ClosedProcessExit
		expectedExit := 1
		if expected == guided115Closed503SHA {
			expectedExit = 0
		}
		if read("closed-handback.json", &handback) != nil || handback.validatePlanExit(labaProfile.Plan, expectedExit) != nil || read("process-exit.json", &exit) != nil || exit.validateExit(expectedExit) != nil {
			return out, errors.New("closed server process and exact separate opaque handback required")
		}
	} else if local {
		var handback local115ClosedHandback
		if read("closed-handback.json", &handback) != nil || handback.validate() != nil {
			return out, errors.New("closed collector and exact separate local opaque handback required")
		}
	} else {
		var handback closedExperienceHandback
		if read("closed-handback.json", &handback) != nil || handback.validate() != nil {
			return out, errors.New("closed collector and actual opaque main handback required")
		}
	}
	runner := "runner.jsonl"
	if local || laba {
		runner = "runner.log"
	}
	if refs[runner] == "" {
		return out, errors.New("original runner evidence omitted")
	}
	out = experienceNativeLineage{Hash: expected, Day: m.Day, Deadline: m.Deadline, Carry: m.Carry, Usage: m.Usage.Inclusive}
	return out, validateExperienceNativeLineage(out)
}

// The owner's shortened route preserves the original57 denominator. Only the
// independently predeclared first33 slots may execute; omitted repetition2 is
// never reported as completed or silently removed from historical accounting.
func closedExperienceExecutionScope(planned, recorded int, slots, outcomes []experienceNativeSlot, entries []experienceNativeEntry, databases []string, journal map[string]json.RawMessage) error {
	limit := func(n int) int {
		if n == 0 {
			return 57
		}
		if n == 33 {
			return 33
		}
		return -1
	}
	n := limit(planned)
	if n < 0 || n != limit(recorded) || len(slots) != 57 || len(outcomes) != 57 {
		return errors.New("exact presealed full or shortened execution scope required")
	}
	if n == 57 {
		return nil
	}
	allowed, exports := map[string]bool{}, map[string]bool{"source-database.json": true}
	for i, slot := range slots {
		if i < n {
			allowed[slot.ID] = true
			exports[fmt.Sprintf("case-%03d-database.json", slot.Sequence)] = true
			continue
		}
		for _, omitted := range []experienceNativeSlot{slot, outcomes[i]} {
			if omitted.Status != "not_executed" || omitted.Started != nil || omitted.Finished != nil || omitted.Blocker != "predeclared_short_smoke_omission" {
				return errors.New("short smoke must retain every intentional omitted original")
			}
		}
	}
	for _, entry := range entries {
		if !allowed[entry.Slot] {
			return errors.New("native entry exceeds shortened execution scope")
		}
	}
	for _, name := range databases {
		if !exports[name] {
			return errors.New("database exceeds shortened execution scope")
		}
	}
	for _, name := range []string{"experience_native_entries", "experience_native_case_databases"} {
		var rows []struct {
			Slot string `json:"slot_id"`
		}
		if json.Unmarshal(journal[name], &rows) != nil || rows == nil {
			return errors.New("shortened global scope witness unavailable")
		}
		for _, row := range rows {
			if !allowed[row.Slot] {
				return errors.New("global entry or clone exceeds shortened execution scope")
			}
		}
	}
	return nil
}

func closedExperienceCaseExport(name string, slots map[string]experienceNativeSlot) bool {
	for _, slot := range slots {
		if slot.Kind == "control" && name == fmt.Sprintf("case-%03d-database.json", slot.Sequence) {
			return true
		}
	}
	return false
}

type closedExperienceProduct struct {
	Table    string
	Raw      []byte
	Row      map[string]json.RawMessage
	Database map[string]bool
}

func closedExperienceProducts(databases map[string]map[string]json.RawMessage, entries []experienceNativeEntry, slots map[string]experienceNativeSlot) (temporalV5CurrentUsage, error) {
	var usage temporalV5CurrentUsage
	products := map[string]closedExperienceProduct{}
	for name, db := range databases {
		var reservations []struct {
			ID      string         `json:"reservation_id"`
			Op      string         `json:"operation_id"`
			Owner   string         `json:"owner_id"`
			Request budget.Request `json:"request_json"`
		}
		if json.Unmarshal(db["execution_budget_reservations"], &reservations) != nil || reservations == nil {
			return usage, errors.New("product owning reservations omitted")
		}
		owned := map[string]string{}
		for _, r := range reservations {
			if r.Request.Charge.ModelTurns == 1 && r.Request.OperationID == r.Op && r.Owner == experienceNativeOwner && r.Request.OwnerID == experienceNativeOwner {
				owned[r.ID] = r.Op
			}
		}
		for _, table := range []string{"execution_model_attempts", "memory_model_attempts"} {
			var rows []json.RawMessage
			if json.Unmarshal(db[table], &rows) != nil || rows == nil {
				return usage, errors.New("explicit complete product population required")
			}
			for _, raw := range rows {
				var row map[string]json.RawMessage
				canonical, err := temporalCanonical(raw)
				if err != nil || json.Unmarshal(raw, &row) != nil {
					return usage, errors.New("invalid product row")
				}
				text := func(key string) string { var s string; _ = json.Unmarshal(row[key], &s); return s }
				op := text("operation_id")
				if op == "" || text("reservation_id") == "" || owned[text("reservation_id")] != op {
					return usage, errors.New("owned product identity required")
				}
				for _, key := range []string{"observed_at", "reservation_id"} {
					if row[key] == nil {
						return usage, errors.New("product NULL key omitted")
					}
				}
				keys := []string{"actual_input_tokens", "actual_output_tokens"}
				if table == "memory_model_attempts" {
					keys = []string{"usage"}
				}
				for _, key := range keys {
					if row[key] == nil {
						return usage, errors.New("product usage NULL key omitted")
					}
				}
				if prior, ok := products[op]; ok {
					if prior.Table != table || !bytes.Equal(prior.Raw, canonical) {
						return usage, errors.New("copied product identity conflicts")
					}
					prior.Database[name] = true
				} else {
					products[op] = closedExperienceProduct{table, canonical, row, map[string]bool{name: true}}
				}
			}
		}
	}
	usage.ProductAttempts = len(products)
	for _, p := range products {
		observed := !bytes.Equal(p.Row["observed_at"], []byte("null"))
		keys := []string{"actual_input_tokens", "actual_output_tokens"}
		if p.Table == "memory_model_attempts" {
			keys = []string{"usage"}
		}
		for _, key := range keys {
			observed = observed && !bytes.Equal(p.Row[key], []byte("null"))
		}
		if observed {
			usage.ProductObserved++
		} else {
			usage.ProductNULL++
		}
	}
	seen, started, perSlot, starts := map[string]bool{}, map[string]string{}, map[string]int{}, map[string]int{}
	for i, e := range entries {
		p, ok := products[e.Operation]
		slot, slotOK := slots[e.Slot]
		var reservation, run string
		_ = json.Unmarshal(p.Row["reservation_id"], &reservation)
		_ = json.Unmarshal(p.Row["run_id"], &run)
		if p.Table == "memory_model_attempts" {
			run = e.Operation
		}
		name := "source-database.json"
		if slot.Kind == "control" {
			name = fmt.Sprintf("case-%03d-database.json", slot.Sequence)
		}
		if !ok || !slotOK || slot.Started == nil || e.Sequence != i+1 || seen[e.Operation] || e.Reservation != reservation || e.RunID != run || !p.Database[name] || e.Finished && (e.Millis == nil || *e.Millis < 0) || !e.Finished && (e.Millis != nil || e.Usage != nil) || !json.Valid(e.Request) {
			return usage, errors.New("native entry lacks exact owning slot/product admission")
		}
		if e.Usage != nil && !bytes.Equal(p.Row["observed_at"], []byte("null")) {
			if p.Table == "memory_model_attempts" && !bytes.Equal(p.Row["usage"], []byte("null")) {
				var stored transport.Usage
				if json.Unmarshal(p.Row["usage"], &stored) != nil || !reflect.DeepEqual(stored, *e.Usage) {
					return usage, errors.New("background product/native usage differs")
				}
			} else if p.Table == "execution_model_attempts" && !bytes.Equal(p.Row["actual_input_tokens"], []byte("null")) && !bytes.Equal(p.Row["actual_output_tokens"], []byte("null")) {
				var input, output int64
				if json.Unmarshal(p.Row["actual_input_tokens"], &input) != nil || json.Unmarshal(p.Row["actual_output_tokens"], &output) != nil || input != int64(e.Usage.InputTokens) || output != int64(e.Usage.OutputTokens) {
					return usage, errors.New("foreground product/native usage differs")
				}
			}
		}
		if e.Kind == "start" {
			if started[e.RunID] != "" {
				return usage, errors.New("native Start repeated")
			}
			started[e.RunID] = e.Slot
			starts[e.Slot]++
			maximum := 1
			if slot.Kind == "original" && slot.Sequence == 3 {
				maximum = 2
			}
			if starts[e.Slot] > maximum {
				return usage, errors.New("slot acquired an undeclared replacement run")
			}
		} else if (e.Kind != "continuation" && e.Kind != "tool_result") || started[e.RunID] != e.Slot {
			return usage, errors.New("native continuation precedes owning Start")
		}
		seen[e.Operation] = true
		perSlot[e.Slot]++
		if perSlot[e.Slot] > slot.Maximum {
			return usage, errors.New("original partition exceeded")
		}
		usage.NativeEntries++
		if err := temporalV6AddNativeUsage(&usage, e.Usage); err != nil {
			return usage, err
		}
	}
	usage.ProductWithoutNativeEntry = usage.ProductAttempts - usage.NativeEntries
	if usage.ProductWithoutNativeEntry < 0 {
		return usage, errors.New("native entry population exceeds products")
	}
	return usage, nil
}

func closedExperienceFinitePlan(plan experienceNativeSeal, previous experienceNativeLineage) error {
	policy, e1 := experienceNativeCarryPolicy(false, previous.Carry)
	source, e2 := experienceNativeCarryPolicy(true, previous.Carry)
	deadline, e3 := experienceNativeWholeDeadline(plan.Created, previous.Day, previous.Deadline)
	if e1 != nil || e2 != nil || e3 != nil || !reflect.DeepEqual(plan.Policy, policy) || !reflect.DeepEqual(plan.SourcePolicy, source) || !plan.Deadline.Equal(deadline) || plan.FGMillis != experienceNativeForegroundLimit.Milliseconds() || plan.MaximumMillis != experienceNativeWholeMaximum.Milliseconds() || len(plan.Binaries) != 5 || len(plan.Environments) != 3 || len(plan.Source) == 0 {
		return errors.New("closed preseal finite source/control profile changed")
	}
	for _, name := range []string{"recipient", "broker", "broker_control", "extension", "http"} {
		if !experienceNativeSHA(plan.Binaries[name]) {
			return errors.New("all5 immutable binary identities required")
		}
	}
	for name, hash := range plan.Source {
		if name == "" || !experienceNativeSHA(hash) {
			return errors.New("exact frozen source identities required")
		}
	}
	for _, p := range []struct {
		name, version, ready string
		outage               bool
	}{{"project-v1-normal", "v1", "true", false}, {"project-v1-receipt-outage", "v1", "true", true}, {"project-v2-normal", "v2", "verified", false}} {
		configuration := experienceEnvironment(experienceRecipientConfig{Version: p.version, Ready: p.ready, Outage: p.outage})
		if plan.Environments[p.name] != experienceDigest(strings.Join([]string{configuration, plan.Binaries["recipient"], plan.Binaries["broker"], plan.Binaries["extension"], plan.Binaries["http"]}, "\x00")) {
			return errors.New("frozen treatment environment identity changed")
		}
	}
	return nil
}

type closedExperienceHandback struct {
	Native   *int `json:"native_status"`
	Handback *int `json:"handback_status"`
	Return   struct {
		CAS     bool  `json:"opaque_main_cas"`
		Checked bool  `json:"active_work_checked"`
		Raw     *bool `json:"raw_material_printed"`
	} `json:"return_safe"`
	Auth struct {
		Resolved bool
		Unsafe   *bool `json:"unsafe_refresh"`
		Expired  *bool
		Trace    []struct {
			Stage, Outcome string
			HTTP           int `json:"http_status"`
		}
	} `json:"main_auth_safe"`
	Snapshot struct {
		Saved bool   `json:"fresh_main_snapshot_saved"`
		Hash  string `json:"ciphertext_sha256"`
		Raw   *bool  `json:"raw_material_printed"`
	} `json:"main_snapshot_safe"`
}

func (h closedExperienceHandback) validate() error {
	if h.Native == nil || *h.Native < 0 || h.Handback == nil || *h.Handback != 0 || !h.Return.CAS || !h.Return.Checked || h.Return.Raw == nil || *h.Return.Raw || !h.Auth.Resolved || h.Auth.Unsafe == nil || *h.Auth.Unsafe || h.Auth.Expired == nil || *h.Auth.Expired || !h.Snapshot.Saved || !experienceNativeSHA(h.Snapshot.Hash) || h.Snapshot.Raw == nil || *h.Snapshot.Raw {
		return errors.New("incomplete safe handback")
	}
	completed := false
	for _, e := range h.Auth.Trace {
		if e.Stage != "paid_discovery" {
			continue
		}
		if e.Outcome == "completed" && e.HTTP == 200 {
			completed = true
		}
		if e.Outcome == "started" && e.HTTP == 0 || (e.Outcome == "response_decoded" || e.Outcome == "completed") && e.HTTP == 200 {
			continue
		}
		return errors.New("discovery handback failed")
	}
	if !completed {
		return errors.New("actual main discovery200 absent")
	}
	return nil
}

func closedExperienceJournal(j map[string]json.RawMessage, slots map[string]experienceNativeSlot, entries []experienceNativeEntry, databases map[string]map[string]json.RawMessage, m closedExperienceManifest, previous experienceNativeLineage, quotes experienceNativeQuotedCosts) error {
	var windows []struct {
		Window        string `json:"window_id"`
		Owner         string `json:"owner_id"`
		Maximum, Used int
		Closed        bool
	}
	var allocations []struct {
		Window        string `json:"window_id"`
		Slot          string `json:"slot_id"`
		Maximum, Used int
	}
	var rows []struct {
		Window      string `json:"window_id"`
		Sequence    int
		Slot        string `json:"slot_id"`
		Run         string `json:"run_id"`
		Kind        string
		Operation   string          `json:"operation_id"`
		Reservation string          `json:"reservation_id"`
		Request     json.RawMessage `json:"request_json"`
		Usage       json.RawMessage `json:"usage_json"`
	}
	var bindings []struct {
		Window      string `json:"window_id"`
		Hash        string `json:"history_sha256"`
		Reservation string `json:"source_carry_reservation_id"`
		Metadata    experienceNativeLineage
	}
	var clones []struct {
		Window string `json:"window_id"`
		Slot   string `json:"slot_id"`
		Name   string `json:"database_name"`
		State  string
	}
	// Local lineage contains a nested pointer. Independently decoded identical
	// JSON must compare by complete value, never by that allocation's address.
	if json.Unmarshal(j["experience_native_windows"], &windows) != nil || len(windows) != 1 || windows[0].Window != m.Preseal || windows[0].Owner != m.Owner || !windows[0].Closed || windows[0].Maximum != 640 || windows[0].Used < len(entries) || windows[0].Used > 640 || json.Unmarshal(j["experience_native_allocations"], &allocations) != nil || len(allocations) != 57 || json.Unmarshal(j["experience_native_entries"], &rows) != nil || len(rows) != windows[0].Used || json.Unmarshal(j["experience_native_history"], &bindings) != nil || len(bindings) != 1 || bindings[0].Window != m.Preseal || bindings[0].Hash != m.Previous || bindings[0].Reservation != quotes.HistoryReservation || !reflect.DeepEqual(bindings[0].Metadata, previous) || json.Unmarshal(j["experience_native_case_databases"], &clones) != nil || clones == nil {
		return errors.New("closed durable global population or original history binding differs")
	}
	used, seen, seq := map[string]int{}, map[string]bool{}, map[int]bool{}
	native := map[string]experienceNativeEntry{}
	for _, e := range entries {
		native[e.Operation] = e
	}
	for _, r := range rows {
		slot, ok := slots[r.Slot]
		if !ok || r.Window != m.Preseal || r.Sequence < 1 || r.Sequence > len(rows) || seq[r.Sequence] || seen[r.Operation] || r.Operation == "" || r.Run == "" || r.Reservation == "" || !json.Valid(r.Request) || r.Usage == nil {
			return errors.New("global original entry identity changed")
		}
		seq[r.Sequence] = true
		seen[r.Operation] = true
		used[r.Slot]++
		if used[r.Slot] > slot.Maximum {
			return errors.New("global original partition exceeded")
		}
		if e, ok := native[r.Operation]; ok {
			a, ae := temporalCanonical(r.Request)
			b, be := temporalCanonical(e.Request)
			u, _ := json.Marshal(e.Usage)
			c, ce := temporalCanonical(r.Usage)
			d, de := temporalCanonical(u)
			if ae != nil || be != nil || ce != nil || de != nil || !bytes.Equal(a, b) || !bytes.Equal(c, d) || r.Sequence != e.Sequence || r.Slot != e.Slot || r.Run != e.RunID || r.Kind != e.Kind || r.Reservation != e.Reservation {
				return errors.New("global and native original entry differ")
			}
		} else {
			// A durable reservation may precede a failed capture-file write. Keep
			// that admission charged and UNKNOWN; it cannot become a native call.
			if !bytes.Equal(r.Usage, []byte("null")) {
				return errors.New("uncaptured global entry claims observed usage")
			}
			name := "source-database.json"
			if slot.Kind == "control" {
				name = fmt.Sprintf("case-%03d-database.json", slot.Sequence)
			}
			found := false
			for _, table := range []string{"execution_model_attempts", "memory_model_attempts"} {
				var products []map[string]json.RawMessage
				_ = json.Unmarshal(databases[name][table], &products)
				for _, p := range products {
					var op, res, run string
					_ = json.Unmarshal(p["operation_id"], &op)
					_ = json.Unmarshal(p["reservation_id"], &res)
					_ = json.Unmarshal(p["run_id"], &run)
					if table == "memory_model_attempts" {
						run = op
					}
					if op == r.Operation && res == r.Reservation && run == r.Run && bytes.Equal(p["observed_at"], []byte("null")) {
						found = true
					}
				}
			}
			if !found {
				return errors.New("uncaptured global entry lacks owning UNKNOWN product")
			}
		}
	}
	for op := range native {
		if !seen[op] {
			return errors.New("native original omitted from global journal")
		}
	}
	allocated := map[string]bool{}
	for _, a := range allocations {
		slot, ok := slots[a.Slot]
		if !ok || allocated[a.Slot] || a.Window != m.Preseal || a.Maximum != slot.Maximum || a.Used != used[a.Slot] {
			return errors.New("global per-slot allocation differs")
		}
		allocated[a.Slot] = true
	}
	cloneNames, cloneSlots := map[string]bool{}, map[string]bool{}
	for _, c := range clones {
		slot, ok := slots[c.Slot]
		if !ok || slot.Kind != "control" || c.Window != m.Preseal || c.Name == "" || cloneNames[c.Name] || cloneSlots[c.Slot] || c.State != "created" && c.State != "failed" && c.State != "reserved" {
			return errors.New("durable clone population changed")
		}
		cloneNames[c.Name] = true
		cloneSlots[c.Slot] = true
		name := fmt.Sprintf("case-%03d-database.json", slot.Sequence)
		if (c.State == "created" || used[c.Slot] > 0) && databases[name] == nil {
			return errors.New("allocated or used database omitted from cost union")
		}
	}
	for name := range databases {
		if name == "source-database.json" {
			continue
		}
		found := false
		for id, slot := range slots {
			if name == fmt.Sprintf("case-%03d-database.json", slot.Sequence) && cloneSlots[id] {
				found = true
			}
		}
		if !found {
			return errors.New("cost export lacks durable clone allocation")
		}
	}
	return nil
}
