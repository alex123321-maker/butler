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
)

// This lab-only consumer independently recomputes a closed cohort's immutable
// cost population. It grants neither provider authority nor quality acceptance.
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
	var out experienceNativeLineage
	root := filepath.Dir(path)
	raw, err := closedExperienceRead(root, filepath.Base(path), expected, 4*1024*1024)
	var m closedExperienceManifest
	if err != nil || json.Unmarshal(raw, &m) != nil || m.Format != "butler.experience.closed-owner-leaf.v1" || m.Owner != experienceNativeOwner || m.Denominator != 57 || m.Maximum != 640 || !m.Complete || m.Qualified == nil || *m.Qualified || m.Review == nil || !*m.Review || m.DiscoveryInference == nil || *m.DiscoveryInference || m.Invoice != "unknown" || !experienceNativeSHA(m.Preseal) || !experienceNativeSHA(m.Previous) {
		return out, errors.New("complete unqualified closed experience leaf required")
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
	if read("plan/preseal.json", &plan) != nil || read("plan/schedule.json", &schedule) != nil || plan.Format != "butler.experience.native-preseal.v1" || schedule.Format != "experience90-window-v1" || schedule.Owner != m.Owner || schedule.Maximum != 640 || plan.Owner != m.Owner || plan.Lineage != previous || !plan.Deadline.Equal(m.Deadline) || m.Day != previous.Day || m.Deadline.After(previous.Deadline) || !reflect.DeepEqual(schedule.Slots, plan.Schedule) {
		return out, errors.New("closed cohort differs from exact original preseal and parent")
	}
	corpus, err := experienceNativeCorpusFiles()
	if err != nil || !reflect.DeepEqual(plan.Schedule, experienceNativeSchedule(corpus)) || plan.Originals != corpus.OriginalsSHA256 || plan.Controls != corpus.ControlsSHA256 {
		return out, errors.New("closed full57 original schedule changed")
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
	for name, hash := range files {
		if old := refs[name]; old != "" && old != hash {
			return out, errors.New("private reference conflicts")
		}
		refs[name] = hash
	}
	var window struct {
		Slots    []experienceNativeSlot
		Entries  []experienceNativeEntry
		Maximum  int       `json:"maximum_entries"`
		Active   int       `json:"active_slot"`
		Started  time.Time `json:"started_at"`
		Finished time.Time `json:"finished_at"`
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
	var domain struct {
		Window   string `json:"window_id"`
		Hash     string `json:"window_hash"`
		Schema   string `json:"schema_name"`
		Owner    string `json:"owner_id"`
		History  string `json:"history_sha256"`
		Day      string
		Deadline time.Time
	}
	if read("actual/domain-claim.json", &domain) != nil || domain.Window != experienceNativeWindow || domain.Hash != m.Preseal || domain.Schema != experienceNativeSchema || domain.Owner != m.Owner || domain.History != m.Previous || domain.Day != m.Day || domain.Deadline != previous.Deadline {
		return out, errors.New("one-use domain receipt differs")
	}
	var handback closedExperienceHandback
	if read("closed-handback.json", &handback) != nil || handback.validate() != nil {
		return out, errors.New("closed collector and actual opaque main handback required")
	}
	if refs["runner.jsonl"] == "" {
		return out, errors.New("original runner evidence omitted")
	}
	out = experienceNativeLineage{Hash: expected, Day: m.Day, Deadline: m.Deadline, Carry: m.Carry, Usage: m.Usage.Inclusive}
	return out, validateExperienceNativeLineage(out)
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
			Request budget.Request `json:"request_json"`
		}
		if json.Unmarshal(db["execution_budget_reservations"], &reservations) != nil || reservations == nil {
			return usage, errors.New("product owning reservations omitted")
		}
		owned := map[string]string{}
		for _, r := range reservations {
			if r.Request.Charge.ModelTurns == 1 && r.Request.OperationID == r.Op && r.Request.OwnerID == experienceNativeOwner {
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
				if op == "" || text("reservation_id") == "" || text("owner_id") != experienceNativeOwner || owned[text("reservation_id")] != op {
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
	seen, started, perSlot := map[string]bool{}, map[string]string{}, map[string]int{}
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
		if e.Kind == "start" {
			if started[e.RunID] != "" {
				return usage, errors.New("native Start repeated")
			}
			started[e.RunID] = e.Slot
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
	if json.Unmarshal(j["experience_native_windows"], &windows) != nil || len(windows) != 1 || windows[0].Window != m.Preseal || windows[0].Owner != m.Owner || !windows[0].Closed || windows[0].Maximum != 640 || windows[0].Used < len(entries) || windows[0].Used > 640 || json.Unmarshal(j["experience_native_allocations"], &allocations) != nil || len(allocations) != 57 || json.Unmarshal(j["experience_native_entries"], &rows) != nil || len(rows) != windows[0].Used || json.Unmarshal(j["experience_native_history"], &bindings) != nil || len(bindings) != 1 || bindings[0].Window != m.Preseal || bindings[0].Hash != m.Previous || bindings[0].Reservation != quotes.HistoryReservation || bindings[0].Metadata != previous || json.Unmarshal(j["experience_native_case_databases"], &clones) != nil || clones == nil {
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
