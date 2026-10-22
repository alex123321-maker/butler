// telegram-rich-lab sends synthetic channel fixtures to one explicit test chat.
// It is not an Orchestrator/model entry point and never enables product mode.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strconv"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/channel/telegramrich"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/jackc/pgx/v5"
)

func main() {
	if run() != nil {
		fmt.Fprintln(os.Stderr, "Telegram rich lab operation failed; no token or remote error body is emitted")
		os.Exit(1)
	}
}
func run() error {
	tokenFile := flag.String("token-file", "", "Private token file outside the repository")
	expectedBot := flag.String("bot", "", "Exact test bot username (without @)")
	action := flag.String("action", "probe", "probe, updates, verify-document, or send-fixtures")
	chatID := flag.Int64("chat-id", 0, "Explicit private test chat; never inferred from an incoming update")
	replyID := flag.Int64("reply-id", 0, "Exact replied-to report ACK for a document verification")
	flag.Parse()
	if *tokenFile == "" || *expectedBot == "" {
		return telegramrich.ErrUnavailable
	}
	client, err := telegramrich.NewClientFromTokenFile(*tokenFile)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	me, err := client.GetMe(ctx)
	if err != nil || me.Username != *expectedBot {
		return telegramrich.ErrUnavailable
	}
	fmt.Printf("Verified test bot @%s\n", me.Username)
	switch *action {
	case "probe":
		return nil
	case "updates":
		updates, err := client.Updates(ctx, 0)
		if err != nil {
			return err
		}
		for _, u := range updates {
			if u.Message != nil && u.Message.Chat.Type == "private" && !u.Message.From.IsBot {
				replyID := int64(0)
				if u.Message.Reply != nil {
					replyID = u.Message.Reply.ID
				}
				fmt.Printf("private update=%d chat=%d user=%d message=%d reply_message=%d\n", u.ID, u.Message.Chat.ID, u.Message.From.ID, u.Message.ID, replyID)
			}
		}
		return nil
	case "send-fixtures":
		if *chatID <= 0 {
			return telegramrich.ErrUnavailable
		}
	case "verify-document":
		if *chatID <= 0 || *replyID <= 0 {
			return telegramrich.ErrUnavailable
		}
		updates, err := client.Updates(ctx, 0)
		if err != nil {
			return err
		}
		for _, u := range updates {
			m := u.Message
			if m == nil || m.Chat.ID != *chatID || m.Chat.Type != "private" || m.From.ID != *chatID || m.From.IsBot || m.Reply == nil || m.Reply.ID != *replyID || m.Reply.From.ID != me.ID {
				continue
			}
			var rich struct {
				Blocks []struct {
					Type     string `json:"type"`
					Document struct {
						ID   string `json:"file_id"`
						Name string `json:"file_name"`
					} `json:"document"`
				} `json:"blocks"`
			}
			if json.Unmarshal(m.Reply.Rich, &rich) != nil {
				return telegramrich.ErrUnavailable
			}
			for _, block := range rich.Blocks {
				if block.Type == "document" && block.Document.Name == "butler-lab.txt" {
					want := media("artifact:lab-document", "butler-lab.txt", "text/plain", []byte("Butler isolated Telegram Rich Message fixture. No production or private data.\n"))
					if client.VerifyDocument(ctx, block.Document.ID, want) != nil {
						return telegramrich.ErrUnavailable
					}
					fmt.Printf("Document verified: %s, bytes=%d, sha256=%s\n", want.Name, len(want.Data), want.SHA256)
					return nil
				}
			}
		}
		return telegramrich.ErrUnavailable
	default:
		return telegramrich.ErrUnavailable
	}
	img := image.NewRGBA(image.Rect(0, 0, 640, 360))
	for y := 0; y < 360; y++ {
		for x := 0; x < 640; x++ {
			img.Set(x, y, color.RGBA{R: 22, G: uint8(50 + x/8), B: uint8(80 + y/4), A: 255})
		}
	}
	var imageData bytes.Buffer
	if err = png.Encode(&imageData, img); err != nil {
		return err
	}
	photo := media("artifact:lab-photo", "butler-lab.png", "image/png", imageData.Bytes())
	doc := media("artifact:lab-document", "butler-lab.txt", "text/plain", []byte("Butler isolated Telegram Rich Message fixture. No production or private data.\n"))
	files := map[string]artifacts.Media{photo.Ref: photo, doc.Ref: doc}
	planner := telegramrich.Planner{Profile: telegramrich.Capabilities{Message: true, Photo: true, Rich: true, Document: true}, Resolve: func(_ context.Context, _ pgx.Tx, _ string, _ communication.Binding, ref string) (artifacts.Media, error) {
		m, ok := files[ref]
		if !ok {
			return m, telegramrich.ErrUnavailable
		}
		return m, nil
	}}
	key := "telegram:" + strconv.FormatInt(*chatID, 10)
	// This explicit operator-only fixture grant is fixed to this test chat. It
	// is not a model tool, durable publication receipt or task completion proof.
	sender := telegramrich.Sender{Client: client, Authorize: func(_ context.Context, m deliveryoutbox.Message) error {
		if m.Recipient != key || m.BindingID != "lab:"+strconv.FormatInt(me.ID, 10) {
			return telegramrich.ErrUnavailable
		}
		return nil
	}, Resolve: func(_ context.Context, _ deliveryoutbox.Message, f telegramrich.File) (artifacts.Media, error) {
		m, ok := files[f.Ref]
		if !ok {
			return m, telegramrich.ErrUnavailable
		}
		return m, nil
	}}
	fixtures := []communicationcontrol.Args{
		{SchemaVersion: 1, Purpose: "progress", Text: "Butler: проверка нового Telegram-адаптера. Это изолированный тест; основной режим выключен."},
		{SchemaVersion: 1, Purpose: "result", Text: "Проверка фотографии с подписью 🧪", Images: []communicationcontrol.Image{{ArtifactRef: photo.Ref, Alt: "Синтетическое тестовое изображение", Caption: "Butler Rich Lab — проверка фото"}}},
		{SchemaVersion: 1, Purpose: "result", Title: "Butler — нативный Rich Message", Summary: "Изолированная проверка таблицы, фотографии и файла.", Blocks: []communicationcontrol.Block{{Type: "heading", Text: "Что проверяем"}, {Type: "paragraph", Text: "Этот отчёт отправлен через sendRichMessage. Завершение модели и доставка пользователю учитываются отдельно."}, {Type: "table", Rows: [][]string{{"Проверка", "Формат"}, {"Таблица", "Нативный блок"}, {"Изображение", "Прямая загрузка"}, {"Файл", "Вложение внутри отчёта"}}}, {Type: "image", ArtifactRef: photo.Ref, Alt: "Синтетическое изображение Butler Rich Lab"}}, Attachments: []string{doc.Ref}},
	}
	for index, a := range fixtures {
		parts, err := planner.Plan(ctx, nil, "lab", communication.Binding{Channel: "telegram"}, a)
		if err != nil {
			return err
		}
		for n, part := range parts {
			out := sender.SendStored(ctx, deliveryoutbox.Message{PublicationID: fmt.Sprintf("lab:%d", index), BindingID: "lab:" + strconv.FormatInt(me.ID, 10), BindingVersion: 1, Recipient: key, Channel: "telegram", ClaimID: "operator-fixture", ChunkIndex: n, Content: part})
			safe, _ := json.Marshal(out)
			fmt.Printf("fixture=%d part=%d outcome=%s\n", index, n, safe)
			if out.Status != "sent" {
				return telegramrich.ErrUnavailable
			}
		}
	}
	return nil
}
func media(ref, name, mime string, data []byte) artifacts.Media {
	h := sha256.Sum256(data)
	return artifacts.Media{Ref: ref, Name: name, MIME: mime, Data: data, SHA256: hex.EncodeToString(h[:])}
}
