package semantic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"math"
	"strconv"
	"time"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/content"
)

// Admission comes from trusted run/job state. Missing admission fails closed;
// model text, caller IDs and scopes do not grant permission or budget.
type Operation struct {
	ID, Kind          string
	Scope             Scope
	Items, InputBytes int
	Timeout           time.Duration
	Origin            content.Origin
}
type originKey struct{}

func WithOrigin(ctx context.Context, origin content.Origin) context.Context {
	return context.WithValue(ctx, originKey{}, content.Derive(content.MemoryDerived, origin))
}
func operationOrigin(ctx context.Context) content.Origin {
	if o, ok := ctx.Value(originKey{}).(content.Origin); ok {
		return o
	}
	return content.Derive(content.MemoryDerived)
}

type Admission interface {
	Admit(context.Context, Operation) (func(), error)
}
type Backend interface {
	Describe(context.Context, *pb.Empty) (*pb.Catalog, error)
	Infer(context.Context, *pb.InferRequest) (*pb.InferResponse, error)
}
type Adapter struct {
	Backend    Backend
	Space      Space
	Admission  Admission
	RerankerID string
}
type Stats struct {
	Device, Fallback     string
	QueueMS, ExecutionMS uint64
	VRAMMiB, RAMMiB      uint32
}

func stats(r *pb.InferResponse) Stats {
	return Stats{r.Device, r.FallbackReason, r.QueueMs, r.ExecutionMs, r.PeakDeviceMib, r.PeakRamMib}
}
func freshID() string { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func (a *Adapter) Verify(ctx context.Context) error {
	if a == nil || a.Backend == nil || a.Space.Validate() != nil {
		return ErrUnavailable
	}
	c, err := a.Backend.Describe(ctx, &pb.Empty{})
	if err != nil || c == nil {
		return ErrUnavailable
	}
	for _, p := range c.Models {
		if p.Id == a.Space.ProfileID {
			if p.Enabled && p.Model == a.Space.Model && p.Revision == a.Space.Revision && p.PreprocessingVersion == a.Space.Preprocessing && p.RuntimeVersion == a.Space.RuntimeVersion && p.Dimensions == 768 && p.Format == "safetensors" && (p.Precision == "BF16" || p.Precision == "FP32") && p.SourceUrl == "https://huggingface.co/"+p.Model {
				for _, c := range p.Capabilities {
					if c == pb.Capability_CAPABILITY_EMBED {
						return nil
					}
				}
			}
			return ErrUnavailable
		}
	}
	return ErrUnavailable
}
func (a *Adapter) Embeddings(ctx context.Context, scope Scope, texts []*pb.TextInput, task pb.EmbeddingTask) ([][]float32, Stats, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if a == nil || a.Admission == nil {
		return nil, Stats{}, ErrAdmission
	}
	origin := operationOrigin(ctx)
	if origin.Sensitivity == content.Secret {
		return nil, Stats{}, content.ErrSecret
	}
	if !scope.valid() || len(texts) == 0 || len(texts) > 4 || task != pb.EmbeddingTask_EMBEDDING_TASK_QUERY && task != pb.EmbeddingTask_EMBEDDING_TASK_CODE_QUERY && task != pb.EmbeddingTask_EMBEDDING_TASK_DOCUMENT {
		return nil, Stats{}, ErrInvalid
	}
	bytes := 0
	seen := map[string]bool{}
	for _, t := range texts {
		if t == nil || t.Id == "" || seen[t.Id] || t.Text == "" {
			return nil, Stats{}, ErrInvalid
		}
		if sanitize.Text(t.Text) != t.Text || sanitize.Text(t.Title) != t.Title {
			return nil, Stats{}, content.ErrSecret
		}
		seen[t.Id] = true
		bytes += len(t.Text) + len(t.Title)
	}
	if bytes > 8192 {
		return nil, Stats{}, ErrInvalid
	}
	if err := a.Verify(ctx); err != nil {
		return nil, Stats{}, err
	}
	id := freshID()
	kind := "query"
	if task == pb.EmbeddingTask_EMBEDDING_TASK_DOCUMENT {
		kind = "index"
	}
	release, err := a.Admission.Admit(ctx, Operation{ID: id, Kind: kind, Scope: scope, Items: len(texts), InputBytes: bytes, Timeout: 60 * time.Second, Origin: origin})
	if err != nil {
		return nil, Stats{}, err
	}
	if release == nil {
		return nil, Stats{}, ErrAdmission
	}
	defer release()
	r, err := a.Backend.Infer(ctx, &pb.InferRequest{RequestId: id, ProfileId: a.Space.ProfileID, CallerId: "memory-" + kind, TimeoutMs: 60000, Input: &pb.InferRequest_Embed{Embed: &pb.EmbedInput{Texts: texts, Task: task, Dimensions: uint32(a.Space.Dimensions)}}})
	if err != nil {
		if ctx.Err() != nil {
			return nil, Stats{}, ctx.Err()
		}
		return nil, Stats{}, ErrUnavailable
	}
	if r == nil || r.RequestId != id || r.ProfileId != a.Space.ProfileID || r.Revision != a.Space.Revision || r.GetEmbed() == nil || r.GetEmbed().IndexSpace != a.Space.WireID() || len(r.GetEmbed().Vectors) != len(texts) {
		return nil, Stats{}, ErrInvalid
	}
	vectors := make([][]float32, len(texts))
	for i, v := range r.GetEmbed().Vectors {
		if v == nil || v.Id != texts[i].Id || ValidateVector(v.Values, a.Space.Dimensions) != nil {
			return nil, Stats{}, ErrInvalid
		}
		vectors[i] = append([]float32(nil), v.Values...)
	}
	return vectors, stats(r), nil
}
func (a *Adapter) Rerank(ctx context.Context, scope Scope, query string, hits []Hit) ([]float64, Stats, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if a == nil || a.Admission == nil {
		return nil, Stats{}, ErrAdmission
	}
	if a.Backend == nil {
		return nil, Stats{}, ErrUnavailable
	}
	parents := []content.Origin{operationOrigin(ctx)}
	for _, h := range hits {
		parents = append(parents, h.Source.MemoryOrigin())
	}
	origin := content.Derive(content.MemoryDerived, parents...)
	if origin.Sensitivity == content.Secret {
		return nil, Stats{}, content.ErrSecret
	}
	if a.RerankerID == "" || !scope.valid() || query == "" || len(hits) == 0 || len(hits) > 4 {
		return nil, Stats{}, ErrInvalid
	}
	if sanitize.Text(query) != query {
		return nil, Stats{}, content.ErrSecret
	}
	catalog, err := a.Backend.Describe(ctx, &pb.Empty{})
	if err != nil || catalog == nil {
		return nil, Stats{}, ErrUnavailable
	}
	revision := ""
	for _, p := range catalog.Models {
		if p.Id == a.RerankerID && p.Enabled && p.Model == "Qwen/Qwen3-Reranker-0.6B" && p.Revision == "e61197ed45024b0ed8a2d74b80b4d909f1255473" && p.PreprocessingVersion == "qwen3-rerank-yesno-v1" && p.Format == "safetensors" && (p.Precision == "BF16" || p.Precision == "FP32") {
			revision = p.Revision
		}
	}
	if revision == "" {
		return nil, Stats{}, ErrUnavailable
	}
	items := make([]*pb.TextInput, len(hits))
	bytes := len(query)
	for i, h := range hits {
		if sanitize.Text(h.Source.Content) != h.Source.Content || sanitize.Text(h.Source.Title) != h.Source.Title {
			return nil, Stats{}, content.ErrSecret
		}
		if h.Source.Scope != scope || h.Source.ID <= 0 {
			return nil, Stats{}, ErrInvalid
		}
		items[i] = &pb.TextInput{Id: strconv.FormatInt(h.Source.ID, 10) + ":" + h.Source.Hash(), Title: h.Source.Title, Text: h.Source.Content}
		bytes += len(h.Source.Title) + len(h.Source.Content)
	}
	if bytes > 8192 {
		return nil, Stats{}, ErrInvalid
	}
	id := freshID()
	release, err := a.Admission.Admit(ctx, Operation{ID: id, Kind: "rerank", Scope: scope, Items: len(hits), InputBytes: bytes, Timeout: 60 * time.Second, Origin: origin})
	if err != nil {
		return nil, Stats{}, err
	}
	if release == nil {
		return nil, Stats{}, ErrAdmission
	}
	defer release()
	r, err := a.Backend.Infer(ctx, &pb.InferRequest{RequestId: id, ProfileId: a.RerankerID, CallerId: "memory-rerank", TimeoutMs: 60000, Input: &pb.InferRequest_Rerank{Rerank: &pb.RerankInput{Query: query, Candidates: items}}})
	if err != nil {
		if ctx.Err() != nil {
			return nil, Stats{}, ctx.Err()
		}
		return nil, Stats{}, ErrUnavailable
	}
	if r == nil || r.RequestId != id || r.ProfileId != a.RerankerID || r.Revision != revision || r.GetRerank() == nil || len(r.GetRerank().Scores) != len(hits) {
		return nil, Stats{}, ErrInvalid
	}
	values := make([]float64, len(hits))
	for i, s := range r.GetRerank().Scores {
		if s == nil || s.Id != items[i].Id || math.IsNaN(s.Value) || math.IsInf(s.Value, 0) || s.Value < 0 || s.Value > 1 {
			return nil, Stats{}, ErrInvalid
		}
		values[i] = s.Value
	}
	return values, stats(r), nil
}
