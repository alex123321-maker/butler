package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/internal/singletabcontrol"
)

func TestGuardedRequestNotificationUsesProtectedOwnerPage(t *testing.T) {
	messages, markups := 0, 0
	client := &Client{}
	client.sendMessage = func(_ context.Context, _ int64, text string) (Message, error) {
		messages++
		if !strings.Contains(text, "Подтверждения") {
			t.Fatal("protected owner page missing")
		}
		return Message{}, nil
	}
	client.sendMessageMarkup = func(context.Context, int64, string, *InlineKeyboardMarkup) (Message, error) {
		markups++
		return Message{}, nil
	}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, flow.NewApprovalGate(), nil)
	if err != nil {
		t.Fatal(err)
	}
	request := flow.ApprovalRequest{SessionKey: "telegram:chat:777", ToolName: singletabcontrol.BindTool, ApprovalType: "browser_tab_selection", TabCandidates: []flow.ApprovalTabCandidate{{CandidateToken: "private-candidate", DisplayLabel: "origin"}}}
	if err = adapter.DeliverApprovalRequest(context.Background(), request); err != nil || messages != 1 || markups != 0 {
		t.Fatal("guarded request used legacy/generic callback", err)
	}
}
