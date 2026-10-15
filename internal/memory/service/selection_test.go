package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

type policyReaderFixture struct {
	semanticFixture
	policies []QueryPolicy
	err      error
	hits     []SemanticHit
}

func (r *policyReaderFixture) SearchWithPolicy(_ context.Context, _ []Scope, _ string, limit int, p QueryPolicy) (SearchResult, error) {
	r.policies = append(r.policies, p)
	return SearchResult{Hits: r.hits[:min(limit, len(r.hits))], Diagnostics: []string{"hybrid_base"}}, r.err
}

func TestLocalQueryPolicyDoesNotReleaseToLegacyExternalBackend(t *testing.T) {
	r := &policyReaderFixture{hits: []SemanticHit{{Title: "Склад", Summary: "Код получения СК-9157", SourceRef: "source:allowed"}}}
	s := New(Config{Semantic: r, Embeddings: panicExternalEmbedding{}})
	p := QueryPolicy{Local: true, External: false}
	b, err := s.BuildBundle(context.Background(), BundleRequest{SessionKey: "s", UserMessage: "Какой код?", QueryPolicy: &p})
	if err != nil || !strings.Contains(b.Prompt, "СК-9157") || len(r.policies) != 1 || r.policies[0].External {
		t.Fatal("local query was suppressed or externally released", err, b.Diagnostics)
	}
	s = New(Config{Semantic: &semanticFixture{}, Embeddings: panicExternalEmbedding{}})
	b, err = s.BuildBundle(context.Background(), BundleRequest{SessionKey: "s", UserMessage: "код", QueryPolicy: &p})
	if err != nil || len(b.Diagnostics.Retrieval) != 1 || b.Diagnostics.Retrieval[0] != "local_query_backend_unavailable" || len(b.Items) != 0 {
		t.Fatal("unproven backend silently succeeded", err, b.Diagnostics)
	}
	r.err = errors.New("current source revoked")
	if _, err = New(Config{Semantic: r}).BuildBundle(context.Background(), BundleRequest{SessionKey: "s", UserMessage: "код", QueryPolicy: &p}); err == nil {
		t.Fatal("source failure became empty success")
	}
}

type panicExternalEmbedding struct{}

func (panicExternalEmbedding) EmbedQuery(context.Context, string) ([]float32, error) {
	panic("external query release")
}

func TestContextSelectionPreservesFourDocumentsUnderLargeRelevantProfile(t *testing.T) {
	profiles := []ProfileEntry{}
	for i := 0; i < 25; i++ {
		profiles = append(profiles, stubProfile{fmt.Sprintf("noise-%02d", i), "Предпочтение: короткие ответы о станции"})
	}
	profiles = append(profiles, stubProfile{"station", "У станции Лесная новый вход со двора"})
	r := &policyReaderFixture{}
	for i := 0; i < 4; i++ {
		r.hits = append(r.hits, SemanticHit{Title: fmt.Sprintf("Станция %d", i), Summary: fmt.Sprintf("Факт станции номер %d", i), SourceRef: fmt.Sprintf("source:%d", i)})
	}
	s := New(Config{ProfileStore: stubProfileStore{entriesByScope: map[string][]ProfileEntry{"session:s": profiles}}, Semantic: r, WorkingStore: stubWorkingStore{snapshot: WorkingSnapshot{Goal: "Не отправлять письмо", Status: "active"}}, ScopeOrder: []string{"session"}})
	req := BundleRequest{SessionKey: "s", UserMessage: "Какой вход у станции Лесная?", IncludeQuery: true}
	b, err := s.BuildBundle(context.Background(), req)
	if err != nil || b.Diagnostics.Selected["chunks"] != 4 || !strings.Contains(b.Prompt, "новый вход со двора") || !strings.Contains(b.Prompt, "Не отправлять письмо") {
		t.Fatal("document or task starvation", err, b.Diagnostics)
	}
	for i := 0; i < 8; i++ {
		again, e := s.BuildBundle(context.Background(), req)
		if e != nil || !reflect.DeepEqual(b, again) {
			t.Fatal("selection not deterministic", e)
		}
	}
	if b.Diagnostics.Estimated > b.Diagnostics.Allowance {
		t.Fatal("overflow", b.Diagnostics)
	}
}

func TestContextSelectionBoundsLongUnicodeAndRejectsOversizedMandatoryState(t *testing.T) {
	r := &policyReaderFixture{hits: []SemanticHit{{Title: "Документ", Summary: "Полезный факт", SourceRef: "source:1"}, {Title: "Документ", Summary: "Полезный факт", SourceRef: "source:1"}}}
	s := New(Config{Semantic: r, ContextBudgetTokens: 1200, ScopeOrder: []string{"session"}, ProfileStore: stubProfileStore{entriesByScope: map[string][]ProfileEntry{"session:s": {stubProfile{"language", strings.Repeat("японский 日本語 ", 1000)}}}}})
	b, err := s.BuildBundle(context.Background(), BundleRequest{SessionKey: "s", UserMessage: "факт", IncludeQuery: true})
	if err != nil || !utf8.ValidString(b.Prompt) || b.Diagnostics.Selected["chunks"] != 1 || b.Diagnostics.Omitted["duplicate"] != 1 || b.Diagnostics.Omitted["class_quota"] != 1 || b.Diagnostics.Estimated > 1200 {
		t.Fatal("unicode/duplicate/length selection", err, b.Diagnostics)
	}
	s = New(Config{ContextBudgetTokens: 256, WorkingStore: stubWorkingStore{snapshot: WorkingSnapshot{Goal: strings.Repeat("Не отправлять. ", 100), Status: "active"}}})
	if _, err = s.BuildBundle(context.Background(), BundleRequest{SessionKey: "s"}); err == nil {
		t.Fatal("mandatory state silently shortened")
	}
	// The optional 8KiB allowance must not become a smaller mandatory-history
	// limit when the complete configured context can still hold that history.
	s = New(Config{SummaryReader: stubSummaryReader{summary: strings.Repeat("История. ", 1000)}})
	b, err = s.BuildBundle(context.Background(), BundleRequest{SessionKey: "s"})
	if err != nil || b.Items["session_summary"] == nil || b.Diagnostics.OptionalEstimated != 0 {
		t.Fatal("optional quota blocked mandatory history", err, b.Diagnostics)
	}
}

func TestContextSelectionPreservesIndependentFactsWithEqualValues(t *testing.T) {
	r := &policyReaderFixture{hits: []SemanticHit{
		{Title: "Завтрак", Summary: "09:00", SourceRef: "source:breakfast"},
		{Title: "Обед", Summary: "09:00", SourceRef: "source:lunch"},
		{Title: "Обед", Summary: "09:00", SourceRef: "source:lunch"},
	}}
	s := New(Config{Semantic: r, ScopeOrder: []string{"session"}, ProfileStore: stubProfileStore{entriesByScope: map[string][]ProfileEntry{
		"session:s": {stubProfile{"завтрак", "09:00"}, stubProfile{"обед", "09:00"}},
	}}})
	b, err := s.BuildBundle(context.Background(), BundleRequest{SessionKey: "s", UserMessage: "Во сколько завтрак и обед?", IncludeQuery: true})
	if err != nil || b.Diagnostics.Selected["profile"] != 2 || b.Diagnostics.Selected["chunks"] != 2 || b.Diagnostics.Omitted["duplicate"] != 1 {
		t.Fatal("distinct keys/sources/classes collapsed or exact duplicate retained", err, b.Diagnostics)
	}
	for _, key := range []string{"завтрак: 09:00", "обед: 09:00"} {
		if !strings.Contains(b.Prompt, key) {
			t.Fatal("independent profile fact lost", b.Prompt)
		}
	}
}

func TestContextSelectionKeepsZeroOverlapProfileBehindMatches(t *testing.T) {
	entries := []map[string]any{
		{"key": "dietary_restrictions", "summary": "Не употребляет арахис.", "scope_type": "user", "scope_id": "owner"},
		{"key": "ужин", "summary": "В 19:00", "scope_type": "user", "scope_id": "owner"},
	}
	selected := relevantProfiles(entries, "Предложи ужин.", 20)
	if len(selected) != 2 || selected[0]["key"] != "ужин" || selected[1]["key"] != "dietary_restrictions" {
		t.Fatal("zero lexical overlap became ineligibility or outranked a match", selected)
	}
	if limited := relevantProfiles(entries, "Предложи ужин.", 1); len(limited) != 1 || limited[0]["key"] != "ужин" {
		t.Fatal("fallback consumed matched profile allowance", limited)
	}
	s := New(Config{ScopeOrder: []string{"session"}, ProfileStore: stubProfileStore{entriesByScope: map[string][]ProfileEntry{
		"session:s": {stubProfile{"dietary_restrictions", "Не употребляет арахис."}},
	}}})
	b, err := s.BuildBundle(context.Background(), BundleRequest{SessionKey: "s", UserMessage: "Предложи ужин."})
	if err != nil || !strings.Contains(b.Prompt, "арахис") {
		t.Fatal("unmatched eligible profile lost despite free quota", b.Diagnostics, err)
	}
}
