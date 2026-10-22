package telegramrich

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/internal/communicationcontrol"
)

func TestCommunicationWidgetTelegramRenderMatrix(t *testing.T) {
	for _, kind := range []string{"single_choice", "multi_choice", "text"} {
		for _, state := range []string{"open", "resolved", "expired", "cancelled", "superseded"} {
			t.Run(kind+"/"+state, func(t *testing.T) {
				view := communication.WidgetView{Token: strings.Repeat("a", 24), Version: 1, DraftVersion: 14, State: state, Question: communicationcontrol.Args{Input: &communicationcontrol.QuestionInput{Type: kind, Options: []communicationcontrol.Option{{ID: "a", Label: "Маргарита"}, {ID: "b", Label: "Пепперони"}}}}, Draft: communicationcontrol.Answer{OptionIDs: []string{"a"}}}
				text, markup := renderWidget(view, "Выберите пиццу")
				raw, _ := json.Marshal(markup)
				if !strings.Contains(text, "Выберите пиццу") {
					t.Fatal("question lost")
				}
				if state != "open" {
					if string(raw) != `{"inline_keyboard":[]}` || strings.Contains(string(raw), "callback_data") {
						t.Fatal("closed buttons", string(raw))
					}
					return
				}
				if kind == "text" {
					if !strings.Contains(string(raw), `"force_reply":true`) {
						t.Fatal(string(raw))
					}
					return
				}
				var data map[string][][]map[string]string
				if json.Unmarshal(raw, &data) != nil {
					t.Fatal("keyboard malformed")
				}
				for _, row := range data["inline_keyboard"] {
					for _, button := range row {
						if n := len(button["callback_data"]); n < 1 || n > 64 {
							t.Fatal("callback size", n)
						}
					}
				}
				if kind == "multi_choice" && (!strings.Contains(string(raw), "☑") || !strings.Contains(string(raw), "Отправить ответ")) {
					t.Fatal(string(raw))
				}
			})
		}
	}
}

func TestCommunicationWidgetSenderReadsCurrentTerminalState(t *testing.T) {
	var sent map[string]any
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		if e := json.NewDecoder(r.Body).Decode(&sent); e != nil {
			t.Error(e)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"chat":{"id":42}}}`))
	})
	part := Part{Version: 1, Method: "sendMessage", Text: "Какую пиццу?", Widget: true}
	s := Sender{Client: c, Authorize: authorize, Widget: func(context.Context, deliveryoutbox.Message) (communication.WidgetView, error) {
		return communication.WidgetView{State: "resolved"}, nil
	}}
	out := s.SendStored(context.Background(), stored(part))
	if out.Status != "sent" || !strings.Contains(sent["text"].(string), "Вопрос закрыт") {
		t.Fatal(out, sent)
	}
	markup, _ := json.Marshal(sent["reply_markup"])
	if string(markup) != `{"inline_keyboard":[]}` {
		t.Fatal("late delivery reopened question", string(markup))
	}
}

func TestCommunicationWidgetCallbackRejectsForeignIdentityBeforeStore(t *testing.T) {
	for _, attack := range []string{"wrong_user", "wrong_chat", "wrong_bot", "group", "both_envelopes", "disabled"} {
		t.Run(attack, func(t *testing.T) {
			in := Ingress{Store: &communication.Store{}, Widgets: true, Binding: OwnerBinding{OwnerID: "owner", BindingID: "bot", Version: 1, BotID: 77, UserID: 42, ChatID: 42}}
			u := Update{ID: 4, Callback: &CallbackQuery{ID: "cb", From: User{ID: 42}, Data: "cw:opaque:1:1:o0", Message: &Message{ID: 7, Chat: Chat{ID: 42, Type: "private"}, From: User{ID: 77, IsBot: true}}}}
			switch attack {
			case "wrong_user":
				u.Callback.From.ID = 43
			case "wrong_chat":
				u.Callback.Message.Chat.ID = 43
			case "wrong_bot":
				u.Callback.Message.From.ID = 78
			case "group":
				u.Callback.Message.Chat.Type = "group"
			case "both_envelopes":
				u.Message = &Message{}
			case "disabled":
				in.Widgets = false
			}
			if e := in.Reply(context.Background(), u); e == nil {
				t.Fatal("foreign callback accepted")
			}
		})
	}
}
