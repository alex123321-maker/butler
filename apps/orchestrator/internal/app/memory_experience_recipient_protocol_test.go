package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/butler/butler/internal/extensions/projectfixture"
)

func experienceProposal(recipient *experienceRecipient, resource, value string) projectfixture.Proposal {
	before := recipient.state.Config.Files[resource]
	after := strings.ReplaceAll(before, "ready=false\n", "ready="+value+"\n")
	after = strings.ReplaceAll(after, "ready=pending\n", "ready="+value+"\n")
	return projectfixture.Proposal{ProjectRevision: recipient.revision(resource), Changes: []projectfixture.Change{{Path: "README.md", Before: before, After: after}}}
}

func experiencePost(t *testing.T, site *httptest.Server, resource, key string, proposal projectfixture.Proposal) (*http.Response, error) {
	t.Helper()
	raw, _ := json.Marshal(proposal)
	req, err := http.NewRequest(http.MethodPost, site.URL+"/project/"+resource+"/patches", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("Butler-Action-Fingerprint", experienceDigest(key))
	return site.Client().Do(req)
}

func experienceResponse(t *testing.T, response *http.Response, want int) string {
	t.Helper()
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != want {
		t.Fatal("recipient response", response.StatusCode, want, err, string(raw))
	}
	return string(raw)
}

func TestExperienceRecipientExactCurrentArtifactAndKnownRejection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("durable recipient directory fsync proof runs on the Linux isolated stand")
	}
	config := experienceRecipientConfig{Version: "v1", Ready: "true", Files: map[string]string{"alpha": "Synthetic project\nready=false\n<!-- provenance: predeclared-a -->\n", "beta": "Synthetic project\nready=false\n<!-- provenance: predeclared-b -->\n"}}
	dir := t.TempDir()
	r, err := newExperienceRecipient(dir, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.close() })
	site := httptest.NewServer(r)
	defer site.Close()
	for _, resource := range []string{"alpha", "beta"} {
		response, err := site.Client().Get(site.URL + "/project/" + resource)
		if err != nil {
			t.Fatal(err)
		}
		body := experienceResponse(t, response, 200)
		if !strings.Contains(body, experienceEnvironment(config)) || !strings.Contains(body, `"accepted_ready_value":"true"`) {
			t.Fatal("current contract/environment not observable", body)
		}
	}
	proposal := experienceProposal(r, "alpha", "true")
	response, err := experiencePost(t, site, "alpha", "one-success", proposal)
	if err != nil {
		t.Fatal(err)
	}
	first := experienceResponse(t, response, 201)
	var receipt struct {
		Artifact string `json:"artifact_id"`
	}
	if json.Unmarshal([]byte(first), &receipt) != nil || receipt.Artifact == "" {
		t.Fatal("exact artifact absent")
	}
	response, err = site.Client().Get(site.URL + "/project/alpha/patches/" + receipt.Artifact + "/checks")
	if err != nil {
		t.Fatal(err)
	}
	checked := experienceResponse(t, response, 200)
	if !strings.Contains(checked, `"checks_passed":true`) || !strings.Contains(checked, `"applied":false`) || !strings.Contains(checked, "predeclared-a") {
		t.Fatal("artifact lost canary or fabricated application", checked)
	}
	response, err = site.Client().Get(site.URL + "/project/beta/patches/" + receipt.Artifact + "/checks")
	if err != nil {
		t.Fatal(err)
	}
	experienceResponse(t, response, 404)
	for _, invalid := range []struct {
		key string
		p   projectfixture.Proposal
	}{{"unsupported", experienceProposal(r, "alpha", "maybe")}, {"stale-before", projectfixture.Proposal{ProjectRevision: r.revision("alpha"), Changes: []projectfixture.Change{{Path: "README.md", Before: "stale", After: proposal.Changes[0].After}}}}, {"stale-revision", projectfixture.Proposal{ProjectRevision: strings.Repeat("0", 64), Changes: proposal.Changes}}, {"caller-path", projectfixture.Proposal{ProjectRevision: r.revision("alpha"), Changes: []projectfixture.Change{{Path: "../private", Before: proposal.Changes[0].Before, After: proposal.Changes[0].After}}}}} {
		response, err = experiencePost(t, site, "alpha", invalid.key, invalid.p)
		if err != nil {
			t.Fatal(err)
		}
		body := experienceResponse(t, response, 400)
		if strings.Contains(body, "butler_effect_receipt") {
			t.Fatal("HTTP rejection asserted typed absence")
		}
		response, err = site.Client().Get(site.URL + "/project/effects/" + invalid.key)
		if err != nil {
			t.Fatal(err)
		}
		body = experienceResponse(t, response, 200)
		if !strings.Contains(body, `"effect_state":"effect_absent"`) || !strings.Contains(body, experienceDigest(invalid.key)) || !strings.Contains(body, `"artifact_id":""`) {
			t.Fatal("independent exact-key rejection lookup missing", body)
		}
	}
	if err = r.close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := newExperienceRecipient(dir, config)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.close()
	restartedSite := httptest.NewServer(restarted)
	defer restartedSite.Close()
	response, err = experiencePost(t, restartedSite, "alpha", "one-success", proposal)
	if err != nil || experienceResponse(t, response, 201) != first {
		t.Fatal("restart changed immutable receipt", err)
	}
	if len(restarted.state.Entries) != 5 || restarted.state.Config.Files["alpha"] != config.Files["alpha"] {
		t.Fatal("rejection created artifact or changed original")
	}
}

func TestExperienceRecipientPersistedOutageAndChangedEnvironment(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("durable recipient directory fsync proof runs on the Linux isolated stand")
	}
	config := experienceRecipientConfig{Version: "v1", Ready: "true", Files: map[string]string{"alpha": "Synthetic\nready=false\n<!-- provenance: fixed-before-work -->\n"}, Outage: true}
	dir := t.TempDir()
	r, err := newExperienceRecipient(dir, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.close() })
	site := httptest.NewServer(r)
	response, err := experiencePost(t, site, "alpha", "lost-receipt", experienceProposal(r, "alpha", "true"))
	if err == nil {
		response.Body.Close()
		t.Fatal("persisted fault returned an acknowledgement")
	}
	site.Close()
	if len(r.state.Entries) != 1 || r.state.Entries["lost-receipt"].State != "effect_confirmed" || r.state.Dispatches != 1 {
		t.Fatal("hidden recipient audit did not prove one persistence")
	}
	if err = r.close(); err != nil {
		t.Fatal(err)
	}
	r, err = newExperienceRecipient(dir, config)
	if err != nil {
		t.Fatal(err)
	}
	defer r.close()
	site = httptest.NewServer(r)
	defer site.Close()
	for _, path := range []string{"/project/effects/lost-receipt", "/project/alpha/patches/" + r.state.Entries["lost-receipt"].Artifact + "/checks"} {
		response, err = site.Client().Get(site.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		if body := experienceResponse(t, response, 503); strings.Contains(body, "butler_effect_receipt") || strings.Contains(body, "artifact_id") {
			t.Fatal("outage leaked hidden persistence as live proof")
		}
	}
	v2 := experienceRecipientConfig{Version: "v2", Ready: "verified", Files: map[string]string{"alpha": "Synthetic\nready=pending\n<!-- provenance: fixed-before-work -->\n"}}
	changed, err := newExperienceRecipient(t.TempDir(), v2)
	if err != nil {
		t.Fatal(err)
	}
	defer changed.close()
	if experienceEnvironment(config) == experienceEnvironment(v2) {
		t.Fatal("changed readiness contract retained environment identity")
	}
	other := httptest.NewServer(changed)
	defer other.Close()
	response, err = experiencePost(t, other, "alpha", "wrong-v1-value", experienceProposal(changed, "alpha", "true"))
	if err != nil {
		t.Fatal(err)
	}
	experienceResponse(t, response, 400)
	response, err = experiencePost(t, other, "alpha", "current-v2-value", experienceProposal(changed, "alpha", "verified"))
	if err != nil {
		t.Fatal(err)
	}
	experienceResponse(t, response, 201)
}
