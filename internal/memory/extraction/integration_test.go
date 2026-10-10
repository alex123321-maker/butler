package extraction

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/security/content"
)

type fakeBackend struct {
	calls  atomic.Int64
	before func()
	output func(string) *pb.ExtractOutput
}

func (f *fakeBackend) Describe(context.Context, *pb.Empty) (*pb.Catalog, error) {
	return &pb.Catalog{Models: []*pb.ModelProfile{{Id: Profile, Model: Model, Revision: Revision, RuntimeVersion: Runtime, Enabled: true, PreferredDevice: "cpu"}}}, nil
}
func (f *fakeBackend) Infer(ctx context.Context, r *pb.InferRequest) (*pb.InferResponse, error) {
	f.calls.Add(1)
	if f.before != nil {
		f.before()
	}
	out := f.output(r.GetExtract().Text)
	return &pb.InferResponse{RequestId: r.RequestId, ProfileId: Profile, Revision: Revision, Device: "cpu", Output: &pb.InferResponse_Extract{Extract: out}}, nil
}
func span(text, label, value string) *pb.Span {
	start := strings.Index(text, value)
	return &pb.Span{Type: label, Text: value, Start: uint32(utf8.RuneCountInString(text[:start])), End: uint32(utf8.RuneCountInString(text[:start+len(value)])), Confidence: .99}
}
func scenario(text string) *pb.ExtractOutput {
	fields := []*pb.Span{span(text, "action", "обновляй"), span(text, "resource", "рабочий сервер"), span(text, "deadline", "До конца месяца"), span(text, "prohibition", "не"), span(text, "exception", "тестовый можно")}
	return &pb.ExtractOutput{SchemaVersion: Schema, Spans: fields, Records: []*pb.ExtractRecord{{Type: "constraint", Fields: fields}}}
}

type fixture struct {
	catalog *catalog.Store
	jobs    *jobs.Store
	budgets *budget.Controller
	owner   context.Context
	source  catalog.Source
	job     jobs.Job
	service *Service
	backend *fakeBackend
	ref     catalog.SourceRef
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := memorytest.Database(t)
	m := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions"))
	if e := m.Initialize(); e != nil {
		t.Fatal(e)
	}
	c := catalog.New(pool, m)
	b := budget.NewController(pool, budget.DefaultPolicy())
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	target, e := budget.NewLocalTarget("local-models:9090", "pinned-local-service")
	if e != nil {
		t.Fatal(e)
	}
	j := jobs.New(pool, c, b, &jobs.LocalTarget{Endpoint: target.Endpoint(), Identity: target.Identity()})
	owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "fixture"})
	at := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)
	src, e := c.Put(owner, catalog.PutSource{ExternalKey: "source", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "До конца месяца не обновляй рабочий сервер, тестовый можно", EventTime: &at, Author: "external document"})
	if e != nil {
		t.Fatal(e)
	}
	ref := catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}
	if _, e = j.Enqueue(owner, jobs.Request{ID: "explicit-owner-extract-policy", Kind: "extract", Purpose: "memory_extract", Sources: []catalog.SourceRef{ref}, Allowance: budget.Units{ModelTurns: 3, Tokens: 10000, ActiveMillis: 180000, ProviderInputBytes: 12000}, Deadline: time.Now().Add(time.Hour)}); e != nil {
		t.Fatal(e)
	}
	job, ok, e := j.Claim(context.Background())
	if e != nil || !ok {
		t.Fatal(e)
	}
	backend := &fakeBackend{output: scenario}
	s, e := New(c, j, backend, target, budget.ModelPrice{Basis: "local_no_billing"}, "UTC")
	if e != nil {
		t.Fatal(e)
	}
	return &fixture{c, j, b, owner, src, job, s, backend, ref}
}
func TestCandidatePublicationRepeatAndSeparateOwnerAcceptanceIntegration(t *testing.T) {
	f := newFixture(t)
	claims, e := f.service.Run(context.Background(), f.job, f.ref)
	if e != nil || len(claims) != 7 {
		t.Fatal(e, len(claims))
	}
	for _, c := range claims {
		var r Record
		if json.Unmarshal(c.Value, &r) != nil || c.Status != "candidate" || !r.CandidateOnly || r.Revision != Revision || r.SourceID != f.source.ID || r.ExtractedAt.Before(r.IngestedAt) || !r.Cues.Negated || !r.Cues.Exception || r.Cues.Scope != "quoted_fragment" || r.Origin.Sensitivity != content.Private || r.Origin.Kind != content.MemoryDerived {
			t.Fatal("lost provenance/candidate boundary")
		}
	}
	var profiles int
	if e = f.catalog.Pool().QueryRow(context.Background(), `SELECT count(*) FROM memory_profile`).Scan(&profiles); e != nil || profiles != 0 {
		t.Fatal("model wrote authoritative profile", e)
	}
	repeated, e := f.service.Run(context.Background(), f.job, f.ref)
	if e != nil || len(repeated) != len(claims) || f.backend.calls.Load() != 1 {
		t.Fatal("repeat generated duplicates/call", e)
	}
	for i, c := range repeated {
		if c.ID != claims[i].ID {
			t.Fatal("retry changed identity")
		}
	}
	ledger, e := f.budgets.Ledger.Get(context.Background(), budget.ScopeID("auxiliary", "owner", f.job.ID))
	if e != nil || ledger.Used.ModelTurns != 1 {
		t.Fatal("actual call not charged once", e)
	}
	// A foreign uploaded instruction has no acceptance privilege. Exact owner
	// acceptance is deliberately tested separately from this extraction job.
	if _, e = f.catalog.Decide(context.Background(), catalog.Decision{ClaimID: claims[0].ID}); !errors.Is(e, catalog.ErrDenied) {
		t.Fatal("unproved acceptance", e)
	}
}
func TestValidEmptyExtractionCompletesAndCachesIntegration(t *testing.T) {
	f := newFixture(t)
	f.backend.output = func(string) *pb.ExtractOutput { return &pb.ExtractOutput{SchemaVersion: Schema} }
	for range 2 {
		claims, e := f.service.Run(context.Background(), f.job, f.ref)
		if e != nil || len(claims) != 0 {
			t.Fatal("valid empty publication rejected", e)
		}
	}
	if f.backend.calls.Load() != 1 {
		t.Fatal("empty publication retried inference")
	}
	var complete bool
	var ids []string
	if e := f.catalog.Pool().QueryRow(context.Background(), `SELECT complete,claim_ids FROM memory_extraction_runs`).Scan(&complete, &ids); e != nil || !complete || ids == nil || len(ids) != 0 {
		t.Fatal("empty publication not durably complete", e)
	}
}
func TestLateSourceChangeCannotPublishCandidateIntegration(t *testing.T) {
	f := newFixture(t)
	f.backend.before = func() {
		_, e := f.catalog.Put(f.owner, catalog.PutSource{ExternalKey: "source", Kind: "document", ScopeType: "user", ScopeID: "owner", ExpectedRevision: 1, Body: "Теперь разрешено другое"})
		if e != nil {
			t.Fatal(e)
		}
	}
	if _, e := f.service.Run(context.Background(), f.job, f.ref); e == nil {
		t.Fatal("late changed source published")
	}
	var count int
	if e := f.catalog.Pool().QueryRow(context.Background(), `SELECT count(*) FROM memory_claims`).Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
}
func TestForgedJobAndSecretSourceNeverReachExtractionIntegration(t *testing.T) {
	f := newFixture(t)
	forged := f.job
	forged.OwnerID = "other"
	if _, e := f.service.Run(context.Background(), forged, f.ref); e == nil || f.backend.calls.Load() != 0 {
		t.Fatal("forged job reached model")
	}
	if _, e := f.catalog.Put(f.owner, catalog.PutSource{ExternalKey: "source", Kind: "document", ScopeType: "user", ScopeID: "owner", ExpectedRevision: 1, Body: "password=synthetic-fixture-secret"}); e != nil {
		t.Fatal(e)
	}
	if _, e := f.service.Run(context.Background(), f.job, f.ref); e == nil || f.backend.calls.Load() != 0 {
		t.Fatal("secret/stale source reached model")
	}
}
func TestPartialOrFabricatedModelOutputFailsWholePublicationIntegration(t *testing.T) {
	f := newFixture(t)
	f.backend.output = func(text string) *pb.ExtractOutput { out := scenario(text); out.Spans[0].Text = "invented"; return out }
	if _, e := f.service.Run(context.Background(), f.job, f.ref); e == nil {
		t.Fatal("fabricated output accepted")
	}
	var count int
	if e := f.catalog.Pool().QueryRow(context.Background(), `SELECT count(*) FROM memory_claims`).Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
}
