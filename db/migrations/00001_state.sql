-- +goose Up
-- A migration waits at most 5s for a lock and runs at most 60s, so it fails rather than stalls.
SET lock_timeout = '5s';
SET statement_timeout = '60s';

-- The dashboard's own state. Everything else it shows is read from the Estate repository, the
-- cluster and Alertmanager, and none of that is copied here. Integrity lives in the database
-- (template-go-vue, docs/blueprints/go-api.md §8.2): each rule the domain checks is a CHECK too.

-- A signed-in admin. The cookie holds this id, sealed, and nothing else; what renews the session
-- through auth's token endpoint stays on this side.
CREATE TABLE IF NOT EXISTS sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject text NOT NULL CHECK (char_length(subject) BETWEEN 1 AND 255),
    refresh_token_sealed bytea NOT NULL CHECK (octet_length(refresh_token_sealed) > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    renewed_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    CONSTRAINT sessions_expire_after_they_start CHECK (expires_at > created_at),
    CONSTRAINT sessions_renew_after_they_start CHECK (renewed_at >= created_at)
);

-- Expired sessions are found, and removed, by when they end.
CREATE INDEX IF NOT EXISTS sessions_expires_at_idx ON sessions (expires_at);

-- Every state an alert was seen in: one row when it fires, one when it resolves. Alertmanager
-- forgets a resolved alert; this is what the alert's detail pane reads its history from. An alert
-- is its fingerprint, and one firing of it is the fingerprint with when it started.
CREATE TABLE IF NOT EXISTS alert_history (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    fingerprint text NOT NULL CHECK (char_length(fingerprint) BETWEEN 1 AND 64),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 255),
    status text NOT NULL CHECK (status IN ('firing', 'resolved')),
    labels jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(labels) = 'object'),
    starts_at timestamptz NOT NULL,
    ends_at timestamptz,
    observed_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT alert_history_ends_after_it_starts CHECK (ends_at IS NULL OR ends_at >= starts_at),
    CONSTRAINT alert_history_resolved_has_an_end CHECK ((status = 'resolved') = (ends_at IS NOT NULL)),
    -- Seeing the same state twice records it once.
    CONSTRAINT alert_history_one_row_per_state UNIQUE (fingerprint, starts_at, status)
);

-- The history is read newest first.
CREATE INDEX IF NOT EXISTS alert_history_observed_at_idx ON alert_history (observed_at DESC, id DESC);

-- Who said they are handling one firing of an alert. A later firing is acknowledged again.
CREATE TABLE IF NOT EXISTS acknowledgements (
    fingerprint text NOT NULL CHECK (char_length(fingerprint) BETWEEN 1 AND 64),
    starts_at timestamptz NOT NULL,
    acknowledged_by text NOT NULL CHECK (char_length(acknowledged_by) BETWEEN 1 AND 255),
    acknowledged_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (fingerprint, starts_at)
);

-- A mirror of the silences the dashboard created in Alertmanager, its only write: who silenced
-- which alert, and until when. Alertmanager holds the silence itself; `id` is its id there.
CREATE TABLE IF NOT EXISTS silences (
    id text PRIMARY KEY CHECK (char_length(id) BETWEEN 1 AND 64),
    fingerprint text NOT NULL CHECK (char_length(fingerprint) BETWEEN 1 AND 64),
    created_by text NOT NULL CHECK (char_length(created_by) BETWEEN 1 AND 255),
    created_at timestamptz NOT NULL DEFAULT now(),
    -- NULL silences until the alert resolves.
    ends_at timestamptz,
    -- When the dashboard saw that Alertmanager no longer holds the silence.
    expired_at timestamptz,
    CONSTRAINT silences_end_after_they_start CHECK (ends_at IS NULL OR ends_at > created_at),
    CONSTRAINT silences_expire_after_they_start CHECK (expired_at IS NULL OR expired_at >= created_at)
);

-- An alert's silences are looked up by the alert.
CREATE INDEX IF NOT EXISTS silences_fingerprint_idx ON silences (fingerprint);
