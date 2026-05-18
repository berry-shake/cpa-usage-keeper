package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type Role string

const (
	RoleAdmin        Role = "admin"
	RoleAPIKeyViewer Role = "api_key_viewer"
)

type Session struct {
	Role        Role
	CPAAPIKeyID int64
	ExpiresAt   time.Time
}

type SessionManager struct {
	ttl      time.Duration
	now      func() time.Time
	generate func() (string, error)

	mu       sync.RWMutex
	sessions map[string]Session
}

func NewSessionManager(ttl time.Duration) *SessionManager {
	return &SessionManager{
		ttl:      ttl,
		now:      time.Now,
		generate: generateToken,
		sessions: make(map[string]Session),
	}
}

func (m *SessionManager) Create() (string, time.Time, error) {
	return m.create(Session{Role: RoleAdmin})
}

func (m *SessionManager) CreateAPIKeyViewer(cpaAPIKeyID int64) (string, time.Time, error) {
	return m.create(Session{Role: RoleAPIKeyViewer, CPAAPIKeyID: cpaAPIKeyID})
}

func (m *SessionManager) create(session Session) (string, time.Time, error) {
	token, err := m.generate()
	if err != nil {
		return "", time.Time{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.cleanupExpiredLocked()
	expiresAt := m.now().Add(m.ttl)
	session.ExpiresAt = expiresAt
	m.sessions[token] = session

	return token, expiresAt, nil
}

func (m *SessionManager) Validate(token string) bool {
	_, ok := m.Get(token)
	return ok
}

func (m *SessionManager) Get(token string) (Session, bool) {
	if token == "" {
		return Session{}, false
	}

	m.mu.RLock()
	session, ok := m.sessions[token]
	m.mu.RUnlock()
	if !ok {
		return Session{}, false
	}
	if !session.ExpiresAt.After(m.now()) {
		m.Delete(token)
		return Session{}, false
	}
	return session, true
}

func (m *SessionManager) Delete(token string) {
	if token == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
}

func (m *SessionManager) CleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredLocked()
}

func (m *SessionManager) cleanupExpiredLocked() {
	now := m.now()
	for token, session := range m.sessions {
		if !session.ExpiresAt.After(now) {
			delete(m.sessions, token)
		}
	}
}

func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
