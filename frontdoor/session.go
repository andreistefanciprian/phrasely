package main

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const sessionTTL = 30 * 24 * time.Hour // matches JWT expiry

type session struct {
	jwt       string
	expiresAt time.Time
}

// SessionStore holds server-side sessions keyed by a random session ID.
// The browser only ever receives the session ID as an httpOnly cookie — the
// JWT never leaves the server.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]session
}

func newSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]session)}
}

func (s *SessionStore) Create(jwt string) string {
	id := randomID()
	s.mu.Lock()
	s.sessions[id] = session{jwt: jwt, expiresAt: time.Now().Add(sessionTTL)}
	s.mu.Unlock()
	return id
}

func (s *SessionStore) Get(id string) string {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok || time.Now().After(sess.expiresAt) {
		return ""
	}
	return sess.jwt
}

func (s *SessionStore) Delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

func randomID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
