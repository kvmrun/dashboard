package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Session is a single logged-in user's state.
type Session struct {
	Username string
	Expires  time.Time
}

// SessionStore is an in-memory session store. Sessions live only for as
// long as the dashboard process runs (restart = everyone logs in again),
// which is consistent with the no-database design of this project.
type SessionStore struct {
	mu       sync.Mutex
	ttl      time.Duration
	sessions map[string]*Session
}

// NewSessionStore returns a store whose sessions expire after ttl. It
// starts a background goroutine that prunes expired sessions.
func NewSessionStore(ttl time.Duration) *SessionStore {
	s := &SessionStore{
		ttl:      ttl,
		sessions: make(map[string]*Session),
	}
	go s.pruneLoop()
	return s
}

// Create starts a new session for username and returns its ID.
func (s *SessionStore) Create(username string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	s.sessions[id] = &Session{Username: username, Expires: time.Now().Add(s.ttl)}
	return id, nil
}

// Get returns the session for id if it exists and has not expired.
func (s *SessionStore) Get(id string) (*Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[id]
	if !ok || time.Now().After(sess.Expires) {
		delete(s.sessions, id)
		return nil, false
	}
	return sess, true
}

// Delete removes a session (logout).
func (s *SessionStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func (s *SessionStore) pruneLocked() {
	now := time.Now()
	for id, sess := range s.sessions {
		if now.After(sess.Expires) {
			delete(s.sessions, id)
		}
	}
}

func (s *SessionStore) pruneLoop() {
	ticker := time.NewTicker(s.ttl / 2)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		s.pruneLocked()
		s.mu.Unlock()
	}
}
