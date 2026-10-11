package voice

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/audio"
	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/semantic"
	"github.com/butler/butler/internal/owneridentity"
)

// These failure cases do not replace or retune the frozen human quality gate.
func TestVoiceActualBoundedFailureMatrix(t *testing.T) {
	token, root := os.Getenv("BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE"), os.Getenv("BUTLER_VOICE_CORPUS")
	if token == "" || root == "" {
		t.Skip("explicit private service and public frozen corpus required")
	}
	if path := os.Getenv("BUTLER_VOICE_FAILURE_REPORT"); path != "" {
		file, e := os.CreateTemp(filepath.Dir(path), ".voice-report-preflight-*")
		if e != nil {
			t.Fatal("failure report preflight unavailable")
		}
		name := file.Name()
		_ = file.Close()
		_ = os.Remove(name)
	}
	f := newFixture(t)
	target, _ := budget.NewLocalTarget("local-models:9090", "voice-fixture")
	actual, closeBackend, e := semantic.NewPinnedBackend(target, token)
	if e != nil {
		t.Fatal("backend unavailable")
	}
	defer closeBackend()
	f.p, e = NewProcessor(f.s, actual, target, budget.ModelPrice{Basis: "local_no_billing"})
	if e != nil {
		t.Fatal(e)
	}
	cases := []map[string]any{}
	defer func() {
		report := map[string]any{"version": "voice-bounded-failures-v1", "quality_gate_changed": false, "human_quality_gate_passed": false, "cases": cases}
		raw, _ := json.MarshalIndent(report, "", "  ")
		if path := os.Getenv("BUTLER_VOICE_FAILURE_REPORT"); path != "" {
			if e := writeVoiceReport(path, raw); e != nil {
				t.Error("failure report persistence unavailable")
			}
		}
		t.Log(string(raw))
	}()
	run := func(name string, wav []byte, role Role) {
		f.ctx = owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "failures:" + name})
		clip, e := f.s.Upload(f.ctx, Upload{WAV: wav, Role: role, RetentionHours: 1})
		if e != nil {
			t.Fatal("fixture upload unavailable")
		}
		if _, e = f.s.Enqueue(f.ctx, clip.ID, allowance(17)); e != nil {
			t.Fatal("finite job unavailable")
		}
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		worked, runErr := f.p.ProcessOne(ctx)
		e = runErr
		if !worked {
			t.Fatal("owned voice job not processed")
		}
		cancel()
		item := map[string]any{"case": name, "millis": time.Since(start).Milliseconds(), "role": role}
		if e != nil {
			item["error"] = safeCode(e)
		}
		current, getErr := f.s.Get(f.ctx, clip.ID)
		if getErr != nil {
			t.Fatal("original source lost")
		}
		item["state"] = current.State
		item["text_present"] = current.Text != ""
		item["automatic_input"] = current.InputID != ""
		if current.InputID != "" {
			t.Fatal("failure probe acquired command authority")
		}
		if _, e = f.s.Original(f.ctx, clip.ID); e != nil {
			t.Fatal("original unavailable after inference")
		}
		item["original_retained"] = true
		if name == "silence" && current.State != "partial" {
			t.Error("silence was not an uncertain preview")
		}
		cases = append(cases, item)
		_ = f.s.Delete(f.ctx, clip.ID)
	}
	silence, _ := audio.Encode(make([]byte, 2*16000*2), 16000)
	run("silence", silence, OwnerCommand)
	random := rand.New(rand.NewPCG(1, 2))
	samples := make([]byte, 2*16000*2)
	for i := 0; i < len(samples); i += 2 {
		binary.LittleEndian.PutUint16(samples[i:], uint16(int16(random.IntN(12001)-6000)))
	}
	noise, _ := audio.Encode(samples, 16000)
	run("deterministic_background_noise", noise, External)
	raw, e := os.ReadFile(filepath.Join(root, "corpus.json"))
	if e != nil {
		t.Fatal("corpus unavailable")
	}
	var corpus speechCorpus
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Samples) != 8 {
		t.Fatal("corpus changed")
	}
	wave, e := os.ReadFile(filepath.Join(root, corpus.Samples[0].File))
	if e != nil {
		t.Fatal("public source unavailable")
	}
	pcm, e := audio.Decode(wave, 16000, 15)
	if e != nil {
		t.Fatal(e)
	}
	long := append(append(append([]byte{}, pcm.Samples...), pcm.Samples...), pcm.Samples...)
	longWave, _ := audio.Encode(long, 16000)
	run("long_note_repeated_public_human_recording", longWave, Note)
	// A real CPU-only request tests the configured conservative FP32 RAM lane.
	f.ctx = owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "failures:cpu"})
	clip, e := f.s.Upload(f.ctx, Upload{WAV: wave, Role: External, RetentionHours: 1})
	if e != nil {
		t.Fatal(e)
	}
	job := f.claim(t, clip, 2)
	clip, e = f.s.Get(f.ctx, clip.ID)
	if e != nil {
		t.Fatal("current clip unavailable")
	}
	src, e := f.s.Catalog.Get(f.ctx, clip.Source.ID)
	if e != nil {
		t.Fatal(e)
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	out, e := f.p.call(f.s.Jobs.Context(ctx, job), job, clip, asrProfile, &pb.InferRequest{CpuOnly: true, Input: &pb.InferRequest_Asr{Asr: &pb.ASRInput{Wav: wave}}}, src.Origin)
	cancel()
	cpu := map[string]any{"case": "asr_cpu_only_finite_ram", "millis": time.Since(started).Milliseconds(), "hard_ram_policy_unchanged": true}
	if e != nil {
		cpu["error"] = safeCode(e)
	} else {
		cpu["device"] = out.Device
		cpu["decoder_complete"] = out.GetAsr().DecoderComplete
	}
	cases = append(cases, cpu)
	_ = f.s.Delete(f.ctx, clip.ID)
	// Close only this caller's gRPC connection; the shared service is untouched.
	closeBackend()
	run("caller_connection_unavailable", wave, External)
}

func writeVoiceReport(path string, raw []byte) error {
	file, e := os.CreateTemp(filepath.Dir(path), ".voice-report-*")
	if e != nil {
		return e
	}
	name := file.Name()
	defer os.Remove(name)
	if _, e = file.Write(raw); e != nil {
		_ = file.Close()
		return e
	}
	if e = file.Sync(); e != nil {
		_ = file.Close()
		return e
	}
	if e = file.Close(); e != nil {
		return e
	}
	return os.Rename(name, path)
}
