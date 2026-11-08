package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/browserspa"
	"github.com/butler/butler/internal/browsersso"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/credentialcontrol"
	"github.com/butler/butler/internal/delegation"
	toolv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/memory/candidatecontrol"
	"github.com/butler/butler/internal/memory/factcontrol"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/schedulecontrol"
	"github.com/butler/butler/internal/singletabcontrol"
	"github.com/butler/butler/internal/skills"
	"github.com/butler/butler/internal/toolcatalog"
	"github.com/butler/butler/internal/workspaceexec"
)

func testSetup() Setup {
	return Setup{Commit: strings.Repeat("b", 40), Environment: strings.Repeat("c", 64)}
}
func TestCatalogMatchesProductAndOracleStaysPrivate(t *testing.T) {
	setup := testSetup()
	setup.Site = Site{"https://recipient.example", "private-read-code", "private-record-code", "https://recipient.example/private-spa", "cobalt", "private-filtered-marker"}
	setup.Memory = SourceFixture{"дневник погрузки", "private-source-id", "private-memory-code"}
	setup.Skill = SkillFixture{"порядок сверки", strings.Repeat("1", 64), "v-private"}
	suite := cases("s", 1, setup)
	all := map[string]bool{}
	names := map[string]bool{}
	for _, c := range suite {
		if c.ID == "" || all[c.ID] || len(c.Steps) == 0 {
			t.Fatalf("bad case %q", c.ID)
		}
		all[c.ID] = true
		for _, s := range c.Steps {
			for _, secret := range []string{setup.Site.Marker, setup.Site.ReadCode, setup.Site.RecordCode, setup.Memory.Code, setup.Memory.ID, setup.Skill.Digest, setup.Skill.Version} {
				if strings.Contains(s.Prompt, secret) {
					t.Fatalf("%s exposes its private oracle", c.ID)
				}
			}
			if c.Group == "discovery" {
				names[s.Answer] = true
			}
		}
	}
	raw, err := os.ReadFile("../../configs/tools.json")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Tools []struct {
			Name string `json:"tool_name"`
		}
	}
	if err = json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	expected := map[string]bool{}
	for _, c := range cfg.Tools {
		expected[c.Name] = true
	}
	for _, contracts := range [][]*toolv1.ToolContract{
		readcontrol.Contracts(), schedulecontrol.Contracts(), skills.Contracts(), credentialcontrol.Contracts(true), communicationcontrol.Contracts(), generatedtools.Contracts(), workspaceexec.Contracts("workspace"),
		{browserspa.Contract(), browsersso.Contract(), browsersso.InspectContract(), factcontrol.Contract(), candidatecontrol.Contract(), delegation.AdmissionContract(), generatedtools.DeveloperAdmissionContract(), singletabcontrol.RequestContract()},
	} {
		for _, c := range contracts {
			expected[c.ToolName] = true
		}
	}
	for name := range expected {
		if !names[name] {
			t.Errorf("missing natural-language task for %q", name)
		}
	}
	for name := range names {
		if !expected[name] {
			t.Errorf("unknown catalog task %q", name)
		}
	}
}

func fixtureCall(name, args, result string) call {
	return call{name, "completed", "effect_confirmed", json.RawMessage(args), json.RawMessage(result)}
}

func TestArtifactBytesAndUnavailableCapabilities(t *testing.T) {
	dir := t.TempDir()
	span := interval{Ref: "../../model-selected-name", Details: json.RawMessage(`{"format":"image_data_url","content_json":{"image_data_url":"data:image/png;base64,YWJj"}}`)}
	name, err := exportArtifact(dir, span)
	if err != nil || filepath.Base(name) != name || filepath.Ext(name) != ".png" {
		t.Fatal(name, err)
	}
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil || string(b) != "abc" {
		t.Fatal("artifact bytes", err)
	}
	span.Details = json.RawMessage(`{"format":"private_media_v1","content_json":{"mime":"text/plain","sha256":"wrong","data":"YWJj"}}`)
	if _, err := exportArtifact(dir, span); err == nil {
		t.Fatal("wrong attachment hash")
	}
	setup := testSetup()
	setup.DisabledTools = []string{"http.parse_html"}
	_, list, err := makePlan(setup, "s", "", "parse-selector,find-http.parse_html", 1, time.Minute)
	if err != nil || len(list) != 2 || list[0].Missing == "" || list[1].Missing == "" {
		t.Fatal("disabled tool was scored", err)
	}
}
func TestEvidenceNeedsExactReceipts(t *testing.T) {
	catalog := Step{Rule: "catalog", Target: "browser.read_spa", Answer: "browser.read_spa"}
	ref := toolcatalog.Ref(browserspa.Contract())
	search := fixtureCall("tools.search", "{}", `{"status":"ok","items":[{"name":"browser.read_spa","tool_ref":"`+ref+`"}]}`)
	if grade(catalog, catalog.Answer, nil) == "pass" || grade(catalog, catalog.Answer, []call{search}) != "pass" {
		t.Fatal("catalog proof")
	}
	if grade(catalog, catalog.Answer, []call{search, fixtureCall("schedule.create", "{}", "{}")}) != "unexpected_tool" {
		t.Fatal("unrequested effect")
	}
	schema := Step{Rule: "schema", Target: catalog.Target, Answer: "url"}
	load := fixtureCall("tools.load", `{"tool_refs":["`+ref+`"]}`, `{"status":"ok","loaded":["`+ref+`"]}`)
	if grade(schema, "url", []call{search, load}) != "pass" || grade(schema, "url", []call{load, search}) == "pass" {
		t.Fatal("exact search/load order")
	}
	load.Result = json.RawMessage(`{"status":"ok","loaded":["other"]}`)
	if grade(schema, "url", []call{search, load}) == "pass" {
		t.Fatal("stale loaded ref")
	}
	get := Step{Rule: "field", Target: "status", Allowed: []string{"doctor.check_system"}}
	c := fixtureCall("doctor.check_system", "{}", `{"status":"degraded"}`)
	if grade(get, "healthy", []call{c}) == "pass" || grade(get, "degraded", []call{c}) != "pass" {
		t.Fatal("status must come from actual result")
	}
	c.Status = "failed"
	if grade(get, "degraded", []call{c}) == "pass" {
		t.Fatal("failed tool")
	}
}
func TestWorkspaceRequiresSameFileVersionAndOneWrite(t *testing.T) {
	value := "abc"
	h := sha256.Sum256([]byte(value))
	snap := `{"path":"bench.txt","version":1,"bytes":3,"sha256":"` + hex.EncodeToString(h[:]) + `"}`
	write := fixtureCall("workspace.write", `{"workspace_id":"w","path":"bench.txt","content":"abc"}`, `{"workspace_id":"w","snapshot":`+snap+"}")
	read := fixtureCall("workspace.read", `{"workspace_id":"w","path":"bench.txt"}`, `{"workspace_id":"w","snapshot":`+snap+`,"content":"abc"}`)
	calls := []call{fixtureCall("workspace.create", "{}", `{"workspace_id":"w"}`), write, read}
	if !workspaceRoundtrip(value, calls) || workspaceRoundtrip(value, append(calls, write)) {
		t.Fatal("one measured write and readback")
	}
	calls[2].Result = json.RawMessage(strings.ReplaceAll(string(read.Result), `"version":1`, `"version":2`))
	if workspaceRoundtrip(value, calls) {
		t.Fatal("stale or foreign readback")
	}
}
func TestOriginalMustMatchSearchedRevision(t *testing.T) {
	s := Step{Rule: "original", Target: "src", Witness: "X-23", Answer: "X-23", Allowed: []string{"messages.search", "memory.open"}}
	hash := strings.Repeat("e", 64)
	result := `{"items":[{"source":{"source_id":"src","revision":2,"access_revision":1,"content_hash":"` + hash + `"},"text":"код X-23"}]}`
	search := fixtureCall("messages.search", "{}", result)
	open := fixtureCall("memory.open", `{"source_id":"src","source_revision":2,"access_revision":1,"content_hash":"`+hash+`"}`, result)
	if grade(s, s.Answer, []call{search, open}) != "pass" {
		t.Fatal("valid source")
	}
	open.Args = json.RawMessage(strings.ReplaceAll(string(open.Args), "revision\":2", "revision\":1"))
	if grade(s, s.Answer, []call{search, open}) == "pass" || grade(s, s.Answer, []call{search}) == "pass" {
		t.Fatal("snippet or stale source credited")
	}
}
func TestScheduleChecksFinalIdentityAndRevision(t *testing.T) {
	job := func(id, state string, version int) string {
		b, _ := json.Marshal(map[string]any{"schedule": map[string]any{"schedule_id": id, "state": state, "title": "title", "revision": version}})
		return string(b)
	}
	create := `{"job":{"title":"title","mode":"reminder","max_occurrences":1,"paused":true,"timing":{"kind":"once","time_zone":"UTC","after_seconds":86400}}}`
	c := []call{fixtureCall("schedule.create", create, job("s", "paused", 1)), fixtureCall("schedule.get", `{"schedule_id":"s"}`, job("s", "paused", 1)), fixtureCall("schedule.cancel", `{"schedule_id":"s","expected_revision":1}`, job("s", "cancelled", 2)), fixtureCall("schedule.get", `{"schedule_id":"s"}`, job("s", "cancelled", 2))}
	if !scheduleLifecycle("title", c) {
		t.Fatal("valid lifecycle")
	}
	c[3].Result = json.RawMessage(job("another", "cancelled", 2))
	if scheduleLifecycle("title", c) {
		t.Fatal("wrong reminder")
	}
}

type simulated struct{ fail bool }

func (sim simulated) turn(_ context.Context, session, event string, step Step, dir string) Turn {
	n := 1
	part := Turn{Event: event, Input: "input-" + event, Run: "run-" + event, Status: "completed", WebReady: true, MatchedModel: true, ModelCalls: &n, Reason: "pass", AnswerHash: strings.Repeat("b", 64)}
	if sim.fail {
		part.Reason = "wrong_answer"
	}
	return part
}
func TestPairedMatrixPreservesFailuresAndInterruptions(t *testing.T) {
	p, work, err := makePlan(testSetup(), "fixed", "", "missing-fact,session-correction", 2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dirs := []string{filepath.Join(root, "a"), filepath.Join(root, "b")}
	for i, d := range dirs {
		r := newRun(p, Candidate{"fixture", []string{"a", "b"}[i]}, testSetup())
		if err := execute(t.Context(), simulated{i == 1}, d, r, work); err != nil {
			t.Fatal(err)
		}
		if err := execute(t.Context(), simulated{}, d, r, work); err == nil {
			t.Fatal("reused existing run")
		}
	}
	a, ar, err := readRun(dirs[0])
	if err != nil {
		t.Fatal(err)
	}
	b, br, err := readRun(dirs[1])
	if err != nil {
		t.Fatal(err)
	}
	report, err := compare(a, ar, b, br)
	if err != nil || report.WinsA != 4 || report.A.Total.Consistent != 2 || report.A.Total.UnknownUsage != 8 {
		t.Fatalf("unexpected paired result: %+v %v", report, err)
	}
	if err = os.Remove(rowFile(dirs[1], "slot", 0)); err != nil {
		t.Fatal(err)
	}
	_, br, err = readRun(dirs[1])
	if err != nil || br[0].Status != "interrupted" {
		t.Fatal("lost attempt hidden", err)
	}
	report, err = compare(a, ar, b, br)
	if err != nil || report.Unscored != 1 || report.B.Total.Planned != 4 {
		t.Fatal("unpaired denominator", err)
	}
	b.PlanHash = strings.Repeat("c", 64)
	if _, err = compare(a, ar, b, br); err == nil {
		t.Fatal("incompatible plans compared")
	}
	if err = writeJSON(rowFile(dirs[1], "slot", 90), ar[0]); err != nil {
		t.Fatal(err)
	}
	if _, _, err = readRun(dirs[1]); err == nil {
		t.Fatal("foreign slot accepted")
	}
}
