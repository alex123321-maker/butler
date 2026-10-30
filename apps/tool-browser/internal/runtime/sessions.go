package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/butler/butler/internal/browserpage"
	"sync"
	"sync/atomic"
	"time"
)

const sessionTTL = 15 * time.Minute
const maxBrowserSessions = 32

type browserSession struct {
	managedPages                  bool
	pages                         map[string]browserpage.Report
	mu                            sync.Mutex
	id, owner, origin, currentURL string
	expires                       time.Time
	version                       uint64
	generation                    uint64
	closed                        bool
	networkFrozen                 bool
	privateFormMetadata           bool
	controlEpoch                  int64
	controller                    string
	allowedDomains                []string
}

// These are transient execution capabilities, not durable Butler identities.
type sessionStore struct {
	mu         sync.Mutex
	sessions   map[string]*browserSession
	generation atomic.Uint64
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make(map[string]*browserSession)}
}
func (s *sessionStore) open(owner, rawURL string) (*browserSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, session := range s.sessions {
		if !time.Now().Before(session.expires) || session.generation != s.generation.Load() {
			delete(s.sessions, id)
		}
	}
	if len(s.sessions) >= maxBrowserSessions {
		return nil, fmt.Errorf("browser session limit reached; close unused sessions")
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, fmt.Errorf("generate browser session identifier")
	}
	session := &browserSession{id: hex.EncodeToString(id), owner: owner, origin: originOf(rawURL), currentURL: rawURL, expires: time.Now().Add(sessionTTL), generation: s.generation.Load()}
	s.sessions[session.id] = session
	return session, nil
}
func (s *sessionStore) acquire(id, owner string) (*browserSession, string) {
	s.mu.Lock()
	session := s.sessions[id]
	s.mu.Unlock()
	if session == nil || session.generation != s.generation.Load() {
		return nil, "session_lost"
	}
	if session.owner != owner {
		return nil, "owner_mismatch"
	}
	if !session.mu.TryLock() {
		return nil, "session_busy"
	}
	return session, ""
}
func (s *sessionStore) invalidateAll()   { s.generation.Add(1) }
func (s *sessionStore) remove(id string) { s.mu.Lock(); delete(s.sessions, id); s.mu.Unlock() }
