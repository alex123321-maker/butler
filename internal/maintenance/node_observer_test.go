package maintenance

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNodeObservationChallengeRejectsArbitrarySigningPayloads(t *testing.T) {
	challenge := ObservationChallenge{ProfileSHA256: strings.Repeat("1", 64), Epoch: 2, Nonce: strings.Repeat("2", 64)}
	raw, err := json.Marshal(challenge)
	if err != nil {
		t.Fatal(err)
	}
	if parsed, err := ParseObservationChallenge(raw); err != nil || parsed != challenge {
		t.Fatal("canonical challenge unavailable", err)
	}
	for _, invalid := range [][]byte{
		nil, append(bytes.Clone(raw), []byte(`{}`)...),
		[]byte(strings.Replace(string(raw), `"epoch":2`, `"epoch":0`, 1)),
		[]byte(strings.Replace(string(raw), `"epoch":2`, `"epoch":1,"epoch":2`, 1)),
		[]byte(strings.TrimSuffix(string(raw), "}") + `,"effect_settled":true}`),
		[]byte(strings.TrimSuffix(string(raw), "}") + `,"container":"other"}`),
		[]byte(strings.TrimSuffix(string(raw), "}") + `,"artifact_sha256":"` + strings.Repeat("3", 64) + `"}`),
		[]byte(strings.Repeat("x", 4097)),
	} {
		if _, err := ParseObservationChallenge(invalid); err == nil {
			t.Fatal("caller-controlled or ambiguous identity challenge accepted")
		}
	}
}
