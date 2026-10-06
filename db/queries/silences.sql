-- The silences the dashboard created in Alertmanager: its one write, mirrored here.

-- name: RecordSilence :exec
INSERT INTO silences (id, fingerprint, created_by, created_at, ends_at)
VALUES (sqlc.arg(id), sqlc.arg(fingerprint), sqlc.arg(created_by), sqlc.arg(created_at), sqlc.narg(ends_at));

-- name: ListUntilResolved :many
SELECT id, fingerprint, created_by, created_at, ends_at
FROM silences
WHERE fingerprint = sqlc.arg(fingerprint) AND ends_at IS NULL AND expired_at IS NULL
ORDER BY created_at, id;

-- name: ExpireSilence :exec
UPDATE silences SET expired_at = sqlc.arg(expired_at)
WHERE id = sqlc.arg(id) AND expired_at IS NULL;
