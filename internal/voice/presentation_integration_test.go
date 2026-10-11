package voice

import (
	"context"
	"github.com/butler/butler/internal/audio"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"testing"
)

func TestVoicePresentationRejectsExpiredAudioAncestorBeforeModelIntegration(t *testing.T) {
	f := newFixture(t)
	clip := f.upload(t, OwnerCommand)
	job := f.claim(t, clip, 3)
	if e := f.p.Transcribe(context.Background(), job, clip.ID); e != nil {
		t.Fatal(e)
	}
	clip, e := f.s.Get(f.ctx, clip.ID)
	if e != nil || clip.Transcript == nil {
		t.Fatal("preview unavailable", e)
	}
	if _, e = f.s.Catalog.Pool().Exec(f.ctx, `UPDATE voice_clips SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE clip_id=$1`, clip.ID); e != nil {
		t.Fatal(e)
	}
	before := f.backend.calls.Load()
	if _, e = f.p.Present(f.ctx, *clip.Transcript, allowance(1)); e == nil {
		t.Fatal("expired audio derivative synthesized")
	}
	if f.backend.calls.Load() != before {
		t.Fatal("expired source reached model")
	}
	// Owner catalog administration is separate from the voice endpoint lifetime.
	if _, e = f.s.Catalog.Get(f.ctx, clip.Transcript.ID); e != nil {
		t.Fatal("TTL unexpectedly physically deleted source")
	}
	if e = f.s.Delete(f.ctx, clip.ID); e != nil {
		t.Fatal("expired recording cannot be explicitly deleted", e)
	}
}

type lateTTSBackend struct {
	*backend
	started, proceed chan struct{}
}

func (b *lateTTSBackend) Infer(ctx context.Context, in *pb.InferRequest) (*pb.InferResponse, error) {
	if in.GetTts() == nil {
		return b.backend.Infer(ctx, in)
	}
	close(b.started)
	<-b.proceed
	wave, _ := audio.Encode(make([]byte, 24000*2), 24000)
	return &pb.InferResponse{RequestId: in.RequestId, ProfileId: in.ProfileId, Revision: ttsProfile.revision, Device: "cpu", Output: &pb.InferResponse_Tts{Tts: &pb.TTSOutput{Wav: wave}}}, nil
}
func TestVoicePresentationOwnerCancellationRetainsTextAndSpentAttemptIntegration(t *testing.T) {
	f := newFixture(t)
	src, e := f.s.Catalog.Put(f.ctx, catalog.PutSource{ExternalKey: "formed-answer", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "Ответ уже сохранён."})
	if e != nil {
		t.Fatal(e)
	}
	late := &lateTTSBackend{backend: f.backend, started: make(chan struct{}), proceed: make(chan struct{})}
	f.p.backend = late
	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan error, 1)
	go func() { _, e := f.p.Present(ctx, ref(src), allowance(1)); done <- e }()
	<-late.started
	cancel()
	close(late.proceed)
	if e = <-done; e == nil {
		t.Fatal("late audio escaped cancelled request")
	}
	current, e := f.s.Catalog.Get(f.ctx, src.ID)
	if e != nil || current.Body != src.Body {
		t.Fatal("synthesis failure removed text", e)
	}
	var attempts int
	if e = f.s.Catalog.Pool().QueryRow(f.ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&attempts); e != nil || attempts != 1 {
		t.Fatal("cancelled actual attempt refunded or lost", attempts, e)
	}
}
