package semantic

import (
	"context"
	"errors"
	"math"
	"strconv"
	"testing"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/security/content"
)

func testSpace(dims int) Space {
	return Space{"embeddinggemma2-text-bf16-v1", "google/embeddinggemma-2", "914f7f89142e33e77833254d9c9b90c3cef7303b", "embeddinggemma2-retrieval-v1", "torch2.10.0+cu128/transformers5.19.0/sentence-transformers6.1.0", "memory-chunk-text-v1", dims}
}
func unitVector(dims int) []float32 { v := make([]float32, dims); v[0] = 1; return v }

type testAdmission struct {
	calls int
	deny  bool
}

func (a *testAdmission) Admit(_ context.Context, o Operation) (func(), error) {
	a.calls++
	if a.deny || o.InputBytes <= 0 {
		return nil, ErrAdmission
	}
	return func() {}, nil
}

type fakeModel struct {
	space  Space
	calls  int
	mutate func(*pb.InferResponse)
	task   pb.EmbeddingTask
}

func (f *fakeModel) Describe(context.Context, *pb.Empty) (*pb.Catalog, error) {
	return &pb.Catalog{Models: []*pb.ModelProfile{{Id: f.space.ProfileID, Model: f.space.Model, Revision: f.space.Revision, PreprocessingVersion: f.space.Preprocessing, RuntimeVersion: f.space.RuntimeVersion, Dimensions: 768, Format: "safetensors", Precision: "BF16", Enabled: true, SourceUrl: "https://huggingface.co/" + f.space.Model, Capabilities: []pb.Capability{pb.Capability_CAPABILITY_EMBED}}}}, nil
}
func (f *fakeModel) Infer(_ context.Context, r *pb.InferRequest) (*pb.InferResponse, error) {
	f.calls++
	f.task = r.GetEmbed().Task
	o := &pb.EmbedOutput{IndexSpace: f.space.WireID()}
	for _, t := range r.GetEmbed().Texts {
		o.Vectors = append(o.Vectors, &pb.Vector{Id: t.Id, Values: unitVector(f.space.Dimensions)})
	}
	out := &pb.InferResponse{RequestId: r.RequestId, ProfileId: r.ProfileId, Revision: f.space.Revision, Device: "gpu", Output: &pb.InferResponse_Embed{Embed: o}}
	if f.mutate != nil {
		f.mutate(out)
	}
	return out, nil
}
func TestAdmissionAndCorrelatedExactEmbeddingSpace(t *testing.T) {
	for _, dims := range []int{256, 768} {
		t.Run(strconv.Itoa(dims), func(t *testing.T) {
			s := testSpace(dims)
			f := &fakeModel{space: s}
			a := &Adapter{Backend: f, Space: s}
			scope := Scope{"session", "test"}
			texts := []*pb.TextInput{{Id: "a", Text: "причина запрета"}}
			if _, _, err := a.Embeddings(context.Background(), scope, texts, pb.EmbeddingTask_EMBEDDING_TASK_QUERY); !errors.Is(err, ErrAdmission) || f.calls != 0 {
				t.Fatal("missing admission executed inference")
			}
			gate := &testAdmission{}
			a.Admission = gate
			vectors, _, err := a.Embeddings(context.Background(), scope, texts, pb.EmbeddingTask_EMBEDDING_TASK_CODE_QUERY)
			if err != nil || len(vectors) != 1 || f.task != pb.EmbeddingTask_EMBEDDING_TASK_CODE_QUERY || gate.calls != 1 {
				t.Fatal(err)
			}
			for _, mutate := range []func(*pb.InferResponse){func(r *pb.InferResponse) { r.Revision = "other" }, func(r *pb.InferResponse) { r.GetEmbed().IndexSpace += "other" }, func(r *pb.InferResponse) { r.GetEmbed().Vectors[0].Id = "wrong" }, func(r *pb.InferResponse) { r.GetEmbed().Vectors[0].Values[0] = float32(math.NaN()) }, func(r *pb.InferResponse) { r.GetEmbed().Vectors[0].Values = nil }, func(r *pb.InferResponse) { r.GetEmbed().Vectors[0].Values[0] = 0 }} {
				f.mutate = mutate
				if _, _, err := a.Embeddings(context.Background(), scope, texts, pb.EmbeddingTask_EMBEDDING_TASK_QUERY); err == nil {
					t.Fatal("invalid result silently accepted")
				}
			}
		})
	}
}
func TestSecretOriginDeniesBeforeModelIPC(t *testing.T) {
	s := testSpace(768)
	f := &fakeModel{space: s}
	gate := &testAdmission{}
	a := &Adapter{Backend: f, Space: s, Admission: gate}
	ctx := WithOrigin(context.Background(), content.Origin{Kind: content.MemoryDerived, SourceRefs: []string{"message:private"}, Sensitivity: content.Secret})
	if _, _, err := a.Embeddings(ctx, Scope{"session", "test"}, []*pb.TextInput{{Id: "a", Text: "secret fixture"}}, pb.EmbeddingTask_EMBEDDING_TASK_QUERY); !errors.Is(err, content.ErrSecret) || f.calls != 0 || gate.calls != 0 {
		t.Fatal("secret reached model")
	}
	origin := Source{ID: 1, ProvenanceJSON: `{"kind":"owner_policy","source_refs":["message:original"],"parent_refs":["file:parent"],"sensitivity":"secret"}`}.MemoryOrigin()
	if origin.Sensitivity != content.Secret || origin.IsInstruction() || len(origin.ParentRefs) != 2 {
		t.Fatal("stored provenance elevated or lost refs")
	}
}
func TestKnownCredentialPatternNeverEntersEmbeddingInput(t *testing.T) {
	s := testSpace(768)
	f := &fakeModel{space: s}
	gate := &testAdmission{}
	a := &Adapter{Backend: f, Space: s, Admission: gate}
	if _, _, err := a.Embeddings(context.Background(), Scope{"session", "test"}, []*pb.TextInput{{Id: "a", Text: "password=synthetic-fixture"}}, pb.EmbeddingTask_EMBEDDING_TASK_DOCUMENT); !errors.Is(err, content.ErrSecret) || f.calls != 0 || gate.calls != 0 {
		t.Fatal("credential-like input reached IPC")
	}
}
func TestSpaceIdentityIncludesPreprocessingAndChunkVersion(t *testing.T) {
	a := testSpace(768)
	b := a
	b.ChunkVersion = "next"
	c := a
	c.Dimensions = 256
	if a.ID() == b.ID() || a.ID() == c.ID() {
		t.Fatal("incompatible spaces share identity")
	}
	a.Revision = "../../../../../../../../../../../../../../"
	if a.Validate() == nil {
		t.Fatal("invalid revision")
	}
}
