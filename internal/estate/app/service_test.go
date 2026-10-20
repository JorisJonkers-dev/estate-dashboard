package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/estate/app"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/estate/domain"
)

type repository struct {
	pins    []domain.Pin
	ledger  domain.Handover
	issues  []domain.Issue
	updates []domain.Update
	asked   int
	err     error
}

func (r *repository) Pins(context.Context) ([]domain.Pin, error) { return r.pins, r.err }

func (r *repository) Handover(context.Context) (domain.Handover, error) { return r.ledger, r.err }

func (r *repository) Deploys(_ context.Context, _ string, limit int) ([]domain.Deploy, error) {
	r.asked = limit
	return nil, r.err
}

func (r *repository) Issues(context.Context) ([]domain.Issue, error) { return r.issues, r.err }

func (r *repository) Updates(context.Context) ([]domain.Update, error) { return r.updates, r.err }

func (r *repository) Rollouts(_ context.Context, _, _ string, limit int) ([]domain.Rollout, error) {
	r.asked = limit
	return nil, r.err
}

func (r *repository) Notes(context.Context, string, string) (domain.Notes, error) {
	return domain.Notes{Version: "1.0.0"}, r.err
}

func TestNotesAreTheRepositorysOrItsFailure(t *testing.T) {
	if n, err := app.New(&repository{}).Notes(t.Context(), "a/b", "1.0.0"); err != nil || n.Version != "1.0.0" {
		t.Fatalf("notes = %+v, %v", n, err)
	}
	if _, err := app.New(&repository{err: domain.ErrNoRelease}).Notes(t.Context(), "a/b", "1.0.0"); !errors.Is(err, domain.ErrNoRelease) {
		t.Fatalf("error = %v", err)
	}
}

func TestRolloutsClampsTheLimit(t *testing.T) {
	for asked, want := range map[int]int{0: app.DefaultRollouts, 3: 3, 101: app.MaxDeploys} {
		repo := &repository{}
		if _, err := app.New(repo).Rollouts(t.Context(), "a/b", "auth", asked); err != nil || repo.asked != want {
			t.Errorf("Rollouts(%d) asked %d, %v", asked, repo.asked, err)
		}
	}
	if _, err := app.New(&repository{err: domain.ErrNoRepository}).Rollouts(t.Context(), "a/b", "auth", 1); !errors.Is(err, domain.ErrNoRepository) {
		t.Fatalf("error = %v", err)
	}
}

func TestPinsAreByProjectAndIssuesTheMostRecentlyUpdatedFirst(t *testing.T) {
	at := time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)
	s := app.New(&repository{
		pins:   []domain.Pin{{Project: "b"}, {Project: "a"}},
		issues: []domain.Issue{{Number: 1, UpdatedAt: at}, {Number: 2, UpdatedAt: at.Add(time.Hour)}, {Number: 3, UpdatedAt: at}},
	})
	pins, err := s.Pins(t.Context())
	if err != nil || pins[0].Project != "a" {
		t.Fatalf("pins = %+v, %v", pins, err)
	}
	issues, err := s.Issues(t.Context())
	if err != nil || issues[0].Number != 2 || issues[1].Number != 1 || issues[2].Number != 3 {
		t.Fatalf("issues = %+v, %v", issues, err)
	}
}

func TestTheHandoverLedgerIsByName(t *testing.T) {
	s := app.New(&repository{ledger: domain.Handover{Estate: []string{"delivery", "app"}, Legacy: []string{"notes", "auth"}}})
	ledger, err := s.Handover(t.Context())
	if err != nil || ledger.Estate[0] != "app" || ledger.Legacy[0] != "auth" {
		t.Fatalf("ledger = %+v, %v", ledger, err)
	}
}

func TestDeploysClampsTheLimit(t *testing.T) {
	for asked, want := range map[int]int{-1: app.DefaultDeploys, 0: app.DefaultDeploys, 1: 1, 100: 100, 101: app.MaxDeploys} {
		repo := &repository{}
		if _, err := app.New(repo).Deploys(t.Context(), "auth", asked); err != nil || repo.asked != want {
			t.Errorf("Deploys(%d) asked %d, %v", asked, repo.asked, err)
		}
	}
}

func TestEveryReadReportsTheRepositorysFailure(t *testing.T) {
	s := app.New(&repository{err: domain.ErrNoRepository})
	_, e1 := s.Pins(t.Context())
	_, e2 := s.Deploys(t.Context(), "auth", 1)
	_, e3 := s.Issues(t.Context())
	_, e4 := s.Handover(t.Context())
	_, e5 := s.Updates(t.Context())
	for _, err := range []error{e1, e2, e3, e4, e5} {
		if !errors.Is(err, domain.ErrNoRepository) {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestUpdatesAreTheNewestPullRequestFirst(t *testing.T) {
	older := domain.Update{Number: 1, Opened: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	newer := domain.Update{Number: 2, Opened: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)}
	got, err := app.New(&repository{updates: []domain.Update{older, newer}}).Updates(t.Context())
	if err != nil || len(got) != 2 || got[0].Number != 2 {
		t.Fatalf("updates = %+v, %v", got, err)
	}
}
