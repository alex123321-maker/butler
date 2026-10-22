package telegramrich

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/internal/communicationcontrol"
)

// Operator transport qualification only: no model, Core task or production
// activation. Fixed synthetic fixtures include an intentionally invalid server
// boundary request; their responses never authorize an agent action.
func TestNativeCommunicationTelegramFormatCorpus(t *testing.T) {
	if os.Getenv("BUTLER_COMMUNICATION_NATIVE_CORPUS") != "1" {
		t.Skip("explicit isolated operator transport corpus required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if os.Getenv("BUTLER_TEST_TELEGRAM_RICH_PROXY_FILE") == "" {
		t.Fatal("explicit proxy reference required")
	}
	c, err := NewClientFromTokenFileWithProxyFile(os.Getenv("BUTLER_TEST_TELEGRAM_RICH_TOKEN_FILE"), os.Getenv("BUTLER_TEST_TELEGRAM_RICH_PROXY_FILE"))
	if err != nil {
		t.Fatal("private client unavailable")
	}
	me, err := c.GetMe(ctx)
	if err != nil || me.ID != 8891439626 || me.Username != "butler_rich_lab_20261021_bot" {
		t.Fatal("only the dedicated pinned lab bot is allowed")
	}
	const chat = int64(1893384316)
	for _, q := range []struct {
		name, text string
		accepted   bool
	}{
		{"message-4096-cyrillic", strings.Repeat("Я", 4096), true},
		{"message-4097-cyrillic", strings.Repeat("Я", 4097), false},
		{"message-4096-utf16-emoji", strings.Repeat("🧪", 2048), true},
	} {
		t.Run(q.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"chat_id": chat, "text": q.text, "disable_notification": true})
			e, status, err := c.call(ctx, "sendMessage", body, "application/json")
			if err != nil {
				t.Fatal("single native attempt indeterminate; do not retry")
			}
			if !q.accepted {
				if status != 400 || *e.OK || e.Code != 400 {
					t.Fatal("documented upper boundary did not reject", status, e.Code)
				}
				t.Log("native HTTP400; explicit absent outcome, no send retry")
				return
			}
			var m Message
			if status != 200 || !*e.OK || json.Unmarshal(e.Result, &m) != nil || m.Chat.ID != chat || m.ID <= 0 || m.Text != q.text {
				t.Fatal("native message did not preserve exact text", status)
			}
			t.Logf("native exact-text ACK=%d bytes=%d", m.ID, len(q.text))
		})
	}
	p := Planner{Profile: Capabilities{Message: true, Photo: true, Rich: true, Document: true}}
	rows := make([][]string, 3)
	for y := range rows {
		for x := 0; x < 20; x++ {
			rows[y] = append(rows[y], "Я 🧪")
		}
	}
	table := communicationcontrol.Args{Title: "Butler — форматный corpus", Summary: "Синтетические границы; не результат поручения", Blocks: []communicationcontrol.Block{{Type: "table", Rows: rows}}}
	for n := 0; n < 47; n++ {
		table.Blocks = append(table.Blocks, communicationcontrol.Block{Type: "paragraph", Text: strings.Repeat("🧪", 40)})
	}
	boundary := communicationcontrol.Args{Title: "Butler — крупный неделимый блок", Blocks: []communicationcontrol.Block{{Type: "code", Text: "x"}}}
	seed, err := p.Plan(ctx, nil, "operator-format-corpus", communication.Binding{Channel: "telegram"}, boundary)
	if err != nil || len(seed) != 1 {
		t.Fatal("boundary setup failed")
	}
	boundary.Blocks[0].Text = strings.Repeat("x", 1+maxPartBytes-len(seed[0]))
	for _, q := range []struct {
		name string
		args communicationcontrol.Args
	}{
		{"rich-50-blocks-20-columns", table},
		{"rich-indivisible-32KiB-manifest", boundary},
	} {
		t.Run(q.name, func(t *testing.T) {
			parts, err := p.Plan(ctx, nil, "operator-format-corpus", communication.Binding{Channel: "telegram"}, q.args)
			if err != nil {
				t.Fatal("supported local corpus rejected")
			}
			if q.name == "rich-indivisible-32KiB-manifest" && (len(parts) != 1 || len(parts[0]) != maxPartBytes) {
				t.Fatal("positive fixture missed exact local byte boundary")
			}
			for n, raw := range parts {
				var part Part
				if json.Unmarshal([]byte(raw), &part) != nil || part.Rich == nil || len(part.Files) != 0 {
					t.Fatal("invalid fixed corpus part")
				}
				body, _ := json.Marshal(map[string]any{"chat_id": chat, "rich_message": part.Rich, "disable_notification": true})
				e, status, err := c.call(ctx, "sendRichMessage", body, "application/json")
				var m Message
				if err != nil || status != 200 || !*e.OK || json.Unmarshal(e.Result, &m) != nil || m.ID <= 0 || m.Chat.ID != chat || len(m.Rich) == 0 {
					t.Fatal("supported rich fixture not acknowledged; do not retry", status)
				}
				t.Logf("native part=%d blocks=%d manifest_bytes=%d ACK=%d", n, len(part.Rich.Blocks), len(raw), m.ID)
			}
		})
	}
	t.Run("indivisible-one-byte-over-local-bound", func(t *testing.T) {
		// Measure the block alone: overflowing the combined heading+body part
		// merely moves an intact block to another part and is supported.
		tiny, _ := json.Marshal(Part{Version: 1, Method: "sendRichMessage", Rich: &RichMessage{Blocks: []Block{{Type: "pre", Text: "x"}}}})
		boundary.Blocks[0].Text = strings.Repeat("x", 2+maxPartBytes-len(tiny))
		if _, err := p.Plan(ctx, nil, "operator-format-corpus", communication.Binding{Channel: "telegram"}, boundary); err == nil {
			t.Fatal("oversized block accepted")
		}
		t.Log("local plan rejected before any HTTP; not a claimed Telegram maximum")
	})
	t.Run("nested-canonical-block-rejected", func(t *testing.T) {
		if _, err := communicationcontrol.DecodeArgs("communication.report", `{"schema_version":1,"purpose":"result","title":"nested","blocks":[{"type":"paragraph","text":"x","blocks":[{"type":"paragraph","text":"y"}]}]}`); err == nil {
			t.Fatal("nested canonical block accepted")
		}
		t.Log("typed canonical schema rejected before any HTTP")
	})
}
