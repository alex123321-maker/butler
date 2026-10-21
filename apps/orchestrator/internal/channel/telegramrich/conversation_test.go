package telegramrich

import (
	"context"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/internal/owneridentity"
)

func TestCommunicationNewConversationHasVerifiedIdentityAndNoReplyFallback(t *testing.T) {
	b := OwnerBinding{OwnerID: "owner", BindingID: "bot", Version: 1, BotID: 77, ChatID: 42, UserID: 42}
	for _, attack := range []string{"valid", "wrong-chat", "wrong-user", "bot-sender", "invalid-reply", "wrong-reply-bot", "oversize", "invalid-utf8"} {
		t.Run(attack, func(t *testing.T) {
			calls := 0
			i := &Ingress{Store: &communication.Store{}, Binding: b, Conversation: func(ctx context.Context, u Update) error {
				calls++
				a, ok := owneridentity.FromContext(ctx)
				if !ok || a.OwnerID != "owner" || a.Channel != "telegram" || a.SessionKey != "telegram:42" || a.DeliveryID != "telegram-rich:77:5" {
					t.Fatal("untrusted conversation actor", a)
				}
				return nil
			}}
			u := Update{ID: 5, Message: &Message{ID: 10, Text: "Hello", Chat: Chat{ID: 42, Type: "private"}, From: User{ID: 42}}}
			switch attack {
			case "wrong-chat":
				u.Message.Chat.ID = 43
			case "wrong-user":
				u.Message.From.ID = 43
			case "bot-sender":
				u.Message.From.IsBot = true
			case "invalid-reply":
				u.Message.Reply = &Message{ID: 9, Chat: Chat{ID: 99}, From: User{ID: 77, IsBot: true}}
			case "wrong-reply-bot":
				u.Message.Reply = &Message{ID: 9, Chat: Chat{ID: 42}, From: User{ID: 78, IsBot: true}}
			case "oversize":
				u.Message.Text = strings.Repeat("a", 4001)
			case "invalid-utf8":
				u.Message.Text = string([]byte{0xff})
			}
			err := i.Reply(context.Background(), u)
			if attack == "valid" {
				if err != nil || calls != 1 {
					t.Fatal(err, calls)
				}
			} else if err == nil || calls != 0 {
				t.Fatal("invalid reply/identity fell back to new conversation", attack, err, calls)
			}
		})
	}
}
