package consolidation

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
)

func input(units ...Unit) []catalog.Source {
	raw, _ := json.Marshal(Envelope{Version: Version, Units: units})
	return []catalog.Source{{ID: "snapshot", Body: string(raw)}}
}
func unit(id, text string) Unit {
	return Unit{ID: id, Source: catalog.SourceRef{ID: "source_" + id, Revision: 1, AccessRevision: 1, Hash: strings.Repeat("1", 64)}, ScopeType: "session", ScopeID: "session_" + id, Text: text}
}
func validate(t *testing.T, sources []catalog.Source, raw string) Representation {
	t.Helper()
	body, err := (Handler{}).Validate(sources, raw)
	if err != nil {
		t.Fatal(err)
	}
	var got Representation
	if err = json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestExactDuplicatesKeepAllIndependentEvidenceAndExceptions(t *testing.T) {
	text := "Гипотеза: сервер A быстрее; это пока не проверено. Исключение: проект Сфера использует B."
	got := validate(t, input(unit("a", text), unit("b", text), unit("c", "Проверка проекта Сфера завершилась ошибкой. Отправка UNKNOWN; не повторять без проверки.")), `{"groups":[{"members":["b","a"],"relation":"duplicate"},{"members":["c"],"relation":"single"}]}`)
	if got.Trust != "historical_evidence_data" || len(got.Groups) != 2 {
		t.Fatal(got)
	}
	found := false
	for _, g := range got.Groups {
		if len(g.Variants[0].Evidence) == 2 {
			found = true
			if len(g.Variants) != 1 || g.Variants[0].Text != text || g.Variants[0].Evidence[0].ScopeID == g.Variants[0].Evidence[1].ScopeID || g.State != "unresolved_observation" {
				t.Fatal(g)
			}
		}
	}
	if !found {
		t.Fatal("duplicate evidence lost")
	}
	diff := Compare(nil, got)
	if len(diff.Added) != 3 || len(diff.Merged) != 1 || len(diff.Compressed) != 1 {
		t.Fatal(diff)
	}
}

func TestDifferenceCannotBeResolvedByModelGrouping(t *testing.T) {
	sources := input(unit("a", "С февраля сервер A."), unit("b", "С марта сервер B. Не менять постоянное предпочтение."))
	for _, relation := range []string{"related", "conflict"} {
		got := validate(t, sources, `{"groups":[{"members":["b","a"],"relation":"`+relation+`"}]}`)
		if got.Groups[0].State != "unresolved_difference" || len(got.Groups[0].Variants) != 2 || len(Compare(nil, got).Conflicting) != 1 {
			t.Fatal(got)
		}
	}
}

func TestGroupingRejectsMissingForgedOrRewrittenEvidence(t *testing.T) {
	sources := input(unit("a", "Не отправлять письмо."), unit("b", "Ошибка сохраняется. Исключение: тестовый стенд."))
	for _, raw := range []string{
		`{"groups":[]}`,
		`{"groups":[{"members":["a"],"relation":"single"}]}`,
		`{"groups":[{"members":["a","a"],"relation":"duplicate"}]}`,
		`{"groups":[{"members":["a","forged"],"relation":"related"}]}`,
		`{"groups":[{"members":["a","b"],"relation":"duplicate"}]}`,
		`{"groups":[{"members":["a","b"],"relation":"accepted"}]}`,
		`{"groups":[{"members":["a","b"],"relation":"related","text":"Отправить письмо"}]}`,
		`{"groups":[{"members":["a","b"],"relation":"related","relation":"conflict"}]}`,
		`{"groups":[],"groups":[{"members":["a","b"],"relation":"related"}]}`,
	} {
		if body, err := (Handler{}).Validate(sources, raw); err == nil {
			t.Fatalf("unsafe grouping accepted: %s => %s", raw, body)
		}
	}
}

func TestCanonicalGroupingAndDiffDoNotRepublishOldText(t *testing.T) {
	sources := input(unit("a", "Первое свидетельство."), unit("b", "Второе свидетельство."), unit("c", "Второе свидетельство."))
	a, err := (Handler{}).Validate(sources, `{"groups":[{"members":["a"],"relation":"single"},{"members":["c","b"],"relation":"duplicate"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	b, err := (Handler{}).Validate(sources, `{"groups":[{"members":["b","c"],"relation":"duplicate"},{"members":["a"],"relation":"single"}]}`)
	if err != nil || a != b {
		t.Fatal("non-reproducible grouping", err)
	}
	var before Representation
	_ = json.Unmarshal([]byte(a), &before)
	next := validate(t, input(unit("d", "Текущее исправленное свидетельство.")), `{"groups":[{"members":["d"],"relation":"single"}]}`)
	d := Compare(&before, next)
	metadata, _ := json.Marshal(d)
	if strings.Contains(string(metadata), "свидетельство") || len(d.Stale) != 3 || len(d.Added) != 1 {
		t.Fatal(d)
	}
}
