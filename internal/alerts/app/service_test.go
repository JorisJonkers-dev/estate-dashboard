package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/app"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/domain"
)

// repository records the limit it was asked for.
type repository struct {
	asked  int
	events []domain.Event
	err    error
}

func (r *repository) History(_ context.Context, limit int) ([]domain.Event, error) {
	r.asked = limit
	return r.events, r.err
}

func (*repository) Record(context.Context, domain.Event) error { return nil }

func (*repository) Open(context.Context) ([]domain.Event, error) { return nil, nil }

func TestHistoryClampsTheLimit(t *testing.T) {
	for asked, want := range map[int]int{-1: app.DefaultLimit, 0: app.DefaultLimit, 1: 1, 100: 100, 101: app.MaxLimit} {
		repo := &repository{events: []domain.Event{{Fingerprint: "a"}}}
		got, err := app.New(repo).History(context.Background(), asked)
		if err != nil || len(got) != 1 {
			t.Fatalf("History(%d) = %v, %v", asked, got, err)
		}
		if repo.asked != want {
			t.Errorf("History(%d) asked the repository for %d, want %d", asked, repo.asked, want)
		}
	}
}

func TestHistoryReturnsTheRepositorysFailure(t *testing.T) {
	boom := errors.New("boom")
	if _, err := app.New(&repository{err: boom}).History(context.Background(), 10); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the repository's", err)
	}
}
