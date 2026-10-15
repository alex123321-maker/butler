package candidates

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/butler/butler/internal/memory/catalog"
)

func validOutput(quote string) string {
	b, _ := json.Marshal(Output{Units: []Unit{{SourceID: "s", Type: "explicit_preference", SpeechAct: "negation", Key: "preference.detail", Quote: quote}}, Skips: []Skip{}})
	return string(b)
}
func TestExactUnicodeCitationAndUnknownTime(t *testing.T) {
	body := "Заметка 🧪. Не сокращай технические объяснения, кроме кратких статусов."
	quote := "Не сокращай технические объяснения, кроме кратких статусов."
	got, err := (Handler{}).Validate([]catalog.Source{{ID: "s", Kind: "owner_statement", Revision: 7, Hash: strings.Repeat("a", 64), Body: body, ScopeType: "user", ScopeID: "owner"}}, validOutput(quote))
	if err != nil {
		t.Fatal(err)
	}
	var result Result
	if json.Unmarshal([]byte(got), &result) != nil || len(result.Candidates) != 1 {
		t.Fatal(got)
	}
	e := result.Candidates[0]
	if e.Start != utf8.RuneCountInString("Заметка 🧪. ") || e.End != utf8.RuneCountInString(body) || e.EventTime != nil || e.ValidTimeQuote != nil || e.ScopeQuote != nil || e.SourceRevision != 7 || e.SourceHash != strings.Repeat("a", 64) {
		t.Fatal(got)
	}
}
func TestRejectInventedIncompleteAmbiguousAndCredentialEvidence(t *testing.T) {
	quote := "Не сокращай объяснения, кроме статусов."
	src := catalog.Source{ID: "s", Kind: "owner_statement", Body: quote}
	for _, raw := range []string{
		validOutput("Сокращай объяснения."), validOutput("сокращай объяснения, кроме статусов."), validOutput("Не сокращай объяснения"),
		strings.Replace(validOutput(quote), `"speech_act":"negation"`, `"speech_act":"hypothesis"`, 1),
		strings.Replace(validOutput(quote), `"key":"preference.detail"`, `"key":"task.mandate"`, 1),
		strings.Replace(validOutput(quote), `"source_id":"s"`, `"source_id":"s","source_id":"s"`, 1),
		strings.Replace(validOutput(quote), `"scope_quote":null`, `"scope_quote":"global"`, 1),
		strings.Replace(validOutput(quote), `"valid_time_quote":null`, `"valid_time_quote":"сейчас"`, 1),
		strings.Replace(validOutput(quote), `"quote":`, `"value":"invented","quote":`, 1),
		`{"units":[],"skips":[]}`, `{"units":[],"skips":[{"source_id":"s","reason":"assumed"}]}`,
	} {
		if _, err := (Handler{}).Validate([]catalog.Source{src}, raw); err == nil {
			t.Fatal("accepted", raw)
		}
	}
	secret := "access_token=synthetic123"
	if _, err := (Handler{}).Validate([]catalog.Source{{ID: "s", Body: secret}}, validOutput(secret)); err == nil {
		t.Fatal("credential persisted")
	}
	if _, err := (Handler{}).Validate([]catalog.Source{{ID: "s", Body: quote + " " + quote}}, validOutput(quote)); err == nil {
		t.Fatal("ambiguous occurrence")
	}
}
func TestSkipsAccountForEverySourceWithoutPromotingModelData(t *testing.T) {
	for _, reason := range []string{"no_durable_unit", "noise", "temporary", "quotation", "hypothesis", "conditional", "ambiguous", "unsupported_source"} {
		raw := `{"units":[],"skips":[{"source_id":"s","reason":"` + reason + `"}]}`
		if _, err := (Handler{}).Validate([]catalog.Source{{ID: "s", Kind: "model_candidate", Body: "quoted"}}, raw); err != nil {
			t.Fatal(reason, err)
		}
	}
	if _, err := (Handler{}).Validate([]catalog.Source{{ID: "s", Kind: "model_candidate", Body: "Заявление."}}, validOutput("Заявление.")); err == nil {
		t.Fatal("recursive model candidate")
	}
}
