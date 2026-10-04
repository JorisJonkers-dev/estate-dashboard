package pg_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/pg/pgtest"
)

// The migrations, applied to a real Postgres, leave the dashboard's four tables, and each holds
// the rules its comment in db/migrations states.
func TestTheMigratedSchemaHoldsTheDashboardsState(t *testing.T) {
	t.Parallel()
	pool, err := pgxpool.New(t.Context(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var tables []string
	rows, err := pool.Query(t.Context(), `SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_name <> 'goose_db_version' ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if got := strings.Join(tables, " "); got != "acknowledgements alert_history sessions silences" {
		t.Fatalf("tables = %q", got)
	}

	accepted := []string{
		`INSERT INTO sessions (subject, refresh_token_sealed, expires_at) VALUES ('joris', '\x01', now() + interval '8 hours')`,
		`INSERT INTO alert_history (fingerprint, name, status, starts_at) VALUES ('9f2c', 'CollectorStopped', 'firing', now())`,
		`INSERT INTO alert_history (fingerprint, name, status, starts_at, ends_at) VALUES ('9f2c', 'CollectorStopped', 'resolved', now() - interval '1 hour', now())`,
		`INSERT INTO acknowledgements (fingerprint, starts_at, acknowledged_by) VALUES ('9f2c', '2026-10-03T07:00:00Z', 'joris')`,
		`INSERT INTO silences (id, fingerprint, created_by, ends_at) VALUES ('s-1', '9f2c', 'joris', now() + interval '4 hours')`,
		`INSERT INTO silences (id, fingerprint, created_by) VALUES ('s-2', '9f2c', 'joris')`, // until it resolves
	}
	for _, sql := range accepted {
		if _, err := pool.Exec(t.Context(), sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}

	const check, unique, notNull = "23514", "23505", "23502"
	refused := map[string]struct{ sql, code string }{
		"a session that ends before it starts": {`INSERT INTO sessions (subject, refresh_token_sealed, expires_at) VALUES ('joris', '\x01', now() - interval '1 hour')`, check},
		"a session with no subject":            {`INSERT INTO sessions (subject, refresh_token_sealed, expires_at) VALUES ('', '\x01', now() + interval '1 hour')`, check},
		"a session with nothing to renew with": {`INSERT INTO sessions (subject, refresh_token_sealed, expires_at) VALUES ('joris', '', now() + interval '1 hour')`, check},
		"a session with no end":                {`INSERT INTO sessions (subject, refresh_token_sealed) VALUES ('joris', '\x01')`, notNull},
		"a session with an endless name":       {`INSERT INTO sessions (subject, name, refresh_token_sealed, expires_at) VALUES ('joris', repeat('n', 256), '\x01', now() + interval '1 hour')`, check},
		"an alert in a state that is not one":  {`INSERT INTO alert_history (fingerprint, name, status, starts_at) VALUES ('a', 'A', 'pending', now())`, check},
		"a firing alert with an end":           {`INSERT INTO alert_history (fingerprint, name, status, starts_at, ends_at) VALUES ('a', 'A', 'firing', now(), now())`, check},
		"a resolved alert with no end":         {`INSERT INTO alert_history (fingerprint, name, status, starts_at) VALUES ('a', 'A', 'resolved', now())`, check},
		"an alert that ends before it starts":  {`INSERT INTO alert_history (fingerprint, name, status, starts_at, ends_at) VALUES ('a', 'A', 'resolved', now(), now() - interval '1 hour')`, check},
		"labels that are not an object":        {`INSERT INTO alert_history (fingerprint, name, status, starts_at, labels) VALUES ('a', 'A', 'firing', now(), '[]')`, check},
		"the same state of a firing, twice": {
			`INSERT INTO alert_history (fingerprint, name, status, starts_at) VALUES ('b', 'B', 'firing', '2026-10-03T07:00:00Z'), ('b', 'B', 'firing', '2026-10-03T07:00:00Z')`, unique,
		},
		"one firing acknowledged twice":        {`INSERT INTO acknowledgements (fingerprint, starts_at, acknowledged_by) VALUES ('9f2c', '2026-10-03T07:00:00Z', 'someone')`, unique},
		"an acknowledgement by no one":         {`INSERT INTO acknowledgements (fingerprint, starts_at, acknowledged_by) VALUES ('c', now(), '')`, check},
		"a silence Alertmanager already gave":  {`INSERT INTO silences (id, fingerprint, created_by) VALUES ('s-1', '9f2c', 'joris')`, unique},
		"a silence that ends before it starts": {`INSERT INTO silences (id, fingerprint, created_by, ends_at) VALUES ('s-3', '9f2c', 'joris', now() - interval '1 hour')`, check},
		"a silence expired before it was made": {`INSERT INTO silences (id, fingerprint, created_by, expired_at) VALUES ('s-4', '9f2c', 'joris', now() - interval '1 hour')`, check},
	}
	for name, c := range refused {
		_, err := pool.Exec(t.Context(), c.sql)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != c.code {
			t.Errorf("%s: error = %v, want SQLSTATE %s", name, err, c.code)
		}
	}
}
