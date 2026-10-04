-- +goose Up
-- A migration waits at most 5s for a lock and runs at most 60s, so it fails rather than stalls.
SET lock_timeout = '5s';
SET statement_timeout = '60s';

-- What the dashboard calls the signed-in admin: the name auth gave at sign-in, and again at every
-- renewal, so a rename in auth shows here without a second sign-in.
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS name text NOT NULL DEFAULT ''
    CONSTRAINT sessions_name_is_short CHECK (char_length(name) <= 255);
