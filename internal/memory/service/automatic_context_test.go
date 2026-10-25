package service

import "testing"

func TestAutomaticContextOmitsUnrelatedNearestNeighbor(t *testing.T) {
	items := []map[string]any{{"title": "payment notice", "summary": "Банк: пополнение счёта"}, {"title": "Премьера", "summary": "Сезон сериала выходит в пятницу"}}
	selected, omitted := relevantAutomaticChunks(items, "Дата выхода сезона сериала")
	if omitted != 1 || len(selected) != 1 || selected[0]["title"] != "Премьера" {
		t.Fatal(selected, omitted)
	}
}
