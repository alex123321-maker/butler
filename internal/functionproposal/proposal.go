// Package functionproposal owns default-off function candidates, not action
// authority. There is intentionally no Execute, Stop or automatic route hook.
package functionproposal

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	toolpb "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/localmodels"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

var ErrDenied = errors.New("function proposal unavailable")

type Backend interface {
	Describe(context.Context, *pb.Empty) (*pb.Catalog, error)
	Infer(context.Context, *pb.InferRequest) (*pb.InferResponse, error)
}
type Observer struct {
	catalog *catalog.Store
	jobs    *jobs.Store
	backend Backend
	target  budget.LocalTarget
}
type Observation struct {
	Calls        []*pb.FunctionCall `json:"calls"`
	Source       catalog.SourceRef  `json:"source"`
	Insufficient bool               `json:"insufficient_data"`
	ShadowOnly   bool               `json:"shadow_only"`
	QueueMS      uint64             `json:"queue_ms"`
	ExecutionMS  uint64             `json:"execution_ms"`
	PeakRAM      uint32             `json:"peak_ram_mib"`
}

func New(c *catalog.Store, j *jobs.Store, b Backend, target budget.LocalTarget, price budget.ModelPrice) (*Observer, error) {
	if c == nil || j == nil || b == nil || target.Endpoint() != "local-models:9090" || target.Identity() == "" || price.Basis != "local_no_billing" || price.InputMicrosPerToken != 0 || price.OutputMicrosPerToken != 0 {
		return nil, ErrDenied
	}
	return &Observer{c, j, b, target}, nil
}
func (s *Observer) source(ctx context.Context, ref catalog.SourceRef) (catalog.Source, error) {
	tx, e := s.catalog.Pool().Begin(ctx)
	if e != nil {
		return catalog.Source{}, ErrDenied
	}
	defer tx.Rollback(ctx)
	values, e := s.catalog.AccessTx(ctx, tx, []catalog.SourceRef{ref}, "function_shadow")
	if e != nil || len(values) != 1 {
		return catalog.Source{}, ErrDenied
	}
	src := values[0]
	if src.Origin.Sensitivity == content.Secret || len(src.Body) > 2048 || sanitize.Text(src.Body) != src.Body {
		return catalog.Source{}, ErrDenied
	}
	return src, tx.Commit(ctx)
}

// Observe needs a current owner-created finite extract/function_shadow job.
// Documents can be measured, but never acquire an owner-command role. Sources
// are rechecked after budget wait and after output; raw input isn't persisted.
func (s *Observer) Observe(ctx context.Context, j jobs.Job, ref catalog.SourceRef) (Observation, error) {
	if s == nil || j.Kind != "extract" || j.Purpose != "function_shadow" {
		return Observation{}, ErrDenied
	}
	ctx = s.jobs.Context(ctx, j)
	src, e := s.source(ctx, ref)
	if e != nil {
		return Observation{}, e
	}
	cat, e := s.backend.Describe(ctx, &pb.Empty{})
	if e != nil || cat == nil {
		return Observation{}, ErrDenied
	}
	ready := false
	for _, p := range cat.Models {
		if p != nil && p.Id == localmodels.FunctionProfile && p.Model == localmodels.FunctionModel && p.Revision == localmodels.FunctionRevision && p.RuntimeVersion == localmodels.FunctionRuntime && p.Enabled && p.PreferredDevice == "cpu" {
			for _, cap := range p.Capabilities {
				if cap == pb.Capability_CAPABILITY_FUNCTION_PROPOSE {
					ready = true
				}
			}
		}
	}
	if !ready {
		return Observation{}, ErrDenied
	}
	id, e := budget.NewOperationID()
	if e != nil {
		return Observation{}, ErrDenied
	}
	request := &pb.InferRequest{RequestId: id, ProfileId: localmodels.FunctionProfile, CallerId: "function-shadow", TimeoutMs: 60000, CpuOnly: true, Input: &pb.InferRequest_Function{Function: &pb.FunctionInput{Text: src.Body, CatalogVersion: localmodels.FunctionCatalogVersion}}}
	raw, _ := json.Marshal(request)
	// Include a fixed 4 KiB bound for the reviewed declarations/control grammar
	// and JSON generation envelope. Profile/context caps independently refuse
	// larger input; model output cannot enlarge this trusted allowance.
	bounded, release, e := s.jobs.Admit(ctx, j, jobs.Operation{ID: id, Kind: j.Kind, Endpoint: s.target.Endpoint(), ServiceIdentity: s.target.Identity(), InputBytes: int64(len(raw)) + 4096, Tokens: 2048 + 192, Timeout: time.Minute, Origin: src.Origin})
	if e != nil {
		return Observation{}, ErrDenied
	}
	defer release()
	if _, e = s.source(bounded, ref); e != nil {
		return Observation{}, ErrDenied
	}
	out, e := s.backend.Infer(bounded, request)
	if e != nil || bounded.Err() != nil || out == nil || out.RequestId != id || out.ProfileId != localmodels.FunctionProfile || out.Revision != localmodels.FunctionRevision || out.Device != "cpu" || out.GetFunction() == nil || out.GetFunction().CatalogVersion != localmodels.FunctionCatalogVersion {
		return Observation{}, ErrDenied
	}
	normalized, e := localmodels.NormalizeFunction(out.GetFunction().Calls)
	if e != nil {
		return Observation{}, ErrDenied
	}
	if _, e = s.source(bounded, ref); e != nil {
		return Observation{}, ErrDenied
	}
	return Observation{Calls: normalized.Calls, Source: ref, Insufficient: normalized.InsufficientData, ShadowOnly: true, QueueMS: out.QueueMs, ExecutionMS: out.ExecutionMs, PeakRAM: out.PeakRamMib}, nil
}

// ToProposal is only schema/version normalization for the ordinary controller.
// It grants no permission/task revision and cannot execute a candidate. The
// caller must obtain current owner-command authority and use normal Broker
// Prepare/Authorize/Claim; a shadow Observation alone is never sufficient.
func ToProposal(call *pb.FunctionCall, current *toolpb.ToolContract) (transport.ToolCallRequest, error) {
	out, e := localmodels.NormalizeFunction([]*pb.FunctionCall{call})
	if e != nil || len(out.Calls) != 1 || current == nil {
		return transport.ToolCallRequest{}, ErrDenied
	}
	tool, revision, schema := "", "", ""
	switch call.Name {
	case "check_system":
		tool, revision, schema = "doctor.check_system", "1", `{"type":"object","additionalProperties":false}`
	case "check_database":
		tool, revision, schema = "doctor.check_database", "1", `{"type":"object","additionalProperties":false}`
	case "read_webpage":
		tool, revision, schema = "web.fetch", "1", `{"type":"object","required":["url"],"properties":{"url":{"type":"string"},"include_html":{"type":"boolean"}},"additionalProperties":false}`
	}
	if current.ToolName != tool || current.ContractRevision != revision || current.Extension != nil || (current.Status != "active" && current.Status != "enabled") {
		return transport.ToolCallRequest{}, ErrDenied
	}
	var a, b any
	if json.Unmarshal([]byte(schema), &a) != nil || json.Unmarshal([]byte(current.InputSchemaJson), &b) != nil {
		return transport.ToolCallRequest{}, ErrDenied
	}
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	if string(x) != string(y) {
		return transport.ToolCallRequest{}, ErrDenied
	}
	return transport.ToolCallRequest{ToolName: tool, ArgsJSON: out.Calls[0].ArgumentsJson}, nil
}
