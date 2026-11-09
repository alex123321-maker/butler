package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/butler/butler/internal/browserspa"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/schedulecontrol"
	"github.com/butler/butler/internal/skills"
	"github.com/butler/butler/internal/toolcatalog"
	"github.com/butler/butler/internal/workspace"
)

// Detailed evidence stays in the private archive; reports summarize it.
type call struct {
	Name, Status, Effect string
	Args, Result         json.RawMessage
}

func field(raw json.RawMessage, key string) json.RawMessage {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return nil
	}
	return obj[key]
}
func str(raw json.RawMessage, key string) string {
	var v string
	if json.Unmarshal(field(raw, key), &v) != nil {
		return ""
	}
	return v
}
func exact(a, b string) bool { return b != "" && strings.TrimSpace(a) == b }
func matches(raw json.RawMessage, expected string) bool {
	var values []string
	return json.Unmarshal(field(raw, "matches"), &values) == nil && slices.Equal(values, []string{expected})
}

func grade(s Step, answer string, calls []call) string {
	for _, c := range calls {
		if !toolcatalog.IsTool(c.Name) && !slices.Contains(s.Allowed, c.Name) {
			return "unexpected_tool"
		}
		if c.Effect == "effect_unknown" || c.Effect == "executing" {
			return "unresolved_effect"
		}
	}
	// Punctuation on the intermediate acknowledgement is not the task result.
	if s.Rule == "" && s.Answer == "ПРИНЯТО" {
		answer = strings.TrimSuffix(strings.TrimSpace(answer), ".")
	}
	if s.Answer != "" && !exact(answer, s.Answer) {
		return "wrong_answer"
	}
	switch s.Rule {
	case "":
		if len(calls) != 0 {
			return "unexpected_tool"
		}
		return "pass"
	case "catalog", "schema":
		refs := map[string]bool{}
		for _, c := range calls {
			if c.Status != "completed" {
				continue
			}
			var page toolcatalog.Result
			if json.Unmarshal(c.Result, &page) != nil || page.Status != "ok" {
				continue
			}
			if c.Name == toolcatalog.Search {
				for _, card := range page.Items {
					hash, ok := strings.CutPrefix(card.Ref, s.Target+"@")
					if card.Name == s.Target && ok && shaRE.MatchString(hash) {
						if s.Rule == "catalog" {
							return "pass"
						}
						refs[card.Ref] = true
					}
				}
			} else if c.Name == toolcatalog.Load && s.Rule == "schema" {
				args, err := toolcatalog.Decode(toolcatalog.Load, string(c.Args))
				if err != nil {
					continue
				}
				for _, ref := range args.Refs {
					if refs[ref] && slices.Contains(page.Loaded, ref) {
						return "pass"
					}
				}
			}
		}
	case "original":
		if verifiedOriginal(s, calls) {
			return "pass"
		}
	case "site_read":
		if observedSiteRead(s, calls) {
			return "pass"
		}

	case "skill":
		found := false
		for _, c := range calls {
			if c.Status != "completed" {
				continue
			}
			if c.Name == "skills.search" {
				var digests []string
				if json.Unmarshal(field(c.Result, "skill_digests"), &digests) == nil && slices.Contains(digests, s.Witness) {
					found = true
				}
				var descriptors []skills.Descriptor
				if json.Unmarshal(field(c.Result, "skills"), &descriptors) == nil {
					for _, d := range descriptors {
						if d.Digest == s.Witness {
							found = true
						}
					}
				}
			}
			if found && c.Name == "skills.load" && str(c.Args, "digest") == s.Witness && str(c.Result, "digest") == s.Witness {
				manifest := field(c.Result, "manifest")
				// Core keeps only the immutable digest when it journals a skill.
				if str(manifest, "version") == s.Answer || (manifest == nil && string(field(c.Result, "reload_current_data")) == "true") {
					return "pass"
				}
			}
		}
	case "workspace":
		if workspaceRoundtrip(s.Witness, calls) {
			return "pass"
		}
	case "schedule":
		if scheduleLifecycle(s.Witness, calls) {
			return "pass"
		}
	case "navigate":
		if navigated(s, calls) {
			return "pass"
		}
	case "two_parses":
		first, second := false, false
		for _, c := range calls {
			if c.Name != "http.parse_html" || c.Status != "completed" {
				continue
			}
			html := str(c.Args, "content")
			left := "<i>" + s.Target + "</i><b class='active'>" + s.Witness + "</b>"
			right := "<b class='active'>" + s.Witness + "</b><s>" + s.Target + "</s>"
			if strings.Contains(html, left) && strings.Contains(html, right) {
				var result []string
				if json.Unmarshal(field(c.Result, "matches"), &result) == nil && slices.Equal(result, []string{s.Witness, s.Witness}) && str(c.Args, "selector") == "b.active" {
					return "pass"
				}
			}
			if matches(c.Result, s.Witness) {
				first = first || html == left
				second = second || html == right
			}
		}
		if first && second {
			return "pass"
		}
	default:
		for _, c := range calls {
			if c.Status != "completed" || !slices.Contains(s.Allowed, c.Name) {
				continue
			}
			switch s.Rule {
			case "read":
				if s.Target == "matches" && matches(c.Result, s.Witness) && strings.Contains(str(c.Args, "content"), s.Witness) && str(c.Args, "selector") == "#current" {
					return "pass"
				}
				if s.Target == "content_text" && strings.Contains(str(c.Args, "html"), "<script>") && strings.Contains(str(c.Result, s.Target), s.Witness) {
					return "pass"
				}
			case "field":
				if exact(answer, str(c.Result, s.Target)) {
					return "pass"
				}
			case "count":
				// An empty schedule page is omitted by the canonical Receipt.
				// Broker confirms the operation, and schedule.list is the sole source.
				var list schedulecontrol.Receipt
				var args schedulecontrol.Args
				if json.Unmarshal(c.Args, &args) == nil && args.AfterID == "" && args.Limit > 0 && args.Limit <= 5 && c.Effect == "effect_confirmed" && string(c.Result) != "null" && json.Unmarshal(c.Result, &list) == nil && exact(answer, strconv.Itoa(len(list.Schedules))) {
					return "pass"
				}
			case "spa":
				args, err := browserspa.Parse(string(c.Args))
				if err != nil || args.URL != s.Target || args.Filter == nil || args.Filter.Value != s.Witness {
					continue
				}
				_, err = browserspa.ReadResultURL(string(c.Result), args)
				if err == nil && c.Effect == "effect_confirmed" && strings.Contains(str(c.Result, "text"), s.Answer) {
					return "pass"
				}
			}
		}
	}
	return "missing_tool_evidence"
}

func navigated(s Step, calls []call) bool {
	session := ""
	submitted, followed := false, false
	for _, c := range calls {
		if c.Status != "completed" {
			continue
		}
		if c.Name == "browser.open" && str(c.Args, "url") == s.Target+"/catalog" {
			session = str(c.Result, "browser_session_id")
			submitted = false
			followed = false
			continue
		}
		if session == "" || str(c.Args, "browser_session_id") != session {
			continue
		}
		if c.Name == "browser.submit_get_form" {
			submitted = true
		}
		if submitted && c.Name == "browser.follow_link" {
			followed = true
		}
		if followed && (c.Name == "browser.follow_link" || c.Name == "browser.snapshot" || c.Name == "browser.extract_text") && strings.Contains(str(c.Result, "text"), s.Witness) {
			return true
		}
	}
	return false
}

func workspaceRoundtrip(value string, calls []call) bool {
	id := ""
	var saved workspace.Snapshot
	digest := sha256.Sum256([]byte(value))
	for _, c := range calls {
		switch c.Name {
		case "workspace.create":
			if id != "" || c.Status != "completed" || c.Effect != "effect_confirmed" {
				return false
			}
			id = str(c.Result, "workspace_id")
		case "workspace.write":
			if id == "" || str(c.Args, "workspace_id") != id || str(c.Result, "workspace_id") != id || str(c.Args, "path") != "bench.txt" || str(c.Args, "content") != value || c.Status != "completed" || c.Effect != "effect_confirmed" || saved.Version != 0 {
				return false
			}
			if json.Unmarshal(field(c.Result, "snapshot"), &saved) != nil || saved.Path != "bench.txt" || saved.Version < 1 || saved.SHA256 != hex.EncodeToString(digest[:]) || saved.Bytes != int64(len(value)) {
				return false
			}
		case "workspace.read":
			if saved.Version == 0 {
				continue
			}
			var got workspace.Snapshot
			if str(c.Args, "workspace_id") == id && str(c.Args, "path") == "bench.txt" && str(c.Result, "workspace_id") == id && c.Status == "completed" && str(c.Result, "content") == value && json.Unmarshal(field(c.Result, "snapshot"), &got) == nil && got == saved {
				// Refuse extra writes/creates even after a valid readback.
				return countCalls(calls, "workspace.create") == 1 && countCalls(calls, "workspace.write") == 1
			}
		}
	}
	return false
}

func countCalls(calls []call, name string) int {
	n := 0
	for _, c := range calls {
		if c.Name == name {
			n++
		}
	}
	return n
}

func verifiedOriginal(s Step, calls []call) bool {
	seen := map[readcontrol.SourceRef]bool{}
	for _, c := range calls {
		if c.Status != "completed" {
			continue
		}
		var result readcontrol.Result
		if json.Unmarshal(c.Result, &result) != nil {
			continue
		}
		for _, item := range result.Items {
			ref := item.Source
			if ref.ID != s.Target || ref.Revision < 1 || ref.AccessRevision < 1 || !shaRE.MatchString(ref.Hash) || item.Redacted {
				continue
			}
			if c.Name == s.Allowed[0] {
				seen[ref] = true
				continue
			}
			if c.Name == "memory.open" && seen[ref] && strings.Contains(item.Text, s.Witness) {
				var a readcontrol.Args
				if json.Unmarshal(c.Args, &a) == nil && a.SourceID == ref.ID && a.Revision == ref.Revision && a.AccessRevision == ref.AccessRevision && a.Hash == ref.Hash {
					return true
				}
			}
		}
	}
	return false
}
func scheduleLifecycle(title string, calls []call) bool {
	if countCalls(calls, "schedule.create") != 1 || countCalls(calls, "schedule.cancel") != 1 {
		return false
	}
	id := ""
	var revision int64
	inspected, cancelled := false, false
	for _, c := range calls {
		if c.Status != "completed" {
			continue
		}
		var r schedulecontrol.Receipt
		if json.Unmarshal(c.Result, &r) != nil || r.Schedule == nil {
			continue
		}
		job := r.Schedule
		switch c.Name {
		case "schedule.create":
			var args schedulecontrol.Args
			if json.Unmarshal(c.Args, &args) != nil || args.Job == nil || !args.Job.Paused || args.Job.Mode != "reminder" || args.Job.Title != title || args.Job.MaxOccurrences != 1 || args.Job.Timing.Kind != "once" || args.Job.Timing.TimeZone != "UTC" || args.Job.Timing.AfterSeconds != 86400 {
				return false
			}
			if c.Effect != "effect_confirmed" || job.Title != title || job.State != "paused" || job.ID == "" || job.Revision < 1 {
				return false
			}
			id, revision = job.ID, job.Revision
		case "schedule.get":
			if id == "" || job.ID != id || str(c.Args, "schedule_id") != id {
				continue
			}
			if !cancelled && job.Revision == revision && job.State == "paused" {
				inspected = true
			}
			if cancelled && job.Revision > revision && job.State == "cancelled" {
				return true
			}
		case "schedule.cancel":
			var a schedulecontrol.Args
			if !inspected || c.Effect != "effect_confirmed" || json.Unmarshal(c.Args, &a) != nil || a.ScheduleID != id || a.ExpectedRevision != revision || job.ID != id || job.State != "cancelled" {
				return false
			}
			cancelled = true
		}
	}
	return false
}

// Verify the recipient's value in a completed read of the requested URL.
// Inline extracts, writes and an unrelated tab cannot establish provenance.
func observedSiteRead(s Step, calls []call) bool {
	found := false
	sessions := map[string]bool{}
	for _, c := range calls {
		if c.Name == "http.request" {
			method := strings.ToUpper(str(c.Args, "method"))
			if (method != "" && method != "GET" && method != "HEAD") || str(c.Args, "body") != "" {
				return false
			}
		}
		if c.Status != "completed" {
			continue
		}
		url := str(c.Args, "url")
		switch c.Name {
		case "web.fetch_batch":
			var urls []string
			var items []json.RawMessage
			if json.Unmarshal(field(c.Args, "urls"), &urls) != nil || !slices.Equal(urls, []string{s.Target}) || json.Unmarshal(field(c.Result, "items"), &items) != nil {
				continue
			}
			for _, item := range items {
				found = found || str(item, "requested_url") == s.Target && strings.Contains(str(item, "content_text"), s.Witness)
			}
		case "browser.open", "browser.read":
			if url != s.Target {
				continue
			}
			found = found || strings.Contains(str(c.Result, "text"), s.Witness)
			if id := str(c.Result, "browser_session_id"); c.Name == "browser.open" && id != "" {
				sessions[id] = true
			}
		case "browser.snapshot", "browser.extract_text":
			found = found || sessions[str(c.Args, "browser_session_id")] && strings.Contains(str(c.Result, "text"), s.Witness)
		case "browser.close":
			delete(sessions, str(c.Args, "browser_session_id"))
		case "web.fetch", "web.extract":
			found = found || url == s.Target && str(c.Args, "html") == "" && str(c.Args, "text") == "" && strings.Contains(str(c.Result, "content_text"), s.Witness)
		case "http.download", "http.request":
			found = found || url == s.Target && strings.Contains(str(c.Result, "body"), s.Witness)
		}
	}
	return found
}
