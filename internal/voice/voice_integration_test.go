package voice

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/audio"
	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

type fixture struct {
	s       *Store
	ctx     context.Context
	p       *Processor
	backend *backend
}
type backend struct {
	calls            atomic.Int32
	complete         bool
	started, proceed chan struct{}
}

func (b *backend) Describe(context.Context, *pb.Empty) (*pb.Catalog, error) {
	result := &pb.Catalog{}
	for _, p := range []voiceProfile{vadProfile, asrProfile, ttsProfile} {
		result.Models = append(result.Models, &pb.ModelProfile{Id: p.id, Model: p.model, Revision: p.revision, Runtime: "torch-voice", Capabilities: []pb.Capability{p.capability}, Enabled: true})
	}
	return result, nil
}
func (b *backend) Infer(ctx context.Context, in *pb.InferRequest) (*pb.InferResponse, error) {
	b.calls.Add(1)
	out := &pb.InferResponse{RequestId: in.RequestId, ProfileId: in.ProfileId, Device: "cpu"}
	if in.GetVad() != nil {
		out.Revision = vadProfile.revision
		out.Output = &pb.InferResponse_Vad{Vad: &pb.VADOutput{SampleRate: 16000, Regions: []*pb.SpeechRegion{{StartSample: 0, EndSample: 16000}}}}
	}
	if in.GetAsr() != nil {
		if b.started != nil {
			close(b.started)
			<-b.proceed
		}
		out.Revision = asrProfile.revision
		out.Output = &pb.InferResponse_Asr{Asr: &pb.ASROutput{Text: "Не отправляй Ивану 17 файлов", DecoderComplete: b.complete}}
	}
	return out, nil // Deliberately supports a late callback after cancellation.
}
func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := memorytest.Database(t)
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if e := manifest.Initialize(); e != nil {
		t.Fatal(e)
	}
	catalogStore := catalog.New(pool, manifest)
	controller := budget.NewController(pool, budget.DefaultPolicy())
	controller.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	js := jobs.New(pool, catalogStore, controller, &jobs.LocalTarget{Endpoint: "local-models:9090", Identity: "voice-fixture"})
	s := &Store{Catalog: catalogStore, Jobs: js}
	target, e := budget.NewLocalTarget("local-models:9090", "voice-fixture")
	if e != nil {
		t.Fatal(e)
	}
	b := &backend{complete: true}
	p, e := NewProcessor(s, b, target, budget.ModelPrice{Basis: "local_no_billing"})
	if e != nil {
		t.Fatal(e)
	}
	return &fixture{s, owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "audio-1"}), p, b}
}
func (f *fixture) upload(t *testing.T, role Role) Clip {
	t.Helper()
	wave, e := audio.Encode(make([]byte, 32000), 16000)
	if e != nil {
		t.Fatal(e)
	}
	c, e := f.s.Upload(f.ctx, Upload{WAV: wave, Role: role, RetentionHours: 1})
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func allowance(turns int64) budget.Units {
	return budget.Units{ModelTurns: turns, Tokens: 60000, ProviderInputBytes: 8 * 1024 * 1024, ActiveMillis: 1200000}
}
func (f *fixture) claim(t *testing.T, c Clip, turns int64) jobs.Job {
	t.Helper()
	if _, e := f.s.Enqueue(f.ctx, c.ID, allowance(turns)); e != nil {
		t.Fatal(e)
	}
	j, ok, e := f.s.Jobs.ClaimPurpose(context.Background(), "ingest", Purpose)
	if e != nil || !ok {
		t.Fatal(e)
	}
	return j
}

func TestVoiceOriginalRestartIdentityPrivacyIntegration(t *testing.T) {
	f := newFixture(t)
	c := f.upload(t, OwnerCommand)
	restarted := &Store{Catalog: f.s.Catalog, Jobs: f.s.Jobs}
	wave, e := restarted.Original(f.ctx, c.ID)
	if e != nil {
		t.Fatal(e)
	}
	same, e := restarted.Upload(f.ctx, Upload{WAV: wave, Role: OwnerCommand, RetentionHours: 1})
	if e != nil || same.ID != c.ID || same.Revision != c.Revision {
		t.Fatal("accepted audio identity changed", e)
	}
	wave[44] = 1
	if _, e = restarted.Upload(f.ctx, Upload{WAV: wave, Role: OwnerCommand, RetentionHours: 1}); e == nil {
		t.Fatal("changed retry audio accepted")
	}
	foreign := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "foreign", Channel: "web", DeliveryID: "x"})
	if _, e = restarted.Original(foreign, c.ID); e == nil {
		t.Fatal("foreign audio disclosure")
	}
	if _, e = restarted.Original(context.Background(), c.ID); e == nil {
		t.Fatal("voice recognition replaced owner proof")
	}
	original, e := restarted.Original(f.ctx, c.ID)
	if e != nil || bytes.Equal(original, wave) {
		t.Fatal("original altered", e)
	}
	if _, e = restarted.Upload(f.ctx, Upload{WAV: original, Role: OwnerCommand, RetentionHours: 1, Secret: true}); e == nil {
		t.Fatal("secret recording admitted")
	}
	if e = restarted.Delete(f.ctx, c.ID); e != nil {
		t.Fatal(e)
	}
	var remaining int
	if e = f.s.Catalog.Pool().QueryRow(f.ctx, `SELECT COALESCE(octet_length(original_bytes),0) FROM memory_source_assets WHERE source_id=$1`, c.Source.ID).Scan(&remaining); e != nil || remaining != 0 {
		t.Fatal("original bytes retained after delete", e)
	}
	if f.backend.calls.Load() != 0 {
		t.Fatal("storage invoked model")
	}
}

type sink struct {
	fail  bool
	calls int
}

func (s *sink) LockTargetTx(context.Context, pgx.Tx, Target) error { return nil }
func (s *sink) SubmitReviewedTx(ctx context.Context, tx pgx.Tx, c ReviewedCommand) (string, error) {
	s.calls++
	_, e := tx.Exec(ctx, `INSERT INTO voice_fixture_inputs(id,text,source) VALUES($1,$2,$3)`, c.ClipID, c.Text, encode(c.Source))
	if e != nil {
		return "", e
	}
	if s.fail {
		return "", errors.New("fixture input persistence failure")
	}
	return c.ClipID, nil
}
func newSink(t *testing.T, f *fixture) *sink {
	t.Helper()
	if _, e := f.s.Catalog.Pool().Exec(f.ctx, `CREATE TABLE voice_fixture_inputs(id TEXT PRIMARY KEY,text TEXT,source JSONB)`); e != nil {
		t.Fatal(e)
	}
	return &sink{}
}
func TestVoiceOwnerReviewAtomicOrdinaryInputIntegration(t *testing.T) {
	f := newFixture(t)
	c := f.upload(t, OwnerCommand)
	j := f.claim(t, c, 3)
	if e := f.p.Transcribe(context.Background(), j, c.ID); e != nil {
		t.Fatal(e)
	}
	c, e := f.s.Get(f.ctx, c.ID)
	if e != nil || c.State != "review" || c.Transcript == nil {
		t.Fatal(e)
	}
	s := newSink(t, f)
	r := Review{ClipID: c.ID, ExpectedRevision: c.Revision, Text: "Не отправляй Ивану 17 файлов", Target: Target{SessionKey: "session"}}
	if _, e = f.s.Confirm(f.ctx, r, s); e == nil || s.calls != 0 {
		t.Fatal("critical details bypassed owner review")
	}
	r.CheckedCriticalDetails = true
	s.fail = true
	if _, e = f.s.Confirm(f.ctx, r, s); e == nil {
		t.Fatal("failed input commit accepted")
	}
	var rows int
	_ = f.s.Catalog.Pool().QueryRow(f.ctx, `SELECT count(*) FROM voice_fixture_inputs`).Scan(&rows)
	if rows != 0 {
		t.Fatal("input escaped failed review transaction")
	}
	c, e = f.s.Get(f.ctx, c.ID)
	if e != nil || c.State != "review" || c.Reviewed != nil {
		t.Fatal("failed review changed source/state", e)
	}
	s.fail = false
	c, e = f.s.Confirm(f.ctx, r, s)
	if e != nil || c.State != "submitted" || c.InputID == "" || c.Reviewed == nil {
		t.Fatal(e)
	}
	reviewed, e := f.s.Catalog.Get(f.ctx, c.Reviewed.ID)
	if e != nil || reviewed.Kind != "model_candidate" {
		t.Fatal("review text became memory authority", e)
	}
	if _, e = f.s.Confirm(f.ctx, r, s); e == nil {
		t.Fatal("duplicate input accepted")
	}
	_ = f.s.Catalog.Pool().QueryRow(f.ctx, `SELECT count(*) FROM voice_fixture_inputs`).Scan(&rows)
	if rows != 1 {
		t.Fatal("ordinary input duplicated")
	}
}
func TestVoicePartialAndExternalSpeechNeverCommandIntegration(t *testing.T) {
	for _, role := range []Role{OwnerCommand, Note, External} {
		t.Run(string(role), func(t *testing.T) {
			f := newFixture(t)
			c := f.upload(t, role)
			f.backend.complete = role != OwnerCommand
			j := f.claim(t, c, 3)
			if e := f.p.Transcribe(context.Background(), j, c.ID); e != nil {
				t.Fatal(e)
			}
			c, e := f.s.Get(f.ctx, c.ID)
			if e != nil {
				t.Fatal(e)
			}
			if role == OwnerCommand && c.State != "partial" {
				t.Fatal("partial decoding marked complete")
			}
			s := newSink(t, f)
			if _, e = f.s.Confirm(f.ctx, Review{ClipID: c.ID, ExpectedRevision: c.Revision, Text: c.Text, CheckedCriticalDetails: true}, s); e == nil || s.calls != 0 {
				t.Fatal("non-command/partial promoted to input")
			}
		})
	}
}
func TestVoiceFiniteActualAttemptsAndLateCancellationIntegration(t *testing.T) {
	t.Run("budget", func(t *testing.T) {
		f := newFixture(t)
		c := f.upload(t, OwnerCommand)
		j := f.claim(t, c, 1)
		if e := f.p.Transcribe(context.Background(), j, c.ID); e == nil {
			t.Fatal("unbudgeted ASR call")
		}
		if f.backend.calls.Load() != 1 {
			t.Fatal("ASR escaped finite allowance")
		}
	})
	for _, erase := range []bool{false, true} {
		name := "cancel"
		if erase {
			name = "delete"
		}
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			c := f.upload(t, OwnerCommand)
			j := f.claim(t, c, 3)
			f.backend.started = make(chan struct{})
			f.backend.proceed = make(chan struct{})
			result := make(chan error, 1)
			go func() { result <- f.p.Transcribe(context.Background(), j, c.ID) }()
			select {
			case <-f.backend.started:
			case <-time.After(5 * time.Second):
				t.Fatal("ASR not reached")
			}
			current, e := f.s.Get(f.ctx, c.ID)
			if e != nil {
				t.Fatal(e)
			}
			if erase {
				e = f.s.Delete(f.ctx, c.ID)
			} else {
				e = f.s.Cancel(f.ctx, c.ID, current.Revision)
			}
			if e != nil {
				t.Fatal(e)
			}
			close(f.backend.proceed)
			select {
			case e = <-result:
				if e == nil {
					t.Fatal("late callback published")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("late callback hung")
			}
			var transcripts int
			_ = f.s.Catalog.Pool().QueryRow(f.ctx, `SELECT count(*) FROM voice_clips WHERE clip_id=$1 AND transcript_ref IS NOT NULL`, c.ID).Scan(&transcripts)
			if transcripts != 0 {
				t.Fatal("late transcript retained")
			}
		})
	}
}
