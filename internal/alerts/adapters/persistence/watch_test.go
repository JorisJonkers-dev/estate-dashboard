package persistence_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/adapters/persistence"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/pg/pgtest"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/pg/queries"
)

func pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	p, err := pgxpool.New(t.Context(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func TestARecordedStateIsKeptOnceAndAFiringStaysOpenUntilItResolves(t *testing.T) {
	t.Parallel()
	db := pool(t)
	repo := persistence.New(queries.New(db))
	start := time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)
	later := start.Add(time.Hour)
	firing := domain.Event{Fingerprint: "9f2c", Name: "ReleaseHeld", Status: domain.Firing, StartsAt: start, ObservedAt: start, Labels: map[string]string{"alertname": "ReleaseHeld"}}
	other := domain.Event{Fingerprint: "71aa", Name: "Odd", Status: domain.Firing, StartsAt: later, ObservedAt: later}
	for _, e := range []domain.Event{firing, firing, other} {
		if err := repo.Record(t.Context(), e); err != nil {
			t.Fatal(err)
		}
	}
	var labels string
	if err := db.QueryRow(t.Context(), `SELECT labels::text FROM alert_history WHERE fingerprint = '9f2c'`).Scan(&labels); err != nil || labels != `{"alertname": "ReleaseHeld"}` {
		t.Fatalf("labels = %q, %v", labels, err)
	}
	open, err := repo.Open(t.Context())
	if err != nil || len(open) != 2 || open[0].Fingerprint != "9f2c" || open[1].Fingerprint != "71aa" {
		t.Fatalf("open = %+v, %v", open, err)
	}

	ends := start.Add(30 * time.Minute)
	if err := repo.Record(t.Context(), domain.Event{Fingerprint: "9f2c", Name: "ReleaseHeld", Status: domain.Resolved, StartsAt: start, EndsAt: &ends, ObservedAt: ends}); err != nil {
		t.Fatal(err)
	}
	open, err = repo.Open(t.Context())
	if err != nil || len(open) != 1 || open[0].Fingerprint != "71aa" {
		t.Fatalf("open after a resolution = %+v, %v", open, err)
	}
	history, err := repo.History(t.Context(), 10)
	if err != nil || len(history) != 3 {
		t.Fatalf("history = %+v, %v", history, err)
	}
	if err := repo.Record(t.Context(), domain.Event{Fingerprint: "x", Status: domain.Resolved, StartsAt: start}); !errors.Is(err, domain.ErrInconsistentEvent) {
		t.Fatalf("an event against the rules = %v", err)
	}
}

func TestTheSilencesMirrorKnowsWhichLastUntilResolvedAndWhichEnded(t *testing.T) {
	t.Parallel()
	mirror := persistence.NewSilences(queries.New(pool(t)))
	created := time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)
	ends := created.Add(time.Hour)
	for _, s := range []domain.Silence{
		{ID: "s-2", Fingerprint: "9f2c", CreatedBy: "joris", CreatedAt: created.Add(time.Minute)},
		{ID: "s-1", Fingerprint: "9f2c", CreatedBy: "joris", CreatedAt: created},
		{ID: "s-3", Fingerprint: "9f2c", CreatedBy: "joris", CreatedAt: created, EndsAt: &ends},
		{ID: "s-4", Fingerprint: "71aa", CreatedBy: "joris", CreatedAt: created},
	} {
		if err := mirror.Record(t.Context(), s); err != nil {
			t.Fatal(err)
		}
	}
	got, err := mirror.UntilResolved(t.Context(), "9f2c")
	if err != nil || len(got) != 2 || got[0].ID != "s-1" || got[1].ID != "s-2" || got[0].EndsAt != nil || got[0].CreatedBy != "joris" || got[0].CreatedAt.Location() != time.UTC {
		t.Fatalf("until resolved = %+v, %v", got, err)
	}
	if err := mirror.Expired(t.Context(), "s-1", created.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err = mirror.UntilResolved(t.Context(), "9f2c")
	if err != nil || len(got) != 1 || got[0].ID != "s-2" {
		t.Fatalf("after one ended = %+v, %v", got, err)
	}
	if err := mirror.Record(t.Context(), domain.Silence{ID: "s-1", Fingerprint: "9f2c", CreatedBy: "joris", CreatedAt: created}); err == nil {
		t.Fatal("one Alertmanager id was mirrored twice")
	}
	if err := mirror.Expired(t.Context(), "s-2", created.Add(-time.Hour)); err == nil {
		t.Fatal("a silence ended before it was set")
	}
}

func TestTheMirrorReportsADatabaseThatIsAway(t *testing.T) {
	t.Parallel()
	db := pool(t)
	mirror := persistence.NewSilences(queries.New(db))
	repo := persistence.New(queries.New(db))
	db.Close()
	if _, err := mirror.UntilResolved(t.Context(), "9f2c"); err == nil {
		t.Fatal("read from a closed pool")
	}
	if err := mirror.Record(t.Context(), domain.Silence{ID: "s", Fingerprint: "f", CreatedBy: "j", CreatedAt: time.Now()}); err == nil {
		t.Fatal("wrote to a closed pool")
	}
	if err := mirror.Expired(t.Context(), "s", time.Now()); err == nil {
		t.Fatal("expired in a closed pool")
	}
	if _, err := repo.Open(t.Context()); err == nil {
		t.Fatal("read the open firings from a closed pool")
	}
	if err := repo.Record(t.Context(), domain.Event{Fingerprint: "f", Name: "n", Status: domain.Firing, StartsAt: time.Now()}); err == nil {
		t.Fatal("recorded in a closed pool")
	}
}
