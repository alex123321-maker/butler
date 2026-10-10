package extraction

import (
	"strings"
	"testing"
	"time"
)

func TestRelativeTimeRequiresOriginalEventAndTrustedTimezone(t *testing.T) {
	at := time.Date(2026, 2, 28, 19, 30, 0, 0, time.UTC)
	for _, c := range []struct {
		raw, zone, date string
		event           *time.Time
	}{
		{"до конца месяца", "Asia/Tomsk", "2026-03-31", &at},
		{"завтра", "UTC", "2026-03-01", &at},
		{"через 2 дня", "UTC", "2026-03-02", &at},
		{"завтра", "UTC", "", nil},
		{"завтра", "", "", &at},
		{"когда-нибудь", "UTC", "", &at},
		{"2026-02-30", "UTC", "", &at},
	} {
		v := NormalizeTime(c.raw, c.event, c.zone)
		if v.Date != c.date || c.date == "" && v.Status != "unresolved" {
			t.Fatal(c, v)
		}
	}
}
func TestLongParagraphQuoteIsBoundedWithExactSourceOffsets(t *testing.T) {
	text := strings.Repeat("я", 10000) + " не удаляй server-a " + strings.Repeat("я", 10000)
	start := uint32(10004)
	quote, left, right, clipped := quotedClause(text, []Field{{Start: start, End: start + 6}})
	if !clipped || len(quote) > 8192 || quote != string([]rune(text)[left:right]) || !strings.Contains(quote, "не удаляй") {
		t.Fatal("bounded quote lost literal provenance")
	}
	q, l, r, c := quotedClause("До завтра не удаляй server-a", []Field{{Start: 13, End: 19}})
	if c || l != 0 || r != uint32(len([]rune(q))) || q != "До завтра не удаляй server-a" {
		t.Fatal("short restriction clipped", q, l, r, c)
	}
}
func TestLiteralCuesAreQuotedEvidenceWithoutAuthority(t *testing.T) {
	text := "До конца месяца не обновляй рабочий сервер, тестовый можно"
	cues := literalCues(text, 3)
	if !cues.Negated || !cues.Exception || cues.Conditional {
		t.Fatal(cues)
	}
	for _, f := range cues.Evidence {
		if string([]rune(text)[f.Start-3:f.End-3]) != f.Text {
			t.Fatal("cue offset invented")
		}
	}
	cues = literalCues("Раньше обновляли; теперь только после теста, если владелец согласен", 0)
	if !cues.Historical || !cues.Conditional {
		t.Fatal(cues)
	}
	if literalCues("Нева и конец", 0).Negated {
		t.Fatal("negation substring matched")
	}
}
func TestWindowOffsetsPreserveOriginalTextAndBoundTotalWork(t *testing.T) {
	text := ""
	for range 1500 {
		text += "🛠я"
	}
	w, e := windows(text)
	if e != nil || len(w) < 2 {
		t.Fatal(e)
	}
	for _, p := range w {
		if len(p.Text) > 1200 || string([]rune(text)[p.Start:p.Start+uint32(len([]rune(p.Text)))]) != p.Text {
			t.Fatal("window truncated Unicode")
		}
	}
}
