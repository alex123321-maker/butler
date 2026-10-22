package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/owneridentity"
)

func TestOwnerBrowseRecentAndTextIntegration(t *testing.T) {
	s, ctx := fixture(t)
	first := put(t, s, ctx, "older", "Обсуждали поездку в Казань")
	second := put(t, s, ctx, "newer", strings.Repeat("длинный текст ", 100))
	third := put(t, s, ctx, "newest", "Обсуждали поездки в Томск")
	for i, src := range []Source{first, second, third} {
		if _, err := s.pool.Exec(ctx, `UPDATE memory_sources SET created_at=$2 WHERE source_id=$1`, src.ID, time.Date(2026, 10, 19+i, 12, 0, 0, 0, time.UTC)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Correct(ctx, Correction{SourceID: first.ID, ExpectedRevision: 1, Title: "Исправленная поездка", Body: first.Body, Author: "owner"}); err != nil {
		t.Fatal(err)
	}
	page, err := s.Browse(ctx, "", 2, 0)
	if err != nil || page.Mode != "recent" || !page.HasMore || len(page.Items) != 2 || page.Items[0].ID != third.ID || page.Items[1].ID != second.ID {
		t.Fatalf("recent order: %+v %v", page, err)
	}
	if !page.Items[1].PreviewTruncated || len([]rune(page.Items[1].Body)) != 512 {
		t.Fatal("unbounded preview")
	}
	tail, err := s.Browse(ctx, "", 2, 2)
	if err != nil || tail.HasMore || len(tail.Items) != 1 || tail.Items[0].ID != first.ID || tail.Items[0].Revision != 2 {
		t.Fatalf("current head page: %+v %v", tail, err)
	}
	found, err := s.Browse(ctx, "поездка", 6, 0)
	if err != nil || found.Mode != "text" || len(found.Diagnostics) != 1 || len(found.Items) != 2 {
		t.Fatalf("Russian text query mislabeled/lost: %+v %v", found, err)
	}
	foreign := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "other", Channel: "web", DeliveryID: "other"})
	other, err := s.Browse(foreign, "поездка", 6, 0)
	if err != nil || len(other.Items) != 0 {
		t.Fatal("foreign owner content", err)
	}
	if _, err = s.Browse(context.Background(), "", 6, 0); !errors.Is(err, ErrDenied) {
		t.Fatal("unproved browse", err)
	}
	if _, err = s.Delete(ctx, third.ID, "revoke", 1); err != nil {
		t.Fatal(err)
	}
	found, err = s.Browse(ctx, "поездка", 6, 0)
	if err != nil || len(found.Items) != 1 || found.Items[0].ID != first.ID {
		t.Fatal("revoked source visible", err)
	}
	for _, q := range []struct {
		query         string
		limit, offset int
	}{{"", 0, 0}, {"", 21, 0}, {"", 6, -1}, {"", 6, 1001}, {strings.Repeat("a", 2049), 6, 0}} {
		if _, err = s.Browse(ctx, q.query, q.limit, q.offset); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid bounds", err)
		}
	}
}

func TestOwnerBrowseRechecksSourceAncestryIntegration(t *testing.T) {
	s, ctx := fixture(t)
	parent := put(t, s, ctx, "original", "Первоначальные сведения")
	child, err := s.Put(ctx, PutSource{ExternalKey: "derived", Kind: "model_candidate", ScopeType: "user", ScopeID: "owner", Body: "Производные сведения", Parents: []SourceRef{ref(parent)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Correct(ctx, Correction{SourceID: parent.ID, ExpectedRevision: 1, Title: parent.Title, Body: "Изменённые сведения", Author: "owner"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Browse(ctx, "Производные", 6, 0); !errors.Is(err, ErrStale) {
		t.Fatalf("stale derivative %s published: %v", child.ID, err)
	}
}

func TestOwnerBrowseClosesCredentialPreviewIntegration(t *testing.T) {
	s, ctx := fixture(t)
	secret := "synthetic-preview-secret"
	_, err := s.Put(ctx, PutSource{ExternalKey: "password=" + secret, Kind: "document", ScopeType: "user", ScopeID: "owner", Title: "Credentials", Body: strings.Repeat("п ", 510) + "Bearer " + secret, Author: "Bearer " + secret, Locator: json.RawMessage(`{"password":"` + secret + `"}`)})
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.Browse(ctx, "", 6, 0)
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("unclosed secret: %+v %v", page, err)
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), secret) {
		t.Fatal("credential metadata copied into preview")
	}
}

func TestOwnerBrowseSearchShowsTheMatchedPassageIntegration(t *testing.T) {
	s, ctx := fixture(t)
	put(t, s, ctx, "long-file", strings.Repeat("Старая история без нужного ответа. ", 100)+"Монстеру поливают вечером после заката. "+strings.Repeat("Остальной текст документа. ", 100))
	page, err := s.Browse(ctx, "монстера", 6, 0)
	if err != nil || len(page.Items) != 1 || !strings.Contains(page.Items[0].Body, "Монстеру") || len([]rune(page.Items[0].Body)) > 512 || strings.Contains(page.Items[0].Body, "<b>") {
		t.Fatalf("unhelpful/unsafe preview: %+v %v", page, err)
	}
}
