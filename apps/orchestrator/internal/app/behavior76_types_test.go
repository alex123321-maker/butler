package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/butler/butler/apps/orchestrator/internal/task"
)

// Stimuli contain no scoring fields. Expected outcomes are held separately by
// the independent checker and are never read by this collector.
type behavior76Episode struct {
	ID         string `json:"id"`
	Family     string `json:"family"`
	Provenance struct {
		Kind       string   `json:"kind"`
		SourceRefs []string `json:"source_refs"`
	} `json:"provenance"`
	Initial struct {
		Tasks             []behavior76Task        `json:"tasks"`
		Profiles          []behavior76Profile     `json:"profiles"`
		Documents         []behavior76Document    `json:"documents"`
		Resources         []behavior76Resource    `json:"resources"`
		PriorObservations []behavior76Observation `json:"prior_observations"`
	} `json:"initial"`
	Steps []behavior76Step `json:"steps"`
}
type behavior76Task struct {
	Key                string            `json:"key"`
	ParentTaskKey      string            `json:"parent_task_key,omitempty"`
	Goal               string            `json:"goal"`
	Constraints        []string          `json:"constraints"`
	CompletionCriteria []string          `json:"completion_criteria"`
	ExecutionPolicy    string            `json:"execution_policy"`
	Status             string            `json:"status"`
	Working            task.WorkingState `json:"working"`
}
type behavior76Profile struct {
	Key       string `json:"key"`
	Summary   string `json:"summary"`
	ScopeType string `json:"scope_type"`
	ScopeID   string `json:"scope_id"`
	SourceKey string `json:"source_key"`
	Revision  int64  `json:"revision"`
	Status    string `json:"status"`
}
type behavior76Document struct {
	Key       string `json:"key"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	SourceKey string `json:"source_key"`
	Revision  int64  `json:"revision"`
}
type behavior76Resource struct {
	Key     string `json:"key"`
	TaskKey string `json:"task_key"`
	Body    string `json:"body"`
	Version int64  `json:"version"`
}
type behavior76Observation struct {
	Kind         string `json:"kind"`
	TaskKey      string `json:"task_key"`
	ResourceKey  string `json:"resource_key"`
	SourceKey    string `json:"source_key"`
	Summary      string `json:"summary"`
	EvidenceKind string `json:"evidence_kind"`
}
type behavior76Step struct {
	ID         string          `json:"id"`
	Event      behavior76Event `json:"event"`
	Evaluate   bool            `json:"evaluate"`
	RunTaskKey *string         `json:"run_task_key"`
}
type behavior76Event struct {
	Type            string    `json:"type"`
	Text            string    `json:"text,omitempty"`
	Intent          string    `json:"intent,omitempty"`
	TargetTaskKey   *string   `json:"target_task_key,omitempty"`
	Kind            string    `json:"kind,omitempty"`
	TaskKey         string    `json:"task_key,omitempty"`
	Instruction     string    `json:"instruction,omitempty"`
	Goal            *string   `json:"goal,omitempty"`
	Constraints     *[]string `json:"constraints,omitempty"`
	ExecutionPolicy string    `json:"execution_policy,omitempty"`
	ResourceKey     string    `json:"resource_key,omitempty"`
	NewBody         string    `json:"new_body,omitempty"`
	Provenance      string    `json:"provenance,omitempty"`
	SourceKey       string    `json:"source_key,omitempty"`
	Operation       string    `json:"operation,omitempty"`
	Summary         *string   `json:"summary,omitempty"`
}

func behavior76ReadEpisodes(r io.Reader) ([]behavior76Episode, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 256<<10)
	var episodes []behavior76Episode
	ids := map[string]bool{}
	families := map[string]bool{}
	for _, f := range strings.Fields("status_continuation amendment scoped_stop_negation new_task_coexistence manual_change owner_done_report clarification preference_exception feedback_candidate preference_lifecycle untrusted_command restart") {
		families[f] = true
	}
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var e behavior76Episode
		dec := json.NewDecoder(strings.NewReader(scanner.Text()))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&e); err != nil {
			return nil, fmt.Errorf("invalid episode JSON: %w", err)
		}
		if err:=dec.Decode(new(any));err!=io.EOF{return nil,fmt.Errorf("trailing episode JSON")}
		if e.ID == "" || ids[e.ID] || !families[e.Family] || e.Provenance.Kind != "synthetic" {
			return nil, fmt.Errorf("invalid episode identity/provenance")
		}
		ids[e.ID] = true
		keys := map[string]bool{}
		for _, t := range e.Initial.Tasks {
			if t.Key == "" || keys[t.Key] || t.Goal == "" || len(t.CompletionCriteria) == 0 {
				return nil, fmt.Errorf("invalid task fixture")
			}
			if t.ParentTaskKey!=""&&!keys[t.ParentTaskKey]{return nil,fmt.Errorf("parent task must precede its child")}
			keys[t.Key] = true
		}
		if len(keys) == 0 {
			return nil, fmt.Errorf("missing task fixture")
		}
		resources := map[string]bool{}
		for _, v := range e.Initial.Resources {
			if v.Key == "" || resources[v.Key] || !keys[v.TaskKey] || v.Version < 1 || strings.ContainsAny(v.Key, "/\\?#") {
				return nil, fmt.Errorf("invalid resource fixture")
			}
			resources[v.Key] = true
		}
		count := 0
		steps:=map[string]bool{}
		sources:=map[string]bool{}
		for _,v:=range e.Initial.Profiles{if v.SourceKey==""||sources[v.SourceKey]{return nil,fmt.Errorf("duplicate/missing source")};sources[v.SourceKey]=true}
		for _,v:=range e.Initial.Documents{if v.SourceKey==""||sources[v.SourceKey]{return nil,fmt.Errorf("duplicate/missing document source")};sources[v.SourceKey]=true}
		for _, s := range e.Steps {
			if s.ID == ""||steps[s.ID]||strings.ContainsAny(s.ID,"/\\")||strings.Contains(s.ID,"..") {
				return nil, fmt.Errorf("missing step identity")
			}
			steps[s.ID]=true
			if s.Evaluate {
				count++
			}
			if s.RunTaskKey != nil && !keys[*s.RunTaskKey] {
				return nil, fmt.Errorf("unknown task target")
			}
			switch s.Event.Type {
			case "owner_message":if s.Event.TargetTaskKey!=nil&&!keys[*s.Event.TargetTaskKey]{return nil,fmt.Errorf("unknown message target")}
			case "owner_control":if !keys[s.Event.TaskKey]{return nil,fmt.Errorf("unknown control target")}
			case "manual_resource_change":if !resources[s.Event.ResourceKey]{return nil,fmt.Errorf("unknown manual resource")}
			case "profile_change", "document_change":if !sources[s.Event.SourceKey]{return nil,fmt.Errorf("unknown lifecycle source")}
			case "service_restart":
			default:
				return nil, fmt.Errorf("unsupported tagged event")
			}
		}
		if count < 2 || count > 3 {
			return nil, fmt.Errorf("two or three evaluated turns required")
		}
		episodes = append(episodes, e)
	}
	return episodes, scanner.Err()
}
