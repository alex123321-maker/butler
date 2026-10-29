package utilitytrace

import (
	"encoding/json"
	"errors"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/skills"
)

// SkillBinding is the collector-owned exact reviewed record frozen before run.
type SkillBinding struct {
	Digest string
	Body   catalog.SourceRef
}

// VerifySkillLoad corroborates a durable ref-only observation against the exact
// current source body and Broker's original normalized-result evidence. It returns
// reconstructed DATA only for digest verification; it never repairs a model final.
func VerifySkillLoad(s Snapshot, b Binding, callID string) (json.RawMessage, error) {
	fail := func() (json.RawMessage, error) { return nil, errors.New("skill load evidence mismatch") }
	calls, e := rows[callRow](s, "calls")
	if e != nil {
		return fail()
	}
	var call *callRow
	for i := range calls {
		if calls[i].ID == callID {
			if call != nil {
				return fail()
			}
			call = &calls[i]
		}
	}
	if call == nil || call.Run != s.RunID || call.Name != "skills.load" || call.Status != "completed" || call.Finished == nil || b.Arm != "C" {
		return fail()
	}
	var args struct {
		Digest   string `json:"digest"`
		MaxBytes int    `json:"max_bytes"`
	}
	if skills.Decode(call.Args, &args) != nil {
		return fail()
	}
	var binding *SkillBinding
	for i := range b.Skills {
		if b.Skills[i].Digest == args.Digest {
			if binding != nil {
				return fail()
			}
			binding = &b.Skills[i]
		}
	}
	if binding == nil {
		return fail()
	}
	var durable struct {
		Digest    string   `json:"digest"`
		Digests   []string `json:"skill_digests"`
		Authority bool     `json:"instruction_authority"`
		Reload    bool     `json:"reload_current_data"`
	}
	if acceptance.DecodeStrict(call.Result, &durable, "digest", "skill_digests", "instruction_authority", "reload_current_data") != nil || durable.Digest != args.Digest || durable.Authority || !durable.Reload || len(durable.Digests) != 0 {
		return fail()
	}
	usages, e := rows[skills.Usage](s, "skill_usages")
	if e != nil {
		return fail()
	}
	n := 0
	for _, u := range usages {
		if u.Digest == args.Digest && u.RunID == s.RunID && u.TaskID == b.TaskID && u.TaskRevision == b.TaskRevision && u.ConstraintsRevision == b.ConstraintsRevision && !u.LoadedAt.IsZero() {
			n++
		}
	}
	if n != 1 {
		return fail()
	}
	var heads []struct {
		Digest       string            `json:"digest"`
		Owner        string            `json:"owner_id"`
		Source       catalog.SourceRef `json:"source_ref"`
		Status       string            `json:"status"`
		Active       bool              `json:"active"`
		Body         string            `json:"body"`
		Revision     int64             `json:"revision"`
		Access       int64             `json:"access_revision"`
		Hash         string            `json:"content_hash"`
		SourceStatus string            `json:"source_status"`
	}
	raw, ok := s.Tables["skill_heads"]
	if !ok || json.Unmarshal(raw, &heads) != nil {
		return fail()
	}
	var reconstructed json.RawMessage
	for _, h := range heads {
		if h.Digest != args.Digest {
			continue
		}
		if reconstructed != nil || h.Owner != b.OwnerID || h.Status != "active" || !h.Active || h.Source != binding.Body || h.Revision != h.Source.Revision || h.Access != h.Source.AccessRevision || h.Hash != h.Source.Hash || h.SourceStatus != "active" {
			return fail()
		}
		p, err := skills.Parse([]byte(h.Body))
		if err != nil || p.Digest != args.Digest {
			return fail()
		}
		limit := args.MaxBytes
		if limit == 0 {
			limit = 8192
		}
		data, _ := json.Marshal(p.Manifest)
		if limit < 1 || limit > 16384 || len(data) > limit {
			return fail()
		}
		reconstructed, _ = json.Marshal(map[string]any{"format": "butler.skill.data.v1", "digest": p.Digest, "manifest": p.Manifest, "instruction_authority": false})
	}
	if reconstructed == nil || !s.AuthorityCurrent || !s.SourcesCurrent {
		return fail()
	}
	intents, e := rows[intentRow](s, "intents")
	if e != nil {
		return fail()
	}
	evs, e := rows[evidenceRow](s, "evidence")
	if e != nil {
		return fail()
	}
	effects, e := rows[effectRow](s, "effects")
	if e != nil {
		return fail()
	}
	dispatches, e := rows[dispatchRow](s, "dispatches")
	if e != nil {
		return fail()
	}
	ah, e := actionauth.ArgumentsHash(string(call.Args))
	if e != nil {
		return fail()
	}
	nd, e := budget.NormalizeResult(string(reconstructed))
	if e != nil {
		return fail()
	}
	matched := 0
	for _, a := range intents {
		if a.Call != call.ID {
			continue
		}
		v := a.Binding
		if a.Run != s.RunID || v.ActionID != a.ID || v.RunID != s.RunID || v.ToolCallID != call.ID || v.ToolName != call.Name || v.TaskID != b.TaskID || v.TaskRevision != b.TaskRevision || v.ConstraintsRevision != b.ConstraintsRevision || v.OwnerID != b.OwnerID || v.ArgsHash != ah {
			return fail()
		}
		count := 0
		for _, ev := range evs {
			x := ev.Evidence
			if ev.Action == a.ID && ev.Effect == a.Effect && x.SourceActionID == a.ID && x.SourceRunID == s.RunID && x.SourceToolCallID == call.ID && x.Source == "runtime_response" && string(x.Verdict) == "effect_confirmed" && x.NormalizedResultDigest == nd {
				count++
			}
		}
		if count != 1 {
			return fail()
		}
		count = 0
		for _, ef := range effects {
			if ef.ID == a.Effect && ef.State == "effect_confirmed" {
				count++
			}
		}
		if count != 1 {
			return fail()
		}
		count = 0
		for _, d := range dispatches {
			if d.Action == a.ID && d.Effect == a.Effect && d.Finished != nil && d.Outcome == "dispatch_completed" {
				count++
			}
		}
		if count != 1 {
			return fail()
		}
		matched++
	}
	if matched != 1 {
		return fail()
	}
	return reconstructed, nil
}
