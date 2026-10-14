package acceptance

import (
	"bytes"
	"encoding/json"
	"errors"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type memoryAuditSourceRef struct {
	ID       string `json:"source_id"`
	Revision int64  `json:"revision"`
	Access   int64  `json:"access_revision"`
	Hash     string `json:"content_hash"`
}

func memoryAuditCompactSHA(raw []byte) string {
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		return ""
	}
	return hashWindowBytes(b.Bytes())
}

func memoryAuditProvenance(s MemorySuite, f MemoryFrozen, ledger map[string]json.RawMessage, directory string) error {
	var sources []struct {
		ID       string `json:"source_id"`
		Owner    string `json:"owner_id"`
		External string `json:"external_key"`
		Status   string `json:"status"`
		Revision int64  `json:"head_revision"`
		Access   int64  `json:"access_revision"`
	}
	var originals []struct {
		ID       string    `json:"source_id"`
		Revision int64     `json:"revision"`
		Hash     string    `json:"content_hash"`
		Body     string    `json:"body"`
		Event    time.Time `json:"event_time"`
		Origin   struct {
			Sensitivity string `json:"sensitivity"`
		} `json:"origin"`
	}
	var bindings []struct {
		ID       int64  `json:"object_id"`
		Source   string `json:"source_id"`
		Revision int64  `json:"source_revision"`
		Type     string `json:"memory_type"`
	}
	var tasks []struct {
		ID                  string   `json:"task_id"`
		Owner               string   `json:"owner_id"`
		Revision            int64    `json:"revision"`
		ConstraintsRevision int64    `json:"constraints_revision"`
		Constraints         []string `json:"constraints_json"`
		Status              string   `json:"status"`
		Goal                string   `json:"goal"`
	}
	var runs []struct {
		ID       string    `json:"run_id"`
		Session  string    `json:"session_key"`
		Task     string    `json:"task_id"`
		Revision int64     `json:"task_revision"`
		State    string    `json:"current_state"`
		Finished time.Time `json:"finished_at"`
	}
	var leases []struct {
		Session string    `json:"session_key"`
		Run     string    `json:"run_id"`
		Lease   string    `json:"lease_id"`
		Epoch   int64     `json:"fence_epoch"`
		Expires time.Time `json:"expires_at"`
	}
	var sessions []struct {
		ID    string `json:"session_key"`
		Owner string `json:"user_id"`
	}
	var guards []struct {
		Run         string          `json:"run_id"`
		Case        string          `json:"case_id"`
		Arm         string          `json:"arm"`
		Operation   string          `json:"operation_id"`
		Phase       string          `json:"phase"`
		Checked     time.Time       `json:"checked_at"`
		Revision    int64           `json:"task_revision"`
		Constraints int64           `json:"constraints_revision"`
		Lease       string          `json:"lease_id"`
		Expires     time.Time       `json:"lease_expires"`
		Sources     json.RawMessage `json:"sources"`
	}
	if json.Unmarshal(ledger["fixture_memory_eval_guards"], &guards) != nil {
		return errors.New("immutable owning guard receipts absent")
	}
	var attempts []struct {
		Run       string `json:"run_id"`
		Operation string `json:"operation_id"`
	}
	if json.Unmarshal(ledger["fixture_memory_eval_attempts"], &attempts) != nil {
		return errors.New("guard owning attempts missing")
	}
	operations := map[string]string{}
	for _, a := range attempts {
		if a.Operation == "" || operations[a.Run] != "" {
			return errors.New("guard operation identity missing/duplicate")
		}
		operations[a.Run] = a.Operation
	}
	for _, x := range []struct {
		key string
		out any
	}{{"memory_sources", &sources}, {"memory_source_revisions", &originals}, {"memory_object_sources", &bindings}, {"tasks", &tasks}, {"runs", &runs}, {"session_ownership", &leases}, {"sessions", &sessions}} {
		if json.Unmarshal(ledger[x.key], x.out) != nil {
			return errors.New("source/task/lease ledger missing")
		}
	}
	load := func(name string, out any) error {
		raw, err := ReadPrivateFile(filepath.Join(directory, name), 16<<20)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, out)
	}
	for _, c := range s.Cases {
		if c.Blocked() {
			continue
		}
		id := MemoryOpaqueCaseID(c)
		owner := "memory-eval-owner:" + id
		var refs map[string]memoryAuditSourceRef
		if load("bindings-"+strings.TrimPrefix(id, "memory:")+".json", &refs) != nil {
			return errors.New("original catalog aliases missing")
		}
		for _, h := range c.History {
			if h.Status == "deleted" {
				for _, src := range sources {
					if src.Owner == owner && src.External == h.ID && src.Status != "deleted" {
						return errors.New("deleted fixture source resurrected")
					}
				}
				continue
			}
			ref, ok := refs[h.ID]
			if !ok {
				return errors.New("permitted original alias missing")
			}
			matched := false
			for _, src := range sources {
				if src.ID == ref.ID && src.Owner == owner && src.External == h.ID && src.Status == "active" && src.Revision == ref.Revision && src.Access == ref.Access {
					for _, o := range originals {
						if o.ID == ref.ID && o.Revision == ref.Revision && o.Hash == ref.Hash && o.Body == h.Body && o.Origin.Sensitivity != "secret" {
							event, _ := time.Parse(time.RFC3339, h.EventUTC)
							matched = o.Event.Equal(event)
						}
					}
				}
			}
			if !matched {
				return errors.New("catalog original/revision/hash/owner differs")
			}
		}
		var bundle memoryservice.Bundle
		bundleRaw, bundleErr := ReadPrivateFile(filepath.Join(directory, "bundle-"+strings.TrimPrefix(id, "memory:")+".json"), 16<<20)
		if bundleErr != nil || json.Unmarshal(bundleRaw, &bundle) != nil {
			return errors.New("original current bundle missing")
		}
		var chunks []struct {
			Ref      string `json:"source_ref"`
			Revision string `json:"source_revision"`
		}
		chunkRaw, _ := json.Marshal(bundle.Items["chunks"])
		if json.Unmarshal(chunkRaw, &chunks) != nil {
			return errors.New("bundle chunks absent")
		}
		var search struct {
			Hits []struct {
				Ref string `json:"SourceRef"`
			}
		}
		if load("search-"+strings.TrimPrefix(id, "memory:")+".json", &search) != nil {
			return errors.New("original hybrid search missing")
		}
		for n, ch := range chunks {
			if n >= len(search.Hits) {
				return errors.New("bundle added unselected source")
			}
			rawID, err := strconv.ParseInt(strings.TrimPrefix(search.Hits[n].Ref, "memory_chunk:"), 10, 64)
			if err != nil {
				return errors.New("invalid raw chunk binding")
			}
			ref, ok := refs[ch.Ref]
			if !ok || ch.Revision != "1" {
				return errors.New("logical original revision differs")
			}
			matched := false
			for _, b := range bindings {
				if b.ID == rawID && b.Source == ref.ID && b.Revision == ref.Revision && b.Type == "chunk" {
					matched = true
				}
			}
			if !matched {
				return errors.New("retrieved chunk original provenance differs")
			}
		}
		selected := []string{}
		for _, ch := range chunks {
			seen := false
			for _, old := range selected {
				if old == ch.Ref {
					seen = true
				}
			}
			if !seen {
				selected = append(selected, ch.Ref)
			}
		}
		matchedCapture := false
		for _, cap := range f.Captures {
			if cap.CaseID == id {
				// Compact the original serialization: decoding Items into any can
				// reorder nested struct fields and change an otherwise identical hash.
				if cap.Prompt != bundle.Prompt || cap.EvidenceSHA256 != memoryAuditCompactSHA(bundleRaw) || MemoryDigest(cap.SourceIDs) != MemoryDigest(selected) {
					return errors.New("actual bundle prompt/digest/source selection differs")
				}
				matchedCapture = true
			}
		}
		if !matchedCapture {
			return errors.New("original capture absent")
		}
	}
	cases := map[string]MemoryCase{}
	for _, c := range s.Cases {
		cases[MemoryOpaqueCaseID(c)] = c
	}
	for _, i := range f.Inputs {
		if i.SkipReason != "" {
			continue
		}
		c := cases[i.CaseID]
		runID := "memory-run-" + MemoryDigest([]string{f.Plan.WindowID, i.CaseID, i.Arm})[:32]
		revision := max(int64(1), c.Task.Revision)
		owner := "memory-eval-owner:" + i.CaseID
		taskID := i.CaseID + ":task"
		matchedRun, matchedTask, matchedLease, matchedSession := false, false, false, false
		var finished time.Time
		for _, r := range runs {
			if r.ID == runID && r.Session == runID+":session" && r.Task == taskID && r.Revision == revision && r.State == "completed" && !r.Finished.IsZero() {
				matchedRun = true
				finished = r.Finished
			}
		}
		for _, t := range tasks {
			goal := c.Task.Goal
			if goal == "" {
				goal = c.Question
			}
			if t.ID == taskID && t.Owner == owner && t.Revision == revision && t.ConstraintsRevision == revision && len(t.Constraints) == 1 && t.Constraints[0] == i.Task.Instruction && t.Status == "active" && t.Goal == goal {
				matchedTask = true
			}
		}
		for _, l := range leases {
			if l.Run == runID && l.Session == runID+":session" && l.Lease == runID && l.Epoch == 1 && l.Expires.After(finished) {
				matchedLease = true
			}
		}
		for _, session := range sessions {
			if session.ID == runID+":session" && session.Owner == owner {
				matchedSession = true
			}
		}
		if !matchedRun || !matchedTask || !matchedLease || !matchedSession {
			return errors.New("owning run/task/current constraints/lease proof differs")
		}
		dispatch, final := 0, 0
		var refs map[string]memoryAuditSourceRef
		if load("bindings-"+strings.TrimPrefix(i.CaseID, "memory:")+".json", &refs) != nil {
			return errors.New("guard source aliases absent")
		}
		expectedRefs := []memoryAuditSourceRef{}
		for _, h := range i.Memory {
			ref, ok := refs[h.ID]
			if !ok {
				return errors.New("guard frozen source unavailable")
			}
			expectedRefs = append(expectedRefs, ref)
		}
		for _, g := range guards {
			if g.Run == runID {
				var actualRefs []memoryAuditSourceRef
				if json.Unmarshal(g.Sources, &actualRefs) != nil || g.Operation != operations[runID] || MemoryDigest(actualRefs) != MemoryDigest(expectedRefs) {
					return errors.New("guard operation/current source refs differ")
				}
				if g.Case != i.CaseID || g.Arm != i.Arm || g.Revision != revision || g.Constraints != revision || g.Lease != runID || !g.Expires.After(g.Checked) || g.Checked.After(finished) {
					return errors.New("owning guard time/revision differs")
				}
				if g.Phase == "dispatch" {
					dispatch++
				}
				if g.Phase == "final" {
					final++
				}
			}
		}
		if dispatch != 2 || final != 1 {
			return errors.New("dispatch/final guard receipt coverage differs")
		}
	}
	return nil
}
