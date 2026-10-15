package semantic

import (
	"context"
	"errors"
	"strings"
	"testing"

	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/security/content"
)

func TestLongQueryPreservesBundleAndReportsLimitWithoutInference(t *testing.T) {
	// No pool/backend: an oversized query must perform neither SQL nor inference.
	r := &HybridReader{Store: &Store{Guard: labSourceGuard{}}}
	policy := memoryservice.QueryPolicy{Local: true, External: false}
	for _, query := range []string{strings.Repeat("я", 1025), strings.Repeat("x", MaxQueryBytes+1)} {
		s := memoryservice.New(memoryservice.Config{Semantic: r})
		b, err := s.BuildBundle(context.Background(), memoryservice.BundleRequest{SessionKey: "s", UserMessage: query, QueryPolicy: &policy})
		if err != nil || len(b.Items) != 0 || len(b.Diagnostics.Retrieval) != 1 || b.Diagnostics.Retrieval[0] != string(QueryInputLimit) {
			t.Fatal("search size limit failed run or became silent success", b.Diagnostics, err)
		}
		if hits, err := r.Search(context.Background(), []memoryservice.Scope{{Type: "session", ID: "s"}}, query, 4); err != nil || len(hits) != 0 {
			t.Fatal("legacy search contract still aborts long owner messages", hits, err)
		}
	}
	query := strings.Repeat("я", 1025)
	if _, err := (&HybridReader{}).SearchDetailed(context.Background(), nil, query, 4); !errors.Is(err, ErrAccess) {
		t.Fatal("size diagnostic hid missing access guard", err)
	}
	for _, scopes := range [][]memoryservice.Scope{
		{{Type: "session", ID: ""}},
		{{Type: "session", ID: "s"}, {Type: "session", ID: "s"}},
	} {
		if _, err := r.SearchDetailed(context.Background(), scopes, query, 4); !errors.Is(err, ErrInvalid) {
			t.Fatal("size diagnostic hid invalid/duplicate scopes", err)
		}
	}
	ctx := WithOrigin(context.Background(), content.Origin{Sensitivity: content.Secret})
	if _, err := r.SearchDetailed(ctx, []memoryservice.Scope{{Type: "session", ID: "s"}}, query, 4); !errors.Is(err, content.ErrSecret) {
		t.Fatal("size diagnostic hid secret classification", err)
	}
}

func TestLongQueryBoundaryIntegration(t *testing.T) {
	ctx, store, _ := database(t)
	r := &HybridReader{Store: store}
	scopes := []memoryservice.Scope{{Type: "session", ID: "semantic:query-limit"}}
	for _, query := range []string{strings.Repeat("x", MaxQueryBytes), strings.Repeat("я", MaxQueryBytes/2)} {
		report, err := r.SearchDetailed(ctx, scopes, query, 4)
		if err != nil {
			t.Fatal("supported boundary rejected", err)
		}
		for _, d := range report.Diagnostics {
			if d == QueryInputLimit {
				t.Fatal("exact byte boundary skipped")
			}
		}
	}
}
