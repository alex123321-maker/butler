package extraction

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var cuesPatterns = []struct {
	label string
	re    *regexp.Regexp
}{
	{"negation", regexp.MustCompile(`(?i)(^|[^\pL])(не|нельзя|запрещено|запрещаю|не надо)([^\pL]|$)`)},
	{"condition", regexp.MustCompile(`(?i)(^|[^\pL])(если|при условии|только после)([^\pL]|$)`)},
	{"historical", regexp.MustCompile(`(?i)(^|[^\pL])(раньше|ранее|прежде)([^\pL]|$)`)},
	{"exception", regexp.MustCompile(`(?i)(^|[^\pL])(кроме|за исключением|можно|но)([^\pL]|$)`)},
}
var days = regexp.MustCompile(`^через ([1-9][0-9]?) (день|дня|дней)$`)

// NormalizeTime interprets only a small pinned grammar. Missing event time or
// trusted timezone stays unresolved. Neither ingestion nor wall clock is a base.
func NormalizeTime(raw string, event *time.Time, zone string) TimeValue {
	v := TimeValue{Raw: raw, Status: "unresolved"}
	phrase := strings.ToLower(strings.TrimSpace(raw))
	if d, e := time.Parse("2006-01-02", phrase); e == nil {
		v.Status = "date"
		v.Date = d.Format("2006-01-02")
		v.Basis = "explicit_iso_date"
		return v
	}
	if event == nil || zone == "" {
		return v
	}
	loc, e := time.LoadLocation(zone)
	if e != nil {
		return v
	}
	at := event.In(loc)
	var d time.Time
	switch phrase {
	case "завтра", "до завтра":
		d = at.AddDate(0, 0, 1)
	case "послезавтра", "до послезавтра":
		d = at.AddDate(0, 0, 2)
	case "сегодня", "до конца дня":
		d = at
	case "до конца месяца", "к концу месяца", "конец месяца":
		d = time.Date(at.Year(), at.Month()+1, 0, 0, 0, 0, 0, loc)
	default:
		m := days.FindStringSubmatch(phrase)
		if m == nil {
			return v
		}
		n, _ := strconv.Atoi(m[1])
		d = at.AddDate(0, 0, n)
	}
	v.Status = "date"
	v.Date = d.Format("2006-01-02")
	v.Zone = zone
	v.Basis = "source_event_time"
	return v
}

func literalCues(quote string, base uint32) Cues {
	c := Cues{Scope: "quoted_fragment"}
	for _, p := range cuesPatterns {
		for _, pos := range p.re.FindAllStringSubmatchIndex(quote, -1) {
			// Capturing group2 is the literal cue, excluding boundary characters.
			start, end := pos[4], pos[5]
			c.Evidence = append(c.Evidence, Field{Type: p.label, Text: quote[start:end], Start: base + uint32(utf8.RuneCountInString(quote[:start])), End: base + uint32(utf8.RuneCountInString(quote[:end])), Method: "literal_rule"})
			switch p.label {
			case "negation":
				c.Negated = true
			case "condition":
				c.Conditional = true
			case "historical":
				c.Historical = true
			case "exception":
				c.Exception = true
			}
		}
	}
	return c
}

func quotedClause(text string, fields []Field) (string, uint32, uint32, bool) {
	r := []rune(text)
	start, end := int(fields[0].Start), int(fields[0].End)
	for _, f := range fields {
		start = min(start, int(f.Start))
		end = max(end, int(f.End))
	}
	// The spans in one validated IPC window total at most1200 UTF-8 bytes.
	// Keep bounded literal context even for a source with a megabyte paragraph.
	// Clipping is explicit: absence of a cue never means affirmative permission.
	left, right := max(0, start-128), min(len(r), end+128)
	for start > left && !strings.ContainsRune(".!?\n", r[start-1]) {
		start--
	}
	for end < right && !strings.ContainsRune(".!?\n", r[end]) {
		end++
	}
	clipped := start > 0 && !strings.ContainsRune(".!?\n", r[start-1]) || end < len(r) && !strings.ContainsRune(".!?\n", r[end])
	return string(r[start:end]), uint32(start), uint32(end), clipped
}
