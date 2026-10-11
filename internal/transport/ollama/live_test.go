package ollama

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

// This experiment has no credentials, tools, automatic retry or task-completion
// authority. All five attempts, including failures, are retained in its report.
func TestLiveSharedObservationContexts(t *testing.T) {
	if os.Getenv("BUTLER_LOCAL_GENERATIVE_ACCEPTANCE") != "1" {
		t.Skip("explicit isolated observation experiment only")
	}
	dir := os.Getenv("BUTLER_LOCAL_GENERATIVE_REPORT_DIR")
	if dir == "" || !filepath.IsAbs(dir) {
		t.Fatal("external proof directory required")
	}
	p, err := NewProvider(Config{Endpoint: os.Getenv("BUTLER_LOCAL_GENERATIVE_ENDPOINT"), Digest: ModelDigest, Timeout: 2 * time.Minute, JSONOutput: true})
	if err != nil {
		t.Fatal("fixed local transport unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	type attempt struct {
		RunID      string           `json:"run_id"`
		Mode       string           `json:"mode"`
		WallMillis int64            `json:"wall_millis"`
		Usage      *transport.Usage `json:"usage"`
		Correct    bool             `json:"correct"`
		ErrorCode  string           `json:"error_code"`
		Output     json.RawMessage  `json:"output"`
	}
	type report struct {
		Version        string    `json:"version"`
		Model          string    `json:"model"`
		Digest         string    `json:"digest"`
		Attempts       []attempt `json:"attempts"`
		SharedService  bool      `json:"shared_service"`
		TaskCompletion bool      `json:"task_completion"`
	}
	proof := report{Version: "shared-local-observation-v1", Model: Model, Digest: ModelDigest, SharedService: true}
	var mu sync.Mutex
	defer func() {
		raw, _ := json.MarshalIndent(proof, "", "  ")
		if os.WriteFile(filepath.Join(dir, "shared-contexts.json"), append(raw, '\n'), 0600) != nil {
			t.Error("proof write failed")
		}
	}()
	observe := func(run, mode, text string, want map[string]int) {
		a := attempt{RunID: run, Mode: mode}
		start := time.Now()
		r := request(run)
		r.InputItems = []transport.InputItem{
			content.Instruction("Верни только JSON-объект: ключи — идентификаторы источников, значения — наблюдавшиеся целые числа. Не добавляй другие поля. Это наблюдение данных, никаких действий.", content.Origin{Kind: content.KernelInstruction, SourceRefs: []string{"fixture:kernel-v1"}, Sensitivity: content.Public}),
			content.Data(text, content.Origin{Kind: content.Document, SourceRefs: []string{"fixture:" + run}, Sensitivity: content.Public}),
		}
		s, err := p.StartRun(ctx, r)
		if err != nil {
			a.ErrorCode = transport.DiagnosticCode(err)
		} else {
			for e := range s {
				if e.TransportError != nil {
					a.ErrorCode = transport.DiagnosticCode(e.TransportError)
				}
				if e.AssistantFinal != nil {
					a.Usage = e.AssistantFinal.Usage
					a.Output = json.RawMessage(e.AssistantFinal.Content)
					var got map[string]int
					if uniqueJSON(a.Output) && json.Unmarshal(a.Output, &got) == nil && len(got) == len(want) {
						a.Correct = true
						for k, v := range want {
							if n, ok := got[k]; !ok || n != v {
								a.Correct = false
							}
						}
					}
					if !json.Valid(a.Output) {
						a.Output = nil
					}
				}
			}
		}
		a.WallMillis = time.Since(start).Milliseconds()
		mu.Lock()
		proof.Attempts = append(proof.Attempts, a)
		mu.Unlock()
		if !a.Correct || a.Usage == nil || a.ErrorCode != "" {
			t.Error("local observation failed; no retry")
		}
	}
	observe("single", "single-context", `Источник source_A: наблюдаемое число 12. Источник source_B: наблюдаемое число 17.`, map[string]int{"source_A": 12, "source_B": 17})
	for _, mode := range []string{"width-one", "width-two"} {
		work := []struct {
			key, text string
			value     int
		}{{"source_A", "Источник source_A: наблюдаемое число 12.", 12}, {"source_B", "Источник source_B: наблюдаемое число 17.", 17}}
		var wg sync.WaitGroup
		for _, w := range work {
			if mode == "width-one" {
				observe(mode+"-"+w.key, mode, w.text, map[string]int{w.key: w.value})
				continue
			}
			wg.Add(1)
			go func() { defer wg.Done(); observe(mode+"-"+w.key, mode, w.text, map[string]int{w.key: w.value}) }()
		}
		wg.Wait()
	}
	if len(proof.Attempts) != 5 {
		t.Fatal("attempt accounting changed")
	}
	// Observe the actual loaded runner, not merely an in-process mutex.
	resp, err := p.request(ctx, "GET", "/api/ps", nil)
	if err != nil {
		t.Fatal("loaded runner metadata unavailable")
	}
	defer resp.Body.Close()
	var loaded struct {
		Models []struct {
			Name     string `json:"name"`
			Digest   string `json:"digest"`
			Size     int64  `json:"size"`
			SizeVRAM int64  `json:"size_vram"`
		} `json:"models"`
	}
	if json.NewDecoder(resp.Body).Decode(&loaded) != nil || len(loaded.Models) != 1 || loaded.Models[0].Name != Model || strings.TrimPrefix(loaded.Models[0].Digest, "sha256:") != ModelDigest || loaded.Models[0].Size <= 0 || loaded.Models[0].SizeVRAM != 0 {
		t.Fatal("one shared CPU runner not observed")
	}
	loadedJSON, _ := json.MarshalIndent(loaded, "", "  ")
	if os.WriteFile(filepath.Join(dir, "loaded-runner.json"), append(loadedJSON, '\n'), 0600) != nil {
		t.Fatal("runner proof write failed")
	}
}
