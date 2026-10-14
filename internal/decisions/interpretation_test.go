package decisions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/localmodels"
)

type interpretationGold struct {
	Quote    string   `json:"quote"`
	Intents  []string `json:"intents"`
	Modality string   `json:"modality"`
	Target   string   `json:"target"`
}
type interpretationCase struct {
	ID         string               `json:"id"`
	Split      string               `json:"split"`
	Family     string               `json:"family"`
	SourceKind string               `json:"source_kind"`
	Context    string               `json:"context"`
	Text       string               `json:"text"`
	Adequate   bool                 `json:"window_adequate"`
	Gold       []interpretationGold `json:"gold"`
}

func readInterpretationCorpus(t *testing.T, phase string) ([]interpretationCase, []byte) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "deploy", "local-models", "russian-interpretation-heldout-v3.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Version string               `json:"version"`
		Cases   []interpretationCase `json:"cases"`
	}
	if json.Unmarshal(raw, &corpus) != nil || corpus.Version != "butler-russian-interpretation-heldout-v3" {
		t.Fatal("invalid corpus")
	}
	var cases []interpretationCase
	for _, c := range corpus.Cases {
		if phase == "all" || c.Split == phase {
			cases = append(cases, c)
		}
	}
	if len(cases) == 0 {
		t.Fatal("empty split")
	}
	return cases, raw
}

func interpretationScores(intents []string, modality, target string) *pb.ClassifyOutput {
	_, d := localmodels.InterpretationSchema()
	out := &pb.ClassifyOutput{}
	for _, option := range d.Intents {
		p := .01
		if hasIntent(intents, option.ID) {
			p = .99
		}
		out.Scores = append(out.Scores, &pb.Score{Id: "intent/" + option.ID, Value: p})
	}
	for _, group := range []struct {
		prefix, selected string
		options          []localmodels.InterpretationOption
	}{{"modality/", modality, d.Modality}, {"target/", target, d.Target}} {
		for _, option := range group.options {
			p := .01 / float64(len(group.options)-1)
			if option.ID == group.selected {
				p = .99
			}
			out.Scores = append(out.Scores, &pb.Score{Id: group.prefix + option.ID, Value: p})
		}
	}
	return out
}

func TestInterpretationMultipleIntentAndUncertainty(t *testing.T) {
	out := interpretationScores([]string{"request_action", "stop_work"}, "direct", "current_operation")
	o, err := NormalizeInterpretation(out)
	if err != nil || o.Escalate || len(o.Intents) != 2 {
		t.Fatal(o, err)
	}
	if TrustedSourceAllowsOwnerIntent("document", o) || !TrustedSourceAllowsOwnerIntent("owner", o) {
		t.Fatal("source text promoted to owner intent")
	}
	for _, mode := range []string{"tentative", "quoted", "negated", "conditional", "unresolved"} {
		o, err = NormalizeInterpretation(interpretationScores([]string{"stop_work"}, mode, "whole_current_task"))
		if err != nil || !o.Escalate || TrustedSourceAllowsOwnerIntent("owner", o) {
			t.Fatal("non-direct proposition gained initiative", mode)
		}
	}
	for _, mutation := range []string{"partial", "duplicate", "nan", "distribution"} {
		out = interpretationScores(nil, "direct", "none")
		switch mutation {
		case "partial":
			out.Scores = out.Scores[:15]
		case "duplicate":
			out.Scores[0].Id = out.Scores[1].Id
		case "nan":
			out.Scores[0].Value = math.NaN()
		case "distribution":
			out.Scores[5].Value = 1
		}
		if _, err = NormalizeInterpretation(out); err == nil {
			t.Fatal("invalid distribution accepted", mutation)
		}
	}
}

func TestInterpretationQuoteSegmentationAndIntegrity(t *testing.T) {
	text := "  Он написал «стоп! продолжать?»; Проверь файл.\nСпасибо 😊"
	windows := InterpretationWindows(text)
	if len(windows) != 3 || windows[0].Quote != "Он написал «стоп! продолжать?»;" {
		t.Fatal(windows)
	}
	for _, w := range windows {
		if ValidateInterpretationQuote(text, w) != nil {
			t.Fatal("invalid Unicode offsets")
		}
	}
	windows[2].Quote = "Спасибо"
	if ValidateInterpretationQuote(text, windows[2]) == nil {
		t.Fatal("fabricated quote accepted")
	}
}

func TestInterpretationCorpusFreezeAndNoAnswerLeak(t *testing.T) {
	cases, raw := readInterpretationCorpus(t, "all")
	if len(cases) != 96 {
		t.Fatal("split drift")
	}
	seen := map[string]bool{}
	splits := map[string]int{}
	families := map[string]int{}
	for _, c := range cases {
		if seen[c.ID] || seen[c.Context+"\n"+c.Text] {
			t.Fatal("duplicate", c.ID)
		}
		seen[c.ID] = true
		seen[c.Context+"\n"+c.Text] = true
		splits[c.Split]++
		families[c.Split+":"+c.Family]++
		windows := InterpretationWindows(c.Text)
		if len(windows) != len(c.Gold) {
			t.Fatal("gold influenced windows or segmentation drift", c.ID, len(windows), len(c.Gold))
		}
		for i, w := range windows {
			if w.Quote != c.Gold[i].Quote || ValidateInterpretationQuote(c.Text, w) != nil {
				t.Fatal("gold quote drift", c.ID, i)
			}
			input := interpretationInput(c.Text, c.Context, w)
			if len(input) > 1600 {
				t.Fatal("source-bound input too large", c.ID, len(input))
			}
			for _, forbidden := range []string{`"gold":`, `"split":`, `"id":`, `"source_kind":`, `"window_adequate":`} {
				if strings.Contains(input, forbidden) {
					t.Fatal("annotation leaked")
				}
			}
			if _, err := NormalizeInterpretation(interpretationScores(c.Gold[i].Intents, c.Gold[i].Modality, c.Gold[i].Target)); err != nil {
				t.Fatal("invalid gold", c.ID)
			}
		}
	}
	if splits["dev"] != 24 || splits["test"] != 72 {
		t.Fatal("split counts")
	}
	for family, n := range families {
		if strings.HasPrefix(family, "dev:") && n != 2 || strings.HasPrefix(family, "test:") && n != 6 {
			t.Fatal("family balance", family)
		}
	}
	freezeRaw, err := os.ReadFile(filepath.Join("..", "..", "deploy", "local-models", "interpretation-v3-freeze.json"))
	if err != nil {
		t.Fatal(err)
	}
	var freeze struct {
		Hashes map[string]string `json:"sha256"`
	}
	if json.Unmarshal(freezeRaw, &freeze) != nil {
		t.Fatal("invalid freeze")
	}
	corpusSum := sha256.Sum256(raw)
	if hex.EncodeToString(corpusSum[:]) != freeze.Hashes["deploy/local-models/russian-interpretation-heldout-v3.json"] {
		t.Fatal("corpus bytes changed")
	}
	designRaw, err := os.ReadFile(filepath.Join("..", "..", "deploy", "local-models", "russian-interpretation-independent-v3.json"))
	if err != nil {
		t.Fatal(err)
	}
	var design struct {
		Cases []interpretationCase `json:"cases"`
	}
	if json.Unmarshal(designRaw, &design) != nil {
		t.Fatal("design fixture")
	}
	for _, candidate := range cases {
		if candidate.Split == "test" {
			for _, previous := range design.Cases {
				if candidate.Text == previous.Text || candidate.Context+"\n"+candidate.Text == previous.Context+"\n"+previous.Text {
					t.Fatal("held-out repeats design input", candidate.ID)
				}
			}
		}
	}
	for file, expected := range freeze.Hashes {
		body, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(file)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != expected {
			t.Fatal("frozen file changed", file)
		}
	}
}
