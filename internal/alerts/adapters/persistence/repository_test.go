package persistence_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/adapters/persistence"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/pg/pgtest"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/pg/queries"
)

func TestHistoryReadsTheMigratedTableNewestFirst(t *testing.T) {
	t.Parallel()
	pool, err := pgxpool.New(t.Context(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	start := time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)
	insert := `INSERT INTO alert_history (fingerprint, name, status, starts_at, ends_at, observed_at) VALUES ($1, $2, $3, $4, $5, $6)`
	for _, row := range [][]any{
		{"9f2c", "CollectorStopped", "firing", start, nil, start.Add(20 * time.Second)},
		{"9f2c", "CollectorStopped", "resolved", start, start.Add(30 * time.Minute), start.Add(31 * time.Minute)},
		{"71aa", "ReleaseHeld", "firing", start.Add(time.Hour), nil, start.Add(time.Hour)},
	} {
		if _, err := pool.Exec(t.Context(), insert, row...); err != nil {
			t.Fatal(err)
		}
	}

	repo := persistence.New(queries.New(pool))
	events, err := repo.History(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Name != "ReleaseHeld" || events[1].Status != domain.Resolved || events[2].Status != domain.Firing {
		t.Fatalf("history = %+v", events)
	}
	resolved := events[1]
	if resolved.EndsAt == nil || !resolved.EndsAt.Equal(start.Add(30*time.Minute)) || resolved.EndsAt.Location() != time.UTC {
		t.Fatalf("the resolved event ends at %v", resolved.EndsAt)
	}
	if events[2].EndsAt != nil || events[2].StartsAt.Location() != time.UTC || !events[2].StartsAt.Equal(start) {
		t.Fatalf("the firing event = %+v", events[2])
	}

	two, err := repo.History(t.Context(), 2)
	if err != nil || len(two) != 2 {
		t.Fatalf("History(2) = %d events, %v", len(two), err)
	}
}

// rows is a Querier that answers with what it was given.
type rows struct {
	queries.Querier
	list []queries.ListAlertHistoryRow
	err  error
}

func (r rows) ListAlertHistory(context.Context, int32) ([]queries.ListAlertHistoryRow, error) {
	return r.list, r.err
}

func TestHistoryRefusesWhatTheDomainWould(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)
	boom := errors.New("boom")

	if _, err := persistence.New(rows{err: boom}).History(t.Context(), 1); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the query's", err)
	}
	for _, limit := range []int{-1, math.MaxInt32 + 1} {
		if _, err := persistence.New(rows{}).History(t.Context(), limit); err == nil || !strings.Contains(err.Error(), "out of range") {
			t.Fatalf("History(%d) error = %v", limit, err)
		}
	}

	cases := map[string]struct {
		row  queries.ListAlertHistoryRow
		want error
	}{
		"a status the domain does not know": {queries.ListAlertHistoryRow{ID: 7, Status: "pending", StartsAt: start}, domain.ErrUnknownStatus},
		"a firing row with an end": {
			queries.ListAlertHistoryRow{ID: 8, Status: "firing", StartsAt: start, EndsAt: pgtype.Timestamptz{Time: start, Valid: true}},
			domain.ErrInconsistentEvent,
		},
	}
	for name, c := range cases {
		_, err := persistence.New(rows{list: []queries.ListAlertHistoryRow{c.row}}).History(t.Context(), 1)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: error = %v, want %v", name, err, c.want)
		}
	}
}
