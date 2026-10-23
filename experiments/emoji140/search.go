// Offline exact lexical demo over explicit AI annotations; not evaluated retrieval.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"
)

type Annotation struct {
	ID       string `json:"id"`
	Observed struct {
		Subjects   []string `json:"subjects"`
		Actions    []string `json:"actions"`
		Expression *string  `json:"expression"`
		Text       *string  `json:"text"`
		Style      *string  `json:"style"`
	} `json:"observed"`
	Inferred struct {
		Meanings []string `json:"possible_meanings"`
	} `json:"inferred"`
}
type result struct {
	ID       string   `json:"id"`
	Score    float64  `json:"score"`
	Subjects []string `json:"subjects"`
	Evidence string   `json:"evidence"`
}

func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}
func rank(q string, rows []Annotation, k int) []result {
	terms := words(q)
	out := []result{}
	for _, r := range rows {
		content := strings.Join(append(append(append([]string{}, r.Observed.Subjects...), r.Observed.Actions...), r.Inferred.Meanings...), " ")
		for _, s := range []*string{r.Observed.Expression, r.Observed.Text, r.Observed.Style} {
			if s != nil {
				content += " " + *s
			}
		}
		tokens := words(content)
		set := map[string]bool{}
		for _, s := range tokens {
			set[s] = true
		}
		hits := 0
		for _, t := range terms {
			if set[t] {
				hits++
			}
		}
		if hits == 0 {
			continue
		}
		out = append(out, result{ID: r.ID, Score: float64(hits) / float64(len(terms)), Subjects: r.Observed.Subjects, Evidence: "AI visual annotation; exact token overlap; uncalibrated"})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > k {
		out = out[:k]
	}
	return out
}
func main() {
	query := flag.String("query", "", "Russian query")
	paths := flag.String("annotations", "", "comma-separated annotation JSON paths")
	top := flag.Int("top", 5, "top results 1..10")
	flag.Parse()
	if *query == "" || *paths == "" || *top < 1 || *top > 10 {
		os.Exit(2)
	}
	rows := []Annotation{}
	for _, p := range strings.Split(*paths, ",") {
		b, e := os.ReadFile(p)
		if e != nil {
			os.Exit(1)
		}
		var v struct {
			Entries []Annotation `json:"entries"`
		}
		if json.Unmarshal(b, &v) != nil {
			os.Exit(1)
		}
		rows = append(rows, v.Entries...)
	}
	b, e := json.MarshalIndent(map[string]any{"query": *query, "method": "AI-annotated lexical demo; not R1 model-caption benchmark", "abstention": "not_calibrated", "results": rank(*query, rows, *top)}, "", "  ")
	if e != nil {
		os.Exit(1)
	}
	fmt.Println(string(b))
}
