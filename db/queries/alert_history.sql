-- name: ListAlertHistory :many
SELECT id, fingerprint, name, status, starts_at, ends_at, observed_at
FROM alert_history
ORDER BY observed_at DESC, id DESC
LIMIT $1;
