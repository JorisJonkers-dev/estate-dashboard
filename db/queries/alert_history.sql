-- name: ListAlertHistory :many
SELECT id, fingerprint, name, status, starts_at, ends_at, observed_at
FROM alert_history
ORDER BY observed_at DESC, id DESC
LIMIT $1;

-- Seeing the same state twice records it once: alert_history_one_row_per_state.
-- name: RecordAlertEvent :exec
INSERT INTO alert_history (fingerprint, name, status, labels, starts_at, ends_at, observed_at)
VALUES (sqlc.arg(fingerprint), sqlc.arg(name), sqlc.arg(status), sqlc.arg(labels), sqlc.arg(starts_at), sqlc.narg(ends_at), sqlc.arg(observed_at))
ON CONFLICT ON CONSTRAINT alert_history_one_row_per_state DO NOTHING;

-- Every firing with no resolution of the same firing recorded.
-- name: ListOpenFirings :many
SELECT f.id, f.fingerprint, f.name, f.status, f.starts_at, f.ends_at, f.observed_at
FROM alert_history f
WHERE f.status = 'firing'
  AND NOT EXISTS (
    SELECT 1 FROM alert_history r
    WHERE r.fingerprint = f.fingerprint AND r.starts_at = f.starts_at AND r.status = 'resolved'
  )
ORDER BY f.starts_at, f.fingerprint;
