package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
)

const coherentNativeWindow = "coherent-whole-v1"
const coherentNativeSchema = "coherent_native_v1"
const coherentNativeCarryPrefix = "issue92-native-current-day-carry:"
const coherentNativeFaultAdmissions = 5 // first answer, outage, UNKNOWN inspect+prepare, late-release fault

type coherentNativeSlot struct {
	Name    string `json:"name"`
	Maximum int    `json:"maximum_entries"`
	State   string `json:"state"`
	Blocker string `json:"blocker"`
}

func coherentNativeSchedule() []coherentNativeSlot {
	slots := []coherentNativeSlot{{Name: "preference_candidate", Maximum: 1}, {Name: "new_conversation", Maximum: 12}, {Name: "simple_question", Maximum: 1}, {Name: "current_english_exception", Maximum: 12}}
	for i := 1; i <= 5; i++ {
		slots = append(slots, coherentNativeSlot{Name: fmt.Sprintf("long_primary_%d", i), Maximum: 2})
	}
	slots = append(slots, coherentNativeSlot{Name: "primary_compaction", Maximum: 1}, coherentNativeSlot{Name: "restarted_continuation", Maximum: 12}, coherentNativeSlot{Name: "verified_original", Maximum: 12}, coherentNativeSlot{Name: "accepted_episode_note", Maximum: 1}, coherentNativeSlot{Name: "native_consolidation", Maximum: 1}, coherentNativeSlot{Name: "consolidated_read", Maximum: 12})
	for i := range slots {
		slots[i].State = "not_executed"
		slots[i].Blocker = "window_not_started"
	}
	return slots
}
func coherentNativeMaximum() int {
	n := 0
	for _, s := range coherentNativeSchedule() {
		n += s.Maximum
	}
	return n
}
func coherentNativePolicy(carry budget.Units) (budget.Policy, error) {
	p := budget.DefaultPolicy()
	n := int64(coherentNativeMaximum() + coherentNativeFaultAdmissions)
	u := budget.Units{ModelTurns: n, Tokens: n * 1081344, ExternalCalls: 256, ActiveMillis: n*300000 + 256*p.ToolMillis, EgressBytes: 256 * p.ToolEgressBytes, ProviderInputBytes: n * p.ModelInputBytes}
	p.Task, p.Mandate, p.Conversation = u, u, u
	p.Day = consolidationAddUnits(u, carry)
	p.ModelMillis, p.OutputTokens = 300000, 32768
	p.Models = map[string]budget.ModelPrice{"antigravity:gemini-3.8-flash-high": {Basis: "subscription_no_overage"}}
	if !consolidationV2Within(u, p.Day) || !consolidationV2Within(carry, p.Day) || p.Validate() != nil {
		return budget.Policy{}, errors.New("finite coherent carry overflow")
	}
	return p, nil
}

type coherentNativeCorpus struct {
	Pins   map[string]string `json:"sha256"`
	Inputs []string          `json:"inputs"`
}

func readCoherentNativeCorpus(root string) (coherentNativeCorpus, error) {
	out := coherentNativeCorpus{Pins: map[string]string{}}
	var manifest struct {
		Format    string
		Documents []struct{ File, Question string }
	}
	raw, err := os.ReadFile(filepath.Join(root, "questions.json"))
	if err != nil || len(raw) > 16384 || json.Unmarshal(raw, &manifest) != nil || manifest.Format != "butler.coherent.primary-inputs.v1" || len(manifest.Documents) != 5 {
		return out, errors.New("five frozen primary documents/questions required")
	}
	out.Pins["questions.json"] = experienceDigest(string(raw))
	total, covered := 0, 0
	seen := map[string]bool{}
	for i, doc := range manifest.Documents {
		if filepath.Base(doc.File) != doc.File || !strings.HasSuffix(doc.File, ".md") || seen[doc.File] || len(doc.Question) < 80 || len(doc.Question) > 1024 {
			return out, errors.New("bounded distinct primary input identity required")
		}
		seen[doc.File] = true
		body, err := os.ReadFile(filepath.Join(root, doc.File))
		if err != nil || len(body) < 4096 || len(body) > 6500 || bytes.Contains(body, []byte("\r")) {
			return out, errors.New("genuinely long frozen LF primary input required")
		}
		// Five independently authored documents, not repeated assistant filler.
		paragraphs := strings.Split(strings.TrimSpace(string(body)), "\n\n")
		unique := map[string]bool{}
		for _, para := range paragraphs {
			if unique[para] {
				return out, errors.New("duplicate primary paragraph")
			}
			unique[para] = true
		}
		if len(unique) < 10 {
			return out, errors.New("varied primary document sections required")
		}
		out.Pins[doc.File] = experienceDigest(string(body))
		input := doc.Question + "\n\nAttached source DATA:\n" + string(body)
		out.Inputs = append(out.Inputs, input)
		total += len(body)
		if i < 3 {
			encoded, _ := json.Marshal(input)
			covered += len(encoded)
		}
	}
	// Production compaction preserves covered user text verbatim and caps output
	// at32768 bytes. Newest four raw events retain the final two user turns.
	if total < 22000 || covered > 22000 {
		return out, errors.New("primary length or compulsory compaction output bound exceeded")
	}
	return out, nil
}

type coherentNativeSeal struct {
	Format               string                  `json:"format"`
	Owner                string                  `json:"owner"`
	Created              time.Time               `json:"created_at"`
	Deadline             time.Time               `json:"deadline"`
	History              experienceNativeLineage `json:"history"`
	Source               map[string]string       `json:"source_files"`
	Binaries             map[string]string       `json:"binaries"`
	Corpus               coherentNativeCorpus    `json:"primary_inputs"`
	Schedule             []coherentNativeSlot    `json:"schedule"`
	Policy               budget.Policy           `json:"policy"`
	ContextTokens        int                     `json:"foreground_context_budget"`
	CompactionTrigger    int                     `json:"compaction_trigger"`
	WholeMillis          int64                   `json:"whole_maximum_millis"`
	ControlledAdmissions int                     `json:"controlled_product_admissions_maximum"`
	AuthReturn           string                  `json:"opaque_auth_return_contract"`
	Checkpoints          []string                `json:"ordered_checkpoints"`
	Faults               map[string]int          `json:"controlled_fault_product_maxima"`
	RubricSHA256         string                  `json:"independent_rubric_sha256"`
	SelectedFeatures     map[string]bool         `json:"selected_features"`
	TemporalPredicate    string                  `json:"temporal_read_predicate"`
	Resources            map[string]int64        `json:"resource_limits"`
}

func coherentNativeCheckpoints() []string {
	names := []string{"owner_statement_to_reviewable_candidate_job", "explicit_owner_acceptance", "new_conversation_real_profile_and_original_open", "independent_simple_question_zero_tools", "same_task_owner_amendment_preserves_global_profile"}
	for i := 1; i <= 5; i++ {
		names = append(names, fmt.Sprintf("ordinary_long_task_turn_%d", i))
	}
	return append(names, "long_task_primary_compaction_published", "restarted_ordinary_task_keeps_summary_current_goal_exception_unknown", "same_root_budget_exhaustion_zero_new_provider_entries", "admitted_provider_unavailable_preserves_null_usage_unknown", "owning_source_pool_unavailable_denies_before_release", "actual_current_original_uses_earlier_preference_and_independent_verification", "earlier_verified_original_to_owner_accepted_episode", "actual_broker_runtime_unknown_effect_stops_without_replay_or_final", "earlier_preference_and_verified_episode_review_diff_activate", "ordinary_broker_reads_earlier_consolidated_episode_with_current_applicability", "actual_custom_dump_of_same_prewithdrawal_timeline", "controlled_late_withdrawal_during_prepared_release_keeps_null_charge_no_final", "late_original_deletion_fences_episode_and_consolidation", "fresh_custom_restore_latest_anchor_erases_original_episode_view_retains_all_charges", "same_restored_timeline_actual_enospc_keeps_failed_deletion_uncommitted")
}

func prepareCoherentNativeSeal(now time.Time, root string, history experienceNativeLineage, binaries map[string]string) (coherentNativeSeal, error) {
	var seal coherentNativeSeal
	if validateExperienceNativeLineage(history) != nil || history.Day != now.UTC().Format("2006-01-02") || !history.Deadline.After(now) {
		return seal, errors.New("current recursively validated latest closed90 history required")
	}
	deadline := now.Add(110 * time.Minute)
	day := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day()+1, 0, 0, 0, 0, time.UTC)
	for _, earlier := range []time.Time{history.Deadline, day} {
		if earlier.Before(deadline) {
			deadline = earlier
		}
	}
	if !deadline.After(now.Add(10 * time.Minute)) {
		return seal, errors.New("insufficient finite owner-day window")
	}
	source, err := experienceNativeSourcePins(root)
	if err != nil {
		return seal, err
	}
	artifacts, err := experienceNativeBinaryPins(binaries)
	if err != nil {
		return seal, err
	}
	corpus, err := readCoherentNativeCorpus(filepath.Join(root, "deploy/memory-coherent/long-v1"))
	if err != nil {
		return seal, err
	}
	policy, err := coherentNativePolicy(history.Carry)
	if err != nil {
		return seal, err
	}
	rubric, err := os.ReadFile(filepath.Join(root, "docs/testing/fixtures/issue92-native/rubric-v1.json"))
	if err != nil || len(rubric) > 32768 {
		return seal, errors.New("frozen independent whole rubric required")
	}
	return coherentNativeSeal{Format: "butler.coherent.native-preseal.v1", Owner: "owner", Created: now.UTC(), Deadline: deadline, History: history, Source: source, Binaries: artifacts, Corpus: corpus, Schedule: coherentNativeSchedule(), Policy: policy, ContextTokens: 131072, CompactionTrigger: 4096, WholeMillis: (110 * time.Minute).Milliseconds(), ControlledAdmissions: coherentNativeFaultAdmissions, AuthReturn: "proven exited collector; latest durable ORIGINAL owning PG setting, never stale restore; serialized main CAS under shared lock; main control-plane 200 and fresh main snapshot; no auth bytes in public proof", Checkpoints: coherentNativeCheckpoints(), Faults: map[string]int{"budget_first_then_denial": 1, "provider_unavailable": 1, "unknown_inspect_then_prepare": 2, "late_prepared_release": 1, "closed_source_pool": 0, "restore_before_reconcile": 0, "bounded_enospc": 0}, RubricSHA256: experienceDigest(string(rubric)), SelectedFeatures: map[string]bool{"read": true, "candidate": true, "summary": true, "consolidation": true, "experience_read": true, "experience_generation": true, "temporal": true, "adaptive": false}, TemporalPredicate: "temporalReadConfigured(true, actual_provider_result, shared_budget_controller) for every foreground", Resources: map[string]int64{"client_cpu": 2, "client_memory_bytes": 3221225472, "postgres_cpu": 2, "postgres_memory_bytes": 1073741824, "bounded_tmpfs_bytes": 8192}}, nil
}
func checkCoherentNativeSeal(raw []byte, sha string, now time.Time, root string, h experienceNativeLineage, binaries map[string]string) (coherentNativeSeal, error) {
	var seal coherentNativeSeal
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > 16*1024*1024 || !experienceNativeSHA(sha) || experienceDigest(string(raw)) != sha || decoder.Decode(&seal) != nil || decoder.Decode(new(any)) != io.EOF || seal.Created.After(now) || !seal.Deadline.After(now) {
		return seal, errors.New("exact current coherent preseal required")
	}
	expected, err := prepareCoherentNativeSeal(seal.Created, root, h, binaries)
	if err != nil || !reflect.DeepEqual(expected, seal) {
		return seal, errors.New("coherent source/artifacts/history/inputs/schedule changed")
	}
	return seal, nil
}

func TestCoherentNativeFrozenCorpusAndFiniteSchedule(t *testing.T) {
	c, err := readCoherentNativeCorpus("../../../../deploy/memory-coherent/long-v1")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Inputs) != 5 || len(c.Pins) != 6 || len(coherentNativeSchedule()) != 15 || coherentNativeMaximum() != 75 || len(coherentNativeCheckpoints()) != 25 {
		t.Fatal("whole denominator changed")
	}
	p, err := coherentNativePolicy(budget.Units{ModelTurns: 7, Tokens: 123})
	if err != nil || p.Day.ModelTurns != 87 || p.Task.ModelTurns != 80 || p.OutputTokens != 32768 || p.ModelMillis != 300000 {
		t.Fatal("finite native/control/carry contract changed", err)
	}
	if _, err = coherentNativePolicy(budget.Units{ModelTurns: int64(^uint64(0) >> 1)}); err == nil {
		t.Fatal("carry overflow admitted")
	}
}
