package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
)

// OwnerControlAuth is deliberately scoped to owner decisions. Browser-extension
// and inter-service tokens have different scopes and cannot act as this owner.
type OwnerControlAuth struct {
	expected   [32]byte
	ownerID    string
	configured bool
}

func NewOwnerControlAuth(token, ownerID string) *OwnerControlAuth {
	return &OwnerControlAuth{expected: sha256.Sum256([]byte(token)), ownerID: ownerID, configured: len(token) >= 32 && strings.TrimSpace(ownerID) != ""}
}
func (a *OwnerControlAuth) Authenticate(r *http.Request) (approvals.Actor, bool) {
	if a == nil || !a.configured {
		return approvals.Actor{}, false
	}
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return approvals.Actor{}, false
	}
	provided := sha256.Sum256([]byte(strings.TrimPrefix(header, "Bearer ")))
	if subtle.ConstantTimeCompare(provided[:], a.expected[:]) != 1 {
		return approvals.Actor{}, false
	}
	delivery := strings.TrimSpace(r.Header.Get("X-Butler-Delivery-ID"))
	if delivery == "" {
		delivery = r.URL.Path + ":" + r.Method
	}
	return approvals.Actor{OwnerID: a.ownerID, Channel: approvals.ResolvedViaWeb, DeliveryID: delivery}, true
}
