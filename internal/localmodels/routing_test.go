package localmodels

import (
	"context"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"sync"
	"testing"
	"time"
)

type fakeRAM struct {
	mu              sync.Mutex
	used, available uint32
}

func (f *fakeRAM) SnapshotRAM(context.Context) (RAMStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return RAMStatus{f.used, f.available}, nil
}
func TestCallerFairnessKeepsFIFOAndGlobalQueueBound(t *testing.T) {
	started := make(chan string, 8)
	release := make(chan struct{}, 8)
	b := &fakeBackend{fn: func(ctx context.Context, _ *pb.ModelProfile, r *pb.InferRequest, _ string, _ uint32) (*pb.InferResponse, error) {
		started <- r.RequestId
		select {
		case <-release:
			return fakeResponse(r), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	m := newManager(t, Config{QueueSize: 5}, []*pb.ModelProfile{profile()}, b, &fakeTelemetry{used: 17})
	done := make(chan error, 6)
	submit := func(id, caller string) {
		r := request(id)
		r.CallerId = caller
		go func() { _, err := m.Infer(context.Background(), r); done <- err }()
	}
	submit("active", "index")
	if <-started != "active" {
		t.Fatal("start")
	}
	for _, item := range [][2]string{{"index1", "index"}, {"index2", "index"}, {"search1", "search"}, {"search2", "search"}, {"audit1", "audit"}} {
		before := len(m.slots)
		submit(item[0], item[1])
		deadline := time.Now().Add(time.Second)
		for len(m.slots) == before && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
	}
	if _, err := m.Infer(context.Background(), request("overflow")); !isFault(err, QueueFull) {
		t.Fatalf("queue not bounded: %v", err)
	}
	for _, expected := range []string{"search1", "audit1", "index1", "search2", "index2"} {
		release <- struct{}{}
		if id := <-started; id != expected {
			t.Fatalf("expected %s got %s", expected, id)
		}
	}
	release <- struct{}{}
	for range 6 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}
func TestCPULaneRunsWhileGPURequestBlocked(t *testing.T) {
	active := make(chan struct{})
	release := make(chan struct{})
	gpu := &fakeBackend{fn: func(ctx context.Context, _ *pb.ModelProfile, r *pb.InferRequest, _ string, _ uint32) (*pb.InferResponse, error) {
		close(active)
		select {
		case <-release:
			return fakeResponse(r), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	p := profile()
	cpuProfile := profile()
	cpuProfile.Id = "classify"
	cpuProfile.Model = "cpu-model"
	cpuProfile.PreferredDevice = "cpu"
	cpuProfile.Capabilities = []pb.Capability{pb.Capability_CAPABILITY_CLASSIFY}
	router, err := NewRouter(Config{}, []*pb.ModelProfile{p, cpuProfile}, gpu, &fakeBackend{}, &fakeTelemetry{used: 17})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	gpuDone := make(chan error, 1)
	go func() { _, err := router.Infer(context.Background(), request("gpu")); gpuDone <- err }()
	<-active
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := router.Infer(ctx, &pb.InferRequest{RequestId: "classify", Input: &pb.InferRequest_Classify{Classify: &pb.ClassifyInput{Text: "query", Labels: []string{"a", "b"}}}})
	if err != nil || result.Device != "cpu" || result.ProfileId != "classify" {
		t.Fatalf("CPU starved by GPU: %v %v", result, err)
	}
	close(release)
	if err := <-gpuDone; err != nil {
		t.Fatal(err)
	}
}
func TestRAMReservationsAreSharedAcrossLanesAndReleased(t *testing.T) {
	ram := &fakeRAM{used: 100, available: 10000}
	p := profile()
	p.PeakRamMib = 4000
	cfg := Config{RAMTelemetry: ram, RAMBudgetMiB: 6144, RAMReserveMiB: 512, RAMGate: &RAMGate{}}
	first := newManager(t, cfg, []*pb.ModelProfile{p}, &fakeBackend{}, &fakeTelemetry{})
	second := newManager(t, cfg, []*pb.ModelProfile{p}, &fakeBackend{}, &fakeTelemetry{})
	release, ok := first.acquireRAM(context.Background(), p, "gpu")
	if !ok {
		t.Fatal("first")
	}
	if _, ok := second.acquireRAM(context.Background(), p, "gpu"); ok {
		t.Fatal("two cold loads overcommit RAM")
	}
	release()
	again, ok := second.acquireRAM(context.Background(), p, "gpu")
	if !ok {
		t.Fatal("reservation leaked")
	}
	again()
	ram.mu.Lock()
	ram.available = 200
	ram.mu.Unlock()
	if _, ok := first.acquireRAM(context.Background(), p, "gpu"); ok {
		t.Fatal("host pressure ignored")
	}
}
func TestRAMOverrunCancelsOwnedCPUWorkAndUnloads(t *testing.T) {
	ram := &fakeRAM{used: 100, available: 10000}
	p := profile()
	p.PeakRamMib = 1000
	b := &fakeBackend{fn: func(ctx context.Context, _ *pb.ModelProfile, _ *pb.InferRequest, _ string, _ uint32) (*pb.InferResponse, error) {
		ram.mu.Lock()
		ram.used = 6000
		ram.mu.Unlock()
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	m := newManager(t, Config{RAMTelemetry: ram, SampleInterval: time.Millisecond}, []*pb.ModelProfile{p}, b, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r := request("ram-overrun")
	r.CpuOnly = true
	if _, err := m.Infer(ctx, r); !isFault(err, Resources) {
		t.Fatalf("RAM guard did not stop CPU: %v", err)
	}
	if m.Status(ctx).ObservedPeakRamMib < 6000 {
		t.Fatal("complete pressure peak not recorded")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.unloads == 0 {
		t.Fatal("owned process residency retained after overrun")
	}
}

func TestCPUFallbackUsesItsLargerFP32RAMAllowance(t *testing.T) {
	ram := &fakeRAM{used: 3000, available: 10000}
	p := profile()
	p.PeakRamMib = 1000
	p.PeakCpuRamMib = 4000
	m := newManager(t, Config{RAMTelemetry: ram}, []*pb.ModelProfile{p}, &fakeBackend{}, nil)
	release, ok := m.acquireRAM(context.Background(), p, "gpu")
	if !ok {
		t.Fatal("GPU allowance should fit")
	}
	release()
	if _, ok := m.acquireRAM(context.Background(), p, "cpu"); ok {
		t.Fatal("FP32 CPU allowance must not reuse smaller GPU RAM quote")
	}
}
