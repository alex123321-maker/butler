package api

import (
	"context"
	"errors"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeOutbox struct {
	messages []deliveryoutbox.Message
	err      error
}

func (f fakeOutbox) ListByRun(context.Context, string) ([]deliveryoutbox.Message, error) {
	return f.messages, f.err
}
func TestTaskDeliveryUsesDurableOutboxUnknownOverPartialStatus(t *testing.T) {
	server := NewTaskViewServer(nil, nil, nil, nil, nil, nil, nil)
	server.SetFinalResults(nil, fakeOutbox{messages: []deliveryoutbox.Message{{Channel: "telegram", Status: "sent", ChunkIndex: 0, ChannelACK: "1"}, {Channel: "telegram", Status: "delivery_unknown", ChunkIndex: 1}, {Channel: "telegram", Status: "pending", ChunkIndex: 2}}})
	state := server.loadDeliveryState(httptest.NewRequest(http.MethodGet, "/", nil), "run")
	if state["state"] != "delivery_unknown" {
		t.Fatal("ambiguous send concealed", state)
	}
	server.SetFinalResults(nil, fakeOutbox{err: errors.New("postgres unavailable")})
	state = server.loadDeliveryState(httptest.NewRequest(http.MethodGet, "/", nil), "run")
	if state["state"] != "delivery_read_unavailable" {
		t.Fatal("read failure presented as sent")
	}
}
