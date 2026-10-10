package semantic

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
)

func TestFusionProtectsExactIdentifiersAndSourceDiversity(t *testing.T) {
	hit := func(id int64, doc, body string) Hit {
		return Hit{Source: Source{ID: id, Scope: Scope{"session", "semantic:test"}, SourceID: doc, Content: body}}
	}
	a, b, c, d, e := hit(1, "long", "one"), hit(2, "long", "two"), hit(3, "long", "three"), hit(4, "other", "four"), hit(5, "copy", "one")
	f := hit(6, "third", "five")
	output := fuse([]Hit{d}, []Hit{a, b, c, d, e, f}, []Hit{a, b, c, f, e, d}, 4)
	if len(output) != 4 || output[0].Source.ID != 4 {
		t.Fatal("exact result or diverse sources lost", output)
	}
	for _, h := range output {
		if h.Source.ID == 3 || h.Source.ID == 5 {
			t.Fatal("duplicate or dominant source retained")
		}
	}
}

func TestTechnicalTermsAndEvidenceWindow(t *testing.T) {
	query := "Где находятся ClaimGrant, internal/actionauth/store.go и v1.27.2?"
	terms := technicalTerms(query)
	if len(terms) != 3 || terms[0] != "ClaimGrant" || terms[1] != "internal/actionauth/store.go" || terms[2] != "v1.27.2" {
		t.Fatal(terms)
	}
	if !exactOnlyQuery("`ClaimGrant`", []string{"ClaimGrant"}) || exactOnlyQuery(query, terms) {
		t.Fatal("exact query classification")
	}
	if got := technicalTerms("каталог/файл.go TLS"); len(got) != 1 {
		t.Fatal("Unicode path lost or generic acronym promoted", got)
	}
	if got := technicalTerms("TLS"); len(got) != 1 {
		t.Fatal("bare identifier lost", got)
	}
	body := strings.Repeat("текст ", 400) + "ClaimGrant выполняет проверку" + strings.Repeat(" конец", 400)
	window, clipped := evidenceExcerpt(body, "ClaimGrant", 1024)
	if !clipped || len(window) > 1024 || !utf8.ValidString(window) || !strings.Contains(window, "ClaimGrant") {
		t.Fatal("invalid evidence window")
	}
}

func TestDistinctCodeCaseIsNotADuplicate(t *testing.T) {
	output := fuse(nil, []Hit{{Source: Source{ID: 1, Content: "Foo()"}}, {Source: Source{ID: 2, Content: "foo()"}}}, nil, 4)
	if len(output) != 2 {
		t.Fatal("case-sensitive code identity discarded")
	}
}

type canceledAdmission struct{}

func (canceledAdmission) Admit(ctx context.Context, _ Operation) (context.Context, func(), error) {
	call, cancel := context.WithCancel(ctx)
	cancel()
	return call, func() {}, nil
}
func TestAdmittedCancellationPreventsInference(t *testing.T) {
	f := &fakeModel{space: testSpace(768)}
	a := &Adapter{Space: f.space, Backend: f, Admission: canceledAdmission{}}
	_, _, err := a.Embeddings(context.Background(), Scope{"session", "semantic:test"}, []*pb.TextInput{{Id: "q", Text: "why"}}, pb.EmbeddingTask_EMBEDDING_TASK_QUERY)
	if !errors.Is(err, context.Canceled) || f.calls != 0 {
		t.Fatal("discarded admitted cancellation", err)
	}
}
