package oidc

import (
	"context"
	"errors"
	"sync"
	"time"
)

// MemStore is an in-memory Store for tests; it keeps refresh tokens in plain memory.
type MemStore struct {
	mu       sync.Mutex
	sessions map[string]*memSession
	Now      func() time.Time
}

type memSession struct {
	Session
	refresh string
}

// NewMemStore returns an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{sessions: map[string]*memSession{}, Now: time.Now}
}

// SignIn implements Store.
func (m *MemStore) SignIn(_ context.Context, id Identity, refreshToken string, expires time.Time) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.Now()
	sid := random()
	m.sessions[sid] = &memSession{
		Session: Session{ID: sid, Sub: id.Sub, Name: id.Name, CreatedAt: now, CheckedAt: now, ExpiresAt: expires},
		refresh: refreshToken,
	}
	return sid, nil
}

func (m *MemStore) live(id string) (*memSession, bool) {
	s, ok := m.sessions[id]
	if !ok || !m.Now().Before(s.ExpiresAt) {
		return nil, false
	}
	return s, true
}

// Load implements Store.
func (m *MemStore) Load(_ context.Context, sessionID string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.live(sessionID)
	if !ok {
		return Session{}, ErrNoSession
	}
	return s.Session, nil
}

// Refresh implements Store; the mutex stands in for the row lock.
func (m *MemStore) Refresh(ctx context.Context, sessionID string, seen time.Time, fn RefreshFunc) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.live(sessionID)
	if !ok {
		return Session{}, ErrNoSession
	}
	if !s.CheckedAt.Equal(seen) {
		return s.Session, nil
	}
	next, id, err := fn(ctx, s.refresh)
	if errors.Is(err, ErrSessionGone) {
		delete(m.sessions, sessionID)
		return Session{}, ErrSessionGone
	}
	if err != nil {
		return Session{}, err
	}
	s.refresh, s.Name, s.CheckedAt = next, id.Name, m.Now()
	if s.CheckedAt.Equal(seen) {
		s.CheckedAt = seen.Add(time.Nanosecond)
	}
	return s.Session, nil
}

// SignOut implements Store.
func (m *MemStore) SignOut(_ context.Context, sessionID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok {
		return "", ErrNoSession
	}
	delete(m.sessions, sessionID)
	return s.refresh, nil
}

// RefreshToken returns the stored refresh token of a session, for tests.
func (m *MemStore) RefreshToken(sessionID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[sessionID]; ok {
		return s.refresh
	}
	return ""
}
