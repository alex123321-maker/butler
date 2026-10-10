package localmodels

import (
	"context"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"google.golang.org/protobuf/proto"
)

type fakeBackend struct {
	mu       sync.Mutex
	fn       func(context.Context, *pb.ModelProfile, *pb.InferRequest, string, uint32) (*pb.InferResponse, error)
	resident []Resident
	unloads  int
}

func (f *fakeBackend) Healthy(context.Context) bool                   { return true }
func (f *fakeBackend) Verify(context.Context, *pb.ModelProfile) error { return nil }
func (f *fakeBackend) Resident(context.Context) ([]Resident, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Resident(nil), f.resident...), nil
}
func (f *fakeBackend) Infer(ctx context.Context, p *pb.ModelProfile, r *pb.InferRequest, d string, b uint32) (*pb.InferResponse, error) {
	if f.fn != nil {
		return f.fn(ctx, p, r, d, b)
	}
	return fakeResponse(r), nil
}
func (f *fakeBackend) Unload(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unloads++
	f.resident = nil
	return nil
}

type fakeTelemetry struct {
	mu          sync.Mutex
	used        uint32
	unavailable bool
}

func (f *fakeTelemetry) Snapshot(context.Context) (*pb.GPUStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.unavailable {
		return &pb.GPUStatus{}, fault(Unavailable)
	}
	return &pb.GPUStatus{Available: true, TotalMib: 6144, UsedMib: f.used, FreeMib: 6144 - f.used}, nil
}
func profile() *pb.ModelProfile {
	return &pb.ModelProfile{Id: "embed-v1", Model: "model", Revision: "revision", Format: "safetensors", Precision: "BF16", License: "Apache-2.0", LicenseUrl: "https://example.org/license", SourceUrl: "https://example.org/model", Runtime: "test", RuntimeVersion: "1", Capabilities: []pb.Capability{pb.Capability_CAPABILITY_EMBED}, MaxBatch: 4, MaxInputBytes: 8192, ContextTokens: 2048, PeakVramMib: 2048, CpuFallback: true, Enabled: true, PreprocessingVersion: "v1", Dimensions: 768}
}
func request(id string) *pb.InferRequest {
	return &pb.InferRequest{RequestId: id, ProfileId: "embed-v1", Input: &pb.InferRequest_Embed{Embed: &pb.EmbedInput{Task: pb.EmbeddingTask_EMBEDDING_TASK_QUERY, Dimensions: 256, Texts: []*pb.TextInput{{Id: "text", Text: "Память хранится в PostgreSQL"}}}}}
}
func fakeResponse(r *pb.InferRequest) *pb.InferResponse {
	if r.GetClassify() != nil {
		return &pb.InferResponse{Output: &pb.InferResponse_Classify{Classify: &pb.ClassifyOutput{InsufficientData: true}}}
	}
	if r.GetRerank() != nil {
		return &pb.InferResponse{Output: &pb.InferResponse_Rerank{Rerank: &pb.RerankOutput{Scores: []*pb.Score{{Id: "candidate", Value: 0.5}}}}}
	}
	out := &pb.EmbedOutput{IndexSpace: "v1"}
	for _, t := range r.GetEmbed().Texts {
		out.Vectors = append(out.Vectors, &pb.Vector{Id: t.Id, Values: []float32{1}})
	}
	return &pb.InferResponse{Output: &pb.InferResponse_Embed{Embed: out}}
}
func newManager(t *testing.T, cfg Config, p []*pb.ModelProfile, b Backend, g Telemetry) *Manager {
	t.Helper()
	m, err := New(cfg, p, b, g)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}
func TestSerializedQueueCancellationAndLimits(t *testing.T) {
	started := make(chan string, 4)
	release := make(chan struct{})
	b := &fakeBackend{}
	b.fn = func(ctx context.Context, _ *pb.ModelProfile, r *pb.InferRequest, _ string, _ uint32) (*pb.InferResponse, error) {
		started <- r.RequestId
		select {
		case <-release:
			return fakeResponse(r), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	m := newManager(t, Config{QueueSize: 1}, []*pb.ModelProfile{profile()}, b, &fakeTelemetry{used: 17})
	firstDone := make(chan error, 1)
	go func() { _, err := m.Infer(context.Background(), request("first")); firstDone <- err }()
	<-started
	secondCtx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() { _, err := m.Infer(secondCtx, request("second")); secondDone <- err }()
	deadline := time.Now().Add(time.Second)
	for len(m.queue) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if _, err := m.Infer(context.Background(), request("third")); !isFault(err, QueueFull) {
		t.Fatalf("expected bounded queue, got %v", err)
	}
	cancel()
	if err := <-secondDone; err != context.Canceled {
		t.Fatalf("canceled queued job: %v", err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-started:
		t.Fatalf("canceled queued input executed: %s", id)
	case <-time.After(20 * time.Millisecond):
	}
}
func TestOOMRetriesWholeInputThenCPU(t *testing.T) {
	var calls []string
	b := &fakeBackend{}
	b.fn = func(_ context.Context, _ *pb.ModelProfile, r *pb.InferRequest, d string, batch uint32) (*pb.InferResponse, error) {
		calls = append(calls, d)
		if d == "gpu" {
			return nil, fault(OOM)
		}
		if batch != 1 || len(r.GetEmbed().Texts) != 4 {
			t.Error("CPU fallback lost input or did not shrink batch")
		}
		return fakeResponse(r), nil
	}
	m := newManager(t, Config{}, []*pb.ModelProfile{profile()}, b, &fakeTelemetry{used: 17})
	r := request("oom")
	for _, id := range []string{"a", "b", "c"} {
		r.GetEmbed().Texts = append(r.GetEmbed().Texts, &pb.TextInput{Id: id, Text: "данные"})
	}
	out, err := m.Infer(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if out.Device != "cpu" || out.FallbackReason != "gpu_resource_fallback" || len(out.GetEmbed().Vectors) != 4 || len(calls) != 3 {
		t.Fatalf("wrong fallback: %v %v", out, calls)
	}
	s := m.Status(context.Background())
	if s.OomRetries != 1 || s.CpuFallbacks != 1 {
		t.Fatalf("wrong resource metrics: %v", s)
	}
}
func TestFullDevicePressureCancelsGPUAndFallsBack(t *testing.T) {
	gpu := &fakeTelemetry{used: 17}
	b := &fakeBackend{}
	b.fn = func(ctx context.Context, _ *pb.ModelProfile, r *pb.InferRequest, device string, _ uint32) (*pb.InferResponse, error) {
		if device == "gpu" {
			gpu.mu.Lock()
			gpu.used = 5800
			gpu.mu.Unlock()
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return fakeResponse(r), nil
	}
	m := newManager(t, Config{SampleInterval: time.Millisecond}, []*pb.ModelProfile{profile()}, b, gpu)
	out, err := m.Infer(context.Background(), request("pressure"))
	if err != nil || out.Device != "cpu" {
		t.Fatalf("pressure fallback: %v %v", out, err)
	}
	if m.Status(context.Background()).ObservedPeakMib < 5800 {
		t.Error("full device usage not observed")
	}
}
func TestShortGPUInferenceChecksFinalWholeDevicePressure(t *testing.T) {
	gpu := &fakeTelemetry{used: 17}
	b := &fakeBackend{}
	b.fn = func(_ context.Context, _ *pb.ModelProfile, r *pb.InferRequest, device string, _ uint32) (*pb.InferResponse, error) {
		if device == "gpu" {
			gpu.mu.Lock()
			gpu.used = 5800
			gpu.mu.Unlock()
		}
		return fakeResponse(r), nil
	}
	m := newManager(t, Config{SampleInterval: time.Hour}, []*pb.ModelProfile{profile()}, b, gpu)
	out, err := m.Infer(context.Background(), request("short-pressure"))
	if err != nil || out.Device != "cpu" || m.Status(context.Background()).ObservedPeakMib < 5800 {
		t.Fatalf("short operation escaped final pressure check: %v %v", out, err)
	}
}
func TestUnknownResidentNeverEvicted(t *testing.T) {
	b := &fakeBackend{resident: []Resident{{Model: "unrelated", VRAMMiB: 3000}}}
	m := newManager(t, Config{}, []*pb.ModelProfile{profile()}, b, &fakeTelemetry{used: 3017})
	out, err := m.Infer(context.Background(), request("external"))
	if err != nil || out.Device != "cpu" {
		t.Fatalf("expected CPU: %v %v", out, err)
	}
	if b.unloads != 0 {
		t.Fatal("evicted unknown model")
	}
}
func TestIdleUnloadAndRestartLeavesNoFalseSuccess(t *testing.T) {
	b := &fakeBackend{}
	m := newManager(t, Config{IdleTimeout: 10 * time.Millisecond}, []*pb.ModelProfile{profile()}, b, &fakeTelemetry{used: 17})
	if _, err := m.Infer(context.Background(), request("idle")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		n := b.unloads
		b.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	b.mu.Lock()
	n := b.unloads
	b.mu.Unlock()
	if n == 0 {
		t.Fatal("idle worker not unloaded")
	}
	m.cancel()
	<-m.done
	if _, err := m.Infer(context.Background(), request("restart")); !isFault(err, Closed) {
		t.Fatalf("stopped manager succeeded: %v", err)
	}
}
func TestAdmissionWithoutTelemetryAndUnsupportedCapability(t *testing.T) {
	p := profile()
	p.CpuFallback = false
	m := newManager(t, Config{}, []*pb.ModelProfile{p}, &fakeBackend{}, &fakeTelemetry{unavailable: true})
	if _, err := m.Infer(context.Background(), request("no-gpu")); !isFault(err, Resources) {
		t.Fatalf("unknown usage treated as zero: %v", err)
	}
	r := request("unsupported")
	r.Input = &pb.InferRequest_Classify{Classify: &pb.ClassifyInput{Text: "Стоп", Labels: []string{"отмена", "продолжение"}}}
	if _, err := m.Infer(context.Background(), r); !isFault(err, Unavailable) {
		t.Fatalf("unsupported capability fabricated result: %v", err)
	}
}
func TestInputLimitsIncludeIdentifiersAndCandidateTitles(t *testing.T) {
	p := profile()
	m := newManager(t, Config{}, []*pb.ModelProfile{p}, &fakeBackend{}, &fakeTelemetry{used: 17})
	r := request("oversized-id")
	r.GetEmbed().Texts[0].Id = strings.Repeat("x", 129)
	if _, err := m.Infer(context.Background(), r); !isFault(err, Invalid) {
		t.Fatalf("unbounded identifier admitted: %v", err)
	}
	p.Capabilities = append(p.Capabilities, pb.Capability_CAPABILITY_RERANK)
	other := newManager(t, Config{}, []*pb.ModelProfile{p}, &fakeBackend{}, &fakeTelemetry{used: 17})
	r = request("oversized-title")
	r.Input = &pb.InferRequest_Rerank{Rerank: &pb.RerankInput{Query: "query", Candidates: []*pb.TextInput{{Id: "candidate", Text: "text", Title: strings.Repeat("x", int(p.MaxInputBytes))}}}}
	if _, err := other.Infer(context.Background(), r); !isFault(err, Invalid) {
		t.Fatalf("candidate title bypassed byte budget: %v", err)
	}
}
func TestWarmCPUModelIsReusedAcrossFallbackRequests(t *testing.T) {
	b := &fakeBackend{}
	m := newManager(t, Config{}, []*pb.ModelProfile{profile()}, b, &fakeTelemetry{unavailable: true})
	for _, id := range []string{"index", "search"} {
		out, err := m.Infer(context.Background(), request(id))
		if err != nil || out.Device != "cpu" {
			t.Fatalf("CPU fallback: %v %v", out, err)
		}
	}
	b.mu.Lock()
	n := b.unloads
	b.mu.Unlock()
	if n != 0 {
		t.Fatal("CPU model unnecessarily unloaded between requests")
	}
}
func TestModelNeutralQueueHandlesRankAndClassification(t *testing.T) {
	p := profile()
	p.Capabilities = append(p.Capabilities, pb.Capability_CAPABILITY_RERANK, pb.Capability_CAPABILITY_CLASSIFY)
	m := newManager(t, Config{}, []*pb.ModelProfile{p}, &fakeBackend{}, &fakeTelemetry{used: 17})
	r := request("rank")
	r.Input = &pb.InferRequest_Rerank{Rerank: &pb.RerankInput{Query: "Память", Candidates: []*pb.TextInput{{Id: "candidate", Text: "PostgreSQL"}}}}
	if out, err := m.Infer(context.Background(), r); err != nil || out.GetRerank() == nil {
		t.Fatalf("rank: %v %v", out, err)
	}
	r = proto.Clone(r).(*pb.InferRequest)
	r.RequestId = "classification"
	r.Input = &pb.InferRequest_Classify{Classify: &pb.ClassifyInput{Text: "Стоп", Labels: []string{"отмена", "продолжение"}}}
	if out, err := m.Infer(context.Background(), r); err != nil || !out.GetClassify().InsufficientData {
		t.Fatalf("classification: %v %v", out, err)
	}
}
func TestNormalizeVectorRejectsInvalidNativeTailAndRenormalizes(t *testing.T) {
	raw := make([]float64, 768)
	for i := range raw {
		raw[i] = 1
	}
	v, err := normalizeVector(raw, 768, 256)
	if err != nil {
		t.Fatal(err)
	}
	norm := float64(0)
	for _, n := range v {
		norm += float64(n * n)
	}
	if math.Abs(norm-1) > 1e-5 {
		t.Fatalf("norm %g", norm)
	}
	raw[767] = math.NaN()
	if _, err := normalizeVector(raw, 768, 256); !isFault(err, Protocol) {
		t.Fatal("invalid discarded tail accepted")
	}
	if _, err := normalizeVector(make([]float64, 768), 768, 256); !isFault(err, Protocol) {
		t.Fatal("zero vector accepted")
	}
}
func TestGPUCSVUnavailableDataRejected(t *testing.T) {
	if _, err := parseGPU("6144, N/A, N/A, GPU, driver\n"); err == nil {
		t.Fatal("missing telemetry accepted")
	}
	out, err := parseGPU("6144, 1000, 5144, NVIDIA RTX 3060, 595.91.07\n")
	if err != nil || out.UsedMib != 1000 {
		t.Fatalf("valid telemetry: %v %v", out, err)
	}
}
