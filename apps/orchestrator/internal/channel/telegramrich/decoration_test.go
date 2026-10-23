package telegramrich

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/internal/communicationcontrol"
)

func TestCommunicationDecorationCanonicalAndUTF16(t *testing.T) {
	d := NewDecorator()
	original := "🧪 Контроль.\nПривет!\nСлушаю музыку.\nСпасибо большое.\nПривет!"
	f := d.Freeze(original, nil)
	if f == nil || len(f.Insertions) != 3 {
		t.Fatalf("choices: %+v", f)
	}
	text, entities, ok := renderDecoration(original, f)
	if !ok || len(entities) != 3 {
		t.Fatal("render")
	}
	units := utf16.Encode([]rune(text))
	for i := len(entities) - 1; i >= 0; i-- {
		e := entities[i]
		if string(utf16.Decode(units[e.Offset:e.Offset+e.Length])) != f.Insertions[i].Emoji {
			t.Fatal("UTF16 entity mismatch")
		}
		units = append(units[:e.Offset], units[e.Offset+e.Length+1:]...)
	}
	if string(utf16.Decode(units)) != original {
		t.Fatal("canonical text changed")
	}
	// A serialized outbox survives restart with exactly the same representation.
	raw, _ := json.Marshal(f)
	var restored FrozenDecoration
	_ = json.Unmarshal(raw, &restored)
	again, ee, valid := renderDecoration(original, &restored)
	if !valid || again != text || !reflect.DeepEqual(ee, entities) {
		t.Fatal("replay changed")
	}
	if _, _, valid = renderDecoration(original+"!", &restored); valid {
		t.Fatal("accepted mismatched source")
	}
	restored.Insertions[0].Emoji = "rewrite"
	if _, _, valid = renderDecoration(original, &restored); valid {
		t.Fatal("accepted arbitrary replacement text")
	}
}

func TestCommunicationDecorationAbstentionAndBoundaries(t *testing.T) {
	d := NewDecorator()
	for _, text := range []string{"Не готово.", "Ошибка. Готово.", "Сервер упал. Спасибо.", "`Привет!`", "«Спасибо»", "https://example.org. Привет!", "В красных наушниках.", "Смерть.", "Слезаю.", "Клан.", "Топорный.", "Привет 🙂!"} {
		if f := d.Freeze(text, nil); f != nil {
			t.Errorf("unexpected enrichment %q: %+v", text, f)
		}
	}
	long := "Привет!" + strings.Repeat(" ", 4096-utf16Len("Привет!"))
	if d.Freeze(long, nil) != nil {
		t.Fatal("decoration exceeded Telegram size")
	}
	f := d.Freeze("Привет!", nil)
	f.Insertions[0].Offset = 1
	if _, _, ok := renderDecoration("Привет!", f); ok {
		t.Fatal("accepted split UTF8")
	}
	// Ambiguous reviewed concepts explicitly abstain.
	d.concepts = append(d.concepts, d.concepts[6])
	if d.Select("Привет").Reason != "ambiguous_concepts" {
		t.Fatal("ambiguous accepted")
	}
}

func TestCommunicationDecorationPlannerAndSender(t *testing.T) {
	p := planner()
	args := communicationcontrol.Args{Text: "Привет!\nСлушаю музыку.\nСпасибо большое."}
	plain, e := p.Plan(context.Background(), nil, "run", communication.Binding{Channel: "telegram"}, args)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(plain[0], "decoration") {
		t.Fatal("default decorated")
	}
	p.Decorator = NewDecorator()
	parts, e := p.Plan(context.Background(), nil, "run", communication.Binding{Channel: "telegram"}, args)
	if e != nil {
		t.Fatal(e)
	}
	var part Part
	_ = json.Unmarshal([]byte(parts[0]), &part)
	if part.Text != args.Text || part.Decoration == nil {
		t.Fatal("lost canonical or choices")
	}
	var sent []map[string]json.RawMessage
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		var fields map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&fields)
		sent = append(sent, fields)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"chat":{"id":42}}}`))
	})
	s := Sender{Client: c, Authorize: authorize}
	for i := 0; i < 2; i++ {
		if out := s.SendStored(context.Background(), stored(part)); out.Status != "sent" {
			t.Fatal(out)
		}
	}
	if !reflect.DeepEqual(sent[0], sent[1]) || len(sent[0]["entities"]) == 0 {
		t.Fatal("choices changed or not sent")
	}
	// Current authority is checked despite a valid persisted decoration.
	s.Authorize = func(context.Context, deliveryoutbox.Message) error { return ErrUnavailable }
	if out := s.SendStored(context.Background(), stored(part)); out.Status != "absent" || len(sent) != 2 {
		t.Fatal("decoration bypassed authority")
	}
}

func TestCommunicationDecorationWidgetStableOptionIDs(t *testing.T) {
	d := NewDecorator()
	f := d.Freeze("Выберите занятие", map[string]string{"music": "Слушаю музыку", "code": "Пишу код", "none": "Квитанция"})
	view := communication.WidgetView{State: "open", Token: strings.Repeat("a", 24), Version: 1, DraftVersion: 1, Question: communicationcontrol.Args{Input: &communicationcontrol.QuestionInput{Type: "multi_choice", Options: []communicationcontrol.Option{{ID: "code", Label: "Пишу код"}, {ID: "music", Label: "Слушаю музыку"}, {ID: "none", Label: "Квитанция"}}}}}
	for _, state := range []string{"open", "resolved"} {
		view.State = state
		view.Draft.OptionIDs = []string{"music"}
		plain, original := renderWidget(view, "Выберите занятие")
		text, decorated := renderWidgetWithIcons(view, "Выберите занятие", f.Options)
		if text != plain {
			t.Fatal("question/state text changed")
		}
		raw, _ := json.Marshal(decorated)
		var got map[string][][]map[string]string
		_ = json.Unmarshal(raw, &got)
		if state == "open" {
			rows := got["inline_keyboard"]
			if rows[0][0]["icon_custom_emoji_id"] != f.Options["code"] || rows[1][0]["icon_custom_emoji_id"] != f.Options["music"] || rows[2][0]["icon_custom_emoji_id"] != "" {
				t.Fatal("icons not bound to stable option IDs")
			}
		}
		for _, row := range got["inline_keyboard"] {
			for _, b := range row {
				delete(b, "icon_custom_emoji_id")
			}
		}
		raw, _ = json.Marshal(got)
		want, _ := json.Marshal(original)
		if string(raw) != string(want) {
			t.Fatal("callbacks, labels or order changed")
		}
	}
}

func TestCommunicationDecorationStoredProjectionIntegration(t *testing.T) {
	var sent map[string]json.RawMessage
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/editMessageText") {
			t.Error("unexpected new send")
		}
		_ = json.NewDecoder(r.Body).Decode(&sent)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"chat":{"id":42}}}`))
	})
	part := Part{Version: 1, Method: "sendMessage", Text: "Привет!", Widget: true, Decoration: NewDecorator().Freeze("Привет!", nil)}
	raw, _ := json.Marshal(part)
	p, core := widgetProjectionFixture(t, c, part)
	if e := p.RefreshWidgets(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(sent["entities"]) == 0 || !strings.Contains(string(sent["text"]), "Вопрос закрыт") {
		t.Fatal("stored projection lost decoration or terminal state")
	}
	var retained string
	if e := core.Pool.QueryRow(context.Background(), `SELECT content FROM delivery_outbox WHERE notification_id='projection-notification'`).Scan(&retained); e != nil || retained != string(raw) {
		t.Fatal("projection mutated immutable manifest", e)
	}
	var status string
	if e := core.Pool.QueryRow(context.Background(), `SELECT status FROM communication_widget_projection_ops WHERE notification_id='projection-notification'`).Scan(&status); e != nil || status != "sent" {
		t.Fatal("edit not acknowledged", e, status)
	}
}
