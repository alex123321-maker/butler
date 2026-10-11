package voice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/semantic"
	"github.com/butler/butler/internal/owneridentity"
)

type speechCase struct {
	ID        string `json:"id"`
	File      string `json:"file"`
	Reference string `json:"reference"`
	PCMHash   string `json:"pcm16_sha256"`
}
type speechCorpus struct {
	Dataset, Revision, License string
	Samples                    []speechCase
}
type qualityCase struct {
	ID         string   `json:"id"`
	Reference  string   `json:"reference"`
	Transcript string   `json:"transcript,omitempty"`
	Error      string   `json:"error,omitempty"`
	State      string   `json:"state,omitempty"`
	Millis     int64    `json:"millis"`
	Edits      int      `json:"edits"`
	Words      int      `json:"reference_words"`
	Missing    []string `json:"critical_misses,omitempty"`
}

// All eight results, including refusals, are retained. This public human speech
// corpus is frozen before inference; no threshold tuning or silent retry occurs.
func TestVoiceActualFrozenRussianHumanQuality(t *testing.T) {
	root := os.Getenv("BUTLER_VOICE_CORPUS")
	token := os.Getenv("BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE")
	if root == "" || token == "" {
		t.Skip("frozen public corpus and private service file required")
	}
	raw, e := os.ReadFile(filepath.Join(root, "corpus.json"))
	if e != nil {
		t.Fatal("corpus unavailable")
	}
	var corpus speechCorpus
	if json.Unmarshal(raw, &corpus) != nil || corpus.Dataset != "google/fleurs" || corpus.Revision != "70bb2e84b976b7e960aa89f1c648e09c59f894dd" || corpus.License != "CC-BY-4.0" || len(corpus.Samples) != 8 {
		t.Fatal("frozen corpus identity mismatch")
	}
	f := newFixture(t)
	target, _ := budget.NewLocalTarget("local-models:9090", "voice-fixture")
	actual, closeBackend, e := semantic.NewPinnedBackend(target, token)
	if e != nil {
		t.Fatal("backend unavailable")
	}
	defer closeBackend()
	f.p, e = NewProcessor(f.s, actual, target, budget.ModelPrice{Basis: "local_no_billing"})
	if e != nil {
		t.Fatal(e)
	}
	results := []qualityCase{}
	edits, words, critical, failures := 0, 0, 0, 0
	report := map[string]any{"version": "butler-voice-ru8-v1", "dataset": corpus.Dataset, "dataset_revision": corpus.Revision, "license": corpus.License, "attribution": "Google FLEURS / Conneau et al. 2022 / https://huggingface.co/datasets/google/fleurs", "selection_frozen_before_inference": true, "gate_max_wer": 0.2, "gate_critical_misses": 0, "limitations": []string{"eight recordings contain repeated sentences across speakers; no population accuracy claim", "numbers reported as literal surface forms for WER and frozen word/digit equivalents for critical fields", "no voice authentication; no command is accepted in this quality test"}}
	defer func() {
		report["cases"] = results
		report["failures"] = failures
		report["critical_misses"] = critical
		report["reference_words"] = words
		report["word_edits"] = edits
		if words > 0 {
			report["wer"] = float64(edits) / float64(words)
		}
		report["quality_gate_passed"] = failures == 0 && critical == 0 && words > 0 && float64(edits)/float64(words) <= 0.2
		data, _ := json.MarshalIndent(report, "", "  ")
		if path := os.Getenv("BUTLER_VOICE_QUALITY_REPORT"); path != "" {
			if e := os.WriteFile(path, data, 0600); e != nil {
				t.Error("quality report unavailable")
			}
		}
		t.Logf("retained=%d failures=%d edits=%d words=%d critical_misses=%d", len(results), failures, edits, words, critical)
	}()
	for _, sample := range corpus.Samples {
		item := qualityCase{ID: sample.ID, Reference: sample.Reference}
		results = append(results, item)
		index := len(results) - 1
		if filepath.Base(sample.File) != sample.File {
			t.Fatal("invalid corpus filename")
		}
		wave, e := os.ReadFile(filepath.Join(root, sample.File))
		if e != nil {
			results[index].Error = "corpus_unavailable"
			failures++
			continue
		}
		digest := sha256.Sum256(wave)
		if hex.EncodeToString(digest[:]) != sample.PCMHash {
			t.Fatal("corpus PCM changed")
		}
		f.ctx = owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "ru8:" + sample.ID})
		clip, e := f.s.Upload(f.ctx, Upload{WAV: wave, Role: External, RetentionHours: 1})
		if e != nil {
			results[index].Error = "upload_rejected"
			failures++
			continue
		}
		job := f.claim(t, clip, 4)
		started := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		e = f.p.Transcribe(ctx, job, clip.ID)
		cancel()
		results[index].Millis = time.Since(started).Milliseconds()
		if e != nil {
			results[index].Error = safeCode(e)
			failures++
			_ = f.s.Delete(f.ctx, clip.ID)
			continue
		}
		clip, e = f.s.Get(f.ctx, clip.ID)
		if e != nil {
			results[index].Error = "source_rejected"
			failures++
			continue
		}
		results[index].Transcript = clip.Text
		results[index].State = clip.State
		reference, actual := speechWords(sample.Reference), speechWords(clip.Text)
		n := wordDistance(reference, actual)
		results[index].Edits = n
		results[index].Words = len(reference)
		edits += n
		words += len(reference)
		missing := criticalSpeechMisses(sample.Reference, clip.Text)
		results[index].Missing = missing
		critical += len(missing)
		if e = f.s.Delete(f.ctx, clip.ID); e != nil {
			t.Fatal("quality original deletion unavailable")
		}
	}
	if len(results) != 8 {
		t.Fatal("incomplete correlated result set")
	}
	// Gate result is retained as measured no-go, not a reason to discard cases.
}
func speechWords(s string) []string {
	return strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if r == 'ё' {
				return 'е'
			}
			return unicode.ToLower(r)
		}
		return ' '
	}, s))
}
func wordDistance(a, b []string) int {
	previous := make([]int, len(b)+1)
	for i := range previous {
		previous[i] = i
	}
	for i, x := range a {
		next := make([]int, len(b)+1)
		next[0] = i + 1
		for j, y := range b {
			cost := 0
			if x != y {
				cost = 1
			}
			next[j+1] = min(next[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous = next
	}
	return previous[len(b)]
}
func criticalSpeechMisses(reference, actual string) []string {
	r, a := " "+strings.Join(speechWords(reference), " ")+" ", " "+strings.Join(speechWords(actual), " ")+" "
	out := []string{}
	if strings.Count(a, " не ") < strings.Count(r, " не ") {
		out = append(out, "negation")
	}
	for _, name := range []string{"карпендо", "индия"} {
		if strings.Contains(r, " "+name+" ") && !strings.Contains(a, " "+name+" ") {
			out = append(out, "name:"+name)
		}
	}
	for _, n := range []struct{ digit, words string }{{"36", "тридцать шесть"}, {"7", "семь"}, {"45", "сорок пять"}, {"117", "сто семнадцать"}} {
		if strings.Contains(r, " "+n.digit+" ") && !strings.Contains(a, " "+n.digit+" ") && !strings.Contains(a, " "+n.words+" ") {
			out = append(out, "number:"+n.digit)
		}
	}
	return out
}
