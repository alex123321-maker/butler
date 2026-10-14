package decisions

// Qwen is a research-only adapter in an isolated network. It does not add a
// production runtime, scheduler, model route or second GPU lane to Butler.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
)

const qwenEvaluationID = "qwen3.5-4b-q4-taxonomy-v2-gpu"
const qwenEvaluationModel = "butler-decision-qwen35-4b:q4-v2"

func qwenPrompt(schema []byte, input string) string {
	var s struct {
		Instructions string            `json:"instructions"`
		Labels       map[string]string `json:"labels"`
	}
	_ = json.Unmarshal(schema, &s)
	system := s.Instructions + "\nReturn only one JSON integer from 0 to 8. Categories:\n"
	for i, label := range Categories() {
		system += fmt.Sprintf("%d: %s (%s)\n", i, s.Labels[label], label)
	}
	return "<|im_start|>system\n" + system + "<|im_end|>\n<|im_start|>user\n" + input + "<|im_end|>\n<|im_start|>assistant\n<think>\n\n</think>\n\n"
}

type tokenProbability struct {
	Token   string  `json:"token"`
	LogProb float64 `json:"logprob"`
}
type qwenResponse struct {
	Response     string `json:"response"`
	Done         bool   `json:"done"`
	PromptTokens int    `json:"prompt_eval_count"`
	LogProbs     []struct {
		tokenProbability
		Top []tokenProbability `json:"top_logprobs"`
	} `json:"logprobs"`
}

func qwenScores(r qwenResponse) (*pb.ClassifyOutput, error) {
	var index int
	if !r.Done || json.Unmarshal([]byte(r.Response), &index) != nil || index < 0 || index > 8 {
		return nil, fmt.Errorf("invalid constrained answer")
	}
	letter := strconv.Itoa(index)
	// A single letter must occupy one token. Retain unknown margin when the API
	// does not expose ALL nine alternatives; never fabricate missing scores.
	for _, position := range r.LogProbs {
		if position.Token != letter {
			continue
		}
		probabilities := map[string]float64{}
		for _, p := range position.Top {
			if len(p.Token) == 1 && p.Token[0] >= '0' && p.Token[0] <= '8' {
				if math.IsNaN(p.LogProb) || math.IsInf(p.LogProb, 0) || p.LogProb > 0 {
					return nil, fmt.Errorf("invalid probability")
				}
				if _, ok := probabilities[p.Token]; ok {
					return nil, fmt.Errorf("duplicate token")
				}
				probabilities[p.Token] = math.Exp(p.LogProb)
			}
		}
		if len(probabilities) == 9 {
			sum := 0.0
			for _, p := range probabilities {
				sum += p
			}
			if sum <= 0 {
				return nil, fmt.Errorf("empty distribution")
			}
			out := &pb.ClassifyOutput{}
			for i, label := range Categories() {
				out.Scores = append(out.Scores, &pb.Score{Id: label, Value: probabilities[strconv.Itoa(i)] / sum})
			}
			return out, nil
		}
		if math.IsNaN(position.LogProb) || math.IsInf(position.LogProb, 0) || position.LogProb > 0 {
			return nil, fmt.Errorf("invalid selected score")
		}
		return &pb.ClassifyOutput{Scores: []*pb.Score{{Id: Categories()[index], Value: math.Exp(position.LogProb)}}}, nil
	}
	return nil, fmt.Errorf("missing single-token score")
}

func TestQwenDistributionDoesNotInventConfidence(t *testing.T) {
	var r qwenResponse
	_ = json.Unmarshal([]byte(`{"response":"0","done":true,"logprobs":[{"token":"0","logprob":-0.1,"top_logprobs":[{"token":"0","logprob":-0.1}]}]}`), &r)
	out, e := qwenScores(r)
	if e != nil {
		t.Fatal(e)
	}
	o, e := normalize(out)
	if e != nil || !o.MarginUnknown || !o.Escalate {
		t.Fatal("partial token scores gained confidence")
	}
	r.Response = `"Z"`
	if _, e = qwenScores(r); e == nil {
		t.Fatal("foreign enum accepted")
	}
}

func evaluateQwen(t *testing.T, f *fixture, backend Backend, cases []taxonomyEvent, schema []byte) (*metrics, []result) {
	t.Helper()
	countsPath := os.Getenv("BUTLER_TEST_QWEN_TOKEN_COUNTS")
	pinsPath := os.Getenv("BUTLER_TEST_QWEN_PINS")
	raw, e := os.ReadFile(countsPath)
	if e != nil {
		t.Fatal(e)
	}
	counts := map[string]int{}
	if json.Unmarshal(raw, &counts) != nil {
		t.Fatal("token preflight missing")
	}
	pins, e := os.ReadFile(pinsPath)
	if e != nil {
		t.Fatal(e)
	}
	var identity struct {
		Digest string `json:"ollama_manifest_sha256"`
	}
	if json.Unmarshal(pins, &identity) != nil || len(identity.Digest) != 64 {
		t.Fatal("model pin missing")
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	post := func(ctx context.Context, path string, payload any) ([]byte, error) {
		data, _ := json.Marshal(payload)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://decision-qwen:11434"+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil || resp.StatusCode != 200 {
			return nil, fmt.Errorf("qwen inference unavailable")
		}
		return body, nil
	}
	// Pin the loaded model identity before model input is released.
	req, _ := http.NewRequest(http.MethodGet, "http://decision-qwen:11434/api/tags", nil)
	resp, e := client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ = io.ReadAll(io.LimitReader(resp.Body, 65536))
	resp.Body.Close()
	var tags struct {
		Models []struct{ Name, Digest string }
	}
	if json.Unmarshal(raw, &tags) != nil {
		t.Fatal("invalid model catalog")
	}
	found := false
	for _, m := range tags.Models {
		if m.Name == qwenEvaluationModel && m.Digest == identity.Digest {
			found = true
		}
	}
	if !found {
		t.Fatal("model identity mismatch")
	}
	// Keep the accepted text embedding profile resident throughout the experiment.
	// Both operations remain sequential. This probes residency, not concurrency.
	embeddingCPU := os.Getenv("BUTLER_TEST_QWEN_EMBEDDING_CPU") == "1"
	var embeddingProbes []map[string]any
	embed := func(ctx context.Context) error {
		start := time.Now()
		out, err := backend.Infer(ctx, &pb.InferRequest{RequestId: "qwen-co-residency-embedding", ProfileId: "embeddinggemma2-text-bf16-v1", TimeoutMs: 60000, CallerId: "decision-co-residency", Input: &pb.InferRequest_Embed{Embed: &pb.EmbedInput{Task: pb.EmbeddingTask_EMBEDDING_TASK_DOCUMENT, Dimensions: 256, Texts: []*pb.TextInput{{Id: "co-residency", Text: "Проверка совместного размещения принятых локальных моделей."}}}}})
		expected := "gpu"
		if embeddingCPU {
			expected = "cpu"
		}
		if err == nil && (out == nil || out.Device != expected || out.FallbackReason != "" || len(out.GetEmbed().Vectors) != 1) {
			return fmt.Errorf("embedding left GPU or fell back")
		}
		if err == nil {
			embeddingProbes = append(embeddingProbes, map[string]any{"wall_ms": time.Since(start).Milliseconds(), "device": out.Device, "process_ram_mib": out.PeakRamMib, "whole_device_peak_mib": out.PeakDeviceMib})
		}
		return err
	}
	coResident := os.Getenv("BUTLER_TEST_QWEN_CORESIDENT") == "1"
	if coResident {
		if e = embed(context.Background()); e != nil {
			t.Fatal("accepted embedding unavailable before Qwen", e)
		}
	}
	m := &metrics{}
	var results []result
	boundary := &Observer{catalog: f.catalog, jobs: f.jobs}
	for i, c := range cases {
		prompt := qwenPrompt(schema, c.input())
		digest := sha256.Sum256([]byte(prompt))
		count, ok := counts[hex.EncodeToString(digest[:])]
		if !ok || count <= 0 || count > 2032 {
			t.Fatal("complete prompt exceeds pinned context or lacks tokenizer preflight")
		}
		j, ref := f.job(t, qwenEvaluationID+":"+c.Split+":"+c.ID, c.input())
		ctx := f.jobs.Context(context.Background(), j)
		if i > 0 {
			var stop context.CancelFunc
			ctx, stop = context.WithTimeout(ctx, 10*time.Second)
			defer stop()
		}
		source, err := boundary.source(ctx, ref, j.Purpose)
		if err != nil {
			t.Fatal(err)
		}
		if source.Body != c.input() {
			t.Fatal("source mismatch")
		}
		id, _ := budget.NewOperationID()
		payload := map[string]any{"model": qwenEvaluationModel, "prompt": prompt, "raw": true, "stream": false, "truncate": false, "think": false, "keep_alive": -1, "format": map[string]any{"type": "integer", "enum": []int{0, 1, 2, 3, 4, 5, 6, 7, 8}}, "logprobs": true, "top_logprobs": 20, "options": map[string]any{"num_ctx": 2048, "num_predict": 8, "temperature": 0, "seed": 42, "top_k": 0, "top_p": 1, "repeat_penalty": 1}}
		wire, _ := json.Marshal(payload)
		timeout := time.Minute
		if i > 0 {
			timeout = 10 * time.Second
		}
		bounded, release, err := f.jobs.Admit(ctx, j, jobs.Operation{ID: id, Kind: j.Kind, Endpoint: f.target.Endpoint(), ServiceIdentity: f.target.Identity(), InputBytes: int64(len(wire)), Tokens: 2048, Timeout: timeout, Origin: source.Origin})
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		body, callError := post(bounded, "/api/generate", payload)
		var response qwenResponse
		var o Observation
		if callError == nil {
			callError = json.Unmarshal(body, &response)
		}
		if callError == nil && response.PromptTokens != count {
			callError = fmt.Errorf("tokenizer count differs or input was truncated")
		}
		if callError == nil {
			var scores *pb.ClassifyOutput
			scores, callError = qwenScores(response)
			if callError == nil {
				o, callError = normalize(scores)
			}
		}
		if callError == nil {
			_, callError = boundary.source(ctx, ref, j.Purpose)
		}
		if bounded.Err() != nil {
			callError = bounded.Err()
		}
		release()
		wall := time.Since(start).Milliseconds()
		fault := ""
		if callError != nil {
			fault = "qwen_protocol_or_runtime"
			t.Logf("Qwen %s: %v", c.ID, callError)
			o = Observation{Label: "недостаточно данных", Escalate: true, Insufficient: true, ShadowOnly: true, EventPreserved: true}
		} else {
			o.Source = catalog.SourceRef(ref)
			o.ModelRevision = identity.Digest
		}
		m.add(c.annotated(), o.Label, o.Escalate, fault, wall, o, i == 0)
		results = append(results, result{c.ID, c.Split, c.Gold, qwenEvaluationID, o, fault, wall})
		checkpoint, _ := json.MarshalIndent(map[string]any{"complete": false, "cases": results, "totals": m, "actual_attempts": len(results)}, "", "  ")
		if e = os.WriteFile(os.Getenv("BUTLER_TEST_DECISION_REPORT")+".qwen-checkpoint.json", checkpoint, 0644); e != nil {
			t.Fatal(e)
		}
		if e = f.jobs.Finish(context.Background(), j, fault, false); e != nil {
			t.Fatal(e)
		}
		if coResident {
			if e = embed(context.Background()); e != nil {
				t.Fatal("embedding could not remain usable with Qwen resident", e)
			}
		}
	}
	m.finish()
	psReq, _ := http.NewRequest(http.MethodGet, "http://decision-qwen:11434/api/ps", nil)
	psResp, err := client.Do(psReq)
	if err != nil {
		t.Fatal(err)
	}
	resident, _ := io.ReadAll(io.LimitReader(psResp.Body, 65536))
	psResp.Body.Close()
	var state struct {
		Models []struct {
			Name, Digest string
			Size         int64 `json:"size"`
			VRAM         int64 `json:"size_vram"`
		}
	}
	if json.Unmarshal(resident, &state) != nil || len(state.Models) != 1 || state.Models[0].Digest != identity.Digest || state.Models[0].VRAM <= 0 || state.Models[0].VRAM != state.Models[0].Size {
		t.Fatal("Qwen is not fully GPU resident")
	}
	if e = os.WriteFile(os.Getenv("BUTLER_TEST_DECISION_REPORT")+".resident.json", resident, 0644); e != nil {
		t.Fatal(e)
	}
	if len(embeddingProbes) > 0 {
		body, _ := json.MarshalIndent(embeddingProbes, "", "  ")
		if e = os.WriteFile(os.Getenv("BUTLER_TEST_DECISION_REPORT")+".embedding.json", body, 0644); e != nil {
			t.Fatal(e)
		}
	}
	return m, results
}

func TestWriteQwenTokenPreflightPrompts(t *testing.T) {
	path := os.Getenv("BUTLER_TEST_QWEN_PREFLIGHT_PROMPTS")
	if path == "" {
		t.Skip("offline tokenizer preflight export only")
	}
	schema, e := os.ReadFile(filepath.Join("..", "localmodels", "decision-schema-v2.json"))
	if e != nil {
		t.Fatal(e)
	}
	prompts := map[string]string{}
	for _, phase := range []string{"dev", "test"} {
		cases, _ := readTaxonomyEvents(t, phase)
		for _, c := range cases {
			p := qwenPrompt(schema, c.input())
			d := sha256.Sum256([]byte(p))
			prompts[hex.EncodeToString(d[:])] = p
		}
	}
	raw, _ := json.Marshal(prompts)
	if e = os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
}
