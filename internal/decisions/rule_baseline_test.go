package decisions

import (
	"encoding/json"
	"os"
	"testing"
)

func TestFrozenRuleBaselineV2(t *testing.T) {
	cases, _ := readTaxonomyEvents(t, "test")
	roles := map[string]string{"owner": "владелец", "document": "внешний документ", "page": "внешняя веб-страница", "retrieved_memory": "извлечённая запись памяти", "verifier": "система проверки", "executor": "исполнитель", "monitor": "мониторинг", "memory_catalog": "каталог памяти", "tool": "HTTP-инструмент"}
	m := &metrics{}
	var rows []map[string]string
	for _, c := range cases {
		a := c.annotated()
		a.Origin = roles[c.SourceRole]
		label := literal(a)
		m.add(a, label, false, "", 0, Observation{}, false)
		rows = append(rows, map[string]string{"case": c.ID, "gold": c.Gold, "prediction": label})
	}
	path := os.Getenv("BUTLER_TEST_RULE_REPORT")
	if path == "" {
		return
	}
	raw, _ := json.MarshalIndent(map[string]any{"baseline": "unchanged literal rules from the original experiment; no tuning on v2 events", "totals": m, "cases": rows}, "", "  ")
	if e := os.WriteFile(path, raw, 0644); e != nil {
		t.Fatal(e)
	}
}
