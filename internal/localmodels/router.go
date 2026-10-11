package localmodels

import (
	"context"
	"fmt"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"google.golang.org/protobuf/proto"
)

// Router separates the CPU-only encoder lane from the serialized GPU lane.
// Each lane reuses one owned process, so callers cannot duplicate model copies.
type Router struct {
	GPU, CPU *Manager
	profiles map[string]*pb.ModelProfile
}

func NewRouter(cfg Config, profiles []*pb.ModelProfile, gpu, cpu Backend, telemetry Telemetry) (*Router, error) {
	if cfg.RAMTelemetry != nil {
		cfg.RAMGate = &RAMGate{}
	}
	r := &Router{profiles: map[string]*pb.ModelProfile{}}
	var gpuProfiles, cpuProfiles []*pb.ModelProfile
	for _, p := range profiles {
		if p == nil {
			return nil, fmt.Errorf("invalid profile")
		}
		if _, exists := r.profiles[p.Id]; exists {
			return nil, fmt.Errorf("duplicate profile")
		}
		switch p.PreferredDevice {
		case "cpu":
			cpuProfiles = append(cpuProfiles, p)
		case "", "gpu":
			gpuProfiles = append(gpuProfiles, p)
		default:
			return nil, fmt.Errorf("invalid preferred device")
		}
		r.profiles[p.Id] = proto.Clone(p).(*pb.ModelProfile)
	}
	var err error
	r.GPU, err = New(cfg, gpuProfiles, gpu, telemetry)
	if err != nil {
		return nil, err
	}
	r.CPU, err = New(cfg, cpuProfiles, cpu, nil)
	if err != nil {
		r.GPU.Close()
		return nil, err
	}
	return r, nil
}
func (r *Router) Close() { r.GPU.Close(); r.CPU.Close() }
func (r *Router) Catalog() *pb.Catalog {
	c := r.GPU.Catalog()
	c.Models = append(c.Models, r.CPU.Catalog().Models...)
	return c
}
func (r *Router) Status(ctx context.Context) *pb.ManagerStatus {
	s := r.GPU.Status(ctx)
	s.CpuLane = r.CPU.Status(ctx)
	s.BackendHealthy = s.BackendHealthy && s.CpuLane.BackendHealthy
	return s
}
func (r *Router) Infer(ctx context.Context, request *pb.InferRequest) (*pb.InferResponse, error) {
	if request == nil {
		return nil, fault(Invalid)
	}
	request = proto.Clone(request).(*pb.InferRequest)
	if request.ProfileId == "" {
		capability := requestCapability(request)
		for _, p := range r.profiles {
			if !p.Enabled {
				continue
			}
			for _, c := range p.Capabilities {
				if c == capability {
					if request.ProfileId != "" {
						return nil, fault(Unavailable)
					}
					request.ProfileId = p.Id
				}
			}
		}
	}
	p, ok := r.profiles[request.ProfileId]
	if !ok {
		return nil, fault(Unavailable)
	}
	if p.PreferredDevice == "cpu" {
		request.CpuOnly = true
		return r.CPU.Infer(ctx, request)
	}
	return r.GPU.Infer(ctx, request)
}
func requestCapability(r *pb.InferRequest) pb.Capability {
	switch r.Input.(type) {
	case *pb.InferRequest_Embed:
		return pb.Capability_CAPABILITY_EMBED
	case *pb.InferRequest_Rerank:
		return pb.Capability_CAPABILITY_RERANK
	case *pb.InferRequest_Extract:
		return pb.Capability_CAPABILITY_EXTRACT
	case *pb.InferRequest_Classify:
		return pb.Capability_CAPABILITY_CLASSIFY
	case *pb.InferRequest_Media:
		return pb.Capability_CAPABILITY_MEDIA
	case *pb.InferRequest_Asr:
		return pb.Capability_CAPABILITY_ASR
	case *pb.InferRequest_Vad:
		return pb.Capability_CAPABILITY_VAD
	case *pb.InferRequest_Tts:
		return pb.Capability_CAPABILITY_TTS
	case *pb.InferRequest_Function:
		return pb.Capability_CAPABILITY_FUNCTION_PROPOSE
	}
	return pb.Capability_CAPABILITY_UNSPECIFIED
}
