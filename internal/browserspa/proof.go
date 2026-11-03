package browserspa

import (
	"encoding/json"

	"github.com/butler/butler/internal/jsonstrict"
)

type NetworkStats struct {
	SentMessages     int   `json:"sent_messages"`
	Requests         int   `json:"requests"`
	Queries          int   `json:"post_queries"`
	WebSockets       int   `json:"websockets"`
	Messages         int   `json:"received_messages"`
	PendingQueries   int   `json:"pending_queries"`
	CompletedQueries int   `json:"completed_queries"`
	MeteredBytes     int64 `json:"metered_bytes"`
}

func ReadResultURL(raw string, a Args) (string, error) {
	var all map[string]json.RawMessage
	if len(raw) > 128*1024 || jsonstrict.Decode([]byte(raw), &all) != nil {
		return "", ErrDenied
	}
	var result struct {
		OK      bool            `json:"ok"`
		URL     string          `json:"final_url"`
		Session string          `json:"browser_session_id"`
		Profile string          `json:"spa_profile"`
		Network string          `json:"network_state"`
		Stats   *NetworkStats   `json:"spa_network"`
		Nonce   string          `json:"spa_nonce"`
		Error   json.RawMessage `json:"error"`
	}
	if json.Unmarshal([]byte(raw), &result) != nil || !result.OK || result.Session != "" || result.Nonce != "" || result.Profile != Profile || result.Network != "frozen" || !NetworkURL(a.URL, result.URL) || result.Stats == nil || len(result.Error) > 0 && string(result.Error) != "null" {
		return "", ErrDenied
	}
	s := result.Stats
	if s.SentMessages < 0 || s.SentMessages > 128 {
		return "", ErrDenied
	}
	if s.Requests < 1 || s.Requests > 128 || s.Queries < 0 || s.Queries > 4 || s.WebSockets < 0 || s.WebSockets > 4 || s.Messages < 0 || s.Messages > 128 || s.PendingQueries != 0 || s.CompletedQueries != s.Queries || s.MeteredBytes < 1 || s.MeteredBytes > 4*1024*1024 {
		return "", ErrDenied
	}
	return result.URL, nil
}
