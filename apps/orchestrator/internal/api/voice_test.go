package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/voice"
)

type voiceAPIFixture struct {
	calls int
	role  voice.Role
	owner string
}

func (f *voiceAPIFixture) Upload(ctx context.Context, u voice.Upload) (voice.Clip, error) {
	f.calls++
	a, _ := owneridentity.FromContext(ctx)
	f.owner = a.OwnerID
	f.role = u.Role
	return voice.Clip{ID: "clip", Role: u.Role}, nil
}
func (f *voiceAPIFixture) Get(context.Context, string) (voice.Clip, error)  { return voice.Clip{}, nil }
func (f *voiceAPIFixture) Original(context.Context, string) ([]byte, error) { return nil, nil }
func (f *voiceAPIFixture) Enqueue(context.Context, string, budget.Units) (jobs.Job, error) {
	f.calls++
	return jobs.Job{}, nil
}
func (f *voiceAPIFixture) Cancel(context.Context, string, int64) error { return nil }
func (f *voiceAPIFixture) Delete(context.Context, string) error        { return nil }
func (f *voiceAPIFixture) Confirm(context.Context, voice.Review, voice.InputSink) (voice.Clip, error) {
	f.calls++
	return voice.Clip{}, errors.New("PRIVATE WORKER BODY")
}
func (f *voiceAPIFixture) ConfirmControl(context.Context, voice.Review, voice.ControlSink) (voice.Clip, error) {
	f.calls++
	return voice.Clip{}, nil
}

func TestVoiceAPIDisabledAuthLimitsAndFixedErrorBoundary(t *testing.T) {
	token := "voice-fixture-owner-token-000000000000000"
	fixture := &voiceAPIFixture{}
	server := &VoiceServer{Auth: NewOwnerControlAuth(token, "owner"), Store: fixture}
	invoke := func(path, method, body string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if auth {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		r.Header.Set("X-Butler-Delivery-ID", "delivery-fixture")
		r.Header.Set("Content-Type", "audio/wav")
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w
	}
	if w := invoke("/api/v3/voice/clips?role=owner_command&access_hours=1", http.MethodPost, "private", false); w.Code != 401 || fixture.calls != 0 {
		t.Fatal("unauthenticated audio processed")
	}
	if w := invoke("/api/v3/voice/clips?role=owner_command&access_hours=1", http.MethodPost, "private", true); w.Code != 503 || fixture.calls != 0 {
		t.Fatal("zero configuration enabled voice")
	}
	server.Enabled = true
	if w := invoke("/api/v3/voice/clips?role=external&access_hours=1&owner_id=foreign", http.MethodPost, "fixture", true); w.Code != 201 || fixture.owner != "owner" || fixture.role != voice.External {
		t.Fatal("body changed owner proof or role")
	}
	fixture.calls = 0
	if w := invoke("/api/v3/voice/clips/clip/review", http.MethodPost, `{"text":"review","owner_id":"foreign"}`, true); w.Code != 400 || fixture.calls != 0 {
		t.Fatal("unknown authority field accepted")
	}
	if w := invoke("/api/v3/voice/clips/clip/review", http.MethodPost, `{"text":"review"}`, true); w.Code != 409 || bytes.Contains(w.Body.Bytes(), []byte("PRIVATE WORKER BODY")) {
		t.Fatal("raw error disclosed")
	}
	fixture.calls = 0
	if w := invoke("/api/v3/voice/clips?role=external&access_hours=1", http.MethodPost, strings.Repeat("x", 3840045), true); w.Code != 413 || fixture.calls != 0 {
		t.Fatal("unbounded upload processed")
	}
}
