package telegramrich

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/butler/butler/internal/credentials"
)

func TestCredentialNoticeProtectedLinkAndClosedState(t *testing.T) {
	var methods []string
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			w.Write([]byte(`{"ok":true,"result":{"id":77,"is_bot":true}}`))
			return
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("invalid card")
		}
		methods = append(methods, r.URL.Path)
		if body["protect_content"] != true {
			t.Error("unprotected card")
		}
		raw, _ := json.Marshal(body)
		if len(methods) == 1 {
			if !strings.Contains(string(raw), "https://butler.example/#credential=creq_fixture") || strings.Contains(string(raw), "callback_data") {
				t.Error("invalid protected form link")
			}
		} else {
			if !strings.Contains(string(raw), "Запрос учётных данных закрыт") || strings.Contains(string(raw), "https://butler.example") {
				t.Error("closed card retained entry link")
			}
		}
		w.Write([]byte(`{"ok":true,"result":{"message_id":12,"chat":{"id":42,"type":"private"}}}`))
	})
	binding := OwnerBinding{OwnerID: "owner", BindingID: "binding", Version: 1, BotID: 77, ChatID: 42, UserID: 42}
	request := credentials.AccessRequest{ID: "creq_fixture", Kind: "access", TargetOrigin: "https://site.example", Purpose: "Sign in", Fields: []string{"username", "password"}, State: "pending"}
	ack, err := c.CredentialNotice(context.Background(), binding, request, "https://butler.example", "")
	if err != nil || ack != "12" {
		t.Fatal("card send")
	}
	request.State = "approved"
	if _, err = c.CredentialNotice(context.Background(), binding, request, "https://butler.example", ack); err != nil {
		t.Fatal("card close")
	}
	if _, err = c.CredentialNotice(context.Background(), binding, request, "http://unprotected.example", ""); err == nil {
		t.Fatal("plaintext form link accepted")
	}
}
