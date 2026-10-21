package transcript

import "encoding/json"

// MessageKind is orthogonal to author role and never grants execution rights.
func MessageKind(metadata string) string {
	var m struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal([]byte(metadata), &m) != nil {
		return ""
	}
	switch m.Kind {
	case "publication", "attempt_final", "owner_input", "interaction_response":
		return m.Kind
	}
	return ""
}
