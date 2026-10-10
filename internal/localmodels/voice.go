package localmodels

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/butler/butler/internal/audio"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/security/workspace"
	"path/filepath"
	"unicode/utf8"
)

const ASRModel = "Qwen/Qwen3-ASR-0.6B"
const ASRRevision = "5eb144179a02acc5e5ba31e748d22b0cf3e303b0"
const TTSModel = "Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice"
const TTSRevision = "85e237c12c027371202489a0ec509ded67b5e4b5"
const VADModel = "snakers4/silero-vad"

// The package checkpoint is pinned by release and its asset digest in preparation.
const VADRevision = "5cd7945676eb32225748052e2e6a0580e4686a08"

func voiceModel(name string) bool { return name == ASRModel || name == TTSModel || name == VADModel }
func voiceOverlay(name string) string {
	switch name {
	case ASRModel:
		return "/data/voice/asr"
	case TTSModel:
		return "/data/voice/tts"
	case VADModel:
		return "/data/voice/vad"
	}
	return ""
}
func (b *Process) verifyVoice(p *pb.ModelProfile) error {
	revision, version, prep, format, source := "", "", "", "safetensors", "https://huggingface.co/"+p.Model
	switch p.Model {
	case ASRModel:
		revision = ASRRevision
		version = "torch2.10.0+cu128/transformers4.57.6/qwen-asr0.0.6"
		prep = "qwen3-asr-ru-pcm-v1"
	case TTSModel:
		revision = TTSRevision
		version = "torch2.10.0+cu128/transformers4.57.3/qwen-tts0.1.1"
		prep = "qwen3-tts-ru-ryan-v1"
	case VADModel:
		revision = VADRevision
		version = "torch2.10.0+cu128/silero-vad6.2.3"
		prep = "silero-pcm16-v1"
		format = "torchscript"
		source = "https://github.com/snakers4/silero-vad"
	}
	if p.Runtime != "torch-voice" || p.Revision != revision || p.RuntimeVersion != version || p.PreprocessingVersion != prep || p.Format != format || p.SourceUrl != source || p.Model == VADModel && (p.Precision != "FP32" || p.PreferredDevice != "cpu") || p.Model != VADModel && p.Precision != "BF16" {
		return fault(Unavailable)
	}
	data, e := workspace.ReadRegular(b.cfg.ModelRoot, filepath.Join(revision, "butler-model.json"), 65536)
	if e != nil {
		return fault(Unavailable)
	}
	var m struct {
		Model    string `json:"model"`
		Revision string `json:"revision"`
		Runtime  string `json:"runtime_version"`
	}
	if json.Unmarshal(data, &m) != nil || m.Model != p.Model || m.Revision != revision || m.Runtime != version {
		return fault(Unavailable)
	}
	return nil
}
func (b *Process) voice(ctx context.Context, p *pb.ModelProfile, r *pb.InferRequest, device string) (*pb.InferResponse, error) {
	if e := b.ensure(ctx, p, device); e != nil {
		return nil, e
	}
	req := map[string]any{}
	if in := r.GetAsr(); in != nil {
		req["op"] = "asr"
		req["wav"] = base64.StdEncoding.EncodeToString(in.Wav)
	}
	if in := r.GetVad(); in != nil {
		req["op"] = "vad"
		req["wav"] = base64.StdEncoding.EncodeToString(in.Wav)
	}
	if in := r.GetTts(); in != nil {
		req["op"] = "tts"
		req["text"] = in.Text
	}
	out, e := b.exchange(ctx, req)
	if e != nil {
		return nil, e
	}
	response := &pb.InferResponse{}
	switch req["op"] {
	case "asr":
		if out.Kind != "asr" || !utf8.ValidString(out.Text) || len(out.Text) > 4096 {
			return nil, fault(Protocol)
		}
		response.Output = &pb.InferResponse_Asr{Asr: &pb.ASROutput{Text: out.Text, DecoderComplete: out.Complete}}
	case "vad":
		pcm, _ := audio.Decode(r.GetVad().Wav, 16000, 120)
		end := uint32(0)
		if out.Kind != "vad" || len(out.Regions) > 64 {
			return nil, fault(Protocol)
		}
		for _, region := range out.Regions {
			if region == nil || region.StartSample < end || region.StartSample >= region.EndSample || uint64(region.EndSample)*2 > uint64(len(pcm.Samples)) {
				return nil, fault(Protocol)
			}
			end = region.EndSample
		}
		response.Output = &pb.InferResponse_Vad{Vad: &pb.VADOutput{Regions: out.Regions, SampleRate: 16000}}
	case "tts":
		wav, e := base64.StdEncoding.Strict().DecodeString(out.WAV)
		if e != nil || out.Kind != "tts" {
			return nil, fault(Protocol)
		}
		if _, e = audio.Decode(wav, 24000, 20); e != nil {
			return nil, fault(Protocol)
		}
		response.Output = &pb.InferResponse_Tts{Tts: &pb.TTSOutput{Wav: wav}}
	default:
		return nil, fault(Protocol)
	}
	return response, nil
}
