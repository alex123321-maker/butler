package api

import (
	"net/http/httptest"
	"testing"
)

func TestBudgetReadCannotSelectOwnerWithoutProof(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v3/execution-budgets?owner_id=owner", nil)
	w := httptest.NewRecorder()
	NewBudgetHandler(nil, NewOwnerControlAuth("", "owner")).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("budget owner accepted from query", w.Code)
	}
}
