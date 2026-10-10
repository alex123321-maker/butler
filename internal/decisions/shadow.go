// Package decisions evaluates local event signals. Observations cannot execute,
// suppress input, revoke explicit stops, grant resources or verify completion.
package decisions

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/content"
)

var ErrDenied = errors.New("shadow decision unavailable")
var categories = []string{"остановка", "изменение условий", "уточнение", "новое поручение", "важное событие", "чужая инструкция", "недостаточно данных", "проверка результата", "подтверждённый успех"}

func Categories() []string { return append([]string(nil), categories...) }

type Backend interface {
	Describe(context.Context, *pb.Empty) (*pb.Catalog, error)
	Infer(context.Context, *pb.InferRequest) (*pb.InferResponse, error)
}
type Profile struct {
	ID       string `json:"profile"`
	Model    string `json:"model"`
	Revision string `json:"revision"`
	Runtime  string `json:"runtime"`
	Tokens   int64  `json:"context_tokens"`
}

func Profiles() []Profile {
	return []Profile{
		{"gliner2.5-multi-cpu-v1", "fastino/gliner2.5-multi-v1", "cf5593a5d45e3bbf204b9df621b13b1c0cf25ee3", "torch2.10.0+cu128/transformers5.19.0/gliner2.1.0", 512},
		{"gliner2.5-decide-cpu-v1", "fastino/GLiNER2.5-multi-Decide", "e173bca1f0c4217d7a55b3b0c705d5ece8a0218d", "torch2.10.0+cu128/transformers5.19.0/gliner2.1.0", 512},
		{"d1-omni-text-cpu-v1", "LiquidAI/d1-omni-600M", "02b55d7076f15129e59ab3f94783f32c4b088674", "torch2.10.0+cu128/transformers5.19.0/d1code02b55d7", 2048},
	}
}

type Observation struct {
	Label          string            `json:"label"`
	Score          float64           `json:"uncalibrated_score"`
	Escalate       bool              `json:"escalate"`
	Insufficient   bool              `json:"insufficient_data"`
	MarginUnknown  bool              `json:"margin_unknown"`
	ShadowOnly     bool              `json:"shadow_only"`
	EventPreserved bool              `json:"event_preserved"`
	Source         catalog.SourceRef `json:"source"`
	ModelRevision  string            `json:"model_revision"`
	QueueMS        uint64            `json:"queue_ms"`
	ExecutionMS    uint64            `json:"execution_ms"`
	PeakRAM        uint32            `json:"peak_ram_mib"`
	PeakGPU        uint32            `json:"peak_gpu_mib"`
}
type Observer struct {
	catalog *catalog.Store
	jobs    *jobs.Store
	backend Backend
	target  budget.LocalTarget
	profile Profile
}

func New(c *catalog.Store, j *jobs.Store, b Backend, target budget.LocalTarget, profileID string, price budget.ModelPrice) (*Observer, error) {
	if c == nil || j == nil || b == nil || target.Endpoint() != "local-models:9090" || target.Identity() == "" || price.Basis != "local_no_billing" || price.InputMicrosPerToken != 0 || price.OutputMicrosPerToken != 0 {
		return nil, ErrDenied
	}
	for _, p := range Profiles() {
		if p.ID == profileID {
			return &Observer{c, j, b, target, p}, nil
		}
	}
	return nil, ErrDenied
}
func normalize(out *pb.ClassifyOutput) (Observation, error) {
	o := Observation{ShadowOnly: true, EventPreserved: true, Escalate: true, Insufficient: true}
	if out == nil {
		return o, ErrDenied
	}
	allowed := map[string]bool{}
	for _, name := range categories {
		allowed[name] = true
	}
	seen := map[string]bool{}
	best, second := -1.0, -1.0
	for _, s := range out.Scores {
		if s == nil || !allowed[s.Id] || seen[s.Id] || math.IsNaN(s.Value) || math.IsInf(s.Value, 0) || s.Value < 0 || s.Value > 1 {
			return o, ErrDenied
		}
		seen[s.Id] = true
		if s.Value > best {
			second = best
			best = s.Value
			o.Label = s.Id
		} else if s.Value > second {
			second = s.Value
		}
	}
	if best < 0 || out.InsufficientData {
		o.Label = "недостаточно данных"
		return o, nil
	}
	o.Score = best
	o.MarginUnknown = second < 0
	o.Insufficient = o.Label == "недостаточно данных"
	o.Escalate = o.Insufficient || o.MarginUnknown || best < .65 || best-second < .15
	return o, nil
}

func (s *Observer) source(ctx context.Context, ref catalog.SourceRef, purpose string) (catalog.Source, error) {
	tx, e := s.catalog.Pool().Begin(ctx)
	if e != nil {
		return catalog.Source{}, ErrDenied
	}
	defer tx.Rollback(ctx)
	sources, e := s.catalog.AccessTx(ctx, tx, []catalog.SourceRef{ref}, purpose)
	if e != nil || len(sources) != 1 {
		return catalog.Source{}, ErrDenied
	}
	src := sources[0]
	if src.Origin.Sensitivity == content.Secret || sanitize.Text(src.Body) != src.Body || len(src.Body) > 1600 {
		return catalog.Source{}, ErrDenied
	}
	if e = tx.Commit(ctx); e != nil {
		return catalog.Source{}, ErrDenied
	}
	return src, nil
}

// Observe is for explicit owned shadow experiments. The extract-kind job uses
// the distinct decision_shadow purpose and publishes no accepted memory. It
// has no completion/control interface, regardless of its selected class.
func (s *Observer) Observe(ctx context.Context, j jobs.Job, ref catalog.SourceRef) (Observation, error) {
	if s == nil || j.Kind != "extract" || j.Purpose != "decision_shadow" || ref.ID == "" || ref.Revision < 1 || ref.AccessRevision < 1 || len(ref.Hash) != 64 {
		return Observation{}, ErrDenied
	}
	ctx = s.jobs.Context(ctx, j)
	source, e := s.source(ctx, ref, j.Purpose)
	if e != nil {
		return Observation{}, ErrDenied
	}
	cat, e := s.backend.Describe(ctx, &pb.Empty{})
	if e != nil || cat == nil {
		return Observation{}, ErrDenied
	}
	ready := false
	for _, p := range cat.Models {
		if p != nil && p.Id == s.profile.ID && p.Model == s.profile.Model && p.Revision == s.profile.Revision && p.RuntimeVersion == s.profile.Runtime && p.PreferredDevice == "cpu" && p.Enabled {
			ready = true
		}
	}
	if !ready {
		return Observation{}, ErrDenied
	}
	id, e := budget.NewOperationID()
	if e != nil {
		return Observation{}, ErrDenied
	}
	request := &pb.InferRequest{RequestId: id, ProfileId: s.profile.ID, CallerId: "decision-shadow", TimeoutMs: 60000, CpuOnly: true, Input: &pb.InferRequest_Classify{Classify: &pb.ClassifyInput{Text: source.Body, Labels: Categories()}}}
	raw, _ := json.Marshal(request)
	bounded, release, e := s.jobs.Admit(ctx, j, jobs.Operation{ID: id, Kind: j.Kind, Endpoint: s.target.Endpoint(), ServiceIdentity: s.target.Identity(), InputBytes: int64(len(raw)), Tokens: s.profile.Tokens, Timeout: time.Minute, Origin: source.Origin})
	if e != nil {
		return Observation{}, ErrDenied
	}
	response, e := s.backend.Infer(bounded, request)
	callError := bounded.Err()
	release()
	if e != nil || callError != nil || response == nil || response.RequestId != id || response.ProfileId != s.profile.ID || response.Revision != s.profile.Revision || response.Device != "cpu" {
		return Observation{}, ErrDenied
	}
	o, e := normalize(response.GetClassify())
	if e != nil {
		return Observation{}, e
	}
	if _, e = s.source(ctx, ref, j.Purpose); e != nil {
		return Observation{}, ErrDenied
	}
	o.Source = ref
	o.ModelRevision = response.Revision
	o.QueueMS = response.QueueMs
	o.ExecutionMS = response.ExecutionMs
	o.PeakRAM = response.PeakRamMib
	o.PeakGPU = response.PeakDeviceMib
	return o, nil
}
