package service

import (
	"encoding/json"
	"fmt"
	"github.com/butler/butler/internal/memory/diagnostics"
	"sort"
	"strings"
	"unicode"
)

// One allowance unit per encoded UTF-8 byte, plus item/section overhead. These
// conservative units are not the provider tokenizer or invoice. This optional
// memory allowance is inside the orchestrator's complete-context admission.
const SelectionVersion = "memory-context-v3"

type SelectionDiagnostics struct {
	Operations        []OperationObservation `json:"operations,omitempty"`
	Version           string                 `json:"version"`
	Estimator         string                 `json:"estimator"`
	Allowance         int                    `json:"allowance"`
	Estimated         int                    `json:"estimated"`
	OptionalAllowance int                    `json:"optional_allowance"`
	OptionalEstimated int                    `json:"optional_estimated"`
	Selected          map[string]int         `json:"selected"`
	Omitted           map[string]int         `json:"omitted"`
	Retrieval         []string               `json:"retrieval,omitempty"`
}

func encodedUnits(v any) int {
	b, err := json.Marshal(v)
	if err != nil {
		return int(^uint(0) >> 1)
	}
	return len(b) + 32
}

func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
}

func relevance(key, text, query string) int {
	score := 0
	terms := words(key + " " + text)
	for _, q := range words(query) {
		if len([]rune(q)) < 3 {
			continue
		}
		switch q {
		case "для", "как", "что", "это", "при", "или", "где", "мой", "моя", "моё", "мои", "мне", "the", "and", "for", "with", "what", "where":
			continue
		}
		for _, term := range terms {
			if term == q {
				score += 4
				break
			}
			// A small language-neutral prefix match helps Russian inflections;
			// this is an explicit lexical heuristic, not semantic certainty.
			a, b := []rune(term), []rune(q)
			if len(a) >= 5 && len(b) >= 5 && string(a[:5]) == string(b[:5]) {
				score++
				break
			}
		}
	}
	return score
}

func relevantProfiles(entries []map[string]any, query string, limit int) []map[string]any {
	return relevantProfilesForTask(entries, query, "", limit)
}

func relevantProfilesForTask(entries []map[string]any, query, taskGoal string, limit int) []map[string]any {
	type ranked struct {
		item         map[string]any
		score, scope int
		key          string
		goalScore    int
	}
	var rankedEntries []ranked
	scopes := map[string]int{}
	for _, item := range entries {
		key, _ := item["key"].(string)
		text, _ := item["summary"].(string)
		scope := fmt.Sprint(item["scope_type"]) + ":" + fmt.Sprint(item["scope_id"])
		if _, ok := scopes[scope]; !ok {
			scopes[scope] = len(scopes)
		}
		score := relevance(key, text, query)
		// Always-applicable presentation preferences remain eligible even when
		// the question does not literally contain "language" or "style".
		if strings.TrimSpace(query) == "" || generalProfileKey(key) {
			score++
		}
		// No lexical overlap is not proof of inapplicability. Keep these
		// already authorized entries behind matches, within the profile quota.
		rankedEntries = append(rankedEntries, ranked{item, score, scopes[scope], key, relevance(key, text, taskGoal)})
	}
	sort.SliceStable(rankedEntries, func(i, j int) bool {
		a, b := rankedEntries[i], rankedEntries[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.goalScore != b.goalScore {
			return a.goalScore > b.goalScore
		}
		if a.scope != b.scope {
			return a.scope < b.scope
		}
		if a.key != b.key {
			return a.key < b.key
		}
		return fmt.Sprint(a.item["summary"]) < fmt.Sprint(b.item["summary"])
	})
	result := []map[string]any{}
	for _, entry := range rankedEntries {
		if len(result) == limit {
			break
		}
		result = append(result, entry.item)
	}
	return result
}

func generalProfileKey(key string) bool {
	switch key {
	case "language", "style", "response_language", "response_style", "name", "timezone", "units", "accessibility":
		return true
	}
	return false
}

func contextItemIdentity(class string, item map[string]any) string {
	var identity any
	switch class {
	case "profile":
		identity = []any{class, item["scope_type"], item["scope_id"], item["key"], item["summary"], item["origin"]}
	case "chunks":
		ref, _ := item["source_ref"].(string)
		if ref == "" {
			return "" // Text alone cannot establish document identity.
		}
		identity = []any{class, ref, item["source_revision"], item["title"], item["summary"], item["origin"]}
	default:
		// Episode retrieval already deduplicates its results. A summary alone
		// cannot prove that entries from different memory classes are copies.
		return ""
	}
	b, err := json.Marshal(identity)
	if err != nil {
		return ""
	}
	return string(b)
}

func (s *Service) selectContext(summary string, working WorkingContext, profile, episodes, chunks []map[string]any) (map[string]any, []map[string]any, []map[string]any, []map[string]any, SelectionDiagnostics, error) {
	d := SelectionDiagnostics{Version: SelectionVersion, Estimator: "utf8_json_bytes_plus_item_overhead", Allowance: s.config.ContextBudgetTokens, Selected: map[string]int{}, Omitted: map[string]int{}}
	d.OptionalAllowance = min(8192, d.Allowance)
	items := map[string]any{}
	count := 0
	if strings.TrimSpace(summary) != "" {
		items["session_summary"] = summary
		count++
	}
	if !working.IsEmpty() {
		items["working"] = working.BundleMap()
		count++
	}
	used := encodedUnits(items) + 128
	if used > d.Allowance {
		return nil, nil, nil, nil, d, fmt.Errorf("%w: %d > %d", diagnostics.ErrContextBudget, used, d.Allowance)
	}
	classes := []string{"chunks", "profile", "episodes"}
	candidates := map[string][]map[string]any{"chunks": chunks, "profile": profile, "episodes": episodes}
	selected := map[string][]map[string]any{}
	classUsed := map[string]int{}
	seen := map[string]bool{}
	for more := true; more; {
		more = false
		for _, class := range classes {
			if len(candidates[class]) == 0 {
				continue
			}
			more = true
			item := candidates[class][0]
			candidates[class] = candidates[class][1:]
			identity := contextItemIdentity(class, item)
			if identity != "" && seen[identity] {
				d.Omitted["duplicate"]++
				continue
			}
			units := encodedUnits(item) + 32
			if count >= s.config.BundleBudget {
				d.Omitted["item_limit"]++
				continue
			}
			if len(chunks) > 0 && class != "chunks" && classUsed[class]+units > d.OptionalAllowance/3 {
				d.Omitted["class_quota"]++
				continue
			}
			if used+units > d.Allowance || d.OptionalEstimated+units > d.OptionalAllowance {
				d.Omitted["context_limit"]++
				continue
			}
			selected[class] = append(selected[class], item)
			classUsed[class] += units
			used += units
			d.OptionalEstimated += units
			count++
			if identity != "" {
				seen[identity] = true
			}
		}
	}
	for _, class := range classes {
		if len(selected[class]) > 0 {
			items[class] = selected[class]
		}
		d.Selected[class] = len(selected[class])
	}
	d.Estimated = used
	return items, selected["profile"], selected["episodes"], selected["chunks"], d, nil
}
