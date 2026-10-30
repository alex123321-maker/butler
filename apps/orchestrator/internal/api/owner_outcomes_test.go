package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/owneridentity"
)

type outcomeAPIStub struct {
	calls int
	actor owneridentity.Actor
}

func (s *outcomeAPIStub) Pending(ctx context.Context) ([]actionauth.OwnerOutcomeReview, error) {
	s.actor, _ = owneridentity.FromContext(ctx)
	s.calls++
	return []actionauth.OwnerOutcomeReview{{ID: "review", OwnerID: "do-not-export", SessionKey: "do-not-export"}}, nil
}
func (s *outcomeAPIStub) Historical(ctx context.Context, _ string, _ string) (actionauth.OwnerOutcomeReview, error) {
	s.actor, _ = owneridentity.FromContext(ctx)
	s.calls++
	return actionauth.OwnerOutcomeReview{ID: "review"}, nil
}
func (s *outcomeAPIStub) Decide(ctx context.Context, _ actionauth.OwnerOutcomeDecision) (actionauth.OwnerOutcomeReview, error) {
	s.actor, _ = owneridentity.FromContext(ctx)
	s.calls++
	return actionauth.OwnerOutcomeReview{ID: "review"}, nil
}

func TestOwnerOutcomeAPIRejectsApprovalAndUntrustedActorBeforeStore(t *testing.T) {
	for _, name := range []string{"anonymous", "duplicate_auth", "approval", "foreign_owner_field", "duplicate_key", "oversize", "valid_list", "valid_report"} {
		t.Run(name, func(t *testing.T) {
			s := &outcomeAPIStub{}
			token := strings.Repeat("owner-token-", 4)
			body := `{"action":"report","decision":{"review_id":"review","fingerprint":"fp","revision":1,"outcome":"unknown"}}`
			method := "POST"
			switch name {
			case "approval":
				body = `{"action":"report","decision":{"review_id":"review","approved":true}}`
			case "foreign_owner_field":
				body = `{"action":"report","owner_id":"other","decision":{"review_id":"review"}}`
			case "duplicate_key":
				body = `{"action":"report","action":"review_historical"}`
			case "oversize":
				body = strings.Repeat("x", 9000)
			case "valid_list":
				method = "GET"
				body = ""
			}
			r := httptest.NewRequest(method, "/api/v4/action-outcomes", strings.NewReader(body))
			if name != "anonymous" {
				r.Header.Set("Authorization", "Bearer "+token)
			}
			if name == "duplicate_auth" {
				r.Header.Add("Authorization", "Bearer "+token)
			}
			r.Header.Set("X-Butler-Delivery-ID", "actual-owner-response")
			w := httptest.NewRecorder()
			OwnerOutcomes(NewOwnerControlAuth(token, "owner"), s).ServeHTTP(w, r)
			valid := name == "valid_list" || name == "valid_report"
			if valid {
				if w.Code != http.StatusOK || s.calls != 1 || s.actor.OwnerID != "owner" || s.actor.DeliveryID != "actual-owner-response" {
					t.Fatal("authenticated owner lost", w.Code, s.actor)
				}
			} else if w.Code == http.StatusOK || s.calls != 0 {
				t.Fatal("untrusted approval reached reconciliation", w.Code)
			}
			if strings.Contains(w.Body.String(), "do-not-export") {
				t.Fatal("private binding fields leaked")
			}
		})
	}
}
