package actionauth

import (
	"encoding/json"
	"testing"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestReceiptContractRejectsUnsupportedClaims(t *testing.T) {
	record := EffectRecord{Fingerprint: "fingerprint", IdempotencyKey: "key"}
	body := func(key, fingerprint, state, artifact string) string {
		b, _ := json.Marshal(map[string]any{"butler_effect_receipt": map[string]string{"idempotency_key": key, "action_fingerprint": fingerprint, "effect_state": state, "artifact_id": artifact}})
		return string(b)
	}
	for _, test := range []struct {
		name, body        string
		status            int
		truncated, lookup bool
		want              EffectState
	}{
		{"complete", body("key", "fingerprint", "effect_confirmed", "artifact"), 200, false, false, StateConfirmed},
		{"wrong key", body("different", "fingerprint", "effect_confirmed", "artifact"), 200, false, false, StateUnknown},
		{"wrong fingerprint", body("key", "different", "effect_confirmed", "artifact"), 200, false, false, StateUnknown},
		{"no artifact", body("key", "fingerprint", "effect_confirmed", ""), 200, false, false, StateUnknown},
		{"generic created", `{"created":true}`, 201, false, false, StateUnknown},
		{"truncated", body("key", "fingerprint", "effect_confirmed", "artifact"), 200, true, false, StateUnknown},
		{"unsuccessful", body("key", "fingerprint", "effect_confirmed", "artifact"), 500, false, false, StateUnknown},
		{"mutation absence", body("key", "fingerprint", "effect_absent", ""), 200, false, false, StateUnknown},
		{"observed absence", body("key", "fingerprint", "effect_absent", ""), 200, false, true, StateAbsent},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"body": test.body, "status_code": test.status, "truncated": test.truncated})
			got, _ := receiptVerdict(record, string(raw), test.lookup)
			if got != test.want {
				t.Fatalf("verdict=%s want=%s", got, test.want)
			}
		})
	}
}
func TestEffectPolicyPinsVersionAndRecipientGuarantee(t *testing.T) {
	_, _, _, _, call, contract := fixture(t)
	b, _, err := BuildBinding(StableActionID("run", "call"), call, contract, ContextSnapshot{RunID: "run", OwnerID: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	original := b.ContractHash
	contract.ContractRevision = "2"
	changed, err := ContractHash(contract)
	if err != nil || original == changed {
		t.Fatal("tool version not pinned")
	}
	contract.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "external_idempotency", VerificationMethod: "external_receipt_v1", IdempotencyHeader: "Idempotency-Key", ScopeOrigin: "https://different.example"}
	if _, err = ResolveEffectPolicy(b, contract); err == nil {
		t.Fatal("unrelated recipient inherited idempotency")
	}
	contract.EffectContract.ScopeOrigin = "https://example.com"
	if _, err = ResolveEffectPolicy(b, contract); err != nil {
		t.Fatal(err)
	}
	contract.EffectContract.VerificationUrl = "https://different.example/effects/{idempotency_key}"
	if _, err = ResolveEffectPolicy(b, contract); err == nil {
		t.Fatal("independent observation escaped pinned origin")
	}
}
