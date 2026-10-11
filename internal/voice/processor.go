package voice

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/audio"
	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/localmodels"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/content"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
	"time"
	"unicode/utf8"
)

type Backend interface {
	Describe(context.Context, *pb.Empty) (*pb.Catalog, error)
	Infer(context.Context, *pb.InferRequest) (*pb.InferResponse, error)
}

// RuntimeFailure retains only a fixed transport enum, never raw worker errors.
type RuntimeFailure struct{ Code codes.Code }

func (e *RuntimeFailure) Error() string { return "voice runtime " + e.Code.String() }

type Processor struct {
	store   *Store
	backend Backend
	target  budget.LocalTarget
}

func NewProcessor(s *Store, b Backend, target budget.LocalTarget, price budget.ModelPrice) (*Processor, error) {
	if s == nil || s.Catalog == nil || s.Jobs == nil || b == nil || target.Endpoint() != "local-models:9090" || target.Identity() == "" || price.Basis != "local_no_billing" || price.InputMicrosPerToken != 0 || price.OutputMicrosPerToken != 0 {
		return nil, ErrDenied
	}
	return &Processor{s, b, target}, nil
}

type voiceProfile struct {
	id, model, revision string
	tokens              int64
	capability          pb.Capability
}

var vadProfile = voiceProfile{"silero-vad-pcm-cpu-v1", localmodels.VADModel, localmodels.VADRevision, 1, pb.Capability_CAPABILITY_VAD}
var asrProfile = voiceProfile{"qwen3-asr-ru-bf16-v1", localmodels.ASRModel, localmodels.ASRRevision, 2048 + 384, pb.Capability_CAPABILITY_ASR}
var ttsProfile = voiceProfile{"qwen3-tts-ru-ryan-bf16-v1", localmodels.TTSModel, localmodels.TTSRevision, 512 + 240, pb.Capability_CAPABILITY_TTS}

func (p *Processor) call(ctx context.Context, j jobs.Job, c Clip, profile voiceProfile, in *pb.InferRequest, origin content.Origin) (*pb.InferResponse, error) {
	cat, e := p.backend.Describe(ctx, &pb.Empty{})
	if e != nil || cat == nil {
		return nil, ErrDenied
	}
	ready := false
	for _, m := range cat.Models {
		if m.Id == profile.id && m.Model == profile.model && m.Revision == profile.revision && m.Runtime == "torch-voice" && m.Enabled {
			for _, capability := range m.Capabilities {
				if capability == profile.capability {
					ready = true
				}
			}
		}
	}
	if !ready {
		return nil, ErrDenied
	}
	id, e := budget.NewOperationID()
	if e != nil {
		return nil, e
	}
	in.RequestId = id
	in.ProfileId = profile.id
	in.CallerId = "voice"
	in.TimeoutMs = 60000
	raw, _ := json.Marshal(in)
	bounded, release, e := p.store.Jobs.Admit(ctx, j, jobs.Operation{ID: id, Kind: j.Kind, Endpoint: p.target.Endpoint(), ServiceIdentity: p.target.Identity(), InputBytes: int64(len(raw)), Tokens: profile.tokens, Timeout: time.Minute, Origin: origin})
	if e != nil {
		return nil, e
	}
	defer release()
	callCtx, cancel := context.WithCancel(bounded)
	defer cancel()
	done := make(chan struct{})
	if c.ID != "" {
		// Admission may have waited on shared budgets. Recheck this exact clip
		// before the first runtime dispatch, independently of the polling fence.
		var current bool
		if e := p.store.Catalog.Pool().QueryRow(callCtx, `SELECT state='pending' AND revision=$2 AND job_id=$3 AND expires_at>clock_timestamp() FROM voice_clips WHERE clip_id=$1`, c.ID, c.Revision, j.ID).Scan(&current); e != nil || !current {
			return nil, ErrDenied
		}
		go func() {
			defer close(done)
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-callCtx.Done():
					return
				case <-ticker.C:
					var yes bool
					check, stop := context.WithTimeout(callCtx, 150*time.Millisecond)
					e := p.store.Catalog.Pool().QueryRow(check, `SELECT state='pending' AND revision=$2 AND job_id=$3 AND expires_at>clock_timestamp() FROM voice_clips WHERE clip_id=$1`, c.ID, c.Revision, j.ID).Scan(&yes)
					stop()
					if e != nil || !yes {
						cancel()
						return
					}
				}
			}
		}()
	} else {
		close(done)
	}
	response, e := p.backend.Infer(callCtx, in)
	callError := callCtx.Err()
	cancel()
	<-done
	if e != nil {
		code := status.Code(e)
		if code == codes.Unknown {
			code = codes.Unavailable
		}
		return nil, &RuntimeFailure{Code: code}
	}
	if callError != nil || response == nil || response.RequestId != id || response.ProfileId != profile.id || response.Revision != profile.revision || (response.Device != "cpu" && response.Device != "gpu") {
		return nil, ErrDenied
	}
	return response, nil
}

func (p *Processor) ProcessOne(ctx context.Context) (bool, error) {
	j, ok, e := p.store.Jobs.ClaimPurpose(ctx, "ingest", Purpose)
	if e != nil || !ok {
		return ok, e
	}
	var id string
	if e = p.store.Catalog.Pool().QueryRow(ctx, `SELECT clip_id FROM voice_clips WHERE job_id=$1 AND state='pending'`, j.ID).Scan(&id); e != nil {
		_ = p.store.Jobs.Finish(ctx, j, "voice_state", false)
		return true, ErrDenied
	}
	e = p.Transcribe(ctx, j, id)
	code := ""
	if e != nil {
		code = "voice_unavailable"
		_, _ = p.store.Catalog.Pool().Exec(ctx, `UPDATE voice_clips SET state='failed',revision=revision+1,error_code=$2 WHERE clip_id=$1 AND job_id=$3 AND state='pending'`, id, code, j.ID)
	}
	finish := p.store.Jobs.Finish(ctx, j, code, false)
	if e != nil {
		return true, e
	}
	return true, finish
}

// Transcribe stores previews only. Empty/noise, truncated decoding and clipped
// segmentation never create an accepted input or an owner command.
func (p *Processor) Transcribe(ctx context.Context, j jobs.Job, id string) error {
	if j.Kind != "ingest" || j.Purpose != Purpose {
		return ErrDenied
	}
	ctx = p.store.Jobs.Context(ctx, j)
	c, wav, e := p.store.read(ctx, id, Purpose)
	if e != nil || c.State != "pending" || c.JobID != j.ID {
		return ErrDenied
	}
	tx, e := p.store.Catalog.Pool().Begin(ctx)
	if e != nil {
		return e
	}
	sources, e := p.store.Catalog.AccessTx(ctx, tx, []catalog.SourceRef{c.Source}, Purpose)
	if e != nil {
		_ = tx.Rollback(ctx)
		return e
	}
	src := sources[0]
	e = tx.Commit(ctx)
	if e != nil {
		return e
	}
	pcm, e := audio.Decode(wav, 16000, 120)
	if e != nil {
		return e
	}
	vad, e := p.call(ctx, j, c, vadProfile, &pb.InferRequest{CpuOnly: true, Input: &pb.InferRequest_Vad{Vad: &pb.AudioInput{Wav: wav}}}, src.Origin)
	if e != nil {
		return e
	}
	if vad.GetVad() == nil || vad.GetVad().SampleRate != 16000 {
		return ErrDenied
	}
	texts := []string{}
	complete := true
	prior := uint32(0)
	segments := 0
	if len(vad.GetVad().Regions) > 64 {
		return ErrDenied
	}
	for _, r := range vad.GetVad().Regions {
		if r == nil || r.StartSample < prior || r.StartSample >= r.EndSample || uint64(r.EndSample)*2 > uint64(len(pcm.Samples)) {
			return ErrDenied
		}
		prior = r.EndSample
		for start := r.StartSample; start < r.EndSample; {
			end := min(start+15*16000, r.EndSample)
			part, e := pcm.Segment(start, end)
			if e != nil {
				return e
			}
			segments++
			if segments > 16 {
				return ErrDenied
			}
			out, e := p.call(ctx, j, c, asrProfile, &pb.InferRequest{Input: &pb.InferRequest_Asr{Asr: &pb.ASRInput{Wav: part}}}, src.Origin)
			if e != nil {
				return e
			}
			if out.GetAsr() == nil {
				return ErrDenied
			}
			text := strings.TrimSpace(out.GetAsr().Text)
			if !utf8.ValidString(text) || sanitize.Text(text) != text || len(text) > 4096 {
				return ErrDenied
			}
			texts = append(texts, text)
			complete = complete && out.GetAsr().DecoderComplete
			// Long continuous speech is split without a linguistic certainty claim.
			if end < r.EndSample {
				complete = false
			}
			start = end
		}
	}
	text := strings.TrimSpace(strings.Join(texts, " "))
	state := "review"
	if !complete || text == "" {
		state = "partial"
	}
	if len(text) > 16384 {
		return ErrDenied
	}
	tx, e = p.store.Catalog.Pool().Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	child, e := p.store.Catalog.CreateDerivedTx(ctx, tx, []catalog.SourceRef{c.Source}, Purpose, "voice transcript", text, encode(map[string]any{"version": Version, "asr_revision": asrProfile.revision, "decoder_complete": complete, "segments": segments, "role": c.Role}))
	if e != nil {
		return e
	}
	tag, e := tx.Exec(ctx, `UPDATE voice_clips SET state=$2,transcript_ref=$3,revision=revision+1,updated_at=clock_timestamp() WHERE clip_id=$1 AND revision=$4 AND state='pending' AND job_id=$5 AND expires_at>clock_timestamp()`, c.ID, state, encode(ref(child)), c.Revision, j.ID)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrDenied
	}
	return tx.Commit(ctx)
}

// Synthesize is presentation only. The exact answer/source job is owner-created
// and current; output is never fed into microphone ingestion or command input.
func (p *Processor) Synthesize(ctx context.Context, j jobs.Job, source catalog.SourceRef) ([]byte, error) {
	if j.Kind != "ingest" || j.Purpose != "voice_tts" {
		return nil, ErrDenied
	}
	ctx = p.store.Jobs.Context(ctx, j)
	tx, e := p.store.Catalog.Pool().Begin(ctx)
	if e != nil {
		return nil, e
	}
	sources, e := p.store.Catalog.AccessTx(ctx, tx, []catalog.SourceRef{source}, j.Purpose)
	if e != nil {
		_ = tx.Rollback(ctx)
		return nil, e
	}
	src := sources[0]
	e = tx.Commit(ctx)
	if e != nil || src.Origin.Sensitivity == content.Secret || sanitize.Text(src.Body) != src.Body || len([]rune(src.Body)) > 200 || len(src.Body) > 1024 {
		return nil, ErrDenied
	}
	voiceUntil, expiryError := p.store.presentationDeadline(ctx, source)
	if expiryError != nil {
		return nil, expiryError
	}
	if voiceUntil != nil {
		bounded, stop := context.WithDeadline(ctx, *voiceUntil)
		defer stop()
		ctx = bounded
	}
	out, e := p.call(ctx, j, Clip{}, ttsProfile, &pb.InferRequest{Input: &pb.InferRequest_Tts{Tts: &pb.TTSInput{Text: src.Body}}}, src.Origin)
	if e != nil {
		return nil, e
	}
	if out.GetTts() == nil {
		return nil, ErrDenied
	}
	// Cancellation/source correction while queueing or during a late callback
	// invalidates presentation, even if the worker returned valid wave bytes.
	tx, e = p.store.Catalog.Pool().Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if _, e = p.store.Catalog.AccessTx(ctx, tx, []catalog.SourceRef{source}, j.Purpose); e != nil {
		return nil, e
	}
	if _, e = sourceVoiceDeadlineTx(ctx, tx, source); e != nil {
		return nil, e
	}
	wav := out.GetTts().Wav
	if _, e = audio.Decode(wav, 24000, 20); e != nil {
		return nil, ErrDenied
	}
	return wav, tx.Commit(ctx)
}
