package voice

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/butler/butler/internal/audio"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/semantic"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Explicit opt-in lab smoke. This synthetic self-roundtrip is runtime evidence,
// not independent Russian ASR quality or proof of correct owner intent.
func TestVoiceActualPinnedModelsSmoke(t *testing.T) {
	token := os.Getenv("BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE")
	if token == "" {
		t.Skip("private pinned service file required")
	}
	f := newFixture(t)
	target, _ := budget.NewLocalTarget("local-models:9090", "voice-fixture")
	actual, closeBackend, e := semantic.NewPinnedBackend(target, token)
	if e != nil {
		t.Fatal("private backend unavailable")
	}
	defer closeBackend()
	f.p, e = NewProcessor(f.s, actual, target, budget.ModelPrice{Basis: "local_no_billing"})
	if e != nil {
		t.Fatal(e)
	}
	ctx, stop := context.WithTimeout(context.Background(), 3*time.Minute)
	defer stop()
	src, e := f.s.Catalog.Put(f.ctx, catalog.PutSource{ExternalKey: "tts-fixture", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "Не отправляй Ивану 17 файлов."})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Jobs.Enqueue(f.ctx, jobs.Request{ID: "tts-actual", Kind: "ingest", Purpose: "voice_tts", Sources: []catalog.SourceRef{ref(src)}, Allowance: allowance(2), Deadline: time.Now().Add(5 * time.Minute)}); e != nil {
		t.Fatal(e)
	}
	job, ok, e := f.s.Jobs.ClaimPurpose(ctx, "ingest", "voice_tts")
	if e != nil || !ok {
		t.Fatal(e)
	}
	report := map[string]any{"version": "voice-actual-v1", "recording_origin": "synthetic_fixed_Ryan_self_roundtrip", "independent_human_quality": false, "asr_revision": asrProfile.revision, "tts_revision": ttsProfile.revision, "vad_revision": vadProfile.revision}
	defer func() {
		raw, _ := json.MarshalIndent(report, "", "  ")
		if path := os.Getenv("BUTLER_VOICE_REPORT"); path != "" {
			if e := os.WriteFile(path, raw, 0600); e != nil {
				t.Error("report persistence unavailable")
			}
		}
		t.Log(string(raw))
	}()
	started := time.Now()
	wave, e := f.p.Synthesize(ctx, job, ref(src))
	report["tts_millis"] = time.Since(started).Milliseconds()
	if e != nil {
		report["tts_error"] = safeCode(e)
		t.Fatal("actual TTS unavailable")
	}
	report["tts_wave_bytes"] = len(wave)
	if path := os.Getenv("BUTLER_VOICE_WAV"); path != "" {
		if e = os.WriteFile(path, wave, 0600); e != nil {
			t.Fatal("public synthetic output save unavailable")
		}
	}
	pcm, e := audio.Decode(wave, 24000, 20)
	if e != nil {
		t.Fatal(e)
	}
	// Explicit test-only linear resampling of the public synthetic output. The
	// production input contract still rejects other rates and compressed codecs.
	samples := make([]byte, (len(pcm.Samples)/2*2/3)*2)
	for i := 0; i < len(samples)/2; i++ {
		pos := float64(i) * 1.5
		n := int(pos)
		v := float64(int16(binary.LittleEndian.Uint16(pcm.Samples[n*2:])))
		if n+1 < len(pcm.Samples)/2 {
			v += float64(int16(binary.LittleEndian.Uint16(pcm.Samples[(n+1)*2:]))-int16(v)) * (pos - float64(n))
		}
		binary.LittleEndian.PutUint16(samples[i*2:], uint16(int16(v)))
	}
	input, e := audio.Encode(samples, 16000)
	if e != nil {
		t.Fatal(e)
	}
	clip, e := f.s.Upload(f.ctx, Upload{WAV: input, Role: OwnerCommand, RetentionHours: 1})
	if e != nil {
		t.Fatal(e)
	}
	asr := f.claim(t, clip, 4)
	started = time.Now()
	e = f.p.Transcribe(ctx, asr, clip.ID)
	report["vad_asr_millis"] = time.Since(started).Milliseconds()
	if e != nil {
		report["asr_error"] = safeCode(e)
		t.Fatal("actual ASR/VAD unavailable")
	}
	clip, e = f.s.Get(f.ctx, clip.ID)
	if e != nil {
		t.Fatal(e)
	}
	report["transcript_state"] = clip.State
	report["synthetic_transcript"] = clip.Text
	var reservations int
	_ = f.s.Catalog.Pool().QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&reservations)
	report["durable_reservations"] = reservations
	if reservations < 3 {
		t.Fatal("actual inference reservations missing")
	}
	if e = f.s.Delete(f.ctx, clip.ID); e != nil {
		t.Fatal(e)
	}
	var bytes int
	_ = f.s.Catalog.Pool().QueryRow(ctx, `SELECT COALESCE(octet_length(original_bytes),0) FROM memory_source_assets WHERE source_id=$1`, clip.Source.ID).Scan(&bytes)
	if bytes != 0 {
		t.Fatal("audio deletion failed")
	}
	report["audio_delete_bytes"] = bytes
}
func safeCode(e error) string {
	var runtime *RuntimeFailure
	if errors.As(e, &runtime) {
		return runtime.Code.String()
	}
	code := status.Code(e)
	if code == codes.Unknown {
		return "caller_rejected"
	}
	return code.String()
}
