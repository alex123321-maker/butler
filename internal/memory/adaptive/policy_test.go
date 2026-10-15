package adaptive

import "testing"

func TestTransparentIntentAndReportedTimeNeverCreatesHistoricalValidity(t *testing.T) {
	for _, tc := range []struct{ input, intent, strategy, from, until string }{
		{"Сколько будет 2 + 2?", "independent", "none", "", ""},
		{"Объясни разницу HTTP и HTTPS", "independent", "none", "", ""},
		{"Напиши короткое поздравление", "independent", "none", "", ""},
		{"Найди ошибку E104", "exact_identifier", "catalog_exact", "", ""},
		{"Вспомни, почему мы выбрали эту архитектуру", "recall", "semantic_hybrid", "", ""},
		{"Сравни наши записи о двух проектах", "multi_hop", "semantic_hybrid", "", ""},
		{"Что мы обсуждали в сентябре 2025?", "reported_time", "catalog_exact", "2025-09-01T00:00:00Z", "2025-10-01T00:00:00Z"},
		{"Вспомни события с 2025-09-20 по 2025-09-22", "reported_time", "catalog_exact", "2025-09-20T00:00:00Z", "2025-09-23T00:00:00Z"},
		{"Вспомни наши записи в сентябре", "reported_time", "semantic_hybrid", "", ""},
		{"Вспомни, что действовало по состоянию на 2025-09-20", "reported_time", "catalog_exact", "", ""},
	} {
		p := Classify(tc.input)
		if !p.Valid() || p.Intent != tc.intent || p.Strategy != tc.strategy || p.EventFrom != tc.from || p.EventUntil != tc.until {
			t.Fatalf("%q: %+v", tc.input, p)
		}
	}
}

func TestReportedWindowRejectsPartialOrExclusiveDateSemantics(t *testing.T) {
	for _, q := range []string{"Вспомни записи с 5 по 10 сентября 2025", "Вспомни записи до сентября 2025", "Вспомни записи после 2025-09-01", "Вспомни записи в начале сентября 2025", "Вспомни записи 2025-09-01T14:25:00Z", "Вспомни записи с сентября 2025", "Вспомни записи с 2025-09-01 до 2025-09-10", "Вспомни записи между 2025-09-01 и 2025-09-10"} {
		p := Classify(q)
		if p.EventFrom != "" || p.EventUntil != "" {
			t.Fatalf("widened partial date %q: %+v", q, p)
		}
		found := false
		for _, r := range p.Reasons {
			found = found || r == "time_needs_clarification"
		}
		if !found {
			t.Fatalf("missing explicit uncertainty %q: %+v", q, p)
		}
	}
}

func TestReportedMonthRequiresCalendarWordNotTechnicalName(t *testing.T) {
	for _, q := range []string{"Вспомни наш смарт-контракт 2025", "Вспомни записи проекта Мартин за 2025"} {
		p := Classify(q)
		if p.Intent == "reported_time" || p.EventFrom != "" || p.EventUntil != "" {
			t.Fatalf("invented calendar from technical name %q: %+v", q, p)
		}
	}
	for _, q := range []string{"Вспомни записи в мае 2025", "Вспомни записи за май 2025"} {
		p := Classify(q)
		if p.EventFrom != "2025-05-01T00:00:00Z" || p.EventUntil != "2025-06-01T00:00:00Z" {
			t.Fatalf("month inflection %q: %+v", q, p)
		}
	}
}
