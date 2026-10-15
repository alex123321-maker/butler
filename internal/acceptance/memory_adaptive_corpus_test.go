package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryAdaptiveProspectiveHoldoutControls(t *testing.T) {
	root := filepath.Join("..", "..", "deploy", "memory-adaptive", "holdout-v1")
	raw, err := os.ReadFile(filepath.Join(root, "corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var suite MemorySemanticSuite
	if err := json.Unmarshal(raw, &suite); err != nil {
		t.Fatal(err)
	}
	if err := suite.Validate(); err != nil {
		t.Fatal(err)
	}
	var controls struct {
		Version string `json:"version"`
		Cases   []struct {
			CaseID          string   `json:"case_id"`
			Group           string   `json:"group"`
			GrantedProjects []string `json:"granted_projects"`
			Permitted       []string `json:"permitted_source_ids"`
			Sources         []struct {
				SourceID   string `json:"source_id"`
				Kind       string `json:"kind"`
				Scope      string `json:"scope"`
				ProjectID  string `json:"project_id"`
				Session    string `json:"source_session"`
				EventTime  string `json:"import_event_time"`
				Accessible bool   `json:"accessible"`
			} `json:"sources"`
		} `json:"cases"`
	}
	raw, err = os.ReadFile(filepath.Join(root, "controls.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &controls); err != nil {
		t.Fatal(err)
	}
	if controls.Version != "butler-memory-adaptive-controls-v1" || len(controls.Cases) != 32 || len(suite.Cases) != 32 {
		t.Fatal("complete32-case corpus and import controls required")
	}
	groups := map[string]int{}
	answerable, selfContained, denied := 0, 0, 0
	for j, c := range suite.Cases {
		p := controls.Cases[j]
		if p.CaseID != c.ID || len(p.Sources) != len(c.History) || len(c.Coverage) != 1 || c.Coverage[0] != p.Group {
			t.Fatal("controls do not bind exact case/originals", c.ID)
		}
		groups[p.Group]++
		if c.Answerable {
			answerable++
		}
		if c.EvidenceBasis == "question" {
			selfContained++
		}
		for k, s := range c.History {
			pin := p.Sources[k]
			if pin.SourceID != s.ID || pin.Session != s.Session || pin.EventTime != s.EventUTC || pin.Kind != "document" || pin.Scope != "project" {
				t.Fatal("import pin differs from original", c.ID, s.ID)
			}
			granted := containsMemory(p.GrantedProjects, pin.ProjectID)
			if pin.Accessible != granted || containsMemory(p.Permitted, s.ID) != (granted && s.Status == "active") {
				t.Fatal("fixture permitted set bypasses actual scope/tombstone", c.ID, s.ID)
			}
			if !granted {
				denied++
			}
		}
		for _, id := range c.OracleSourceIDs {
			if !containsMemory(p.Permitted, id) {
				t.Fatal("oracle uses ungranted/deleted original", c.ID, id)
			}
		}
	}
	if answerable != 24 || selfContained != 8 || denied != 1 || groups["exact_id"] != 4 || groups["event_time"] != 4 || groups["cross_session"] != 4 || groups["multi_hop"] != 4 {
		t.Fatal("prospective coverage or refusal denominator changed", groups, answerable, selfContained, denied)
	}
}
