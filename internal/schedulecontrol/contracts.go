// Package schedulecontrol defines the fixed Broker-to-Orchestrator schedule
// capability. Model arguments describe work and time, never identity or rights.
package schedulecontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/scheduling"
)

const Path = "/internal/schedules"
const Target = "broker://schedules-v1"
const Revision = "2"

var ErrDenied = errors.New("schedule request unavailable, stale or outside policy")

type Timing struct {
	Kind            string    `json:"kind"`
	TimeZone        string    `json:"time_zone"`
	At              time.Time `json:"at,omitempty"`
	AfterSeconds    int       `json:"after_seconds,omitempty"`
	Hour            int       `json:"hour,omitempty"`
	Minute          int       `json:"minute,omitempty"`
	IntervalSeconds int       `json:"interval_seconds,omitempty"`
	Weekdays        []int     `json:"weekdays,omitempty"`
	MissedPolicy    string    `json:"missed_policy"`
	GraceSeconds    int       `json:"grace_seconds"`
}
type Spec struct {
	Title              string    `json:"title"`
	Mode               string    `json:"mode"`
	Instruction        string    `json:"instruction"`
	CompletionCriteria []string  `json:"completion_criteria"`
	Timing             Timing    `json:"timing"`
	ExpiresAt          time.Time `json:"expires_at"`
	MaxOccurrences     int       `json:"max_occurrences"`
	Paused             bool      `json:"paused"`
	Monitor            bool      `json:"monitor,omitempty"`
}
type Args struct {
	Job              *Spec  `json:"job,omitempty"`
	ScheduleID       string `json:"schedule_id,omitempty"`
	ExpectedRevision int64  `json:"expected_revision,omitempty"`
	Limit            int    `json:"limit,omitempty"`
	AfterID          string `json:"after_id,omitempty"`
}
type Envelope struct {
	RunID        string `json:"run_id"`
	ToolCallID   string `json:"tool_call_id"`
	EffectID     string `json:"effect_id"`
	Tool         string `json:"tool"`
	ArgsJSON     string `json:"args_json"`
	ContractHash string `json:"contract_hash"`
}
type Occurrence struct {
	ID            string    `json:"occurrence_id"`
	ScheduledAt   time.Time `json:"scheduled_at"`
	State         string    `json:"state"`
	Reason        string    `json:"reason"`
	TaskID        string    `json:"task_id,omitempty"`
	InputID       string    `json:"input_id,omitempty"`
	RunState      string    `json:"run_state,omitempty"`
	TaskStatus    string    `json:"task_status,omitempty"`
	ResultVerdict string    `json:"result_verdict,omitempty"`
	DeliveryState string    `json:"delivery_state,omitempty"`
}
type View struct {
	ID       string `json:"schedule_id"`
	Revision int64  `json:"revision"`
	Spec
	State           string       `json:"state"`
	Reason          string       `json:"reason"`
	NextAt          *time.Time   `json:"next_at"`
	OccurrenceCount int          `json:"occurrence_count"`
	History         []Occurrence `json:"history,omitempty"`
}
type Receipt struct {
	Schedule   *View  `json:"schedule,omitempty"`
	Schedules  []View `json:"schedules,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
}
type Service interface {
	ExecuteDispatched(context.Context, Envelope) (Receipt, error)
}

func IsTool(name string) bool {
	switch name {
	case "schedule.create", "schedule.list", "schedule.get", "schedule.update", "schedule.cancel":
		return true
	}
	return false
}
func ReadOnly(name string) bool { return name == "schedule.list" || name == "schedule.get" }
func Contracts() []*toolbrokerv1.ToolContract {
	timing := `{"type":"object","required":["kind","time_zone","missed_policy","grace_seconds"],"properties":{"kind":{"type":"string","enum":["once","daily","interval"]},"time_zone":{"type":"string"},"at":{"type":"string"},"after_seconds":{"type":"integer"},"hour":{"type":"integer"},"minute":{"type":"integer"},"interval_seconds":{"type":"integer"},"weekdays":{"type":"array","items":{"type":"integer"}},"missed_policy":{"type":"string","enum":["skip","coalesce"]},"grace_seconds":{"type":"integer"}},"additionalProperties":false}`
	spec := `{"type":"object","required":["title","mode","instruction","completion_criteria","timing","expires_at","max_occurrences","paused"],"properties":{"title":{"type":"string"},"mode":{"type":"string","enum":["reminder","agent_task"]},"instruction":{"type":"string"},"completion_criteria":{"type":"array","items":{"type":"string"}},"timing":` + timing + `,"expires_at":{"type":"string"},"max_occurrences":{"type":"integer"},"paused":{"type":"boolean"},"monitor":{"type":"boolean","description":"agent_task checks fresh evidence and notifies only meaningful changes; each message must carry an observation."}},"additionalProperties":false}`
	specs := []struct{ name, description, schema string }{
		{"schedule.create", "Persist a bounded reminder or future agent task. Explicit timezone and expiry are required. Reminder delivers saved text without inference. Agent task uses ordinary tools and current policy. For changing events use monitor=true, actual source checks and changes-only notification, not a timer claiming the event occurred. Create only after owner consent. A receipt proves scheduling only; confirm its actual next_at to the owner.", `{"type":"object","required":["job"],"properties":{"job":` + spec + `},"additionalProperties":false}`},
		{"schedule.list", "List schedules in the current owner's conversation, including blocked/paused states. Use next_cursor as after_id for the next page.", `{"type":"object","properties":{"limit":{"type":"integer"},"after_id":{"type":"string"}},"additionalProperties":false}`},
		{"schedule.get", "Inspect a schedule and recent occurrence, execution and delivery states. A completed run does not verify task success.", `{"type":"object","required":["schedule_id"],"properties":{"schedule_id":{"type":"string"}},"additionalProperties":false}`},
		{"schedule.update", "Replace a schedule using its current expected_revision. Set job.paused to pause or resume. Existing occurrence history and allowance usage are retained. A changed plan does not authorize effects.", `{"type":"object","required":["schedule_id","expected_revision","job"],"properties":{"schedule_id":{"type":"string"},"expected_revision":{"type":"integer"},"job":` + spec + `},"additionalProperties":false}`},
		{"schedule.cancel", "Permanently cancel future occurrences using expected_revision. Already admitted tasks retain ordinary Task controls; cancellation does not undo a sent message or external effect.", `{"type":"object","required":["schedule_id","expected_revision"],"properties":{"schedule_id":{"type":"string"},"expected_revision":{"type":"integer"}},"additionalProperties":false}`},
	}
	result := make([]*toolbrokerv1.ToolContract, 0, len(specs))
	for _, s := range specs {
		result = append(result, &toolbrokerv1.ToolContract{ToolName: s.name, Description: s.description, ToolClass: "schedule_control", InputSchemaJson: s.schema, OutputSchemaJson: `{"type":"object","properties":{"schedule":{"type":"object"},"schedules":{"type":"array","items":{"type":"object"}},"next_cursor":{"type":"string"}},"additionalProperties":false}`, RuntimeTarget: Target, Status: "active", ContractRevision: Revision, RiskLevel: "medium", EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "local_runtime_ack"}})
	}
	return result
}
func Contract(name string) *toolbrokerv1.ToolContract {
	for _, c := range Contracts() {
		if c.ToolName == name {
			return c
		}
	}
	return nil
}

// StrictJSON rejects duplicate members, trailing values, unknown fields and deep
// input before any interpretation can differ from the exact approved intent.
func StrictJSON(raw []byte, out any) error {
	if len(raw) > 128*1024 || !utf8.Valid(raw) {
		return ErrDenied
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 12 {
			return ErrDenied
		}
		token, err := d.Token()
		if err != nil {
			return ErrDenied
		}
		if token == json.Delim('{') {
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return ErrDenied
				}
				key, ok := k.(string)
				if !ok || seen[key] {
					return ErrDenied
				}
				seen[key] = true
				if walk(depth+1) != nil {
					return ErrDenied
				}
			}
			t, e := d.Token()
			if e != nil || t != json.Delim('}') {
				return ErrDenied
			}
		} else if token == json.Delim('[') {
			for d.More() {
				if walk(depth+1) != nil {
					return ErrDenied
				}
			}
			t, e := d.Token()
			if e != nil || t != json.Delim(']') {
				return ErrDenied
			}
		} else if _, ok := token.(json.Delim); ok {
			return ErrDenied
		}
		return nil
	}
	if walk(0) != nil {
		return ErrDenied
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrDenied
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return ErrDenied
	}
	return nil
}
func validID(id string) bool {
	if len(id) != 36 || !strings.HasPrefix(id, "sch_") {
		return false
	}
	for _, c := range id[4:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func DecodeArgs(name, raw string) (Args, error) {
	var a Args
	if len(raw) > 64*1024 {
		return a, ErrDenied
	}
	if !IsTool(name) || StrictJSON([]byte(raw), &a) != nil {
		return a, ErrDenied
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal([]byte(raw), &fields)
	allowed := map[string]bool{}
	switch name {
	case "schedule.create":
		allowed["job"] = true
		if a.Job == nil {
			return a, ErrDenied
		}
	case "schedule.update":
		for _, k := range []string{"schedule_id", "expected_revision", "job"} {
			allowed[k] = true
		}
		if a.Job == nil || !validID(a.ScheduleID) || a.ExpectedRevision < 1 {
			return a, ErrDenied
		}
	case "schedule.cancel":
		allowed["schedule_id"] = true
		allowed["expected_revision"] = true
		if !validID(a.ScheduleID) || a.ExpectedRevision < 1 {
			return a, ErrDenied
		}
	case "schedule.get":
		allowed["schedule_id"] = true
		if !validID(a.ScheduleID) {
			return a, ErrDenied
		}
	case "schedule.list":
		allowed["limit"] = true
		allowed["after_id"] = true
		if a.Limit < 0 || a.Limit > 10 || a.AfterID != "" && !validID(a.AfterID) {
			return a, ErrDenied
		}
	}
	for k := range fields {
		if !allowed[k] {
			return a, ErrDenied
		}
	}
	if a.Job != nil {
		var job map[string]json.RawMessage
		_ = json.Unmarshal(fields["job"], &job)
		for _, k := range []string{"title", "mode", "instruction", "completion_criteria", "timing", "expires_at", "max_occurrences", "paused"} {
			if v, ok := job[k]; !ok || bytes.Equal(v, []byte("null")) {
				return a, ErrDenied
			}
		}
		var timing map[string]json.RawMessage
		_ = json.Unmarshal(job["timing"], &timing)
		for _, k := range []string{"kind", "time_zone", "missed_policy", "grace_seconds"} {
			if _, ok := timing[k]; !ok {
				return a, ErrDenied
			}
		}
		for _, v := range timing {
			if bytes.Equal(v, []byte("null")) {
				return a, ErrDenied
			}
		}
		if a.Job.Validate() != nil {
			return a, ErrDenied
		}
	}
	return a, nil
}
func (s Spec) Validate() error {
	if strings.TrimSpace(s.Title) == "" || len(s.Title) > 256 || strings.TrimSpace(s.Instruction) == "" || len(s.Instruction) > 8192 || (s.Mode != "reminder" && s.Mode != "agent_task") || s.ExpiresAt.IsZero() || s.MaxOccurrences < 1 || s.MaxOccurrences > 100 || len(s.CompletionCriteria) > 16 || s.CompletionCriteria == nil {
		return ErrDenied
	}
	if s.Mode == "agent_task" && len(s.CompletionCriteria) == 0 {
		return ErrDenied
	}
	if s.Monitor && s.Mode != "agent_task" {
		return ErrDenied
	}
	seen := map[string]bool{}
	for _, v := range s.CompletionCriteria {
		if strings.TrimSpace(v) == "" || len(v) > 1024 || seen[v] {
			return ErrDenied
		}
		seen[v] = true
	}
	if s.Timing.Kind == "once" && s.MaxOccurrences != 1 {
		return ErrDenied
	}
	if s.Mode == "reminder" && len(s.Instruction) > 4096 {
		return ErrDenied
	}
	_, err := s.Timing.Schedule(time.Now().UTC())
	return err
}
func (t Timing) Schedule(now time.Time) (scheduling.Schedule, error) {
	if t.Kind == "interval" && (t.Hour != 0 || t.Minute != 0) {
		return scheduling.Schedule{}, ErrDenied
	}
	if t.AfterSeconds < 0 || t.AfterSeconds > 30*24*3600 || t.AfterSeconds != 0 && (t.Kind != "once" || !t.At.IsZero()) {
		return scheduling.Schedule{}, ErrDenied
	}
	at := t.At
	if t.AfterSeconds > 0 {
		at = now.Add(time.Duration(t.AfterSeconds) * time.Second)
	}
	s := scheduling.Schedule{ID: "schedule", SourceID: "job", TimeZone: t.TimeZone, Kind: t.Kind, At: at, Hour: t.Hour, Minute: t.Minute, IntervalSeconds: t.IntervalSeconds, Weekdays: t.Weekdays, MissedPolicy: t.MissedPolicy, GraceSeconds: t.GraceSeconds, Enabled: true}
	if s.Validate() != nil {
		return s, ErrDenied
	}
	return s, nil
}
