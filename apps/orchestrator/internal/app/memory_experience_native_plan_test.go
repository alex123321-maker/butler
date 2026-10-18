package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

const experienceNativeForegroundLimit = 40 * time.Minute
const experienceNativeWholeMaximum = 54*experienceNativeForegroundLimit + 4*5*time.Minute + 2*time.Hour

// The conditional original03 read is a separate visible foreground input, with
// no additional logical slot or model quote. Include its normal40m lifetime in
// the finite whole-window maximum, then clamp by the actual owner/day lineage.
func experienceNativeWholeDeadline(now time.Time, day string, historyDeadline time.Time) (time.Time, error) {
	now = now.UTC()
	if day != now.Format("2006-01-02") || !historyDeadline.After(now) {
		return time.Time{}, errors.New("current owner/day history is expired or belongs to another day")
	}
	deadline := now.Add(experienceNativeWholeMaximum)
	dayEnd := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	if dayEnd.Before(deadline) {
		deadline = dayEnd
	}
	if historyDeadline.Before(deadline) {
		deadline = historyDeadline
	}
	return deadline, nil
}

// Only executable current inputs are inspected. Oracle/class/requested-outcome
// fields are absent from experienceNativeTask and never enter this function.
func validateExperienceNativeMechanicalInputs(c experienceNativeCorpus) error {
	var profile struct {
		Language string `json:"answer_language"`
		NoOther  bool   `json:"no_other_preferences"`
	}
	if json.Unmarshal(c.Originals.CommonProfile, &profile) != nil || profile.Language != "ru" || !profile.NoOther {
		return errors.New("fixed current owner profile differs")
	}
	if len(c.Originals.Tasks) != 4 || len(c.Controls.Cases) != 8 {
		return errors.New("full original/control population required")
	}
	for group, tasks := range [][]experienceNativeTask{c.Originals.Tasks, c.Controls.Cases} {
		for index, item := range tasks {
			prefix := "original"
			if group == 1 {
				prefix = "control"
			}
			if item.ID != fmt.Sprintf("%s-%02d", prefix, index+1) {
				return errors.New("predeclared input identity differs")
			}
			switch item.EnvironmentProfile {
			case "project-v1-normal", "project-v1-receipt-outage", "project-v2-normal":
			default:
				return errors.New("unsupported fixed environment")
			}
			if _, err := experienceNativePermissionTools(item.PermissionProfile); err != nil {
				return errors.New("unsupported current permission profile")
			}
			if group == 0 {
				if item.Reset != "empty_patch_store_original_readme" && item.Reset != "independent_empty_patch_store_original_readme" {
					return errors.New("unsupported original reset")
				}
			} else {
				switch item.Reset {
				case "empty_patch_store_original_readme", "empty_beta_patch_store_original_readme", "empty_patch_store_v2_readme", "exact_actual_original04_recipient_and_content_free_effect_snapshot", "empty_patch_store_original_readme_and_delete_original01_source_after_frozen_notes_skills":
				default:
					return errors.New("unsupported current reset")
				}
			}
			if item.CurrentState != nil && (item.CurrentState.Effect != "effect_unknown" || item.CurrentState.NewPrepareAllowed || item.CurrentState.EffectReference == "" || item.Reset != "exact_actual_original04_recipient_and_content_free_effect_snapshot" || item.PermissionProfile != "read_and_verify_existing_effect_only") {
				return errors.New("current effect reference is not a bounded read-only UNKNOWN")
			}
		}
	}
	return nil
}

func experienceNativePermissionTools(profile string) ([]string, error) {
	switch profile {
	case "inspect_and_check_only", "read_and_verify_existing_effect_only":
		return []string{"extension.project.inspect", "extension.project.check"}, nil
	case "prepare_and_verify_exact_resource", "one_prepare_attempt_and_verify_exact_resource", "one_prepare_attempt_and_bounded_verification_exact_resource", "prepare_and_verify_beta_only_no_alpha_memory_grants", "prepare_and_verify_exact_resource_with_current_remaining_source_grants":
		return []string{"extension.project.inspect", "extension.project.prepare-patch", "extension.project.check"}, nil
	default:
		return nil, errors.New("unsupported frozen permission profile")
	}
}

func TestExperienceNativeWholeDeadlinePreservesForegroundAndUTC(t *testing.T) {
	now := time.Date(2026, 10, 18, 6, 0, 0, 0, time.UTC)
	if experienceNativeWholeMaximum != 38*time.Hour+20*time.Minute || experienceNativeForegroundLimit != 40*time.Minute {
		t.Fatal("whole/foreground capacity was shortened")
	}
	end, err := experienceNativeWholeDeadline(now, "2026-10-18", now.Add(48*time.Hour))
	if err != nil || !end.Equal(time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("whole window escaped its UTC day")
	}
	prior := now.Add(3 * time.Hour)
	if end, err = experienceNativeWholeDeadline(now, "2026-10-18", prior); err != nil || !end.Equal(prior) {
		t.Fatal("earlier frozen lineage deadline was enlarged")
	}
	if _, err = experienceNativeWholeDeadline(now, "2026-10-17", prior); err == nil {
		t.Fatal("old-day lineage opened a new current window")
	}
	if _, err = experienceNativeWholeDeadline(now, "2026-10-18", now); err == nil {
		t.Fatal("expired lineage was renewed")
	}
}

func TestExperienceNativeMechanicalInputsFailClosed(t *testing.T) {
	actual, err := experienceNativeCorpusFiles()
	if err != nil || validateExperienceNativeMechanicalInputs(actual) != nil {
		t.Fatal("the full frozen actual corpus is outside the mechanical input contract")
	}
	fresh := func() experienceNativeCorpus {
		c := experienceWindowFixture()
		c.Originals.CommonProfile = json.RawMessage(`{"answer_language":"ru","no_other_preferences":true}`)
		for group, items := range [][]experienceNativeTask{c.Originals.Tasks, c.Controls.Cases} {
			for i := range items {
				prefix := "original"
				if group == 1 {
					prefix = "control"
				}
				items[i].ID = fmt.Sprintf("%s-%02d", prefix, i+1)
				items[i].EnvironmentProfile, items[i].PermissionProfile, items[i].Reset = "project-v1-normal", "prepare_and_verify_exact_resource", "empty_patch_store_original_readme"
			}
		}
		return c
	}
	if err := validateExperienceNativeMechanicalInputs(fresh()); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*experienceNativeCorpus){
		"changed_identity":    func(c *experienceNativeCorpus) { c.Controls.Cases[0].ID = "current-other" },
		"changed_environment": func(c *experienceNativeCorpus) { c.Controls.Cases[0].EnvironmentProfile = "dynamic" },
		"changed_permission":  func(c *experienceNativeCorpus) { c.Controls.Cases[0].PermissionProfile = "unrestricted" },
		"changed_reset":       func(c *experienceNativeCorpus) { c.Controls.Cases[0].Reset = "retry_previous" },
		"changed_profile": func(c *experienceNativeCorpus) {
			c.Originals.CommonProfile = json.RawMessage(`{"answer_language":"en","no_other_preferences":true}`)
		},
		"unknown_write": func(c *experienceNativeCorpus) {
			_ = json.Unmarshal([]byte(`{"effect":"effect_unknown","effect_reference":"actual-original04","new_prepare_allowed":true}`), &c.Controls.Cases[3].CurrentState)
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := fresh()
			mutate(&c)
			if validateExperienceNativeMechanicalInputs(c) == nil {
				t.Fatal("unsupported current mechanics entered the frozen collector")
			}
		})
	}
}
