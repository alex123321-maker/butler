package browsersso

import (
	"encoding/json"
	"github.com/butler/butler/internal/browserpage"
	"strings"
	"testing"
	"time"
)

func inspectionScope() Inspection {
	return Inspection{Browser: strings.Repeat("a", 32), RPPage: browserpage.Ref{ID: strings.Repeat("b", 32), DocumentID: strings.Repeat("c", 32), Version: 2}, RPURL: "https://rp.example/", Trigger: "button >> nth=2", IdPOrigin: "https://idp.example", VerifyURL: "https://rp.example/account"}
}

func TestSSOInspectionScopeRejectsCredentialsAndAmbiguousDestinations(t *testing.T) {
	a := inspectionScope()
	raw, _ := json.Marshal(a)
	if _, err := ParseInspection(string(raw)); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Inspection){func(a *Inspection) { a.IdPOrigin = "" }, func(a *Inspection) { a.IdPOrigin = "https://rp.example" }, func(a *Inspection) { a.IdPOrigin += "/login" }, func(a *Inspection) { a.IdPOrigin = "http://idp.example" }, func(a *Inspection) { a.RPPage.Version = 0 }, func(a *Inspection) { a.RPURL += "?state=private" }, func(a *Inspection) { a.VerifyURL = "https://other.example/account" }, func(a *Inspection) { a.Trigger = "\nbutton" }} {
		copy := a
		mutate(&copy)
		encoded, _ := json.Marshal(copy)
		if _, err := ParseInspection(string(encoded)); err == nil {
			t.Fatal("invalid inspection accepted")
		}
	}
	for _, extra := range []string{`"username":{"type":"credential_ref","alias":"record","field":"username"}`, `"password":"private"`, `"rp_url":"https://rp.example/"`} {
		value := strings.TrimSuffix(string(raw), "}") + "," + extra + "}"
		if _, err := ParseInspection(value); err == nil {
			t.Fatal("credential or duplicate input accepted")
		}
	}
}

func TestSSOInspectionFormMetadataAndPrivateDispatch(t *testing.T) {
	a := inspectionScope()
	f := InspectedForm{IdPURL: a.IdPOrigin + "/authorize", TargetURL: a.IdPOrigin + "/login", Form: "form >> nth=1", Username: "input >> nth=2", Password: "input >> nth=3"}
	if scope, err := f.Scope(a); err != nil || scope.RPPage != a.RPPage || scope.Trigger != a.Trigger {
		t.Fatal("actual form metadata refused", err)
	}
	for _, mutate := range []func(*InspectedForm){func(f *InspectedForm) { f.IdPURL = "https://foreign.example/authorize" }, func(f *InspectedForm) { f.TargetURL += "?csrf=private" }, func(f *InspectedForm) { f.Form = "form >> nth=8" }, func(f *InspectedForm) { f.Username = "input[name=private]" }, func(f *InspectedForm) { f.Password = "input >> nth=128" }, func(f *InspectedForm) { f.Password = f.Username }, func(f *InspectedForm) { f.Username = "input >> nth=02" }} {
		copy := f
		mutate(&copy)
		if _, err := copy.Scope(a); err == nil {
			t.Fatal("unsafe form projection accepted")
		}
	}
	d := Dispatch{Page: browserpage.Binding{Ref: a.RPPage, URL: a.RPURL, RuntimeVersion: 4}, Digest: a.Digest(), ExpiresAt: time.Now().Add(time.Minute), Allowed: []NetworkScope{{Origin: "https://rp.example", PathPrefix: "/"}, {Origin: a.IdPOrigin, PathPrefix: "/"}}}
	if !d.ValidInspection(a) {
		t.Fatal("bounded private dispatch refused")
	}
	d.Digest = strings.Repeat("d", 64)
	if d.ValidInspection(a) {
		t.Fatal("foreign inspection digest accepted")
	}
	d.Digest = a.Digest()
	d.Denied = []NetworkScope{{Origin: a.IdPOrigin, PathPrefix: "/"}}
	if d.ValidInspection(a) {
		t.Fatal("IdP denial bypassed")
	}
	if InspectContract().SupportsCredentialRefs || InspectContract().RequiresApproval || InspectContract().EffectContract.RetrySafety != "read_only" {
		t.Fatal("inspection became a credential/POST contract")
	}
}
