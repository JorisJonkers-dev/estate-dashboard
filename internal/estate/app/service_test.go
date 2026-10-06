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
	pins   []domain.Pin
	issues []domain.Issue
	asked  int
	err    error
}

func (r *repository) Pins(context.Context) ([]domain.Pin, error) { return r.pins, r.err }

func (r *repository) Deploys(_ context.Context, _ string, limit int) ([]domain.Deploy, error) {
	r.asked = limit
	return nil, r.err
}

func (r *repository) Issues(context.Context) ([]domain.Issue, error) { return r.issues, r.err }

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
	for _, err := range []error{e1, e2, e3} {
		if !errors.Is(err, domain.ErrNoRepository) {
			t.Fatalf("error = %v", err)
		}
	}
}
