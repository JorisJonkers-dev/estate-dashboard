package app_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/checks/app"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/checks/domain"
)

type monitor struct {
	checks []domain.Check
	err    error
}

func (m monitor) Checks(context.Context) ([]domain.Check, error) { return m.checks, m.err }

func TestTheChecksComeByGroupThenName(t *testing.T) {
	got, err := app.New(monitor{checks: []domain.Check{
		{Group: "public-apps", Name: "tribelt"}, {Group: "home-media", Name: "sonarr"}, {Group: "home-media", Name: "radarr"},
	}}).Checks(t.Context())
	var names []string
	for _, c := range got {
		names = append(names, c.Group+"/"+c.Name)
	}
	if err != nil || !slices.Equal(names, []string{"home-media/radarr", "home-media/sonarr", "public-apps/tribelt"}) {
		t.Fatalf("checks %v, %v", names, err)
	}
}

func TestReadingTheChecksReportsTheMonitorsFailure(t *testing.T) {
	if _, err := app.New(monitor{err: domain.ErrNoMonitor}).Checks(t.Context()); !errors.Is(err, domain.ErrNoMonitor) {
		t.Fatalf("error = %v", err)
	}
}
