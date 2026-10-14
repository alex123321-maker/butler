package decisions

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
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/localmodels"
	"github.com/butler/butler/internal/memory/jobs"
)

const interpretationQwenID = "qwen3.5-4b-q4-interpretation-v3-gpu"
const interpretationQwenTokens = 2048 + 128

func interpretationQwenPrompt(input string) string {
	_, d := localmodels.InterpretationSchema()
	system := d.Instructions + "\nReturn one JSON object with exactly seven named fields. The five intention fields are independent booleans: true present, false absent. Modality and target are integers from their lists below.\n"
	for _, o := range d.Intents {
		system += fmt.Sprintf("Boolean field %s: %s\n", o.ID, o.Description)
	}
	for _, group := range []struct {
		name    string
		options []localmodels.InterpretationOption
	}{{"modality", d.Modality}, {"target", d.Target}} {
		system += fmt.Sprintf("Integer field %s:\n", group.name)
		for i, o := range group.options {
			system += fmt.Sprintf("%d: %s: %s\n", i, o.ID, o.Description)
		}
	}
	return "<|im_start|>system\n" + system + "<|im_end|>\n<|im_start|>user\n" + input + "<|im_end|>\n<|im_start|>assistant\n<think>\n\n</think>\n\n"
}

func interpretationQwenFormat() map[string]any {
	_, d := localmodels.InterpretationSchema()
	properties := map[string]any{}
	var required []string
	for _, o := range d.Intents {
		properties[o.ID] = map[string]any{"type": "boolean"}
		required = append(required, o.ID)
	}
	for _, group := range []struct {
		name  string
		count int
	}{{"modality", len(d.Modality)}, {"target", len(d.Target)}} {
		enum := []int{}
		for i := 0; i < group.count; i++ {
			enum = append(enum, i)
		}
		properties[group.name] = map[string]any{"type": "integer", "enum": enum}
		required = append(required, group.name)
	}
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

func interpretationQwenFields(text string) ([]string, []int, error) {
	_, d := localmodels.InterpretationSchema()
	intent := map[string]bool{}
	for _, o := range d.Intents {
		intent[o.ID] = true
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(text)))
	first, err := dec.Token()
	if err != nil || first != json.Delim('{') {
		return nil, nil, fmt.Errorf("invalid object")
	}
	var names []string
	var values []int
	seen := map[string]bool{}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		name, ok := key.(string)
		if !ok || seen[name] || !intent[name] && name != "modality" && name != "target" {
			return nil, nil, fmt.Errorf("duplicate or unknown field")
		}
		seen[name] = true
		value := 0
		if intent[name] {
			var b bool
			if err = dec.Decode(&b); err != nil {
				return nil, nil, err
			}
			if b {
				value = 1
			}
		} else if err = dec.Decode(&value); err != nil {
			return nil, nil, err
		}
		names = append(names, name)
		values = append(values, value)
	}
	last, err := dec.Token()
	if err != nil || last != json.Delim('}') || len(names) != 7 {
		return nil, nil, fmt.Errorf("incomplete object")
	}
	if _, err = dec.Token(); err != io.EOF {
		return nil, nil, fmt.Errorf("trailing object content")
	}
	return names, values, nil
}

func interpretationQwenAnswer(r qwenResponse) (Interpretation, error) {
	names, values, err := interpretationQwenFields(r.Response)
	_, d := localmodels.InterpretationSchema()
	if !r.Done || err != nil {
		return Interpretation{}, fmt.Errorf("invalid seven-field answer")
	}
	o := Interpretation{Intents: []string{}, Escalate: true, ConfidenceUnknown: true}
	for i, v := range values {
		limit := 1
		if names[i] == "modality" {
			limit = len(d.Modality) - 1
		}
		if names[i] == "target" {
			limit = len(d.Target) - 1
		}
		if v < 0 || v > limit {
			return o, fmt.Errorf("out-of-range answer")
		}
	}
	decoded := map[string]int{}
	for i, name := range names {
		decoded[name] = values[i]
	}
	for _, intent := range d.Intents {
		if decoded[intent.ID] == 1 {
			o.Intents = append(o.Intents, intent.ID)
		}
	}
	o.Modality = d.Modality[decoded["modality"]].ID
	o.Target = d.Target[decoded["target"]].ID
	var positions []struct {
		tokenProbability
		Top []tokenProbability
	}
	for _, p := range r.LogProbs {
		token := p.Token
		if token == "true" || token == "false" || len(token) == 1 && token[0] >= '0' && token[0] <= '5' {
			positions = append(positions, struct {
				tokenProbability
				Top []tokenProbability
			}{p.tokenProbability, p.Top})
		}
	}
	// Valid semantic answers remain observed but escalated when the API omits
	// alternatives or merges a digit token. Missing scores are never invented.
	if len(positions) != 7 {
		return o, nil
	}
	out := &pb.ClassifyOutput{}
	for i, pos := range positions {
		selected := fmt.Sprint(values[i])
		if names[i] != "modality" && names[i] != "target" {
			selected = "false"
			if values[i] == 1 {
				selected = "true"
			}
		}
		if pos.Token != selected {
			return o, nil
		}
		n := 2
		if names[i] == "modality" {
			n = len(d.Modality)
		}
		if names[i] == "target" {
			n = len(d.Target)
		}
		probs := map[int]float64{}
		for _, p := range pos.Top {
			token := p.Token
			v := -1
			if names[i] != "modality" && names[i] != "target" {
				if token == "false" {
					v = 0
				}
				if token == "true" {
					v = 1
				}
			} else if len(token) == 1 && token[0] >= '0' && int(token[0]-'0') < n {
				v = int(token[0] - '0')
			}
			if v < 0 {
				continue
			}
			if math.IsNaN(p.LogProb) || math.IsInf(p.LogProb, 0) || p.LogProb > 0 {
				return o, fmt.Errorf("invalid token probability")
			}
			if _, ok := probs[v]; ok {
				return o, fmt.Errorf("duplicate token alternative")
			}
			probs[v] = math.Exp(p.LogProb)
		}
		if len(probs) != n {
			return o, nil
		}
		sum := 0.0
		for _, p := range probs {
			sum += p
		}
		if sum <= 0 {
			return o, fmt.Errorf("empty alternatives")
		}
		if names[i] != "modality" && names[i] != "target" {
			out.Scores = append(out.Scores, &pb.Score{Id: "intent/" + names[i], Value: probs[1] / sum})
		} else {
			options, prefix := d.Modality, "modality/"
			if names[i] == "target" {
				options = d.Target
				prefix = "target/"
			}
			for j, option := range options {
				out.Scores = append(out.Scores, &pb.Score{Id: prefix + option.ID, Value: probs[j] / sum})
			}
		}
	}
	scored, err := NormalizeInterpretation(out)
	if err != nil {
		return o, err
	}
	if !sameIntents(scored.Intents, o.Intents) || scored.Modality != o.Modality || scored.Target != o.Target {
		return o, fmt.Errorf("decoded answer and score winner differ")
	}
	return scored, nil
}

type interpretationQwen struct {
	f         *fixture
	backend   Backend
	client    *http.Client
	counts    map[string]int
	digest    string
	embedding []map[string]any
}

func newInterpretationQwen(t *testing.T, f *fixture, b Backend) (*interpretationQwen, error) {
	t.Helper()
	q := &interpretationQwen{f: f, backend: b, counts: map[string]int{}, client: &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	raw, err := os.ReadFile(os.Getenv("BUTLER_TEST_QWEN_TOKEN_COUNTS"))
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(raw, &q.counts) != nil {
		return nil, fmt.Errorf("invalid token preflight")
	}
	raw, err = os.ReadFile(os.Getenv("BUTLER_TEST_QWEN_PINS"))
	if err != nil {
		return nil, err
	}
	var pin struct {
		Digest string `json:"ollama_manifest_sha256"`
	}
	if json.Unmarshal(raw, &pin) != nil || len(pin.Digest) != 64 {
		return nil, fmt.Errorf("invalid model pins")
	}
	q.digest = pin.Digest
	raw, err = q.request(context.Background(), http.MethodGet, "/api/tags", nil)
	if err != nil {
		return nil, err
	}
	var tags struct {
		Models []struct{ Name, Digest string }
	}
	if json.Unmarshal(raw, &tags) != nil {
		return nil, fmt.Errorf("invalid catalog")
	}
	found := false
	for _, m := range tags.Models {
		if m.Name == qwenEvaluationModel && m.Digest == q.digest {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("model identity mismatch")
	}
	if err = q.embed(t); err != nil {
		return nil, err
	}
	return q, nil
}
func (q *interpretationQwen) request(ctx context.Context, method, path string, wire []byte) ([]byte, error) {
	r, err := http.NewRequestWithContext(ctx, method, "http://decision-qwen:11434"+path, bytes.NewReader(wire))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	response, err := q.client.Do(r)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || response.StatusCode != 200 {
		return nil, fmt.Errorf("Qwen request unavailable")
	}
	return body, nil
}
func (q *interpretationQwen) embed(t *testing.T) error {
	t.Helper()
	// Own every actual placement probe and retain its separate spent reservation.
	text := "Поиск исходного ограничения и связанного поручения в памяти."
	j, ref := q.f.interpretationJob(t, fmt.Sprintf("embedding-probe-%d", len(q.embedding)), text, 2048)
	boundary := &Observer{catalog: q.f.catalog, jobs: q.f.jobs}
	ctx := q.f.jobs.Context(context.Background(), j)
	source, err := boundary.source(ctx, ref, j.Purpose)
	if err != nil {
		return err
	}
	id, err := budget.NewOperationID()
	if err != nil {
		return err
	}
	r := &pb.InferRequest{RequestId: id, ProfileId: "embeddinggemma2-text-bf16-v1", CallerId: "interpretation-placement", CpuOnly: true, TimeoutMs: 60000, Input: &pb.InferRequest_Embed{Embed: &pb.EmbedInput{Task: pb.EmbeddingTask_EMBEDDING_TASK_QUERY, Dimensions: 256, Texts: []*pb.TextInput{{Id: "probe", Text: text}}}}}
	wire, _ := json.Marshal(r)
	bounded, release, err := q.f.jobs.Admit(ctx, j, jobs.Operation{ID: id, Kind: j.Kind, Endpoint: q.f.target.Endpoint(), ServiceIdentity: q.f.target.Identity(), InputBytes: int64(len(wire)), Tokens: 2048, Timeout: time.Minute, Origin: source.Origin})
	if err != nil {
		return err
	}
	start := time.Now()
	response, err := q.backend.Infer(bounded, r)
	canceled := bounded.Err()
	release()
	if err != nil || canceled != nil || response == nil || response.RequestId != id || response.Revision != "914f7f89142e33e77833254d9c9b90c3cef7303b" || response.Device != "cpu" || response.FallbackReason != "" || len(response.GetEmbed().Vectors) != 1 {
		return fmt.Errorf("CPU embedding placement failed")
	}
	if _, err = boundary.source(ctx, ref, j.Purpose); err != nil {
		return err
	}
	q.embedding = append(q.embedding, map[string]any{"wall_ms": time.Since(start).Milliseconds(), "device": response.Device, "peak_service_ram_mib": response.PeakRamMib, "source": ref, "request_id": id})
	return q.f.jobs.Finish(context.Background(), j, "", false)
}
func (q *interpretationQwen) observe(t *testing.T, c interpretationCase, i int, w InterpretationWindow, first bool) (interpretationRow, bool) {
	t.Helper()
	row := interpretationRow{Case: c.ID, Window: i, Profile: interpretationQwenID, SourceKind: c.SourceKind, Quote: w, Gold: c.Gold[i]}
	input := interpretationInput(c.Text, c.Context, w)
	prompt := interpretationQwenPrompt(input)
	sum := sha256.Sum256([]byte(prompt))
	count, ok := q.counts[hex.EncodeToString(sum[:])]
	if !ok || count < 1 || count > 1920 {
		t.Fatal("complete prompt not preflighted")
	}
	j, ref := q.f.interpretationJob(t, interpretationQwenID+":"+c.Split+":"+c.ID+fmt.Sprint(i), input, interpretationQwenTokens)
	row.Source = ref
	start := time.Now()
	ctx, cancel := context.WithTimeout(q.f.jobs.Context(context.Background(), j), time.Minute)
	if !first {
		cancel()
		ctx, cancel = context.WithTimeout(q.f.jobs.Context(context.Background(), j), 20*time.Second)
	}
	defer cancel()
	boundary := &Observer{catalog: q.f.catalog, jobs: q.f.jobs}
	source, err := boundary.source(ctx, ref, j.Purpose)
	if err != nil || source.Body != input {
		t.Fatal("source mismatch", err)
	}
	payload := map[string]any{"model": qwenEvaluationModel, "prompt": prompt, "raw": true, "stream": false, "truncate": false, "think": false, "keep_alive": -1, "format": interpretationQwenFormat(), "logprobs": true, "top_logprobs": 20, "options": map[string]any{"num_ctx": 2048, "num_predict": 128, "temperature": 0, "seed": 42, "top_k": 0, "top_p": 1, "repeat_penalty": 1}}
	wire, _ := json.Marshal(payload)
	id, err := budget.NewOperationID()
	if err != nil {
		t.Fatal(err)
	}
	timeout := time.Minute
	if !first {
		timeout = 20 * time.Second
	}
	if deadline, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(deadline))
	}
	timeout = timeout.Truncate(time.Millisecond)
	bounded, release, err := q.f.jobs.Admit(ctx, j, jobs.Operation{ID: id, Kind: j.Kind, Endpoint: q.f.target.Endpoint(), ServiceIdentity: q.f.target.Identity(), InputBytes: int64(len(wire)), Tokens: interpretationQwenTokens, Timeout: timeout, Origin: source.Origin})
	if err != nil {
		t.Fatal("Qwen admission failed", err)
	}
	raw, callError := q.request(bounded, http.MethodPost, "/api/generate", wire)
	var response qwenResponse
	if callError == nil {
		callError = json.Unmarshal(raw, &response)
		row.QwenResponse = &response
	}
	if callError == nil && response.PromptTokens != count {
		callError = fmt.Errorf("token count differs from exact preflight")
	}
	if callError == nil {
		row.Observation, callError = interpretationQwenAnswer(response)
	}
	if callError == nil {
		_, callError = boundary.source(ctx, ref, j.Purpose)
	}
	if bounded.Err() != nil {
		callError = bounded.Err()
	}
	release()
	row.WallMS = time.Since(start).Milliseconds()
	if callError != nil {
		row.Error = "qwen_protocol_or_runtime"
		row.Observation = Interpretation{Escalate: true, ConfidenceUnknown: true}
		t.Logf("Qwen %s window %d: %v", c.ID, i, callError)
	}
	if err = q.f.jobs.Finish(context.Background(), j, row.Error, false); err != nil {
		t.Fatal(err)
	}
	if err = q.embed(t); err != nil {
		t.Fatal(err)
	}
	return row, true
}
func (q *interpretationQwen) finish(t *testing.T, path string) {
	raw, err := q.request(context.Background(), http.MethodGet, "/api/ps", nil)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Models []struct {
			Name, Digest string
			Size         int64 `json:"size"`
			VRAM         int64 `json:"size_vram"`
		}
	}
	if json.Unmarshal(raw, &state) != nil || len(state.Models) != 1 || state.Models[0].Name != qwenEvaluationModel || state.Models[0].Digest != q.digest || state.Models[0].VRAM <= 0 || state.Models[0].VRAM != state.Models[0].Size {
		t.Fatal("Qwen not fully GPU resident")
	}
	if err = os.WriteFile(path+".resident.json", raw, 0644); err != nil {
		t.Fatal(err)
	}
	writeInterpretationReport(t, path+".embedding.json", q.embedding)
}
func TestWriteInterpretationQwenPreflight(t *testing.T) {
	path := os.Getenv("BUTLER_TEST_QWEN_PREFLIGHT_PROMPTS")
	if path == "" {
		t.Skip("offline export only")
	}
	cases, _ := readInterpretationCorpus(t, "all")
	prompts := map[string]string{}
	for _, c := range cases {
		for _, w := range InterpretationWindows(c.Text) {
			prompt := interpretationQwenPrompt(interpretationInput(c.Text, c.Context, w))
			sum := sha256.Sum256([]byte(prompt))
			prompts[hex.EncodeToString(sum[:])] = prompt
		}
	}
	raw, _ := json.Marshal(prompts)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestInterpretationQwenPartialConfidence(t *testing.T) {
	r := qwenResponse{Response: `{"request_action":true,"change_constraint":false,"stop_work":true,"ask_information":false,"provide_information":false,"modality":2,"target":0}`, Done: true}
	o, err := interpretationQwenAnswer(r)
	if err != nil || !o.Escalate || !o.ConfidenceUnknown || len(o.Intents) != 2 || o.Modality != "quoted" {
		t.Fatal(o, err)
	}
	r.Response = `{"request_action":true,"change_constraint":false,"stop_work":false,"ask_information":false,"provide_information":false,"modality":0,"target":7}`
	if _, err = interpretationQwenAnswer(r); err == nil {
		t.Fatal("foreign target accepted")
	}
	r.Response = `{"request_action":true,"request_action":false,"stop_work":false,"ask_information":false,"provide_information":false,"modality":0,"target":0}`
	if _, err = interpretationQwenAnswer(r); err == nil {
		t.Fatal("duplicate JSON field accepted")
	}
}

func TestInterpretationQwenScoresFollowNamedFieldOrder(t *testing.T) {
	text := `{"target":1,"stop_work":true,"request_action":true,"provide_information":false,"modality":0,"change_constraint":false,"ask_information":false}`
	names, values, err := interpretationQwenFields(text)
	if err != nil {
		t.Fatal(err)
	}
	positions := []map[string]any{}
	for i, name := range names {
		count := 2
		if name == "modality" {
			count = 6
		}
		if name == "target" {
			count = 5
		}
		selected := fmt.Sprint(values[i])
		binary := name != "modality" && name != "target"
		if binary {
			selected = "false"
			if values[i] == 1 {
				selected = "true"
			}
		}
		top := []tokenProbability{}
		for j := 0; j < count; j++ {
			token := fmt.Sprint(j)
			if binary {
				token = "false"
				if j == 1 {
					token = "true"
				}
			}
			p := .1 / float64(count-1)
			if j == values[i] {
				p = .9
			}
			top = append(top, tokenProbability{token, math.Log(p)})
		}
		positions = append(positions, map[string]any{"token": selected, "logprob": math.Log(.9), "top_logprobs": top})
	}
	raw, _ := json.Marshal(map[string]any{"response": text, "done": true, "logprobs": positions})
	var r qwenResponse
	if json.Unmarshal(raw, &r) != nil {
		t.Fatal("fixture")
	}
	o, err := interpretationQwenAnswer(r)
	if err != nil || o.ConfidenceUnknown || o.Escalate || !sameIntents(o.Intents, []string{"request_action", "stop_work"}) || o.Target != "current_operation" {
		t.Fatal("field order changed the score meaning", o, err)
	}
}
