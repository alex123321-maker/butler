package acceptance

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"
)

// Author-side checks deliberately do not call the candidate V2 scorer. The
// independent reviewer owns the production parser, criteria and approval.
type freshAuthorGroup struct {
	ID             string `json:"id"`
	Split          string `json:"split"`
	SemanticFamily string `json:"semantic_family"`
	AttackFamily   string `json:"attack_family"`
	Provenance     string `json:"provenance"`
}

type freshAuthorCase struct {
	ID        string            `json:"id"`
	Group     string            `json:"group"`
	Kind      string            `json:"kind"`
	Input     string            `json:"input"`
	Documents map[string]string `json:"documents"`
	Golden    json.RawMessage   `json:"golden"`
	NoModel   *bool             `json:"no_model"`
}

type freshAuthorCorpus struct {
	Version    string             `json:"version"`
	Groups     []freshAuthorGroup `json:"groups"`
	Comparison []string           `json:"comparison_case_ids"`
	Cases      []freshAuthorCase  `json:"cases"`
}

func freshAuthorDecode(raw []byte, out any) error {
	if err := freshAuthorUniqueKeys(json.NewDecoder(bytes.NewReader(raw))); err != nil {
		return err
	}
	if err := freshAuthorShape(raw, reflect.TypeOf(out).Elem()); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return errors.New("corpus schema denied")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("corpus trailing data")
	}
	return nil
}

// encoding/json accepts case aliases by default. Candidate fixture fields are
// exact, all required, and recursively checked before typed decoding.
func freshAuthorShape(raw []byte, typ reflect.Type) error {
	if typ == reflect.TypeOf(json.RawMessage{}) {
		return nil
	}
	if typ.Kind() == reflect.Pointer {
		return freshAuthorShape(raw, typ.Elem())
	}
	switch typ.Kind() {
	case reflect.Struct:
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil || len(fields) != typ.NumField() {
			return errors.New("corpus exact fields denied")
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			value, ok := fields[field.Tag.Get("json")]
			if !ok {
				return errors.New("corpus field alias denied")
			}
			if err := freshAuthorShape(value, field.Type); err != nil {
				return err
			}
		}
	case reflect.Slice:
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil || values == nil {
			return errors.New("corpus array required")
		}
		for _, value := range values {
			if err := freshAuthorShape(value, typ.Elem()); err != nil {
				return err
			}
		}
	case reflect.Map:
		var values map[string]json.RawMessage
		if json.Unmarshal(raw, &values) != nil || values == nil {
			return errors.New("corpus map required")
		}
		for _, value := range values {
			if err := freshAuthorShape(value, typ.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

func freshAuthorUniqueKeys(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return errors.New("corpus JSON denied")
	}
	delim, container := token.(json.Delim)
	if !container {
		return nil
	}
	seen := map[string]bool{}
	for d.More() {
		if delim == '{' {
			key, err := d.Token()
			if err != nil {
				return errors.New("corpus key denied")
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("corpus duplicate key")
			}
			seen[name] = true
		}
		if err := freshAuthorUniqueKeys(d); err != nil {
			return err
		}
	}
	end, err := d.Token()
	if err != nil || (delim == '{' && end != json.Delim('}')) || (delim == '[' && end != json.Delim(']')) {
		return errors.New("corpus container denied")
	}
	return nil
}

func freshAuthorLoad(t *testing.T) freshAuthorCorpus {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/acceptance/ru-cloud-fresh-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var c freshAuthorCorpus
	if err = freshAuthorDecode(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func freshAuthorValidate(c freshAuthorCorpus) error {
	if c.Version != "butler-ru-cloud-fresh-v2" || len(c.Groups) != 30 || len(c.Cases) != 240 {
		return errors.New("corpus fixed cardinality denied")
	}
	groups, families := map[string]freshAuthorGroup{}, map[string]string{}
	for _, g := range c.Groups {
		if g.ID == "" || groups[g.ID].ID != "" || g.SemanticFamily == "" || g.AttackFamily == "" || (g.Provenance != "synthetic_public" && g.Provenance != "synthetic_secret") {
			return errors.New("corpus group denied")
		}
		if g.Split != "development" && g.Split != "tuning" && g.Split != "holdout" {
			return errors.New("corpus split denied")
		}
		for _, family := range []string{"semantic:" + g.SemanticFamily, "mechanism:" + g.AttackFamily} {
			if old, ok := families[family]; ok && old != g.Split {
				return errors.New("corpus family crossed split")
			}
			families[family] = g.Split
		}
		groups[g.ID] = g
	}
	ids, counts, blocked, perGroup := map[string]freshAuthorCase{}, map[string]int{}, map[string]int{}, map[string]map[string]int{}
	for _, e := range c.Cases {
		g, ok := groups[e.Group]
		if !ok || e.ID == "" || ids[e.ID].ID != "" || e.Input == "" || e.Documents == nil || e.NoModel == nil {
			return errors.New("corpus case denied")
		}
		if perGroup[e.Group] == nil {
			perGroup[e.Group] = map[string]int{}
		}
		perGroup[e.Group][e.Kind]++
		counts[g.Split]++
		secret := g.Provenance == "synthetic_secret" && len(e.Documents) > 0
		if *e.NoModel != secret {
			return errors.New("corpus secret export boundary denied")
		}
		if secret {
			blocked[g.Split]++
		}
		if err := freshAuthorGolden(e); err != nil {
			return err
		}
		if e.Kind == "commands" {
			var answer struct {
				Intent    string `json:"intent"`
				Authority string `json:"authority"`
			}
			if err := freshAuthorDecode(e.Golden, &answer); err != nil {
				return err
			}
			if answer.Intent != "clarify" && freshAuthorIntentPart(answer.Intent) != g.Split {
				return errors.New("corpus positive intent family crossed split")
			}
		}
		ids[e.ID] = e
	}
	for _, kinds := range perGroup {
		if len(kinds) != 4 {
			return errors.New("corpus group kind count denied")
		}
		for _, kind := range []string{"retrieval", "extraction", "classification", "commands"} {
			if kinds[kind] != 2 {
				return errors.New("corpus group balance denied")
			}
		}
	}
	if len(perGroup) != 30 || counts["development"] != 96 || counts["tuning"] != 64 || counts["holdout"] != 80 || blocked["development"] != 0 || blocked["tuning"] != 0 || blocked["holdout"] != 4 {
		return errors.New("corpus fixed split denied")
	}
	wanted := []string{"q201a", "q202c", "q203e", "q204g"}
	wantedKinds := []string{"retrieval", "extraction", "classification", "commands"}
	if len(c.Comparison) != len(wanted) {
		return errors.New("corpus comparison denied")
	}
	for i, id := range wanted {
		selected, exists := ids[id]
		if c.Comparison[i] != id || !exists || selected.Kind != wantedKinds[i] || groups[selected.Group].Split != "development" || selected.NoModel == nil || *selected.NoModel {
			return errors.New("corpus comparison identity denied")
		}
	}
	return freshAuthorLeakage(c, groups)
}

func freshAuthorIntentPart(intent string) string {
	for split, list := range map[string]string{
		"development": "stop_tree pause_task prepare_only correct_preference restrict_budget amend_prepare_only",
		"tuning":      "revoke_extension correct_source restrict_privacy export_private delete_source restrict_scope",
		"holdout":     "revoke_mandate suspend_initiative restrict_resource require_evidence reconcile_only",
	} {
		for _, value := range strings.Fields(list) {
			if intent == value {
				return split
			}
		}
	}
	return ""
}

func freshAuthorGolden(e freshAuthorCase) error {
	for id, original := range e.Documents {
		if id == "" || original == "" {
			return errors.New("corpus original denied")
		}
	}
	switch e.Kind {
	case "retrieval":
		var answer struct {
			Refs []string `json:"refs"`
		}
		if freshAuthorDecode(e.Golden, &answer) != nil || answer.Refs == nil || !sort.StringsAreSorted(answer.Refs) {
			return errors.New("corpus retrieval golden denied")
		}
		for i, id := range answer.Refs {
			if e.Documents[id] == "" || (i > 0 && id == answer.Refs[i-1]) {
				return errors.New("corpus retrieval witness denied")
			}
		}
	case "extraction":
		var answer struct {
			Facts []struct {
				Key      string `json:"key"`
				Value    string `json:"value"`
				SourceID string `json:"source_id"`
				Quote    string `json:"quote"`
			} `json:"facts"`
		}
		if freshAuthorDecode(e.Golden, &answer) != nil || answer.Facts == nil {
			return errors.New("corpus extraction golden denied")
		}
		seen := map[string]bool{}
		for _, fact := range answer.Facts {
			if fact.Key == "" || fact.Value == "" || fact.Value != fact.Quote || !strings.Contains(e.Documents[fact.SourceID], fact.Quote) || seen[fact.Key] {
				return errors.New("corpus exact original witness denied")
			}
			seen[fact.Key] = true
		}
	case "classification":
		var answer struct {
			Label string `json:"label"`
		}
		if freshAuthorDecode(e.Golden, &answer) != nil || (answer.Label != "important" && answer.Label != "observe") {
			return errors.New("corpus classification golden denied")
		}
	case "commands":
		var answer struct {
			Intent    string `json:"intent"`
			Authority string `json:"authority"`
		}
		intents := strings.Fields("stop_tree pause_task amend_prepare_only prepare_only delete_source restrict_scope correct_preference restrict_budget export_private revoke_extension require_evidence correct_source restrict_privacy revoke_mandate reconcile_only suspend_initiative restrict_resource clarify")
		if freshAuthorDecode(e.Golden, &answer) != nil {
			return errors.New("corpus command schema denied")
		}
		known := false
		for _, intent := range intents {
			known = known || intent == answer.Intent
		}
		if !known || (answer.Intent == "clarify" && answer.Authority != "none") || (answer.Intent != "clarify" && answer.Authority != "owner_required") {
			return errors.New("corpus command authority denied")
		}
	default:
		return errors.New("corpus kind denied")
	}
	return nil
}

var freshOpaque = regexp.MustCompile(`(?i)\b(?:task|source|src|group|case|q|g)[-_]?[a-z0-9_-]+\b|[А-ЯЁA-Z]{1,4}-[0-9]+`)
var freshQuoted = regexp.MustCompile(`«[^»]+»|"[^"]+"`)
var freshNumbers = regexp.MustCompile(`[0-9]+(?:[.,:][0-9]+)*`)

func freshAuthorNormalized(s string) string {
	s = freshOpaque.ReplaceAllString(s, " object ")
	s = freshQuoted.ReplaceAllString(s, " quoted ")
	s = freshNumbers.ReplaceAllString(s, " number ")
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }), " ")
}

func freshAuthorNear(a, b string) bool {
	x, y := freshAuthorNormalized(a), freshAuthorNormalized(b)
	if x == y {
		return true
	}
	grams := func(s string) map[string]bool {
		words, out := strings.Fields(s), map[string]bool{}
		for i := 0; i+2 < len(words); i++ {
			out[strings.Join(words[i:i+3], " ")] = true
		}
		return out
	}
	gx, gy := grams(x), grams(y)
	if len(gx) == 0 || len(gy) == 0 {
		return false
	}
	intersection := 0
	for gram := range gx {
		if gy[gram] {
			intersection++
		}
	}
	return float64(intersection)/float64(len(gx)+len(gy)-intersection) >= .85
}

func freshAuthorLeakage(c freshAuthorCorpus, groups map[string]freshAuthorGroup) error {
	for i, e := range c.Cases {
		for _, prior := range c.Cases[:i] {
			if groups[e.Group].Split == groups[prior.Group].Split {
				continue
			}
			if freshAuthorNear(e.Input, prior.Input) {
				return errors.New("corpus input lexical split leak")
			}
			for _, a := range e.Documents {
				for _, b := range prior.Documents {
					if freshAuthorNear(a, b) {
						return errors.New("corpus original lexical split leak")
					}
				}
			}
		}
	}
	return nil
}

func TestCloudFreshV2CorpusAuthorship(t *testing.T) {
	if err := freshAuthorValidate(freshAuthorLoad(t)); err != nil {
		t.Fatal(err)
	}
}

func TestCloudFreshV2CorpusExactWitness(t *testing.T) {
	c := freshAuthorLoad(t)
	for _, raw := range []string{
		`{"facts":[{"key":"адрес выдачи","value":"док № 4","source_id":"src201x","quote":"Адрес выдачи: док № 4"}]}`,
		`{"facts":[{"key":"адрес выдачи","value":"док № 4 ","source_id":"src201x","quote":"док № 4 "}]}`,
		`{"facts":[{"key":"адрес выдачи","value":"док № 4","source_id":"src999x","quote":"док № 4"}]}`,
		`{"facts":[{"key":"адрес выдачи","value":"док № 4","source_id":"src201x","quote":"док № 4","approved":true}]}`,
		`{"facts":[{"Key":"адрес выдачи","value":"док № 4","source_id":"src201x","quote":"док № 4"}]}`,
	} {
		e := c.Cases[2]
		e.Golden = json.RawMessage(raw)
		if freshAuthorGolden(e) == nil {
			t.Fatal("altered witness was accepted")
		}
	}
}

func TestCloudFreshV2CorpusDuplicateAndSchema(t *testing.T) {
	for _, raw := range []string{
		`{"version":"a","version":"b"}`,
		`{"documents":{"src":"one","src":"two"}}`,
		`{"golden":{"facts":[{"key":"a","key":"b"}]}}`,
		`{"golden":{"facts":[{"key":"a","\u006bey":"b"}]}}`,
		`{"version":"x","unknown":true}`,
		`{} {}`,
	} {
		var c freshAuthorCorpus
		if freshAuthorDecode([]byte(raw), &c) == nil {
			t.Fatal("duplicate/unknown/trailing schema accepted")
		}
	}
}

func TestCloudFreshV2CorpusLexicalLeakage(t *testing.T) {
	if !freshAuthorNear("Проверь срок поручения task-aa11 для «Проекта Берег» в 18:20 и ничего не отправляй.", "ПРОВЕРЬ срок поручения task-bb22 для «Проекта Остров» в 09:50; и ничего не отправляй!") {
		t.Fatal("renamed entity/number scaffold escaped screening")
	}
	if freshAuthorNear("Контрольная ванна имеет поправку прибора минус три десятых градуса.", "Курьер принял конверт в северном отделении, квитанция ещё не выдана.") {
		t.Fatal("unrelated originals collapsed")
	}
	c := freshAuthorLoad(t)
	c.Cases[160].Input = c.Cases[0].Input
	if freshAuthorValidate(c) == nil {
		t.Fatal("cross-split copied input accepted")
	}
}

func TestCloudFreshV2CorpusBoundaries(t *testing.T) {
	for _, mutation := range []string{"family", "positive_intent", "secret", "missing_field", "comparison", "split"} {
		t.Run(mutation, func(t *testing.T) {
			c := freshAuthorLoad(t)
			switch mutation {
			case "family":
				c.Groups[20].SemanticFamily = c.Groups[0].SemanticFamily
			case "positive_intent":
				c.Cases[166].Golden = json.RawMessage(`{"intent":"stop_tree","authority":"owner_required"}`)
			case "secret":
				allowed := false
				c.Cases[232].NoModel = &allowed
			case "missing_field":
				c.Cases[0].Documents = nil
			case "comparison":
				c.Comparison[0] = "q221a"
			case "split":
				c.Groups[0].Split = "tuning"
			}
			if freshAuthorValidate(c) == nil {
				t.Fatal("fixed corpus boundary accepted mutation")
			}
		})
	}
}
