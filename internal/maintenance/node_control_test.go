package maintenance

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Signatures here are synthetic in-process fixtures. These tests verify the
// actual private HTTP/Store boundary, not physical node execution or release QA.
func TestMaintenanceNodeControlDispatchOutcomeIntegration(t *testing.T) {
	store, request, controllerKey, nodeKey := publicClaimStoreKeys(t)
	claim, err := store.ReserveClaim(t.Context(), request, controllerKey)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("public-node-control-", 3)
	server := httptest.NewServer(NewNodeControlHandler(store, request.OwnerID, request.TargetID, token))
	defer server.Close()
	client, err := NewNodeControlClient(server.URL+NodeControlPath, token)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	admitted, err := client.AdmitDispatch(t.Context(), claim.Raw, claim.Signature)
	if err != nil || admitted.AttemptID != claim.Claim.AttemptID || admitted.Fence != claim.Claim.Fence {
		t.Fatal("exact HTTP dispatch unavailable", err)
	}
	if _, err = client.AdmitDispatch(t.Context(), claim.Raw, claim.Signature); err == nil {
		t.Fatal("HTTP client replayed consumed dispatch")
	}
	if _, err = store.Stop(t.Context(), request.OwnerID, request.TargetID); err != nil {
		t.Fatal(err)
	}
	var now time.Time
	if err = store.Pool.QueryRow(t.Context(), `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	outcome := NodeOutcome{Version: "butler.maintenance.node-outcome.v1", AttemptID: admitted.AttemptID, ClaimSHA256: Digest(claim.Raw), State: "completed", ArtifactSHA256: admitted.Binding.CandidateSHA256, RuntimeSHA256: admitted.Binding.RuntimeSHA256, EvidenceSHA256: Digest([]byte("synthetic signed measurement; no physical operation")), ObservedAt: now}
	raw, signature, err := SignNodeOutcome(outcome, nodeKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.RecordOutcome(t.Context(), raw, signature); err != nil {
		t.Fatal("signed node HTTP outcome unavailable", err)
	}
	target, err := store.Target(t.Context(), request.OwnerID, request.TargetID)
	if err != nil || !target.Stopped || target.UnresolvedAttempt != "" || target.ArtifactSHA256 != admitted.Binding.CandidateSHA256 {
		t.Fatal("HTTP outcome lost the newer owner stop or actual signed identity", err)
	}
	var state string
	var consumed int
	if err = store.Pool.QueryRow(t.Context(), `SELECT state,(SELECT used_claims FROM maintenance_permissions WHERE permission_id=$2) FROM maintenance_attempts WHERE attempt_id=$1`, admitted.AttemptID, request.PermissionID).Scan(&state, &consumed); err != nil || state != "completed" || consumed != 1 {
		t.Fatal("HTTP boundary rewrote one-use accounting", err)
	}
}

func TestMaintenanceNodeControlRequestBoundariesIntegration(t *testing.T) {
	store, request, key := publicClaimStore(t)
	claim, err := store.ReserveClaim(t.Context(), request, key)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("public-node-control-", 3)
	handler := NewNodeControlHandler(store, request.OwnerID, request.TargetID, token)
	good, _ := json.Marshal(NodeControlRequest{Operation: "dispatch", Raw: claim.Raw, Signature: claim.Signature})
	for _, name := range []string{"owner_credential", "duplicate_credential", "owner_stop", "create_permission", "reserve_claim", "alias", "duplicate_operation", "foreign_target_field", "query"} {
		t.Run(name, func(t *testing.T) {
			body := bytes.Clone(good)
			path := NodeControlPath
			switch name {
			case "owner_stop", "create_permission", "reserve_claim":
				operation := map[string]string{"owner_stop": "stop", "create_permission": "permission", "reserve_claim": "claim"}[name]
				body, _ = json.Marshal(NodeControlRequest{Operation: operation, Raw: claim.Raw, Signature: claim.Signature})
			case "alias":
				body = bytes.Replace(body, []byte(`"operation"`), []byte(`"Operation"`), 1)
			case "duplicate_operation":
				body = append([]byte(`{"operation":"dispatch",`), body[1:]...)
			case "foreign_target_field":
				body = append([]byte(`{"target_id":"other",`), body[1:]...)
			case "query":
				path += "?target=other"
			}
			r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
			r.Header.Set("Authorization", "Bearer "+token)
			if name == "owner_credential" {
				r.Header.Set("Authorization", "Bearer "+strings.Repeat("separate-owner-", 3))
			}
			if name == "duplicate_credential" {
				r.Header.Add("Authorization", "Bearer "+token)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, r)
			if response.Code < 400 {
				t.Fatal("ambiguous or out-of-role node request accepted")
			}
		})
	}
	var state string
	if err = store.Pool.QueryRow(t.Context(), `SELECT state FROM maintenance_attempts WHERE attempt_id=$1`, claim.Claim.AttemptID).Scan(&state); err != nil || state != "pending" {
		t.Fatal("rejected HTTP requests consumed dispatch", err)
	}
	target, err := store.Target(t.Context(), request.OwnerID, request.TargetID)
	if err != nil || target.Stopped {
		t.Fatal("node request changed owner stop state", err)
	}
}

func TestMaintenanceNodeControlClientUnavailableIsNotRetried(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client, err := NewNodeControlClient(server.URL+NodeControlPath, strings.Repeat("public-node-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.AdmitDispatch(context.Background(), []byte(`{"synthetic":"not an admitted claim"}`), make([]byte, ed25519.SignatureSize)); err == nil || calls.Load() != 1 {
		t.Fatal("unavailable private controller reply became success or was retried")
	}
	for _, endpoint := range []string{"http://example.invalid" + NodeControlPath, server.URL + NodeControlPath + "?query=1", server.URL + "/private/maintenance/owner", server.URL + NodeControlPath + "#fragment"} {
		if _, err := NewNodeControlClient(endpoint, strings.Repeat("public-node-", 3)); err == nil {
			t.Fatal("noncanonical private node destination accepted")
		}
	}
}
