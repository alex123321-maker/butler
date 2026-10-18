// Package mobile owns authenticated Android observation ingress. It has no
// dependency on model transport, tools, tasks or initiative admission.
package mobile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

var ErrInvalid = errors.New("invalid_mobile_request")
var ErrDenied = errors.New("device_unauthorized")
var ErrLimit = errors.New("rate_limited")
var ErrConflict = errors.New("pairing_conflict")
var ErrPaused = errors.New("device_paused")

const MaxBatch = 50
const MaxBody = 256 * 1024

type Message struct {
	Sender    *string `json:"sender"`
	Text      string  `json:"text"`
	Timestamp int64   `json:"timestamp_epoch_ms"`
}
type Progress struct {
	Current       int  `json:"current"`
	Max           int  `json:"max"`
	Indeterminate bool `json:"indeterminate"`
}
type Payload struct {
	Title        *string   `json:"title"`
	Text         *string   `json:"text"`
	BigText      *string   `json:"big_text"`
	SubText      *string   `json:"sub_text"`
	Summary      *string   `json:"summary_text"`
	Info         *string   `json:"info_text"`
	Conversation *string   `json:"conversation_title"`
	Messages     []Message `json:"messages"`
	Progress     *Progress `json:"progress"`
}
type Event struct {
	Schema       int      `json:"schema_version"`
	ID           string   `json:"device_event_id"`
	Key          string   `json:"notification_key"`
	Previous     *string  `json:"previous_device_event_id"`
	Package      string   `json:"package_name"`
	Channel      *string  `json:"channel_id"`
	Change       string   `json:"change_type"`
	Initial      bool     `json:"is_initial_snapshot"`
	Posted       int64    `json:"posted_at_epoch_ms"`
	Observed     int64    `json:"observed_at_epoch_ms"`
	Category     *string  `json:"category"`
	Visibility   string   `json:"visibility"`
	GroupSummary bool     `json:"is_group_summary"`
	GroupKey     *string  `json:"group_key"`
	Ongoing      bool     `json:"is_ongoing"`
	Clearable    bool     `json:"is_clearable"`
	Redacted     bool     `json:"content_redacted_by_user_rule"`
	Omitted      bool     `json:"content_omitted_by_mode"`
	OTPRedacted  bool     `json:"system_otp_redacted"`
	Truncated    bool     `json:"is_truncated"`
	ContentHash  string   `json:"content_hash"`
	Payload      *Payload `json:"payload"`
}
type Batch struct {
	Schema int     `json:"schema_version"`
	ID     string  `json:"batch_id"`
	Sent   int64   `json:"sent_at_epoch_ms"`
	Events []Event `json:"events"`
}
type Rejected struct {
	ID     string `json:"device_event_id"`
	Code   string `json:"error_code"`
	Reason string `json:"reason"`
}
type ACK struct {
	Accepted  []string   `json:"accepted_ids"`
	Duplicate []string   `json:"duplicate_ids"`
	Conflict  []string   `json:"conflict_ids"`
	Rejected  []Rejected `json:"rejected"`
	Time      int64      `json:"server_time_epoch_ms"`
}
type PairRequest struct {
	Code    string `json:"pairing_code"`
	Device  string `json:"device_id"`
	Name    string `json:"device_name"`
	SDK     int    `json:"android_sdk"`
	Version string `json:"app_version"`
}
type PairResult struct {
	Device string    `json:"device_id"`
	Token  string    `json:"device_token"`
	Server string    `json:"server_name"`
	Paired time.Time `json:"paired_at"`
}

var identifier = regexp.MustCompile(`^[a-zA-Z0-9_-]{16,128}$`)
var hexHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func validTime(ms int64, now time.Time) bool {
	return ms >= time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli() && ms <= now.Add(5*time.Minute).UnixMilli()
}
func (e Event) Validate(now time.Time) error {
	if e.Schema != 1 || !identifier.MatchString(e.ID) || len(e.Key) == 0 || len(e.Key) > 2048 || len(e.Package) == 0 || len(e.Package) > 256 || !hexHash.MatchString(e.ContentHash) || !validTime(e.Observed, now) || !validTime(e.Posted, now) {
		return ErrInvalid
	}
	if e.Change != "POSTED" && e.Change != "UPDATED" && e.Change != "REMOVED" {
		return ErrInvalid
	}
	if e.Previous != nil && !identifier.MatchString(*e.Previous) {
		return ErrInvalid
	}
	for _, v := range []*string{e.Channel, e.Category, e.GroupKey} {
		if v != nil && len(*v) > 2048 {
			return ErrInvalid
		}
	}
	if e.Payload != nil {
		p := e.Payload
		for _, v := range []*string{p.Title, p.Text, p.BigText, p.SubText, p.Summary, p.Info, p.Conversation} {
			if v != nil && len(*v) > 16000 {
				return ErrInvalid
			}
		}
		if len(p.Messages) > 25 {
			return ErrInvalid
		}
		for _, m := range p.Messages {
			if len(m.Text) > 16000 || m.Sender != nil && len(*m.Sender) > 8000 {
				return ErrInvalid
			}
		}
	}
	b, _ := json.Marshal(e)
	if len(b) > 128*1024 {
		return ErrInvalid
	}
	return nil
}
func (b Batch) Validate(now time.Time) error {
	if b.Schema != 1 || !identifier.MatchString(b.ID) || !validTime(b.Sent, now) || len(b.Events) == 0 || len(b.Events) > MaxBatch {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, e := range b.Events {
		if seen[e.ID] || e.Validate(now) != nil {
			return ErrInvalid
		}
		seen[e.ID] = true
	}
	return nil
}
func (p PairRequest) Validate() error {
	if !identifier.MatchString(p.Device) || len(p.Name) == 0 || len(p.Name) > 128 || p.SDK < 24 || p.SDK > 100 || len(p.Version) == 0 || len(p.Version) > 64 {
		return ErrInvalid
	}
	return nil
}
