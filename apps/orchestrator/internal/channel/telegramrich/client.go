package telegramrich

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/internal/communicationcontrol"
)

type Client struct {
	token string
	base  string
	http  *http.Client
}

func (*Client) String() string   { return "telegramrich.Client([redacted])" }
func (*Client) GoString() string { return "telegramrich.Client([redacted])" }

// NewClientFromTokenFile reads a private operator reference, never a model arg.
// The token cannot be formatted through this type or returned by an API error.
func NewClientFromTokenFile(path string) (*Client, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 256 {
		return nil, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(f, 257))
	if err != nil {
		return nil, ErrUnavailable
	}
	token := strings.TrimSpace(string(raw))
	if !regexp.MustCompile(`^[0-9]{5,20}:[A-Za-z0-9_-]{30,100}$`).MatchString(token) {
		return nil, ErrUnavailable
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{token: token, base: "https://api.telegram.org", http: &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

type User struct {
	ID       int64  `json:"id"`
	IsBot    bool   `json:"is_bot"`
	Username string `json:"username"`
}
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}
type Message struct {
	ID    int64           `json:"message_id"`
	Chat  Chat            `json:"chat"`
	From  User            `json:"from"`
	Text  string          `json:"text"`
	Reply *Message        `json:"reply_to_message,omitempty"`
	Rich  json.RawMessage `json:"rich_message,omitempty"`
}
type envelope struct {
	OK         *bool           `json:"ok"`
	Result     json.RawMessage `json:"result"`
	Code       int             `json:"error_code"`
	Parameters struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// call performs one HTTP attempt. It never exposes URL/body/errors containing
// auth material. Redirects, malformed acknowledgements and timeouts are unknown.
func (c *Client) call(ctx context.Context, method string, body []byte, contentType string) (envelope, int, error) {
	if c == nil || c.http == nil {
		return envelope{}, 0, ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/bot"+c.token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return envelope{}, 0, ErrUnavailable
	}
	req.Header.Set("Content-Type", contentType)
	client := *c.http
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return envelope{}, 0, ErrUnavailable
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
	if err != nil || len(raw) > 1024*1024 {
		return envelope{}, resp.StatusCode, ErrUnavailable
	}
	var out envelope
	if json.Unmarshal(raw, &out) != nil || out.OK == nil {
		return envelope{}, resp.StatusCode, ErrUnavailable
	}
	return out, resp.StatusCode, nil
}
func (c *Client) GetMe(ctx context.Context) (User, error) {
	e, status, err := c.call(ctx, "getMe", []byte(`{}`), "application/json")
	var u User
	if err != nil || status != 200 || !*e.OK || json.Unmarshal(e.Result, &u) != nil || !u.IsBot || u.ID <= 0 {
		return u, ErrUnavailable
	}
	return u, nil
}

type Sender struct {
	Client *Client
	// Required: current stored claim/binding/owner/source checks, repeated after
	// retained media reads and immediately before the network boundary.
	Authorize func(context.Context, deliveryoutbox.Message) error
	Resolve   func(context.Context, deliveryoutbox.Message, File) (artifacts.Media, error)
}

func (s *Sender) SendStored(ctx context.Context, m deliveryoutbox.Message) deliveryoutbox.Outcome {
	absent := deliveryoutbox.Outcome{Status: "absent", ErrorCode: "telegram_rejected"}
	unknown := deliveryoutbox.Outcome{Status: "delivery_unknown", ErrorCode: "transport_outcome_unknown"}
	chat, err := strconv.ParseInt(strings.TrimPrefix(m.Recipient, "telegram:"), 10, 64)
	if s == nil || s.Client == nil || s.Authorize == nil || err != nil || chat <= 0 || !strings.HasPrefix(m.Recipient, "telegram:") || m.Channel != "telegram" || m.PublicationID == "" || m.BindingID == "" || m.BindingVersion <= 0 || m.ClaimID == "" {
		return absent
	}
	var part Part
	if len(m.Content) > maxPartBytes || communicationcontrol.StrictJSON([]byte(m.Content), &part) != nil || part.Version != 1 || len(part.Files) > 50 {
		return absent
	}
	fields := map[string]any{"chat_id": chat, "protect_content": true}
	switch part.Method {
	case "sendMessage":
		if part.Text == "" || len(deliveryoutbox.Split(part.Text, 4096)) != 1 || part.Rich != nil || part.Photo != "" || len(part.Files) > 0 {
			return absent
		}
		fields["text"] = part.Text
	case "sendPhoto":
		if len(part.Files) != 1 || part.Photo != "attach://"+part.Files[0].Field || part.Rich != nil || len(deliveryoutbox.Split(part.Caption, 1024)) > 1 {
			return absent
		}
		fields["photo"] = part.Photo
		fields["caption"] = part.Caption
	case "sendRichMessage":
		if part.Rich == nil || len(part.Rich.Blocks) == 0 || len(part.Rich.Blocks) > 50 || part.Text != "" || part.Photo != "" {
			return absent
		}
		fields["rich_message"] = part.Rich
	default:
		return absent
	}
	if s.Authorize(ctx, m) != nil {
		return absent
	}
	files := make([]artifacts.Media, 0, len(part.Files))
	total := 0
	seen := map[string]bool{}
	for _, f := range part.Files {
		if s.Resolve == nil || !regexp.MustCompile(`^file[0-9]{1,4}$`).MatchString(f.Field) || seen[f.Field] {
			return absent
		}
		seen[f.Field] = true
		media, err := s.Resolve(ctx, m, f)
		if err != nil || media.Ref != f.Ref || media.Name != f.Name || media.MIME != f.MIME || media.SHA256 != f.SHA256 || len(media.Data) != f.Size || media.Validate() != nil || part.Method == "sendPhoto" && !strings.HasPrefix(media.MIME, "image/") {
			return absent
		}
		total += len(media.Data)
		if total > maxPartMediaBytes {
			return absent
		}
		files = append(files, media)
	}
	var body []byte
	contentType := "application/json"
	if len(files) == 0 {
		body, err = json.Marshal(fields)
	} else {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		for key, value := range fields {
			var val string
			if text, ok := value.(string); ok {
				val = text
			} else {
				raw, e := json.Marshal(value)
				if e != nil {
					return absent
				}
				val = string(raw)
			}
			if w.WriteField(key, val) != nil {
				return absent
			}
		}
		for i, file := range files {
			h := textproto.MIMEHeader{}
			h.Set("Content-Disposition", `form-data; name="`+part.Files[i].Field+`"; filename="`+file.Name+`"`)
			h.Set("Content-Type", file.MIME)
			fw, e := w.CreatePart(h)
			if e != nil {
				return absent
			}
			if _, e = fw.Write(file.Data); e != nil {
				return absent
			}
		}
		if w.Close() != nil {
			return absent
		}
		body = buf.Bytes()
		contentType = w.FormDataContentType()
	}
	if err != nil || s.Authorize(ctx, m) != nil {
		return absent
	}
	e, status, err := s.Client.call(ctx, part.Method, body, contentType)
	if err != nil {
		return unknown
	}
	if !*e.OK {
		if e.Code < 400 || e.Code > 599 || status < 400 || status > 599 {
			return unknown
		}
		absent.Retryable = e.Code == 429 || e.Code >= 500
		if e.Parameters.RetryAfter > 0 && e.Parameters.RetryAfter <= 3600 {
			absent.RetryAfter = time.Duration(e.Parameters.RetryAfter) * time.Second
		}
		return absent
	}
	var msg Message
	if status != 200 || json.Unmarshal(e.Result, &msg) != nil || msg.ID <= 0 || msg.Chat.ID != chat || part.Method == "sendRichMessage" && (len(msg.Rich) == 0 || string(msg.Rich) == "null") {
		return unknown
	}
	return deliveryoutbox.Outcome{Status: "sent", ACK: strconv.FormatInt(msg.ID, 10)}
}
