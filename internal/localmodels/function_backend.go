package localmodels

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/security/workspace"
)

const functionAddress = "127.0.0.1:11435"
const functionBinaryHash = "8ff456e3f8c08b7fb211857c634d4df8e1a84cacaaf5b9e7b6bbf89e0f9ae8e0"

//go:embed function_runtime_manifest.json
var functionRuntimeManifest []byte

// FunctionBackend adds one CPU-only owned child to the existing CPU lane. It
// accepts no executable/model/server path in inference input. No independent
// server/queue/admission is exposed; the Manager owns every request and unload.
type FunctionBackend struct {
	Base                  Backend
	Executable, ModelRoot string
	mu                    sync.Mutex
	cmd                   *exec.Cmd
	wait                  chan error
	home                  string
	verified              bool
	client                *http.Client
}

func NewFunctionBackend(base Backend, executable, models string) (*FunctionBackend, error) {
	if base == nil || (executable == "") != (models == "") || executable != "" && (runtime.GOOS != "linux" || !filepath.IsAbs(executable) || !filepath.IsAbs(models)) {
		return nil, fault(Invalid)
	}
	return &FunctionBackend{Base: base, Executable: executable, ModelRoot: models, client: &http.Client{Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 2 * time.Second}).DialContext}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (b *FunctionBackend) Healthy(ctx context.Context) bool { return b.Base.Healthy(ctx) } // optional runtime absence is not base-service outage
func (b *FunctionBackend) Verify(ctx context.Context, p *pb.ModelProfile) error {
	if p.Model != FunctionModel {
		return b.Base.Verify(ctx, p)
	}
	if p.Id != FunctionProfile || p.Revision != FunctionRevision || p.Runtime != "ollama-owned-cpu" || p.RuntimeVersion != FunctionRuntime || p.Format != "gguf" || p.Precision != "Q8_0" || p.PreferredDevice != "cpu" || p.PreprocessingVersion != FunctionCatalogVersion || p.SourceUrl != "https://ollama.com/library/functiongemma:270m-it-q8_0" || p.License != "Gemma-Terms-2025-03-24" || p.ContextTokens != 2048 || p.MaxInputBytes > 2048 || p.MaxBatch != 1 || b.Executable == "" || !p.CpuFallback {
		return fault(Unavailable)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.verified {
		return nil
	}
	if e := verifyFunctionRuntime(ctx, b.Executable); e != nil {
		return e
	}
	if e := hashRegular(ctx, filepath.Dir(b.Executable), filepath.Base(b.Executable), functionBinaryHash, 40<<20); e != nil {
		return e
	}
	manifest, e := workspace.ReadRegular(b.ModelRoot, "manifests/registry.ollama.ai/library/functiongemma/270m-it-q8_0", 65536)
	if e != nil {
		return fault(Unavailable)
	}
	sum := sha256.Sum256(manifest)
	if hex.EncodeToString(sum[:]) != FunctionRevision {
		return fault(Unavailable)
	}
	var data struct {
		Config struct {
			Digest string
			Size   int64
		}
		Layers []struct {
			Digest string
			Size   int64
		}
	}
	if json.Unmarshal(manifest, &data) != nil || len(data.Layers) != 4 {
		return fault(Unavailable)
	}
	blobs := map[string]int64{data.Config.Digest: data.Config.Size}
	for _, l := range data.Layers {
		blobs[l.Digest] = l.Size
	}
	for digest, size := range blobs {
		if len(digest) != 71 || digest[:7] != "sha256:" || size <= 0 || size > 350<<20 {
			return fault(Unavailable)
		}
		if e := hashRegular(ctx, b.ModelRoot, "blobs/sha256-"+digest[7:], digest[7:], size); e != nil {
			return e
		}
	}
	b.verified = true
	return nil
}

func verifyFunctionRuntime(ctx context.Context, executable string) error {
	var meta struct {
		Files map[string]string
		Links map[string]string
	}
	if json.Unmarshal(functionRuntimeManifest, &meta) != nil || len(meta.Files) < 20 {
		return fault(Unavailable)
	}
	rootPath := filepath.Dir(filepath.Dir(executable))
	if filepath.Base(filepath.Dir(executable)) != "bin" || filepath.Base(executable) != "ollama" {
		return fault(Unavailable)
	}
	root, e := os.OpenRoot(rootPath)
	if e != nil {
		return fault(Unavailable)
	}
	defer root.Close()
	for name, want := range meta.Files {
		if e := hashRegular(ctx, rootPath, name, want, 40<<20); e != nil {
			return e
		}
	}
	for name, want := range meta.Links {
		target, e := root.Readlink(name)
		if e != nil || target != want {
			return fault(Unavailable)
		}
	}
	return nil
}
func hashRegular(ctx context.Context, root, name, want string, maxBytes int64) error {
	r, e := os.OpenRoot(root)
	if e != nil {
		return fault(Unavailable)
	}
	defer r.Close()
	info, e := r.Lstat(name)
	if e != nil || !info.Mode().IsRegular() || info.Size() > maxBytes {
		return fault(Unavailable)
	}
	f, e := r.Open(name)
	if e != nil {
		return fault(Unavailable)
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(info, opened) {
		return fault(Unavailable)
	}
	h := sha256.New()
	buf := make([]byte, 32768)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, e := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return fault(Unavailable)
		}
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return fault(Unavailable)
	}
	return nil
}
func (b *FunctionBackend) Resident(ctx context.Context) ([]Resident, error) {
	base, e := b.Base.Resident(ctx)
	if e != nil {
		return nil, e
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cmd != nil {
		// Full subtree RAM is continuously bounded by ProcMemory/cgroup at Manager.
		base = append(base, Resident{Model: FunctionModel})
	}
	return base, nil
}
func (b *FunctionBackend) stop() {
	if b.cmd == nil {
		return
	}
	killOwnedGroup(b.cmd)
	<-b.wait
	b.cmd = nil
	b.wait = nil
	_ = os.RemoveAll(b.home)
	b.home = "" // only this process's os.MkdirTemp directory
}
func (b *FunctionBackend) Unload(ctx context.Context, model string) error {
	if model != FunctionModel {
		return b.Base.Unload(ctx, model)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stop()
	return nil
}
func (b *FunctionBackend) ensure(ctx context.Context) error {
	if b.cmd != nil {
		select {
		case <-b.wait:
			b.cmd = nil
			b.wait = nil
			_ = os.RemoveAll(b.home)
			b.home = ""
		default:
			return nil
		}
	}
	// Refuse an existing listener; never send input to an unrelated local daemon.
	l, e := net.Listen("tcp", functionAddress)
	if e != nil {
		return fault(Unavailable)
	}
	_ = l.Close()
	home, e := os.MkdirTemp("", "butler-function-")
	if e != nil {
		return fault(Unavailable)
	}
	cmd := exec.Command(b.Executable, "serve")
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + home, "TMPDIR=" + home, "OLLAMA_HOST=" + functionAddress, "OLLAMA_MODELS=" + b.ModelRoot, "OLLAMA_NO_CLOUD=1", "OLLAMA_NOHISTORY=1", "OLLAMA_DEBUG_LOG_REQUESTS=0", "OLLAMA_NOPRUNE=1", "OLLAMA_NUM_PARALLEL=1", "OLLAMA_MAX_LOADED_MODELS=1", "OLLAMA_MAX_QUEUE=1", "OLLAMA_LOAD_TIMEOUT=45s", "OLLAMA_KEEP_ALIVE=30s", "OLLAMA_CONTEXT_LENGTH=2048", "OLLAMA_VULKAN=0", "CUDA_VISIBLE_DEVICES=-1", "HIP_VISIBLE_DEVICES=-1", "ROCR_VISIBLE_DEVICES=-1"}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	setOwnedGroup(cmd)
	if e = cmd.Start(); e != nil {
		_ = os.RemoveAll(home)
		return fault(Unavailable)
	}
	b.cmd = cmd
	b.home = home
	b.wait = make(chan error, 1)
	go func() { b.wait <- cmd.Wait() }()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+functionAddress+"/api/version", nil)
		resp, e := b.client.Do(req)
		if e == nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			var v struct{ Version string }
			if resp.StatusCode == 200 && json.Unmarshal(body, &v) == nil && v.Version == "0.40.2" {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			b.stop()
			return ctx.Err()
		case <-timer.C:
			b.stop()
			return fault(Unavailable)
		case <-time.After(25 * time.Millisecond):
		}
	}
}
func (b *FunctionBackend) Infer(ctx context.Context, p *pb.ModelProfile, r *pb.InferRequest, device string, batch uint32) (*pb.InferResponse, error) {
	if p.Model != FunctionModel {
		return b.Base.Infer(ctx, p, r, device, batch)
	}
	if device != "cpu" || batch != 1 || r.GetFunction() == nil || r.GetFunction().CatalogVersion != FunctionCatalogVersion {
		return nil, fault(Invalid)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.ensure(ctx); e != nil {
		return nil, e
	}
	// Cancel the process group (including llama runner), not just HTTP. Wait and
	// unload before releasing shared RAM reservation; no opaque work survives.
	command := b.cmd
	done := make(chan struct{})
	cancel := context.AfterFunc(ctx, func() { killOwnedGroup(command); close(done) })
	defer func() {
		if !cancel() {
			<-done
		}
	}()
	prompt, e := functionPrompt(r.GetFunction().Text)
	if e != nil {
		return nil, e
	}
	request := map[string]any{"model": FunctionModel, "stream": false, "raw": true, "truncate": false, "shift": false, "prompt": prompt, "keep_alive": "30s", "options": map[string]any{"num_gpu": 0, "num_ctx": 2048, "num_predict": 192, "num_thread": 4, "temperature": 0, "seed": 42, "stop": []string{"<end_of_turn>", "<eos>", "<start_function_response>"}}}
	data, _ := json.Marshal(request)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+functionAddress+"/api/generate", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, e := b.client.Do(req)
	if e != nil {
		b.stop()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fault(Unavailable)
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if e != nil || len(raw) > 65536 || resp.StatusCode != 200 {
		b.stop()
		return nil, fault(Unavailable)
	}
	if ctx.Err() != nil {
		b.stop()
		return nil, ctx.Err()
	}
	var result struct {
		Model      string
		Done       bool
		DoneReason string `json:"done_reason"`
		Response   string
	}
	if json.Unmarshal(raw, &result) != nil || result.Model != FunctionModel || !result.Done || result.DoneReason != "stop" {
		b.stop()
		return nil, fault(Protocol)
	}
	out, e := parseFunctionText(result.Response)
	if e != nil {
		return nil, e
	}
	return &pb.InferResponse{Output: &pb.InferResponse_Function{Function: out}}, nil
}
