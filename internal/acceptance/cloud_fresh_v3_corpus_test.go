package acceptance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// These author checks reuse only the old generic exact-JSON/original-witness
// helpers. They never call a candidate model scorer or assert semantic approval.
func freshV3AuthorLoad(t *testing.T) freshAuthorCorpus {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/acceptance/ru-cloud-fresh-v3.json")
	var c freshAuthorCorpus
	if err != nil || freshAuthorDecode(raw, &c) != nil {
		t.Fatal("new author corpus exact JSON denied")
	}
	return c
}

func freshV3IntentPart(intent string) string {
	for part, list := range map[string]string{
		"development": "export_private restrict_resource require_evidence restrict_privacy reconcile_only correct_source",
		"tuning":      "correct_preference restrict_budget revoke_mandate amend_prepare_only prepare_only pause_task",
		"holdout":     "stop_tree restrict_scope revoke_extension suspend_initiative delete_source",
	} {
		for _, candidate := range strings.Fields(list) {
			if candidate == intent {
				return part
			}
		}
	}
	return ""
}

var freshV3SourceID = regexp.MustCompile(`^d_[0-9a-f]{20}$`)

func freshV3AbsentExtraction(id string) bool {
	switch id {
	case "q305d", "q312d", "q315d", "q320d", "q323d", "q328d":
		return true
	default:
		return false
	}
}

func freshV3AuthorValidate(c freshAuthorCorpus) error {
	if c.Version != "butler-ru-cloud-fresh-v3" || len(c.Groups) != 30 || len(c.Cases) != 240 {
		return errors.New("new corpus cardinality denied")
	}
	groups, families := map[string]freshAuthorGroup{}, map[string]string{}
	for i, g := range c.Groups {
		part := "development"
		if i >= 20 {
			part = "holdout"
		} else if i >= 12 {
			part = "tuning"
		}
		provenance := "synthetic_public"
		if i == 29 {
			provenance = "synthetic_secret"
		}
		if g.ID != fmt.Sprintf("g%d", 301+i) || g.Split != part || g.Provenance != provenance || g.SemanticFamily == "" || g.AttackFamily == "" {
			return errors.New("new corpus group contract denied")
		}
		for _, family := range []string{"semantic:" + g.SemanticFamily, "attack:" + g.AttackFamily} {
			if previous := families[family]; previous != "" && previous != part {
				return errors.New("new corpus family crossed split")
			}
			families[family] = part
		}
		groups[g.ID] = g
	}
	counts, blocked, intents := map[string]int{}, map[string]int{}, map[string]bool{}
	absent := map[string]int{}
	originals, sourceGroups := map[string]string{}, map[string]string{}
	for i, e := range c.Cases {
		n, suffix := 301+i/8, byte('a'+i%8)
		kind := []string{"retrieval", "extraction", "classification", "commands"}[(i%8)/2]
		g := groups[e.Group]
		if e.ID != fmt.Sprintf("q%d%c", n, suffix) || e.Group != fmt.Sprintf("g%d", n) || e.Kind != kind || e.Input == "" || e.Documents == nil || e.NoModel == nil {
			return errors.New("new corpus case contract denied")
		}
		secret := n == 330 && suffix <= 'd'
		if *e.NoModel != secret || secret && len(e.Documents) != 2 {
			return errors.New("new corpus secret boundary denied")
		}
		counts[g.Split]++
		if secret {
			blocked[g.Split]++
		}
		for id, body := range e.Documents {
			if !freshV3SourceID.MatchString(id) || body == "" || sourceGroups[id] != "" && sourceGroups[id] != e.Group || originals[id] != "" && originals[id] != body {
				return errors.New("opaque source identity or original bytes drift")
			}
			originals[id], sourceGroups[id] = body, e.Group
		}
		if err := freshAuthorGolden(e); err != nil {
			return err
		}
		if kind == "extraction" {
			var answer struct {
				Facts []map[string]string `json:"facts"`
			}
			if json.Unmarshal(e.Golden, &answer) != nil || answer.Facts == nil || (len(answer.Facts) == 0) != freshV3AbsentExtraction(e.ID) {
				return errors.New("new absent-property extraction inventory denied")
			}
			if len(answer.Facts) == 0 {
				absent[g.Split]++
			}
		}
		if kind == "commands" {
			var answer struct {
				Intent, Authority string
			}
			if json.Unmarshal(e.Golden, &answer) != nil || answer.Intent != "clarify" && freshV3IntentPart(answer.Intent) != g.Split {
				return errors.New("new command construction family crossed split")
			}
			if answer.Intent != "clarify" {
				intents[answer.Intent] = true
			}
		}
	}
	if len(originals) != 60 || len(intents) != 17 || counts["development"] != 96 || counts["tuning"] != 64 || counts["holdout"] != 80 || blocked["development"] != 0 || blocked["tuning"] != 0 || blocked["holdout"] != 4 {
		return errors.New("new split/source/control inventory denied")
	}
	if absent["development"] != 2 || absent["tuning"] != 2 || absent["holdout"] != 2 {
		return errors.New("new absent-property extraction split balance denied")
	}
	wanted := []string{"q301a", "q302c", "q303e", "q304g"}
	if len(c.Comparison) != 4 {
		return errors.New("new comparison selection denied")
	}
	for i, id := range wanted {
		if c.Comparison[i] != id {
			return errors.New("new comparison identity drift")
		}
	}
	return nil
}

type freshV3Text struct {
	id, group, part, normalized string
	grams                       map[string]bool
}

func freshV3TextOf(id, group, part, raw string) freshV3Text {
	normalized := freshAuthorNormalized(raw)
	words, grams := strings.Fields(normalized), map[string]bool{}
	for i := 0; i+2 < len(words); i++ {
		grams[strings.Join(words[i:i+3], " ")] = true
	}
	return freshV3Text{id, group, part, normalized, grams}
}

func freshV3Texts(c freshAuthorCorpus) []freshV3Text {
	parts := map[string]string{}
	for _, g := range c.Groups {
		parts[g.ID] = g.Split
	}
	seen, out := map[string]bool{}, []freshV3Text{}
	for _, e := range c.Cases {
		out = append(out, freshV3TextOf(e.ID, e.Group, parts[e.Group], e.Input))
		for id, raw := range e.Documents {
			if !seen[id] {
				seen[id] = true
				out = append(out, freshV3TextOf(id, e.Group, parts[e.Group], raw))
			}
		}
	}
	return out
}

func freshV3Near(a, b freshV3Text) bool {
	if a.normalized == b.normalized {
		return true
	}
	intersection := 0
	for gram := range a.grams {
		if b.grams[gram] {
			intersection++
		}
	}
	union := len(a.grams) + len(b.grams) - intersection
	return union > 0 && intersection*100 >= union*85
}

func TestCloudFreshV3AuthorWholeInventory(t *testing.T) {
	if err := freshV3AuthorValidate(freshV3AuthorLoad(t)); err != nil {
		t.Fatal(err)
	}
}

func TestCloudFreshV3AuthorExactOriginalWitness(t *testing.T) {
	c := freshV3AuthorLoad(t)
	e := c.Cases[2]
	var answer struct {
		Facts []map[string]string `json:"facts"`
	}
	if json.Unmarshal(e.Golden, &answer) != nil || len(answer.Facts) != 1 {
		t.Fatal("exact witness fixture missing")
	}
	for _, mutation := range []string{"quote_only", "unknown_source", "prefixed_source", "normalized_units", "extra_field"} {
		t.Run(mutation, func(t *testing.T) {
			fact := map[string]string{}
			for key, value := range answer.Facts[0] {
				fact[key] = value
			}
			switch mutation {
			case "quote_only":
				fact["quote"] += "."
			case "unknown_source":
				fact["source_id"] = "d_00000000000000000000"
			case "prefixed_source":
				fact["source_id"] = "doc:" + fact["source_id"]
			case "normalized_units":
				fact["value"], fact["quote"] = "0.180 kilograms", "0.180 kilograms"
			case "extra_field":
				fact["approved"] = "true"
			}
			changed := e
			changed.Golden, _ = json.Marshal(map[string]any{"facts": []map[string]string{fact}})
			if freshAuthorGolden(changed) == nil {
				t.Fatal("altered or foreign original witness accepted")
			}
		})
	}
}

func TestCloudFreshV3AuthorFixedBoundaries(t *testing.T) {
	for _, mutation := range []string{"family", "command_part", "secret", "public_block", "comparison", "source_role_hint", "body_drift", "no_answer_lost", "no_answer_extra", "no_answer_null"} {
		t.Run(mutation, func(t *testing.T) {
			c := freshV3AuthorLoad(t)
			switch mutation {
			case "family":
				c.Groups[20].SemanticFamily = c.Groups[0].SemanticFamily
			case "command_part":
				c.Cases[102].Golden = json.RawMessage(`{"intent":"export_private","authority":"owner_required"}`)
			case "secret":
				value := false
				c.Cases[232].NoModel = &value
			case "public_block":
				value := true
				c.Cases[0].NoModel = &value
			case "comparison":
				c.Comparison[0] = "q321a"
			case "source_role_hint":
				c.Cases[0].Documents["orig301x"] = "renamed answer role"
			case "body_drift":
				for id := range c.Cases[0].Documents {
					c.Cases[0].Documents[id] += " changed"
					break
				}
			case "no_answer_lost":
				// Another real value from the same original is still unrelated to
				// the requested absent property; it cannot remove this branch.
				c.Cases[35].Golden = c.Cases[34].Golden
			case "no_answer_extra":
				c.Cases[2].Golden = json.RawMessage(`{"facts":[]}`)
			case "no_answer_null":
				c.Cases[35].Golden = json.RawMessage(`{"facts":null}`)
			}
			if freshV3AuthorValidate(c) == nil {
				t.Fatal("fixed corpus boundary mutation accepted")
			}
		})
	}
}

func TestCloudFreshV3AuthorLexicalSplitScreen(t *testing.T) {
	texts := freshV3Texts(freshV3AuthorLoad(t))
	for i, a := range texts {
		for _, b := range texts[:i] {
			if a.part != b.part && freshV3Near(a, b) {
				t.Fatalf("normalized cross-split text collision: %s %s", a.id, b.id)
			}
		}
	}
	old := freshV3TextOf("old", "", "", "Возобнови задачу task-old12 с сохранённого шага для «Карты Луны» в 17:30.")
	renamed := freshV3TextOf("new", "", "", "ВОЗОБНОВИ задачу task-new93 с сохранённого шага для «Карты Сада» в 08:20!")
	if !freshV3Near(old, renamed) {
		t.Fatal("renamed-entity/time template escaped lexical screen")
	}
}

func TestCloudFreshV3AuthorCrossOldCorpusScreen(t *testing.T) {
	texts := freshV3Texts(freshV3AuthorLoad(t))
	prior := []freshV3Text{}
	for _, name := range []string{"ru-scenarios-v1.json", "ru-scenarios-v2.json", "ru-scenarios-v3.json", "ru-cloud-fresh-v2.json"} {
		raw, err := os.ReadFile("../../testdata/acceptance/" + name)
		var object any
		if err != nil || json.Unmarshal(raw, &object) != nil {
			t.Fatal("old immutable corpus unavailable for reuse screen")
		}
		var walk func(any, string)
		walk = func(value any, field string) {
			switch v := value.(type) {
			case map[string]any:
				for key, child := range v {
					if field == "documents" {
						walk(child, "document")
					} else {
						walk(child, key)
					}
				}
			case []any:
				for _, child := range v {
					walk(child, field)
				}
			case string:
				if strings.Contains(" document question missing_question important quiet command ambiguous injection_document input ", " "+field+" ") {
					prior = append(prior, freshV3TextOf(name+":"+field, "", "", v))
				}
			}
		}
		walk(object, "")
	}
	for _, a := range texts {
		for _, b := range prior {
			if freshV3Near(a, b) {
				t.Fatalf("new text reuses old normalized scaffold: %s %s", a.id, b.id)
			}
		}
	}
}
