package telegramrich

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/jackc/pgx/v5"
)

func picture() artifacts.Media {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 20, 20)))
	h := sha256.Sum256(b.Bytes())
	return artifacts.Media{Ref: "artifact:photo", Name: "photo.png", MIME: "image/png", Data: b.Bytes(), SHA256: hex.EncodeToString(h[:])}
}
func planner() Planner {
	return Planner{Profile: Capabilities{Message: true, Photo: true, Rich: true, Document: true}, Resolve: func(_ context.Context, _ pgx.Tx, _ string, _ communication.Binding, ref string) (artifacts.Media, error) {
		m := picture()
		if ref != m.Ref {
			return artifacts.Media{}, ErrUnavailable
		}
		return m, nil
	}}
}
func stored(p Part) deliveryoutbox.Message {
	raw, _ := json.Marshal(p)
	return deliveryoutbox.Message{PublicationID: "publication:test", BindingID: "new-bot:42", BindingVersion: 1, Recipient: "telegram:42", Channel: "telegram", ClaimID: "claim", Content: string(raw)}
}
func textPart() Part { return Part{Version: 1, Method: "sendMessage", Text: "Привет 🧪"} }
func client(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Client{token: "test-token-never-expose", base: srv.URL, http: srv.Client()}
}
func authorize(context.Context, deliveryoutbox.Message) error { return nil }

func TestCommunicationRichOutcomeMatrix(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
		retry            bool
	}{
		{"sent", `{"ok":true,"result":{"message_id":7,"chat":{"id":42}}}`, "sent", 200, false},
		{"wrong-chat", `{"ok":true,"result":{"message_id":7,"chat":{"id":43}}}`, "delivery_unknown", 200, false},
		{"missing-id", `{"ok":true,"result":{"chat":{"id":42}}}`, "delivery_unknown", 200, false},
		{"rate-limit", `{"ok":false,"error_code":429,"parameters":{"retry_after":13}}`, "absent", 429, true},
		{"rejected", `{"ok":false,"error_code":400,"description":"test-token-never-expose"}`, "absent", 400, false},
		{"ambiguous", `{"ok":false}`, "delivery_unknown", 200, false},
		{"broken", `{`, "delivery_unknown", 502, false},
		{"redirect", `{}`, "delivery_unknown", 307, false},
		{"lost-response", "", "delivery_unknown", 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			c := client(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/bottest-token-never-expose/sendMessage" {
					t.Error("wrong method")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			s := Sender{Client: c, Authorize: authorize}
			out := s.SendStored(context.Background(), stored(textPart()))
			if out.Status != tc.want || out.Retryable != tc.retry || calls.Load() != 1 {
				t.Fatal(out, calls.Load())
			}
			if strings.Contains(fmt.Sprint(out, c, fmt.Sprintf("%#v", c)), c.token) {
				t.Fatal("secret reflected")
			}
		})
	}
}

func TestCommunicationRichNativeReportAndMultipart(t *testing.T) {
	a := communicationcontrol.Args{SchemaVersion: 1, Purpose: "result", Title: "Проверка", Summary: "Изолированный отчёт", Blocks: []communicationcontrol.Block{{Type: "paragraph", Text: "<script>alert(1)</script>"}, {Type: "table", Rows: [][]string{{"Шаг", "Итог"}, {"Фото", "OK"}}}, {Type: "image", ArtifactRef: "artifact:photo", Alt: "Тест"}}, Attachments: []string{"artifact:photo"}}
	parts, err := planner().Plan(context.Background(), nil, "run", communication.Binding{Channel: "telegram"}, a)
	if err != nil || len(parts) != 1 {
		t.Fatal(parts, err)
	}
	var p Part
	_ = json.Unmarshal([]byte(parts[0]), &p)
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendRichMessage") {
			t.Fatal("rich method not used")
		}
		if r.ParseMultipartForm(2<<20) != nil {
			t.Fatal("bad multipart")
		}
		var rich RichMessage
		if json.Unmarshal([]byte(r.FormValue("rich_message")), &rich) != nil || len(rich.Blocks) != 6 || rich.Blocks[3].Cells[0][0].Text != "Шаг" {
			t.Fatal("lost rich content")
		}
		if r.FormValue("chat_id") != "42" || r.FormValue("allow_paid_broadcast") != "" || r.FormValue("protect_content") == "true" {
			t.Fatal("unexpected send settings")
		}
		for _, f := range p.Files {
			file, _, e := r.FormFile(f.Field)
			if e != nil {
				t.Fatal(e)
			}
			_ = file.Close()
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":9,"chat":{"id":42},"rich_message":{"blocks":[]}}}`))
	})
	s := Sender{Client: c, Authorize: authorize, Resolve: func(context.Context, deliveryoutbox.Message, File) (artifacts.Media, error) { return picture(), nil }}
	if out := s.SendStored(context.Background(), stored(p)); out.Status != "sent" || out.ACK != "9" {
		t.Fatal(out)
	}
}

func TestCommunicationRichNoSendAfterLateRevocationOrChangedMedia(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(fmt.Sprint(changed), func(t *testing.T) {
			parts, err := planner().Plan(context.Background(), nil, "run", communication.Binding{Channel: "telegram"}, communicationcontrol.Args{Images: []communicationcontrol.Image{{ArtifactRef: "artifact:photo", Alt: "photo"}}})
			if err != nil {
				t.Fatal(err)
			}
			var p Part
			_ = json.Unmarshal([]byte(parts[0]), &p)
			var calls atomic.Int32
			var revoked atomic.Bool
			c := client(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) })
			s := Sender{Client: c, Authorize: func(context.Context, deliveryoutbox.Message) error {
				if revoked.Load() {
					return ErrUnavailable
				}
				return nil
			}, Resolve: func(context.Context, deliveryoutbox.Message, File) (artifacts.Media, error) {
				m := picture()
				if changed {
					m.Name = "replacement.png"
				} else {
					revoked.Store(true)
				}
				return m, nil
			}}
			if out := s.SendStored(context.Background(), stored(p)); out.Status != "absent" || out.Retryable || calls.Load() != 0 {
				t.Fatal(out, calls.Load())
			}
		})
	}
}

func TestCommunicationRichDeterministicPartsAndBounds(t *testing.T) {
	p := planner()
	b := communication.Binding{Channel: "telegram"}
	a := communicationcontrol.Args{Title: "Report", Summary: "Summary"}
	for n := 0; n < 110; n++ {
		a.Blocks = append(a.Blocks, communicationcontrol.Block{Type: "paragraph", Text: strings.Repeat("🧪", 150)})
	}
	first, err := p.Plan(context.Background(), nil, "r", b, a)
	if err != nil || len(first) < 3 {
		t.Fatal(len(first), err)
	}
	second, err := p.Plan(context.Background(), nil, "r", b, a)
	if err != nil || fmt.Sprint(first) != fmt.Sprint(second) {
		t.Fatal("unstable manifest")
	}
	blocks := 0
	for _, raw := range first {
		var v Part
		_ = json.Unmarshal([]byte(raw), &v)
		blocks += len(v.Rich.Blocks)
	}
	if blocks != 112 {
		t.Fatal("dropped report blocks", blocks)
	}
	a.Blocks = []communicationcontrol.Block{{Type: "code", Text: strings.Repeat("x", maxPartBytes+1)}}
	if _, err = p.Plan(context.Background(), nil, "r", b, a); err == nil {
		t.Fatal("oversized atomic block accepted")
	}
	a = communicationcontrol.Args{Text: strings.Repeat("🧪", 4000)}
	parts, err := p.Plan(context.Background(), nil, "r", b, a)
	if err != nil || len(parts) != 2 {
		t.Fatal(len(parts), err)
	}
	text := ""
	for _, raw := range parts {
		var v Part
		_ = json.Unmarshal([]byte(raw), &v)
		text += v.Text
	}
	if text != a.Text {
		t.Fatal("Unicode lost")
	}
	if _, err = p.Plan(context.Background(), nil, "r", communication.Binding{Channel: "web"}, a); err == nil {
		t.Fatal("advertised Web readiness")
	}
	a = communicationcontrol.Args{Title: "Report", Summary: "Summary", Blocks: []communicationcontrol.Block{{Type: "paragraph", Text: "text", Items: []string{"must not disappear"}}}}
	if _, err = p.Plan(context.Background(), nil, "r", b, a); err == nil {
		t.Fatal("silently discarded report fields")
	}
}

func TestCommunicationRichMissingGuardAndLegacyAreDenied(t *testing.T) {
	var calls atomic.Int32
	c := client(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) })
	for _, m := range []deliveryoutbox.Message{stored(textPart()), {Channel: "telegram", SessionKey: "telegram:42", Content: "legacy"}} {
		s := Sender{Client: c}
		if out := s.SendStored(context.Background(), m); out.Status != "absent" {
			t.Fatal(out)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("unguarded or legacy send")
	}
}

func TestCommunicationRichTransportLossAndRedirectRefusal(t *testing.T) {
	for _, mode := range []string{"lost-after-accept", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			var attempts, redirected atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
			defer target.Close()
			c := client(t, func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				if mode == "redirect" {
					w.Header().Set("Location", target.URL)
					w.WriteHeader(http.StatusTemporaryRedirect)
					return
				}
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = conn.Close()
			})
			s := Sender{Client: c, Authorize: authorize}
			if out := s.SendStored(context.Background(), stored(textPart())); out.Status != "delivery_unknown" || out.Retryable || attempts.Load() != 1 || redirected.Load() != 0 {
				t.Fatal(out, attempts.Load(), redirected.Load())
			}
		})
	}
}
