package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/voice"
)

type VoiceStorage interface {
	Upload(context.Context, voice.Upload) (voice.Clip, error)
	Get(context.Context, string) (voice.Clip, error)
	Original(context.Context, string) ([]byte, error)
	Enqueue(context.Context, string, budget.Units) (jobs.Job, error)
	Cancel(context.Context, string, int64) error
	Delete(context.Context, string) error
	Confirm(context.Context, voice.Review, voice.InputSink) (voice.Clip, error)
	ConfirmControl(context.Context, voice.Review, voice.ControlSink) (voice.Clip, error)
}
type VoicePresentation interface {
	Present(context.Context, catalog.SourceRef, budget.Units) ([]byte, error)
}
type VoiceServer struct {
	Enabled      bool
	Auth         *OwnerControlAuth
	Store        VoiceStorage
	Input        voice.InputSink
	Controls     voice.ControlSink
	Presentation VoicePresentation
}

// Register this optional handler explicitly. The zero configuration is disabled;
// no microphone or voice worker is activated merely by exposing a Web page.
func (s *VoiceServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := s.Auth.Authenticate(r)
	if !ok {
		writeJSON(w, 401, map[string]string{"error": "voice_owner_required"})
		return
	}
	ctx := owneridentity.WithActor(r.Context(), actor)
	path := strings.TrimPrefix(r.URL.Path, "/api/v3/voice/")
	if path == "status" && r.Method == http.MethodGet {
		writeJSON(w, 200, map[string]any{"enabled": s.Enabled && s.Store != nil, "quality_gate_passed": false, "local_audio_only": true, "max_seconds": 120, "explicit_owner_review": true})
		return
	}
	if !s.Enabled || s.Store == nil {
		writeJSON(w, 503, map[string]string{"error": "voice_disabled"})
		return
	}
	if r.Method != http.MethodGet && (len(r.Header.Get("X-Butler-Delivery-ID")) < 8 || len(r.Header.Get("X-Butler-Delivery-ID")) > 512) {
		writeJSON(w, 400, map[string]string{"error": "voice_delivery_required"})
		return
	}
	fail := func() { writeJSON(w, 409, map[string]string{"error": "voice_current_source_or_budget_unavailable"}) }
	if path == "clips" && r.Method == http.MethodPost {
		if r.Header.Get("Content-Type") != "audio/wav" {
			writeJSON(w, 415, map[string]string{"error": "voice_pcm_wav_required"})
			return
		}
		hours, e := strconv.ParseUint(r.URL.Query().Get("access_hours"), 10, 32)
		if e != nil {
			writeJSON(w, 400, map[string]string{"error": "voice_access_hours_required"})
			return
		}
		raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 44+120*16000*2))
		if e != nil {
			writeJSON(w, 413, map[string]string{"error": "voice_recording_bound"})
			return
		}
		clip, e := s.Store.Upload(ctx, voice.Upload{WAV: raw, Role: voice.Role(r.URL.Query().Get("role")), RetentionHours: uint32(hours)})
		if e != nil {
			fail()
			return
		}
		writeJSON(w, 201, clip)
		return
	}
	if path == "speech" && r.Method == http.MethodPost {
		if s.Presentation == nil {
			fail()
			return
		}
		var source catalog.SourceRef
		if !voiceJSON(w, r, &source) {
			return
		}
		wav, e := s.Presentation.Present(ctx, source, budget.Units{ModelTurns: 1, Tokens: 752, ActiveMillis: 60000, ProviderInputBytes: 8192})
		if e != nil {
			fail()
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		w.WriteHeader(200)
		_, _ = w.Write(wav)
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 || len(parts) > 3 || parts[0] != "clips" || parts[1] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[1]
	op := ""
	if len(parts) == 3 {
		op = parts[2]
	}
	switch {
	case op == "" && r.Method == http.MethodGet:
		clip, e := s.Store.Get(ctx, id)
		if e != nil {
			fail()
			return
		}
		writeJSON(w, 200, clip)
	case op == "" && r.Method == http.MethodDelete:
		if e := s.Store.Delete(ctx, id); e != nil {
			fail()
			return
		}
		writeJSON(w, 200, map[string]bool{"deleted": true})
	case op == "original" && r.Method == http.MethodGet:
		wav, e := s.Store.Original(ctx, id)
		if e != nil {
			fail()
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(wav)
	case op == "transcribe" && r.Method == http.MethodPost:
		job, e := s.Store.Enqueue(ctx, id, budget.Units{ModelTurns: 17, Tokens: 45000, ActiveMillis: 1200000, ProviderInputBytes: 32 << 20})
		if e != nil {
			fail()
			return
		}
		writeJSON(w, 202, map[string]string{"job_id": job.ID})
	case op == "cancel" && r.Method == http.MethodPost:
		var body struct {
			ExpectedRevision int64 `json:"expected_revision"`
		}
		if !voiceJSON(w, r, &body) {
			return
		}
		if e := s.Store.Cancel(ctx, id, body.ExpectedRevision); e != nil {
			fail()
			return
		}
		writeJSON(w, 200, map[string]bool{"cancelled": true})
	case (op == "review" || op == "control") && r.Method == http.MethodPost:
		var review voice.Review
		if !voiceJSON(w, r, &review) {
			return
		}
		review.ClipID = id
		var clip voice.Clip
		var e error
		if op == "control" {
			clip, e = s.Store.ConfirmControl(ctx, review, s.Controls)
		} else {
			clip, e = s.Store.Confirm(ctx, review, s.Input)
		}
		if e != nil {
			fail()
			return
		}
		writeJSON(w, 200, clip)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
func voiceJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil {
		writeJSON(w, 400, map[string]string{"error": "voice_request_invalid"})
		return false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		writeJSON(w, 400, map[string]string{"error": "voice_request_invalid"})
		return false
	}
	return true
}
