package localmodels

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/security/workspace"
	"google.golang.org/protobuf/proto"
)

type ProcessConfig struct {
	Python, Script, ModelRoot string
	GPUIndex                  uint32
}
type Process struct {
	cfg           ProcessConfig
	mu            sync.Mutex
	stateMu       sync.Mutex
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	stdout        *bufio.Reader
	model, device string
	profile       *pb.ModelProfile
	memory        uint32
	pid           int
}
type workerMessage struct {
	Kind       string      `json:"kind"`
	Code       string      `json:"code"`
	VRAM       uint32      `json:"vram_mib"`
	Embeddings [][]float64 `json:"embeddings"`
	Scores     []float64   `json:"scores"`
	Spans      []*pb.Span  `json:"spans"`
	Labels     []string    `json:"labels"`
}

func NewProcess(cfg ProcessConfig) (*Process, error) {
	if !filepath.IsAbs(cfg.Python) || !filepath.IsAbs(cfg.Script) || !filepath.IsAbs(cfg.ModelRoot) {
		return nil, fmt.Errorf("absolute worker paths required")
	}
	return &Process{cfg: cfg}, nil
}
func (b *Process) Healthy(context.Context) bool {
	for _, path := range []string{b.cfg.Python, b.cfg.Script, b.cfg.ModelRoot} {
		if _, err := os.Stat(path); err != nil {
			return false
		}
	}
	return true
}
func (b *Process) Verify(_ context.Context, p *pb.ModelProfile) error {
	runtime := map[string]string{"google/embeddinggemma-2": "torch-sentence-transformers", "Qwen/Qwen3-Reranker-0.6B": "torch-transformers", "fastino/gliner2.5-multi-v1": "torch-gliner2"}[p.Model]
	if runtime == "" || p.Runtime != runtime || p.Format != "safetensors" || (p.Precision != "BF16" && p.Precision != "FP32") || len(p.Revision) != 40 {
		return fault(Unavailable)
	}
	if p.SourceUrl != "https://huggingface.co/"+p.Model {
		return fault(Unavailable)
	}
	for _, c := range p.Revision {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return fault(Unavailable)
		}
	}
	data, err := workspace.ReadRegular(b.cfg.ModelRoot, filepath.Join(p.Revision, "butler-model.json"), 65536)
	if err != nil {
		return fault(Unavailable)
	}
	var meta struct {
		Model          string `json:"model"`
		Revision       string `json:"revision"`
		RuntimeVersion string `json:"runtime_version"`
	}
	if json.Unmarshal(data, &meta) != nil || meta.Model != p.Model || meta.Revision != p.Revision || meta.RuntimeVersion != p.RuntimeVersion {
		return fault(Unavailable)
	}
	return nil
}
func (b *Process) Resident(context.Context) ([]Resident, error) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.model == "" {
		return nil, nil
	}
	ram := uint32(0)
	if b.pid != 0 {
		if data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(b.pid), "status")); err == nil {
			ram = uint32((procKiB(string(data), "VmRSS:") + 1023) / 1024)
		}
	}
	return []Resident{{Model: b.model, VRAMMiB: b.memory, RAMMiB: ram}}, nil
}
func (b *Process) Unload(_ context.Context, model string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.model != "" && b.model != model {
		return fault(Unavailable)
	}
	b.stop()
	return nil
}
func (b *Process) stop() {
	if b.cmd != nil {
		_ = b.cmd.Process.Kill()
		_ = b.cmd.Wait()
		_ = b.stdin.Close()
		b.cmd = nil
		b.stdin = nil
		b.stdout = nil
	}
	b.stateMu.Lock()
	b.model = ""
	b.device = ""
	b.profile = nil
	b.memory = 0
	b.pid = 0
	b.stateMu.Unlock()
}
func (b *Process) exchange(ctx context.Context, message any) (workerMessage, error) {
	// Cancel kills the owned worker and waits for it; no opaque inference survives.
	cmd := b.cmd
	stop := context.AfterFunc(ctx, func() { _ = cmd.Process.Kill() })
	defer stop()
	data, err := json.Marshal(message)
	if err != nil {
		return workerMessage{}, fault(Invalid)
	}
	data = append(data, '\n')
	if _, err = b.stdin.Write(data); err != nil {
		b.stop()
		if ctx.Err() != nil {
			return workerMessage{}, ctx.Err()
		}
		return workerMessage{}, fault(Unavailable)
	}
	line, err := b.stdout.ReadSlice('\n')
	if err != nil || len(line) > 8<<20 {
		b.stop()
		if ctx.Err() != nil {
			return workerMessage{}, ctx.Err()
		}
		return workerMessage{}, fault(Unavailable)
	}
	var response workerMessage
	if json.Unmarshal(line, &response) != nil {
		b.stop()
		return response, fault(Protocol)
	}
	if ctx.Err() != nil {
		b.stop()
		return response, ctx.Err()
	}
	if response.Kind == "error" {
		switch ErrorKind(response.Code) {
		case OOM:
			return response, fault(OOM)
		case Invalid:
			return response, fault(Invalid)
		default:
			return response, fault(Unavailable)
		}
	}
	return response, nil
}
func (b *Process) ensure(ctx context.Context, p *pb.ModelProfile, device string) error {
	if b.cmd != nil && b.device == device && proto.Equal(b.profile, p) {
		return nil
	}
	b.stop()
	cmd := exec.Command(b.cfg.Python, "-u", b.cfg.Script)
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/tmp", "USER=butler", "LOGNAME=butler", "TORCHINDUCTOR_CACHE_DIR=/tmp/torch", "HF_HOME=/tmp/huggingface", "HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1", "HF_HUB_DISABLE_TELEMETRY=1", "BUTLER_MODEL_ROOT=" + b.cfg.ModelRoot, "BUTLER_MODEL_CPU_THREADS=4", "CUDA_VISIBLE_DEVICES=" + strconv.Itoa(int(b.cfg.GPUIndex))}
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fault(Unavailable)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fault(Unavailable)
	}
	if err = cmd.Start(); err != nil {
		return fault(Unavailable)
	}
	b.cmd = cmd
	b.stdin = stdin
	b.stdout = bufio.NewReaderSize(stdout, 8<<20)
	ready, err := b.exchange(ctx, map[string]any{"op": "init", "profile": p, "device": device})
	if err != nil {
		b.stop()
		return err
	}
	if ready.Kind != "ready" || device == "cpu" && ready.VRAM != 0 || device == "gpu" && ready.VRAM == 0 {
		b.stop()
		return fault(Protocol)
	}
	b.stateMu.Lock()
	b.model = p.Model
	b.device = device
	b.profile = proto.Clone(p).(*pb.ModelProfile)
	b.memory = ready.VRAM
	b.pid = cmd.Process.Pid
	b.stateMu.Unlock()
	return nil
}
func (b *Process) Infer(ctx context.Context, p *pb.ModelProfile, r *pb.InferRequest, device string, batch uint32) (*pb.InferResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r.GetRerank() != nil {
		return b.rank(ctx, p, r.GetRerank(), device, batch)
	}
	if r.GetExtract() != nil || r.GetClassify() != nil {
		return b.extract(ctx, p, r, device)
	}
	if r.GetMedia() != nil {
		return b.media(ctx, p, r.GetMedia(), device)
	}
	input := r.GetEmbed()
	if input == nil || p.PreprocessingVersion != "embeddinggemma2-retrieval-v1" || p.Dimensions != 768 || batch == 0 {
		return nil, fault(Unavailable)
	}
	if err := b.ensure(ctx, p, device); err != nil {
		return nil, err
	}
	output := &pb.EmbedOutput{IndexSpace: p.Id + ":" + p.Revision + ":" + p.PreprocessingVersion + ":" + strconv.Itoa(int(input.Dimensions))}
	for start := 0; start < len(input.Texts); start += int(batch) {
		end := min(start+int(batch), len(input.Texts))
		texts := make([]string, end-start)
		for i, t := range input.Texts[start:end] {
			texts[i] = embeddingText(t, input.Task)
		}
		response, err := b.exchange(ctx, map[string]any{"op": "embed", "texts": texts})
		if err != nil {
			return nil, err
		}
		if response.Kind != "embeddings" || len(response.Embeddings) != len(texts) {
			return nil, fault(Protocol)
		}
		for i, raw := range response.Embeddings {
			vector, err := normalizeVector(raw, int(p.Dimensions), int(input.Dimensions))
			if err != nil {
				return nil, err
			}
			output.Vectors = append(output.Vectors, &pb.Vector{Id: input.Texts[start+i].Id, Values: vector})
		}
	}
	return &pb.InferResponse{Output: &pb.InferResponse_Embed{Embed: output}}, nil
}
func (b *Process) media(ctx context.Context, p *pb.ModelProfile, input *pb.MediaInput, device string) (*pb.InferResponse, error) {
	if p.Model != "google/embeddinggemma-2" || p.PreprocessingVersion != "embeddinggemma2-image-v1" || input.MimeType != "image/png" && input.MimeType != "image/jpeg" {
		return nil, fault(Unavailable)
	}
	if err := b.ensure(ctx, p, device); err != nil {
		return nil, err
	}
	response, err := b.exchange(ctx, map[string]any{"op": "image", "content": base64.StdEncoding.EncodeToString(input.Content)})
	if err != nil {
		return nil, err
	}
	if response.Kind != "embeddings" || len(response.Embeddings) != 1 {
		return nil, fault(Protocol)
	}
	vector, err := normalizeVector(response.Embeddings[0], 768, 768)
	if err != nil {
		return nil, err
	}
	return &pb.InferResponse{Output: &pb.InferResponse_Media{Media: &pb.MediaOutput{Embedding: vector}}}, nil
}

func (b *Process) rank(ctx context.Context, p *pb.ModelProfile, input *pb.RerankInput, device string, batch uint32) (*pb.InferResponse, error) {
	if p.Model != "Qwen/Qwen3-Reranker-0.6B" || p.PreprocessingVersion != "qwen3-rerank-yesno-v1" || batch == 0 {
		return nil, fault(Unavailable)
	}
	if err := b.ensure(ctx, p, device); err != nil {
		return nil, err
	}
	output := &pb.RerankOutput{}
	for start := 0; start < len(input.Candidates); start += int(batch) {
		end := min(start+int(batch), len(input.Candidates))
		texts := make([]string, end-start)
		for i, t := range input.Candidates[start:end] {
			texts[i] = t.Text
		}
		response, err := b.exchange(ctx, map[string]any{"op": "rank", "query": input.Query, "texts": texts})
		if err != nil {
			return nil, err
		}
		if response.Kind != "scores" || len(response.Scores) != len(texts) {
			return nil, fault(Protocol)
		}
		for i, s := range response.Scores {
			if !validScore(s) {
				return nil, fault(Protocol)
			}
			output.Scores = append(output.Scores, &pb.Score{Id: input.Candidates[start+i].Id, Value: s})
		}
	}
	return &pb.InferResponse{Output: &pb.InferResponse_Rerank{Rerank: output}}, nil
}
func validScore(s float64) bool { return !math.IsNaN(s) && !math.IsInf(s, 0) && s >= 0 && s <= 1 }
func (b *Process) extract(ctx context.Context, p *pb.ModelProfile, r *pb.InferRequest, device string) (*pb.InferResponse, error) {
	if p.Model != "fastino/gliner2.5-multi-v1" || device != "cpu" || p.PreprocessingVersion != "gliner2.5-schema-v1" {
		return nil, fault(Unavailable)
	}
	if err := b.ensure(ctx, p, device); err != nil {
		return nil, err
	}
	if input := r.GetExtract(); input != nil {
		response, err := b.exchange(ctx, map[string]any{"op": "extract", "text": input.Text, "labels": input.EntityTypes})
		if err != nil {
			return nil, err
		}
		if response.Kind != "spans" {
			return nil, fault(Protocol)
		}
		labels := map[string]bool{}
		for _, s := range input.EntityTypes {
			labels[s] = true
		}
		runes := []rune(input.Text)
		for _, s := range response.Spans {
			if s == nil || !labels[s.Type] || s.Start >= s.End || int(s.End) > len(runes) || string(runes[s.Start:s.End]) != s.Text || !validScore(s.Confidence) {
				return nil, fault(Protocol)
			}
		}
		return &pb.InferResponse{Output: &pb.InferResponse_Extract{Extract: &pb.ExtractOutput{Spans: response.Spans, SchemaVersion: input.SchemaVersion}}}, nil
	}
	input := r.GetClassify()
	response, err := b.exchange(ctx, map[string]any{"op": "classify", "text": input.Text, "labels": input.Labels})
	if err != nil {
		return nil, err
	}
	if response.Kind != "classes" || len(response.Scores) != len(response.Labels) {
		return nil, fault(Protocol)
	}
	labels := map[string]bool{}
	for _, s := range input.Labels {
		labels[s] = true
	}
	out := &pb.ClassifyOutput{}
	for i, label := range response.Labels {
		if !labels[label] || !validScore(response.Scores[i]) {
			return nil, fault(Protocol)
		}
		out.Scores = append(out.Scores, &pb.Score{Id: label, Value: response.Scores[i]})
	}
	out.InsufficientData = len(out.Scores) == 0
	return &pb.InferResponse{Output: &pb.InferResponse_Classify{Classify: out}}, nil
}
