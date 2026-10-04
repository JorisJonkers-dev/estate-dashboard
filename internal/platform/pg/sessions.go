package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/oidc"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/pg/queries"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/session"
)

// sealedAs names what a sealed refresh token is, so nothing else the codec seals opens as one.
const sealedAs = "refresh-token"

// Sessions is the dashboard's server-side sessions, in the sessions table. A refresh token is
// stored sealed: a copy of the table is not a set of tokens auth accepts.
type Sessions struct {
	store *Store
	codec *session.Codec
	now   func() time.Time
}

var _ oidc.Store = (*Sessions)(nil)

// Sessions returns the session store, sealing refresh tokens with codec.
func (s *Store) Sessions(codec *session.Codec) *Sessions {
	return &Sessions{store: s, codec: codec, now: time.Now}
}

// SignIn implements oidc.Store.
func (s *Sessions) SignIn(ctx context.Context, id oidc.Identity, refreshToken string, expires time.Time) (string, error) {
	sealed, err := s.seal(refreshToken, expires)
	if err != nil {
		return "", err
	}
	sid, err := s.store.Queries().CreateSession(ctx, queries.CreateSessionParams{
		Subject: id.Sub, Name: id.Name, RefreshTokenSealed: sealed, ExpiresAt: expires,
	})
	if err != nil {
		return "", fmt.Errorf("pg: create session: %w", err)
	}
	return sid.String(), nil
}

// Load implements oidc.Store.
func (s *Sessions) Load(ctx context.Context, sessionID string) (oidc.Session, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return oidc.Session{}, oidc.ErrNoSession
	}
	row, err := s.store.Queries().GetSession(ctx, id)
	if err != nil {
		return oidc.Session{}, gone(err, "load session")
	}
	return oidc.Session{
		ID: row.ID.String(), Sub: row.Subject, Name: row.Name,
		CreatedAt: row.CreatedAt, CheckedAt: row.RenewedAt, ExpiresAt: row.ExpiresAt,
	}, nil
}

// Refresh implements oidc.Store. The row stays locked while fn asks auth, so of the requests that
// find the same session stale, one spends its refresh token and the rest read what it stored.
func (s *Sessions) Refresh(ctx context.Context, sessionID string, seen time.Time, fn oidc.RefreshFunc) (oidc.Session, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return oidc.Session{}, oidc.ErrNoSession
	}
	tx, err := s.store.pool.Begin(ctx)
	if err != nil {
		return oidc.Session{}, fmt.Errorf("pg: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op once committed
	q := queries.New(tx)

	row, err := q.LockSession(ctx, id)
	if err != nil {
		return oidc.Session{}, gone(err, "lock session")
	}
	current := oidc.Session{
		ID: row.ID.String(), Sub: row.Subject, Name: row.Name,
		CreatedAt: row.CreatedAt, CheckedAt: row.RenewedAt, ExpiresAt: row.ExpiresAt,
	}
	if !row.RenewedAt.Equal(seen) {
		return current, nil
	}
	var refreshToken string
	if err := s.codec.Open(sealedAs, string(row.RefreshTokenSealed), s.now(), &refreshToken); err != nil {
		// Sealed under a key this process does not hold: nothing can renew it.
		return oidc.Session{}, s.end(ctx, tx, q, id)
	}
	next, who, err := fn(ctx, refreshToken)
	if errors.Is(err, oidc.ErrSessionGone) {
		return oidc.Session{}, s.end(ctx, tx, q, id)
	}
	if err != nil {
		return oidc.Session{}, err
	}
	sealed, err := s.seal(next, row.ExpiresAt)
	if err != nil {
		return oidc.Session{}, err
	}
	renewed, err := q.RenewSession(ctx, queries.RenewSessionParams{ID: id, Name: who.Name, RefreshTokenSealed: sealed})
	if err != nil {
		return oidc.Session{}, fmt.Errorf("pg: renew session: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return oidc.Session{}, fmt.Errorf("pg: commit: %w", err)
	}
	current.Name, current.CheckedAt = renewed.Name, renewed.RenewedAt
	return current, nil
}

// end deletes a session auth no longer stands behind. The session is gone whether or not the
// delete lands: a database that fails here must not turn auth's refusal into an error a caller
// could read as auth being away.
func (s *Sessions) end(ctx context.Context, tx pgx.Tx, q *queries.Queries, id uuid.UUID) error {
	if _, err := q.DeleteSession(ctx, id); err != nil {
		return fmt.Errorf("%w: pg: end session: %w", oidc.ErrSessionGone, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%w: pg: commit: %w", oidc.ErrSessionGone, err)
	}
	return oidc.ErrSessionGone
}

// SignOut implements oidc.Store. It returns the refresh token, so the caller can revoke it at auth.
func (s *Sessions) SignOut(ctx context.Context, sessionID string) (string, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return "", oidc.ErrNoSession
	}
	sealed, err := s.store.Queries().DeleteSession(ctx, id)
	if err != nil {
		return "", gone(err, "delete session")
	}
	var refreshToken string
	_ = s.codec.Open(sealedAs, string(sealed), s.now(), &refreshToken) // one that does not open is not revoked
	return refreshToken, nil
}

// Sweep deletes every session past its end, and returns how many.
func (s *Sessions) Sweep(ctx context.Context) (int64, error) {
	return s.store.Queries().DeleteExpiredSessions(ctx)
}

// seal seals a refresh token until its session ends.
func (s *Sessions) seal(refreshToken string, expires time.Time) ([]byte, error) {
	now := s.now()
	sealed, err := s.codec.Seal(sealedAs, refreshToken, now, expires.Sub(now))
	if err != nil {
		return nil, fmt.Errorf("pg: seal refresh token: %w", err)
	}
	return []byte(sealed), nil
}

func gone(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return oidc.ErrNoSession
	}
	return fmt.Errorf("pg: %s: %w", what, err)
}
