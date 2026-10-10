package localmodels

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
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
}
type workerMessage struct {
	Kind       string      `json:"kind"`
	Code       string      `json:"code"`
	VRAM       uint32      `json:"vram_mib"`
	Embeddings [][]float64 `json:"embeddings"`
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
	if p.Runtime != "torch-sentence-transformers" || p.Model != "google/embeddinggemma-2" || p.Format != "safetensors" || p.Precision != "BF16" || len(p.Revision) != 40 {
		return fault(Unavailable)
	}
	for _, c := range p.Revision {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return fault(Unavailable)
		}
	}
	data, err := os.ReadFile(filepath.Join(b.cfg.ModelRoot, p.Revision, "butler-model.json"))
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
	return []Resident{{Model: b.model, VRAMMiB: b.memory}}, nil
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
	b.stateMu.Unlock()
	return nil
}
func (b *Process) Infer(ctx context.Context, p *pb.ModelProfile, r *pb.InferRequest, device string, batch uint32) (*pb.InferResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
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
