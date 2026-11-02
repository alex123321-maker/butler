package browserpage

import (
	"encoding/json"
	"time"
)

// Observation contains only the identity of an already mediated page result.
// It is neither an external receipt nor permission to select another page.
type Observation struct {
	OK        bool      `json:"ok"`
	SessionID string    `json:"browser_session_id"`
	Page      *Ref      `json:"page"`
	URL       string    `json:"final_url"`
	Network   string    `json:"network_state"`
	Expires   time.Time `json:"expires_at"`
}

func ReadObservation(raw string) (Observation, error) {
	var o Observation
	if !uniqueJSON([]byte(raw)) || json.Unmarshal([]byte(raw), &o) != nil || !o.OK || !ID(o.SessionID) || o.Page == nil || !(Binding{Ref: *o.Page, URL: o.URL, RuntimeVersion: 1}).Valid(true) {
		return Observation{}, ErrDenied
	}
	return o, nil
}

func OutcomeGesture(tool string) bool { return tool == Click || tool == Fill || tool == Type }
