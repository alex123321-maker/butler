package maintenance

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/butler/butler/internal/actionauth"
)

// OwnerControlRequest enters only the independent private owner endpoint. It
// is never a model tool, Input message, channel reply or public HTTP capability.
// Target and owner identity are fixed by operator configuration, not this body.
type OwnerControlRequest struct {
	Operation             string           `json:"operation"`
	ExpectedProfileSHA256 string           `json:"expected_profile_sha256,omitempty"`
	ExpectedEpoch         int64            `json:"expected_epoch,omitempty"`
	Permission            *OwnerPermission `json:"permission,omitempty"`
	PermissionID          string           `json:"permission_id,omitempty"`
}

type OwnerControlResponse struct {
	Target     *Target            `json:"target,omitempty"`
	Permission *PermissionReceipt `json:"permission,omitempty"`
}

// NewOwnerControlHandler copies the trusted startup identities. Its separate
// bearer credential belongs to the owner control client, outside Core/model and
// checker/node configuration. No request can sign arbitrary bytes or choose a
// target, key file, checker, Docker address or deployment profile.
func NewOwnerControlHandler(store Store, owner, targetID, token string, key ed25519.PrivateKey) http.Handler {
	key = append(ed25519.PrivateKey(nil), key...)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if len(token) < 32 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "private owner authentication required", http.StatusUnauthorized)
			return
		}
		if owner == "" || targetID == "" || store.Pool == nil || len(key) != ed25519.PrivateKeySize {
			http.Error(w, "owner control unavailable", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		if r.Method != http.MethodPost || r.URL.Path != "/private/maintenance/owner" || r.URL.RawQuery != "" {
			http.Error(w, "fixed owner control POST required", http.StatusMethodNotAllowed)
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 65536))
		if err != nil {
			http.Error(w, "bounded owner request required", http.StatusBadRequest)
			return
		}
		if _, err = actionauth.ArgumentsHash(string(raw)); err != nil {
			http.Error(w, "unambiguous owner request required", http.StatusBadRequest)
			return
		}
		var request OwnerControlRequest
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "typed owner request required", http.StatusBadRequest)
			return
		}
		response := OwnerControlResponse{}
		switch request.Operation {
		case "status", "stop":
			if request.ExpectedProfileSHA256 != "" || request.ExpectedEpoch != 0 || request.Permission != nil || request.PermissionID != "" {
				http.Error(w, "unexpected owner scope", http.StatusBadRequest)
				return
			}
			var target Target
			if request.Operation == "stop" {
				target, err = store.Stop(r.Context(), owner, targetID)
			} else {
				target, err = store.Target(r.Context(), owner, targetID)
			}
			response.Target = &target
		case "resume":
			if !validDigest(request.ExpectedProfileSHA256) || request.ExpectedEpoch < 1 || request.Permission != nil || request.PermissionID != "" {
				http.Error(w, "exact resume identity required", http.StatusBadRequest)
				return
			}
			var target Target
			target, err = store.Resume(r.Context(), owner, targetID, request.ExpectedProfileSHA256, request.ExpectedEpoch)
			response.Target = &target
		case "permission":
			if request.Permission == nil || request.ExpectedProfileSHA256 != "" || request.ExpectedEpoch != 0 || request.PermissionID != "" || request.Permission.OwnerID != owner || request.Permission.TargetID != targetID {
				http.Error(w, "exact fixed-target permission required", http.StatusBadRequest)
				return
			}
			var body, signature []byte
			body, signature, err = SignOwnerPermission(*request.Permission, key)
			if err == nil {
				var receipt PermissionReceipt
				receipt, err = store.RegisterPermission(r.Context(), owner, targetID, body, signature)
				response.Permission = &receipt
			}
		case "revoke":
			if request.PermissionID == "" || request.ExpectedProfileSHA256 != "" || request.ExpectedEpoch != 0 || request.Permission != nil {
				http.Error(w, "exact revocation required", http.StatusBadRequest)
				return
			}
			var receipt PermissionReceipt
			receipt, err = store.RevokePermission(r.Context(), owner, targetID, request.PermissionID)
			response.Permission = &receipt
		default:
			http.Error(w, "unsupported owner operation", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "owner operation rejected or unavailable", http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
}
