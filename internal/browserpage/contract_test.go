package browserpage

import (
	"encoding/json"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"strings"
	"testing"
)

func TestPageContractRejectsAmbiguousOrCredentialInputs(t *testing.T) {
	a := Args{SessionID: strings.Repeat("a", 32), Page: &Ref{ID: strings.Repeat("b", 32), DocumentID: strings.Repeat("c", 32), Version: 1}, Selector: "#go"}
	raw, _ := json.Marshal(a)
	if _, e := Parse(Click, string(raw)); e != nil {
		t.Fatal("valid exact page denied")
	}
	for _, s := range []string{strings.Replace(string(raw), `"page_id":`, `"PAGE_ID":`, 1), strings.Replace(string(raw), `"page_id":`, `"page_id":"`+strings.Repeat("d", 32)+`","page_id":`, 1), strings.Replace(string(raw), `"selector":"#go"`, `"selector":"#go","value":{"type":"credential_ref"}`, 1), string(raw) + `{}`, `{"browser_session_id":"` + a.SessionID + `","selector":"#go"}`} {
		if _, e := Parse(Click, s); e == nil {
			t.Fatal("ambiguous/unsupported page input accepted")
		}
	}
	c := &toolbrokerv1.ToolContract{ToolName: Click, ContractRevision: Revision, InputSchemaJson: Schema(Click), RequiresApproval: true, EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "none"}}
	if !ValidContract(c) {
		t.Fatal("valid page contract denied")
	}
	c.RequiresApproval = false
	if ValidContract(c) {
		t.Fatal("page mutation lost strict approval")
	}
	c.RequiresApproval = true
	c.SupportsCredentialRefs = true
	if ValidContract(c) {
		t.Fatal("credential contract upgraded silently")
	}
}
