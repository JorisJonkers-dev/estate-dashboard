-- Every time in these queries is the caller's, never the database's: the service measures a
-- session's age on its own clock, and a second clock that drifts from it would stretch that age.

-- name: CreateSession :one
INSERT INTO sessions (subject, name, refresh_token_sealed, created_at, renewed_at, expires_at)
VALUES (sqlc.arg(subject), sqlc.arg(name), sqlc.arg(refresh_token_sealed), sqlc.arg(now), sqlc.arg(now), sqlc.arg(expires_at))
RETURNING id;

-- name: GetSession :one
SELECT id, subject, name, created_at, renewed_at, expires_at
FROM sessions
WHERE id = sqlc.arg(id) AND expires_at > sqlc.arg(now);

-- The row lock is what makes concurrent requests spend a rotating refresh token once.
-- name: LockSession :one
SELECT id, subject, name, refresh_token_sealed, created_at, renewed_at, expires_at
FROM sessions
WHERE id = sqlc.arg(id) AND expires_at > sqlc.arg(now)
FOR UPDATE;

-- renewed_at always moves, so a request that loaded the session before sees it was renewed.
-- name: RenewSession :one
UPDATE sessions
SET name = sqlc.arg(name),
    refresh_token_sealed = sqlc.arg(refresh_token_sealed),
    renewed_at = greatest(sqlc.arg(now)::timestamptz, renewed_at + interval '1 microsecond')
WHERE id = sqlc.arg(id)
RETURNING id, subject, name, created_at, renewed_at, expires_at;

-- name: DeleteSession :one
DELETE FROM sessions
WHERE id = sqlc.arg(id)
RETURNING refresh_token_sealed;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions
WHERE expires_at <= sqlc.arg(now);
