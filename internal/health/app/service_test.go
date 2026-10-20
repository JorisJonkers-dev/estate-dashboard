package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/health/app"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/health/domain"
)

// metrics answers with samples, and records what it was asked.
type metrics struct {
	samples map[time.Time]domain.Sample
	err     error
	asked   []string
	from    time.Time
	to      time.Time
}

func (m *metrics) Samples(_ context.Context, project string, checks []string, start, end time.Time, _ time.Duration) (map[time.Time]domain.Sample, error) {
	m.asked = append([]string{project}, checks...)
	m.from, m.to = start, end
	return m.samples, m.err
}

var now = time.Date(2026, 10, 20, 12, 0, 17, 0, time.UTC)

func timeline(t *testing.T, m *metrics) []domain.Block {
	t.Helper()
	blocks, err := app.New(m, func() time.Time { return now }).Timeline(t.Context(), "arr", []string{"home-media_sonarr"})
	if err != nil {
		t.Fatal(err)
	}
	return blocks
}

func f(v float64) *float64 { return &v }

func TestATimelineIsTheLastHourInBlocksOfThirtySeconds(t *testing.T) {
	m := &metrics{}
	blocks := timeline(t, m)
	end := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	if len(blocks) != 120 || !blocks[119].At.Equal(end) || !blocks[0].At.Equal(end.Add(-119*30*time.Second)) || !m.from.Equal(blocks[0].At) || !m.to.Equal(end) {
		t.Fatalf("%d blocks, from %v to %v", len(blocks), blocks[0].At, blocks[len(blocks)-1].At)
	}
	if m.asked[0] != "arr" || m.asked[1] != "home-media_sonarr" {
		t.Fatalf("asked %v", m.asked)
	}
}

func TestAStepWithNoDataIsEmptyNotHealthy(t *testing.T) {
	if b := timeline(t, &metrics{})[0]; b.State != domain.Empty {
		t.Fatalf("block = %+v", b)
	}
}

func TestEachBlockIsTheWorstOfItsPodsItsChecksAndItsUnit(t *testing.T) {
	end := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name   string
		sample domain.Sample
		state  domain.State
		detail string
	}{
		{"everything well", domain.Sample{PodsReady: f(1), ChecksUp: f(1), UnitReady: f(1)}, domain.OK, ""},
		{"a unit not ready", domain.Sample{PodsReady: f(1), UnitReady: f(0)}, domain.Warn, "its Reconcile Unit was not ready"},
		{"some pods not ready", domain.Sample{PodsReady: f(0.5), UnitReady: f(0)}, domain.Warn, "its Reconcile Unit was not ready"},
		{"a pod not ready alone", domain.Sample{PodsReady: f(0.5)}, domain.Warn, "a pod was not ready"},
		{"no pod ready", domain.Sample{PodsReady: f(0), UnitReady: f(0)}, domain.Crit, "no pod was ready"},
		{"a check failing", domain.Sample{PodsReady: f(0.5), ChecksUp: f(0)}, domain.Crit, "an uptime check failed"},
		{"a check failing beside no pod", domain.Sample{PodsReady: f(0), ChecksUp: f(0)}, domain.Crit, "no pod was ready"},
	} {
		t.Run(c.name, func(t *testing.T) {
			b := timeline(t, &metrics{samples: map[time.Time]domain.Sample{end: c.sample}})[119]
			if b.State != c.state || b.Detail != c.detail {
				t.Fatalf("block = %+v", b)
			}
		})
	}
}

func TestANameThatCannotBeAProjectHasNoTimeline(t *testing.T) {
	_, err := app.New(&metrics{}, time.Now).Timeline(t.Context(), "_x--/", nil)
	if !errors.Is(err, app.ErrNotAProject) {
		t.Fatalf("error = %v", err)
	}
}

func TestMetricsThatDoNotAnswerHaveNoTimeline(t *testing.T) {
	_, err := app.New(&metrics{err: domain.ErrNoMetrics}, time.Now).Timeline(t.Context(), "arr", nil)
	if !errors.Is(err, domain.ErrNoMetrics) {
		t.Fatalf("error = %v", err)
	}
}
