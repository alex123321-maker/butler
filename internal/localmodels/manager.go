package localmodels

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"google.golang.org/protobuf/proto"
)

type Config struct {
	QueueSize      int
	BudgetMiB      uint32
	ReserveMiB     uint32
	IdleTimeout    time.Duration
	RequestTimeout time.Duration
	SampleInterval time.Duration
	RAMBudgetMiB   uint32
	RAMReserveMiB  uint32
	RAMTelemetry   RAMTelemetry
	RAMGate        *RAMGate
}
type job struct {
	ctx       context.Context
	request   *pb.InferRequest
	submitted time.Time
	reply     chan result
}
type result struct {
	response *pb.InferResponse
	err      error
}
type Manager struct {
	cfg        Config
	profiles   map[string]*pb.ModelProfile
	backend    Backend
	telemetry  Telemetry
	queue      chan job
	slots      chan struct{}
	pending    []job
	lastCaller string
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	mu         sync.Mutex
	status     pb.ManagerStatus
	lastUsed   time.Time
}

func New(cfg Config, profiles []*pb.ModelProfile, backend Backend, telemetry Telemetry) (*Manager, error) {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 16
	}
	if cfg.QueueSize > 256 {
		return nil, fmt.Errorf("queue size must be <=256")
	}
	if cfg.BudgetMiB == 0 {
		cfg.BudgetMiB = 6144
	}
	if cfg.ReserveMiB == 0 {
		cfg.ReserveMiB = 512
	}
	if cfg.ReserveMiB >= cfg.BudgetMiB {
		return nil, fmt.Errorf("reserve must be smaller than GPU budget")
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 30 * time.Second
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 60 * time.Second
	}
	if cfg.SampleInterval <= 0 {
		cfg.SampleInterval = 250 * time.Millisecond
	}
	if backend == nil {
		return nil, fmt.Errorf("backend is required")
	}
	catalog := map[string]*pb.ModelProfile{}
	for _, p := range profiles {
		if p == nil || p.Id == "" || p.Model == "" || p.Revision == "" || p.Format == "" || p.Precision == "" || p.License == "" || p.LicenseUrl == "" || p.SourceUrl == "" || p.Runtime == "" || p.RuntimeVersion == "" || p.MaxBatch == 0 || p.MaxInputBytes == 0 || p.ContextTokens == 0 || len(p.Capabilities) == 0 {
			return nil, fmt.Errorf("incomplete local model profile")
		}
		if _, ok := catalog[p.Id]; ok {
			return nil, fmt.Errorf("duplicate model profile")
		}
		catalog[p.Id] = proto.Clone(p).(*pb.ModelProfile)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if cfg.RAMTelemetry != nil {
		if cfg.RAMBudgetMiB == 0 {
			cfg.RAMBudgetMiB = 6144
		}
		if cfg.RAMReserveMiB == 0 {
			cfg.RAMReserveMiB = 512
		}
		if cfg.RAMReserveMiB >= cfg.RAMBudgetMiB {
			cancel()
			return nil, fmt.Errorf("RAM reserve must be smaller than budget")
		}
		if cfg.RAMGate == nil {
			cfg.RAMGate = &RAMGate{}
		}
	}
	m := &Manager{cfg: cfg, profiles: catalog, backend: backend, telemetry: telemetry, queue: make(chan job, cfg.QueueSize), slots: make(chan struct{}, cfg.QueueSize), ctx: ctx, cancel: cancel, done: make(chan struct{}), status: pb.ManagerStatus{State: "idle", BudgetMib: cfg.BudgetMiB, ReserveMib: cfg.ReserveMiB, RamBudgetMib: cfg.RAMBudgetMiB}}
	go m.loop()
	return m, nil
}
func (m *Manager) Close() { m.cancel(); <-m.done }
func (m *Manager) Catalog() *pb.Catalog {
	c := &pb.Catalog{}
	for _, p := range m.profiles {
		c.Models = append(c.Models, proto.Clone(p).(*pb.ModelProfile))
	}
	sort.Slice(c.Models, func(i, j int) bool { return c.Models[i].Id < c.Models[j].Id })
	return c
}
func (m *Manager) Status(ctx context.Context) *pb.ManagerStatus {
	m.mu.Lock()
	s := proto.Clone(&m.status).(*pb.ManagerStatus)
	m.mu.Unlock()
	s.Queued = uint32(len(m.slots))
	s.BackendHealthy = m.backend.Healthy(ctx)
	if m.telemetry != nil {
		s.Gpu, _ = m.telemetry.Snapshot(ctx)
	}
	if s.Gpu == nil {
		s.Gpu = &pb.GPUStatus{}
	}
	if m.cfg.RAMTelemetry != nil {
		if ram, err := m.cfg.RAMTelemetry.SnapshotRAM(ctx); err == nil {
			s.ProcessRamMib = ram.UsedMiB
		}
	}
	return s
}
func (m *Manager) Infer(ctx context.Context, request *pb.InferRequest) (*pb.InferResponse, error) {
	if request == nil {
		return nil, fault(Invalid)
	}
	// The caller cannot mutate queued input or a catalog entry after validation.
	request = proto.Clone(request).(*pb.InferRequest)
	p, ok := m.profiles[request.ProfileId]
	if !ok || !p.Enabled {
		return nil, fault(Unavailable)
	}
	if err := validateRequest(p, request); err != nil {
		return nil, err
	}
	timeout := m.cfg.RequestTimeout
	if request.TimeoutMs > 0 && time.Duration(request.TimeoutMs)*time.Millisecond < timeout {
		timeout = time.Duration(request.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	j := job{ctx: ctx, request: request, submitted: time.Now(), reply: make(chan result, 1)}
	select {
	case <-m.ctx.Done():
		return nil, fault(Closed)
	default:
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case m.slots <- struct{}{}:
	default:
		return nil, fault(QueueFull)
	}
	select {
	case m.queue <- j:
	case <-m.ctx.Done():
		<-m.slots
		return nil, fault(Closed)
	case <-ctx.Done():
		<-m.slots
		return nil, ctx.Err()
	}
	select {
	case <-m.ctx.Done():
		return nil, fault(Closed)
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-j.reply:
		return r.response, r.err
	}
}
func (m *Manager) loop() {
	defer close(m.done)
	tick := time.NewTicker(max(time.Millisecond, min(m.cfg.IdleTimeout/2, time.Second)))
	defer tick.Stop()
	defer m.unload()
	for {
		if m.ctx.Err() != nil {
			return
		}
		// Take the complete accepted waiting set before choosing a caller. New
		// arrivals cannot overtake earlier work for the same caller or fill more
		// than QueueSize slots while the previous operation is executing.
		for len(m.queue) > 0 {
			m.pending = append(m.pending, <-m.queue)
		}
		if len(m.pending) > 0 {
			j := m.nextJob()
			<-m.slots
			m.executeJob(j)
			continue
		}
		select {
		case <-m.ctx.Done():
			return
		case <-tick.C:
			if !m.lastUsed.IsZero() && time.Since(m.lastUsed) >= m.cfg.IdleTimeout {
				m.unload()
			}
		case j := <-m.queue:
			m.pending = append(m.pending, j)
		}
	}
}
func (m *Manager) nextJob() job {
	callers := []string{}
	seen := map[string]bool{}
	for _, j := range m.pending {
		if !seen[j.request.CallerId] {
			callers = append(callers, j.request.CallerId)
			seen[j.request.CallerId] = true
		}
	}
	caller := callers[0]
	for i, c := range callers {
		if c == m.lastCaller {
			caller = callers[(i+1)%len(callers)]
			break
		}
	}
	index := 0
	for i, j := range m.pending {
		if j.request.CallerId == caller {
			index = i
			break
		}
	}
	j := m.pending[index]
	m.pending = append(m.pending[:index], m.pending[index+1:]...)
	m.lastCaller = j.request.CallerId
	return j
}
func (m *Manager) executeJob(j job) {
	if j.ctx.Err() != nil {
		return
	}
	started := time.Now()
	ctx, cancel := context.WithCancel(j.ctx)
	stop := context.AfterFunc(m.ctx, cancel)
	response, err := m.execute(ctx, m.profiles[j.request.ProfileId], j.request)
	stop()
	cancel()
	m.lastUsed = time.Now()
	m.mu.Lock()
	m.status.State = "idle"
	if err != nil {
		m.status.Failed++
	} else {
		m.status.Completed++
	}
	m.mu.Unlock()
	if response != nil {
		response.RequestId = j.request.RequestId
		response.ProfileId = j.request.ProfileId
		response.Revision = m.profiles[j.request.ProfileId].Revision
		response.QueueMs = uint64(started.Sub(j.submitted).Milliseconds())
		response.ExecutionMs = uint64(time.Since(started).Milliseconds())
	}
	j.reply <- result{response: response, err: err}
}
func (m *Manager) unload() error {
	m.mu.Lock()
	model := m.status.ActiveModel
	m.mu.Unlock()
	if model == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.backend.Unload(ctx, model); err != nil {
		m.mu.Lock()
		m.status.State = "unload_failed"
		m.mu.Unlock()
		return err
	}
	m.mu.Lock()
	m.status.ActiveModel = ""
	m.status.ActiveDevice = ""
	m.status.State = "idle"
	m.mu.Unlock()
	m.lastUsed = time.Time{}
	return nil
}
func (m *Manager) execute(ctx context.Context, p *pb.ModelProfile, request *pb.InferRequest) (*pb.InferResponse, error) {
	if err := m.backend.Verify(ctx, p); err != nil {
		return nil, err
	}
	m.mu.Lock()
	active := m.status.ActiveModel
	device := m.status.ActiveDevice
	m.status.State = "running"
	m.mu.Unlock()
	if active != "" && (active != p.Model || request.CpuOnly && device != "cpu") {
		if err := m.unload(); err != nil {
			return nil, fault(Resources)
		}
	}
	device = "gpu"
	reason := ""
	if request.CpuOnly {
		device = "cpu"
	} else if !m.admitGPU(ctx, p) {
		device = "cpu"
		reason = "insufficient_gpu_resources"
	}
	if device == "cpu" && !p.CpuFallback {
		return nil, fault(Resources)
	}
	m.mu.Lock()
	residentDevice := m.status.ActiveDevice
	m.mu.Unlock()
	if residentDevice != "" && residentDevice != device {
		if err := m.unload(); err != nil {
			return nil, fault(Resources)
		}
	}
	response, err := m.run(ctx, p, request, device, p.MaxBatch)
	if device == "gpu" && (isFault(err, OOM) || isFault(err, Resources)) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err := m.unload(); err != nil {
			return nil, fault(Resources)
		}
		if isFault(err, OOM) && p.MaxBatch > 1 && m.admitGPU(ctx, p) {
			m.mu.Lock()
			m.status.OomRetries++
			m.mu.Unlock()
			response, err = m.run(ctx, p, request, "gpu", 1)
		}
		if (isFault(err, OOM) || isFault(err, Resources)) && p.CpuFallback {
			if err := m.unload(); err != nil {
				return nil, fault(Resources)
			}
			device = "cpu"
			reason = "gpu_resource_fallback"
			response, err = m.run(ctx, p, request, device, 1)
		}
	}
	if err != nil {
		return nil, err
	}
	if response == nil || response.Output == nil {
		return nil, fault(Protocol)
	}
	response.Device = device
	response.FallbackReason = reason
	if reason != "" {
		m.mu.Lock()
		m.status.CpuFallbacks++
		m.mu.Unlock()
	}
	return response, nil
}
func (m *Manager) admitGPU(ctx context.Context, p *pb.ModelProfile) bool {
	if m.telemetry == nil || p.PeakVramMib == 0 {
		return false
	}
	gpu, err := m.telemetry.Snapshot(ctx)
	if err != nil || gpu == nil || !gpu.Available {
		return false
	}
	resident, err := m.backend.Resident(ctx)
	if err != nil {
		return false
	}
	own := uint32(0)
	for _, r := range resident {
		// Never evict an unknown model, even if another process shares this backend.
		if r.Model != p.Model {
			return false
		}
		own += r.VRAMMiB
	}
	required := p.PeakVramMib
	if own < required {
		required -= own
	} else {
		required = 0
	}
	limit := min(m.cfg.BudgetMiB, gpu.TotalMib) - min(m.cfg.ReserveMiB, min(m.cfg.BudgetMiB, gpu.TotalMib))
	return gpu.UsedMib <= limit && required <= limit-gpu.UsedMib && required <= gpu.FreeMib
}
func (m *Manager) run(ctx context.Context, p *pb.ModelProfile, r *pb.InferRequest, device string, batch uint32) (*pb.InferResponse, error) {
	release, admitted := m.acquireRAM(ctx, p, device)
	if !admitted {
		return nil, fault(Resources)
	}
	defer release()
	m.mu.Lock()
	m.status.ActiveModel = p.Model
	m.status.ActiveDevice = device
	m.status.State = "running"
	m.mu.Unlock()
	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	peak := uint32(0)
	ramPeak := uint32(0)
	done := make(chan struct{})
	joined := make(chan struct{})
	if device == "gpu" || m.cfg.RAMTelemetry != nil {
		go func() {
			defer close(joined)
			ticker := time.NewTicker(m.cfg.SampleInterval)
			defer ticker.Stop()
			for {
				sampleCtx, stop := context.WithTimeout(runCtx, 2*time.Second)
				var gpu *pb.GPUStatus
				var err error
				if device == "gpu" {
					gpu, err = m.telemetry.Snapshot(sampleCtx)
				}
				if m.cfg.RAMTelemetry != nil {
					ram, ramErr := m.cfg.RAMTelemetry.SnapshotRAM(sampleCtx)
					if ramErr == nil {
						ramPeak = max(ramPeak, ram.UsedMiB)
					}
					if ramErr != nil || ram.UsedMiB+m.cfg.RAMReserveMiB > m.cfg.RAMBudgetMiB || ram.AvailableMiB < m.cfg.RAMReserveMiB {
						stop()
						cancel(fault(Resources))
						return
					}
					ramPeak = max(ramPeak, ram.UsedMiB)
				}
				stop()
				if runCtx.Err() != nil {
					return
				}
				if device == "gpu" && (err != nil || gpu == nil || !gpu.Available) {
					cancel(fault(Resources))
					return
				}
				if device == "gpu" {
					peak = max(peak, gpu.UsedMib)
				}
				if device == "gpu" && gpu.UsedMib+m.cfg.ReserveMiB > min(m.cfg.BudgetMiB, gpu.TotalMib) {
					cancel(fault(Resources))
					return
				}
				select {
				case <-done:
					return
				case <-runCtx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	} else {
		close(joined)
	}
	response, err := m.backend.Infer(runCtx, p, r, device, batch)
	close(done)
	<-joined
	if m.cfg.RAMTelemetry != nil && runCtx.Err() == nil {
		ram, ramErr := m.cfg.RAMTelemetry.SnapshotRAM(runCtx)
		if ramErr == nil {
			ramPeak = max(ramPeak, ram.UsedMiB)
		}
		if ramErr != nil || ram.UsedMiB+m.cfg.RAMReserveMiB > m.cfg.RAMBudgetMiB || ram.AvailableMiB < m.cfg.RAMReserveMiB {
			cancel(fault(Resources))
		} else {
			ramPeak = max(ramPeak, ram.UsedMiB)
		}
	}
	if device == "gpu" && runCtx.Err() == nil {
		// A short inference can finish between periodic samples. Include residency
		// after execution, when the allocator still owns its cached buffers.
		sampleCtx, stop := context.WithTimeout(runCtx, 2*time.Second)
		gpu, sampleErr := m.telemetry.Snapshot(sampleCtx)
		stop()
		if sampleErr != nil || gpu == nil || !gpu.Available {
			cancel(fault(Resources))
		} else {
			peak = max(peak, gpu.UsedMib)
			if gpu.UsedMib+m.cfg.ReserveMiB > min(m.cfg.BudgetMiB, gpu.TotalMib) {
				cancel(fault(Resources))
			}
		}
	}
	if cause := context.Cause(runCtx); cause != nil {
		err = cause
		// Release even a warm worker when final sampling discovers an overrun.
		// Cancellation must not leave tensors resident until an idle timeout.
		_ = m.unload()
	}
	m.mu.Lock()
	m.status.ObservedPeakMib = max(m.status.ObservedPeakMib, peak)
	m.status.ObservedPeakRamMib = max(m.status.ObservedPeakRamMib, ramPeak)
	m.mu.Unlock()
	if response != nil {
		response.PeakDeviceMib = peak
		response.PeakRamMib = ramPeak
	}
	return response, err
}

type RAMGate struct {
	mu       sync.Mutex
	reserved uint32
}

func (m *Manager) acquireRAM(ctx context.Context, p *pb.ModelProfile, device string) (func(), bool) {
	if m.cfg.RAMTelemetry == nil {
		return func() {}, true
	}
	m.cfg.RAMGate.mu.Lock()
	defer m.cfg.RAMGate.mu.Unlock()
	peak := p.PeakRamMib
	if device == "cpu" && p.PeakCpuRamMib > 0 {
		peak = p.PeakCpuRamMib
	}
	if peak == 0 {
		return func() {}, false
	}
	ram, err := m.cfg.RAMTelemetry.SnapshotRAM(ctx)
	if err != nil || ram.UsedMiB > m.cfg.RAMBudgetMiB-m.cfg.RAMReserveMiB {
		return func() {}, false
	}
	resident, err := m.backend.Resident(ctx)
	if err != nil {
		return func() {}, false
	}
	var own uint32
	for _, r := range resident {
		if r.Model == p.Model {
			own += r.RAMMiB
		}
	}
	needed := peak - min(peak, own)
	available := m.cfg.RAMBudgetMiB - m.cfg.RAMReserveMiB - ram.UsedMiB
	if m.cfg.RAMGate.reserved > available || needed > available-m.cfg.RAMGate.reserved || needed+m.cfg.RAMGate.reserved+m.cfg.RAMReserveMiB > ram.AvailableMiB {
		return func() {}, false
	}
	m.cfg.RAMGate.reserved += needed
	return func() { m.cfg.RAMGate.mu.Lock(); m.cfg.RAMGate.reserved -= needed; m.cfg.RAMGate.mu.Unlock() }, true
}
