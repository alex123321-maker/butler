package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/memorytest"
)

func TestBudgetReadCannotSelectOwnerWithoutProof(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v3/execution-budgets?owner_id=owner", nil)
	w := httptest.NewRecorder()
	NewBudgetHandler(nil, NewOwnerControlAuth("", "owner"), true, true).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("budget owner accepted from query", w.Code)
	}
}

func TestBudgetReadReportsCurrentOwnerDayPolicyWithHistoricalScopesIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ledger := budget.NewLedger(pool)
	_, err := ledger.Ensure(context.Background(), budget.ScopeSpec{Kind: "owner_day", OwnerID: "owner", SubjectID: "historical-day", Deadline: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("t", 32)
	for _, enabled := range []bool{true, false} {
		r := httptest.NewRequest("GET", "/api/v3/execution-budgets", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		NewBudgetHandler(ledger, NewOwnerControlAuth(token, "owner"), enabled, !enabled).ServeHTTP(w, r)
		var got struct {
			budget.Snapshot
			Enabled                       *bool `json:"owner_day_enabled"`
			ConversationModelTurnsEnabled *bool `json:"conversation_model_turns_enabled"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || got.Enabled == nil || *got.Enabled != enabled || got.ConversationModelTurnsEnabled == nil || *got.ConversationModelTurnsEnabled == enabled || len(got.Scopes) != 1 || got.Scopes[0].Kind != "owner_day" {
			t.Fatal("current enforcement confused with retained history", w.Code, w.Body.String())
		}
	}
}
