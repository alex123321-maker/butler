package localmodels

import (
	"context"
	"github.com/butler/butler/internal/audio"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"testing"
)

func TestVoiceRequestBoundsAndCapabilities(t *testing.T) {
	wav, _ := audio.Encode(make([]byte, 16000*2), 16000)
	p := profile()
	p.Capabilities = []pb.Capability{pb.Capability_CAPABILITY_ASR}
	p.MaxInputBytes = 480044
	r := &pb.InferRequest{RequestId: "voice", ProfileId: p.Id, Input: &pb.InferRequest_Asr{Asr: &pb.ASRInput{Wav: wav}}}
	if e := validateRequest(p, r); e != nil {
		t.Fatal(e)
	}
	long, _ := audio.Encode(make([]byte, 16000*2*16), 16000)
	r.GetAsr().Wav = long
	if !isFault(validateRequest(p, r), Invalid) {
		t.Fatal("ASR accepted too-long segment")
	}
	r.GetAsr().Wav = wav
	p.Capabilities = []pb.Capability{pb.Capability_CAPABILITY_MEDIA}
	if !isFault(validateRequest(p, r), Unavailable) {
		t.Fatal("vision profile accepted audio")
	}
}
func TestMalformedVoiceWorkerOutputRejected(t *testing.T) {
	wav, _ := audio.Encode(make([]byte, 16000*2), 16000)
	p := profile()
	p.Model = VADModel
	b := processFixture(t, "read first\nprintf '%s\\n' '{\"kind\":\"ready\"}'\nread request\nprintf '%s\\n' '{\"kind\":\"vad\",\"regions\":[{\"start_sample\":0,\"end_sample\":16001}]}'\nsleep 1\n")
	_, e := b.Infer(context.Background(), p, &pb.InferRequest{Input: &pb.InferRequest_Vad{Vad: &pb.AudioInput{Wav: wav}}}, "cpu", 1)
	if !isFault(e, Protocol) {
		t.Fatal("VAD bounds lost", e)
	}
}
