// Package adaptive implements bounded retrieval choices and opened-evidence
// bookkeeping. A plan is never a source grant, task mandate or fact decision.
package adaptive

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const Version = "adaptive-retrieval-v1"

type Plan struct {
	Version          string   `json:"version"`
	Intent           string   `json:"intent"`
	Strategy         string   `json:"strategy"`
	QueryHash        string   `json:"query_sha256"`
	EventFrom        string   `json:"event_from,omitempty"`
	EventUntil       string   `json:"event_until,omitempty"`
	MaxCalls         int      `json:"max_calls"`
	MaxSearches      int      `json:"max_searches"`
	MaxResponseBytes int      `json:"max_response_bytes"`
	MaxMillis        int      `json:"max_millis"`
	Reasons          []string `json:"reasons"`
	// Suggested queries are transient current-input transformations. Trace and
	// database plan rows retain only their digest, never another owner-text copy.
	Queries []string `json:"-"`
}

func Digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

var identifier = regexp.MustCompile(`(?:source_[0-9a-f]{64}|message:[^\s"']{1,160}|[\pL][\pL\pN]*[._/#:-][\pL\pN._/#:-]*[\pL\pN]|[A-ZА-Я]{1,12}[0-9][A-ZА-Я0-9_-]{0,80})`)
var dates = regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}`)
var dateClock = regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}[tT ]+[0-9]{1,2}`)
var year = regexp.MustCompile(`\b(?:19|20)[0-9]{2}\b`)

func hasAny(s string, words ...string) bool {
	for _, word := range words {
		if strings.Contains(s, word) {
			return true
		}
	}
	return false
}

// Classify is intentionally a small transparent lexical policy. It neither
// calls a classifier nor claims semantic sufficiency. Current scoped accepted
// preferences, working memory, transcript and typed constraints stay outside
// this optional-search gate and are loaded through their existing guards.
func Classify(message string) Plan {
	p := Plan{Version: Version, Intent: "independent", Strategy: "none", QueryHash: Digest([]byte(message)), Reasons: []string{"no_optional_memory_intent"}, Queries: []string{}}
	q := strings.ToLower(strings.TrimSpace(message))
	terms := identifier.FindAllString(message, 4)
	recall := hasAny(q, "помни", "вспомни", "мои ", "моё ", "моей ", "моих ", "наши ", "нашем ", "наших ", "мы обсужд", "я говорил", "я просил", "я сохранял", "я запис", "из документа", "в документе", "из запис", "в записи", "прошлой бесед", "другой бесед", "истории переписки", "remember", "my notes", "our project", "previous conversation", "in the document", "from the document")
	exact := len(terms) > 0 && (recall || len(strings.Fields(message)) <= 4 || hasAny(q, "найди", "проверь", "открой", "ошибк", "идентификатор", "исходник", "find ", "lookup ", "look up "))
	if !recall && !exact {
		return p
	}
	p.Intent, p.Strategy, p.MaxCalls, p.MaxSearches = "recall", "semantic_hybrid", 4, 1
	p.MaxResponseBytes, p.MaxMillis = 65536, 120000
	p.Reasons, p.Queries = []string{"explicit_memory_intent"}, []string{strings.TrimSpace(message)}
	if exact {
		p.Intent, p.Strategy, p.Queries, p.Reasons = "exact_identifier", "catalog_exact", terms[:1], []string{"literal_identifier"}
	}
	if hasAny(q, "сопостав", "сравни", "связь между", "взаимосвяз", "compare ", "connect ", "relationship between") {
		p.Intent, p.MaxCalls, p.MaxSearches = "multi_hop", 8, 3
		p.Reasons = append(p.Reasons, "explicit_comparison_or_join")
	}
	if from, until, temporal, known := reportedWindow(q); temporal {
		p.Intent = "reported_time"
		p.Reasons = append(p.Reasons, "reported_event_time_only")
		if known {
			p.EventFrom, p.EventUntil = from, until
			p.Strategy = "catalog_exact"
		} else {
			p.Reasons = append(p.Reasons, "time_needs_clarification")
		}
	}
	if hasAny(q, "по состоянию на", "действовало", "было актуальн", "as of", "valid at") {
		// EventTime is not valid-time. #88 owns any future historical predicate.
		p.EventFrom, p.EventUntil = "", ""
		p.Reasons = append(p.Reasons, "valid_time_not_proven_by_event_time")
	}
	return p
}

func reportedWindow(q string) (string, string, bool, bool) {
	values := dates.FindAllString(q, 3)
	if len(values) > 0 {
		// This policy understands complete UTC days, not partial-day clocks,
		// exclusive boundaries, relative periods or arbitrary date prose.
		if len(values) > 2 || partialPeriod(q) || strings.Contains(q, ":") || dateClock.MatchString(q) {
			return "", "", true, false
		}
		if len(values) == 2 && !(strings.Contains(q, "с "+values[0]+" по "+values[1]) || strings.Contains(q, "from "+values[0]+" through "+values[1])) {
			return "", "", true, false
		}
		if len(values) == 1 && (strings.Contains(q, "с "+values[0]) || strings.Contains(q, "from "+values[0])) {
			return "", "", true, false
		}
		from, err := time.Parse("2006-01-02", values[0])
		if err != nil {
			return "", "", true, false
		}
		until := from.AddDate(0, 0, 1)
		if len(values) == 2 {
			last, err := time.Parse("2006-01-02", values[1])
			if err != nil || last.Before(from) {
				return "", "", true, false
			}
			until = last.AddDate(0, 0, 1)
		}
		return from.Format(time.RFC3339), until.Format(time.RFC3339), true, true
	}
	months := []string{"январ", "феврал", "март", "апрел", "ма", "июн", "июл", "август", "сентябр", "октябр", "ноябр", "декабр"}
	indices := namedMonths(q)
	for i := range indices {
		y := year.FindAllString(q, 2)
		if len(y) != 1 || len(indices) != 1 || partialPeriod(q) || strings.Contains(q, "с "+months[i]) {
			return "", "", true, false
		}
		// Any additional digit can be a day/day-range/time. Do not silently
		// widen it into the entire named month. Ambiguity remains explicit.
		withoutYear := strings.Replace(q, y[0], "", 1)
		if strings.IndexFunc(withoutYear, unicode.IsDigit) >= 0 {
			return "", "", true, false
		}
		from, err := time.Parse("2006-01-02", y[0]+"-01-01")
		if err != nil {
			return "", "", true, false
		}
		from = from.AddDate(0, i, 0)
		return from.Format(time.RFC3339), from.AddDate(0, 1, 0).Format(time.RFC3339), true, true
	}
	return "", "", false, false
}

// Only complete calendar words count. Substring stems would misclassify
// technical names such as смарт-контракт or a person named Мартин as March.
func namedMonths(q string) map[int]bool {
	forms := []string{
		"январь января январе январю январём январем",
		"февраль февраля феврале февралю февралём февралем",
		"март марта марте марту мартом",
		"апрель апреля апреле апрелю апрелем",
		"май мая мае маю маем",
		"июнь июня июне июню июнем",
		"июль июля июле июлю июлем",
		"август августа августе августу августом",
		"сентябрь сентября сентябре сентябрю сентябрём сентябрем",
		"октябрь октября октябре октябрю октябрём октябрем",
		"ноябрь ноября ноябре ноябрю ноябрём ноябрем",
		"декабрь декабря декабре декабрю декабрём декабрем",
	}
	indices := map[int]bool{}
	for _, word := range strings.FieldsFunc(q, func(r rune) bool { return !unicode.IsLetter(r) }) {
		for i, group := range forms {
			for _, form := range strings.Fields(group) {
				if word == form {
					indices[i] = true
				}
			}
		}
	}
	return indices
}

func partialPeriod(q string) bool {
	return hasAny(q, "до ", "после ", "раньше ", "позже ", "начиная ", "включ", "исключ", "половин", "начале ", "конце ", "середин", "недел", "before ", "after ", "since ", "until ", "except ")
}

func (p Plan) SearchEnabled() bool { return p.Strategy != "none" }

func (p Plan) Valid() bool {
	if p.Version != Version || len(p.QueryHash) != 64 || p.MaxResponseBytes < 0 || p.MaxResponseBytes > 65536 || p.MaxCalls < 0 || p.MaxCalls > 8 || p.MaxSearches < 0 || p.MaxSearches > 3 || p.MaxSearches > p.MaxCalls || p.MaxMillis < 0 || p.MaxMillis > 120000 {
		return false
	}
	if _, err := hex.DecodeString(p.QueryHash); err != nil {
		return false
	}
	if len(p.Reasons) < 1 || len(p.Reasons) > 5 {
		return false
	}
	for _, reason := range p.Reasons {
		switch reason {
		case "no_optional_memory_intent", "explicit_memory_intent", "literal_identifier", "explicit_comparison_or_join", "reported_event_time_only", "time_needs_clarification", "valid_time_not_proven_by_event_time":
		default:
			return false
		}
	}
	if p.EventFrom != "" || p.EventUntil != "" {
		from, e1 := time.Parse(time.RFC3339, p.EventFrom)
		until, e2 := time.Parse(time.RFC3339, p.EventUntil)
		if e1 != nil || e2 != nil || !from.Before(until) || p.Intent != "reported_time" || p.Strategy != "catalog_exact" {
			return false
		}
	}
	if !p.SearchEnabled() {
		return p.Intent == "independent" && p.MaxCalls == 0 && p.MaxSearches == 0 && p.MaxMillis == 0 && p.MaxResponseBytes == 0 && p.EventFrom == "" && p.EventUntil == ""
	}
	if p.Strategy != "catalog_exact" && p.Strategy != "semantic_hybrid" || p.MaxCalls < 1 || p.MaxSearches < 1 || p.MaxResponseBytes < 1 || p.MaxMillis < 1 {
		return false
	}
	switch p.Intent {
	case "recall", "exact_identifier", "reported_time", "multi_hop":
	default:
		return false
	}
	return true
}
