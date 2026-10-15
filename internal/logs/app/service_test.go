package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/logs/app"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/logs/domain"
)

var now = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

// store records what it was asked.
type store struct {
	namespace string
	since     time.Time
	limit     int
	err       error
}

func (s *store) Streams(_ context.Context, namespace string, since time.Time) ([]domain.Stream, error) {
	s.namespace, s.since = namespace, since
	return nil, s.err
}

func (s *store) Lines(_ context.Context, namespace string, _ domain.Stream, since time.Time, limit int) ([]domain.Line, error) {
	s.namespace, s.since, s.limit = namespace, since, limit
	return nil, s.err
}

func TestAProjectsLogsAreInItsNamespaceOverAClampedWindow(t *testing.T) {
	for asked, want := range map[time.Duration]time.Duration{0: app.DefaultSince, time.Hour: time.Hour, 1000 * time.Hour: app.MaxSince} {
		s := &store{}
		if _, err := app.New(s, func() time.Time { return now }).Streams(t.Context(), "auth", asked); err != nil || s.namespace != "auth-system" || !s.since.Equal(now.Add(-want)) {
			t.Errorf("Streams(%v) asked %s from %v, %v", asked, s.namespace, s.since, err)
		}
	}
	for asked, want := range map[int]int{0: app.DefaultLines, 10: 10, 9999: app.MaxLines} {
		s := &store{}
		if _, err := app.New(s, time.Now).Lines(t.Context(), "auth", "auth-api-1", "auth-api", time.Hour, asked); err != nil || s.limit != want {
			t.Errorf("Lines(%d) asked %d, %v", asked, s.limit, err)
		}
	}
}

func TestANameThatCannotBeOneIsRefusedBeforeLokiIsAsked(t *testing.T) {
	s := &store{}
	svc := app.New(s, time.Now)
	if _, err := svc.Streams(t.Context(), "Not A Project", time.Hour); !errors.Is(err, domain.ErrNotAName) {
		t.Errorf("project = %v", err)
	}
	for _, c := range [][3]string{{"auth", `pod"}`, "c"}, {"auth", "pod", "Container"}, {"BAD", "p", "c"}} {
		if _, err := svc.Lines(t.Context(), c[0], c[1], c[2], time.Hour, 1); !errors.Is(err, domain.ErrNotAName) {
			t.Errorf("Lines%v = %v", c, err)
		}
	}
	if s.namespace != "" {
		t.Fatalf("Loki was asked about %s", s.namespace)
	}
}

func TestEveryReadReportsTheStoresFailure(t *testing.T) {
	svc := app.New(&store{err: domain.ErrNoStore}, time.Now)
	_, e1 := svc.Streams(t.Context(), "auth", time.Hour)
	_, e2 := svc.Lines(t.Context(), "auth", "p", "c", time.Hour, 1)
	if !errors.Is(e1, domain.ErrNoStore) || !errors.Is(e2, domain.ErrNoStore) {
		t.Fatalf("errors = %v %v", e1, e2)
	}
}
