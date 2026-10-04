-- name: CreateSession :one
INSERT INTO sessions (subject, name, refresh_token_sealed, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: GetSession :one
SELECT id, subject, name, created_at, renewed_at, expires_at
FROM sessions
WHERE id = $1 AND expires_at > now();

-- The row lock is what makes concurrent requests spend a rotating refresh token once.
-- name: LockSession :one
SELECT id, subject, name, refresh_token_sealed, created_at, renewed_at, expires_at
FROM sessions
WHERE id = $1 AND expires_at > now()
FOR UPDATE;

-- renewed_at always moves, so a request that loaded the session before sees it was renewed.
-- name: RenewSession :one
UPDATE sessions
SET name = $2,
    refresh_token_sealed = $3,
    renewed_at = greatest(clock_timestamp(), renewed_at + interval '1 microsecond')
WHERE id = $1
RETURNING id, subject, name, created_at, renewed_at, expires_at;

-- name: DeleteSession :one
DELETE FROM sessions
WHERE id = $1
RETURNING refresh_token_sealed;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions
WHERE expires_at <= now();
