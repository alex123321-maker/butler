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

func TestLocalBrowserPreparationProofPinsOnlyFrozenLocalState(t *testing.T) {
	for _, tool := range []string{"browser.prepare_fill", "browser.prepare_type"} {
		call := &toolbrokerv1.ToolCall{ToolName: tool, ArgsJson: `{"browser_session_id":"context","expected_url":"https://example.com/form","observation_version":7,"selector":"#input","value":"draft"}`}
		b := ActionBinding{ToolName: tool, Effect: EffectPrepare, ContractRevision: "2", Resource: "browser:context:https://example.com/form"}
		b.ArgsHash, _ = ArgumentsHash(call.ArgsJson)
		contract := &toolbrokerv1.ToolContract{EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "local_browser_preparation_v1"}}
		policy, err := ResolveEffectPolicy(b, contract, call)
		if err != nil {
			t.Fatal(err)
		}
		base := map[string]any{"ok": true, "local_preparation": true, "network_state": "frozen", "browser_session_id": "context", "final_url": "https://example.com/form", "observation_version": 8}
		raw, _ := json.Marshal(base)
		if localPreparationVerdict(policy, b, string(raw)) != StateConfirmed {
			t.Fatal("valid local proof rejected")
		}
		for _, field := range []string{"ok", "local_preparation", "network_state", "browser_session_id", "final_url", "observation_version"} {
			copy := map[string]any{}
			for k, v := range base {
				copy[k] = v
			}
			delete(copy, field)
			raw, _ := json.Marshal(copy)
			if localPreparationVerdict(policy, b, string(raw)) != StateUnknown {
				t.Fatalf("missing %s accepted", field)
			}
		}
		for key, value := range map[string]any{"browser_session_id": "other", "final_url": "https://example.com/other", "observation_version": 7, "network_state": "read_only", "error": map[string]any{}, "truncated": true} {
			copy := map[string]any{}
			for k, v := range base {
				copy[k] = v
			}
			copy[key] = value
			raw, _ := json.Marshal(copy)
			if localPreparationVerdict(policy, b, string(raw)) != StateUnknown {
				t.Fatalf("wrong %s accepted", key)
			}
		}
		b.ToolName = "browser.click"
		if localPreparationVerdict(policy, b, string(raw)) != StateUnknown {
			t.Fatal("local proof confirmed external click")
		}
		if _, err := ResolveEffectPolicy(b, contract, call); err == nil {
			t.Fatal("click borrowed local verification")
		}
	}
}

func TestBrowserReadContractsRequireEnforcedRevision(t *testing.T) {
	for _, tool := range []string{"browser.read", "browser.open", "browser.navigate"} {
		if classifyEffect(tool, nil) != EffectRead {
			t.Fatal("enforced browser read remains opaque")
		}
		binding := ActionBinding{ToolName: tool, Effect: EffectRead, ContractRevision: "1"}
		contract := &toolbrokerv1.ToolContract{EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "read_result"}}
		if _, err := ResolveEffectPolicy(binding, contract); err == nil {
			t.Fatal("old read contract inherited new enforcement")
		}
		binding.ContractRevision = "2"
		if _, err := ResolveEffectPolicy(binding, contract); err != nil {
			t.Fatal(err)
		}
	}
}
