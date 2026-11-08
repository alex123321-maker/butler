package browserspa

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"

	"github.com/butler/butler/internal/jsonstrict"
)

const SocketProtocol = "graphql-transport-ws"

// SocketPolicy is a bounded GraphQL read-protocol state machine, never a model
// assertion that an arbitrary WebSocket payload is read-only. IDs cannot replay.
type SocketPolicy struct {
	mu          sync.Mutex
	init, acked bool
	seen        map[string]bool
	active      map[string]bool
	pong        []byte
}

type socketMessage struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func decodeSocket(data []byte) (socketMessage, error) {
	var m socketMessage
	if len(data) > 64*1024 || jsonstrict.Decode(data, &m) != nil || len(m.ID) > 64 || strings.ContainsAny(m.ID, "\x00\r\n\t") {
		return m, ErrDenied
	}
	return m, nil
}
func emptyPayload(p []byte) bool {
	return len(p) == 0 || bytes.Equal(bytes.TrimSpace(p), []byte("null")) || bytes.Equal(bytes.TrimSpace(p), []byte("{}"))
}
func canonicalPayload(p []byte) []byte {
	if len(p) == 0 {
		return nil
	}
	var v any
	if json.Unmarshal(p, &v) != nil {
		return nil
	}
	raw, _ := json.Marshal(v)
	return raw
}

func (s *SocketPolicy) Client(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := decodeSocket(data)
	if err != nil {
		return err
	}
	switch m.Type {
	case "connection_init":
		if s.init || m.ID != "" || !emptyPayload(m.Payload) {
			return ErrDenied
		}
		s.init = true
		s.seen = map[string]bool{}
		s.active = map[string]bool{}
	case "subscribe":
		if !s.acked || m.ID == "" || s.seen[m.ID] || len(s.seen) >= 4 || QueryBody(m.Payload) != nil && SubscriptionBody(m.Payload) != nil {
			return ErrDenied
		}
		s.seen[m.ID] = true
		s.active[m.ID] = true
	case "complete":
		if !s.acked || !s.active[m.ID] || !emptyPayload(m.Payload) {
			return ErrDenied
		}
		delete(s.active, m.ID)
	case "ping":
		if !s.init || m.ID != "" || !emptyPayload(m.Payload) {
			return ErrDenied
		}
	case "pong":
		if !s.init || m.ID != "" || !emptyPayload(m.Payload) && !bytes.Equal(canonicalPayload(m.Payload), s.pong) {
			return ErrDenied
		}
		s.pong = nil
	default:
		return ErrDenied
	}
	return nil
}
func (s *SocketPolicy) Server(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := decodeSocket(data)
	if err != nil {
		return err
	}
	switch m.Type {
	case "connection_ack":
		if !s.init || s.acked || m.ID != "" {
			return ErrDenied
		}
		s.acked = true
	case "next":
		if !s.acked || !s.active[m.ID] || len(m.Payload) == 0 {
			return ErrDenied
		}
	case "error", "complete":
		if !s.acked || !s.active[m.ID] {
			return ErrDenied
		}
		delete(s.active, m.ID)
	case "ping":
		if !s.init || m.ID != "" {
			return ErrDenied
		}
		s.pong = canonicalPayload(m.Payload)
	case "pong":
		if !s.init || m.ID != "" {
			return ErrDenied
		}
	default:
		return ErrDenied
	}
	return nil
}
