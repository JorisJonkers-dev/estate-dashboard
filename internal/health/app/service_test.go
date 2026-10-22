package app_test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/health/app"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/health/domain"
)

// metrics answers with samples, and records what it was asked.
type metrics struct {
	readiness domain.Series
	weights   domain.Series
	jobs      []domain.JobPod
	samples   map[time.Time]domain.Sample
	err       error
	asked     []string
	from      time.Time
	to        time.Time
}

func (m *metrics) Samples(_ context.Context, project string, checks []string, start, end time.Time, _ time.Duration) (map[time.Time]domain.Sample, error) {
	m.asked = append([]string{project}, checks...)
	m.from, m.to = start, end
	return m.samples, m.err
}

func (m *metrics) Rollout(_ context.Context, project string, start, end time.Time, _ time.Duration) (domain.Observed, error) {
	m.asked = []string{project}
	m.from, m.to = start, end
	return domain.Observed{Readiness: m.readiness, Weights: m.weights, Jobs: m.jobs}, m.err
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

func at(seconds int) time.Time { return time.Date(2026, 10, 20, 9, 0, seconds, 0, time.UTC) }

func rollout(t *testing.T, m *metrics) domain.Window {
	t.Helper()
	w, err := app.New(m, time.Now).Rollout(t.Context(), "auth", at(0), at(60))
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestAPodsSpanIsWhenItWasFirstSeenFirstReadyAndLastSeen(t *testing.T) {
	w := rollout(t, &metrics{readiness: domain.Series{
		"auth-api-primary-5b6d7c8f9-z4w8n": {at(0): 1, at(5): 1, at(10): 1},
		"auth-api-7f8c9d6b5-kq2lm":         {at(20): 0, at(25): 0, at(30): 1, at(60): 1},
	}})
	if len(w.Pods) != 2 {
		t.Fatalf("pods = %+v", w.Pods)
	}
	old, fresh := w.Pods[0], w.Pods[1]
	if old.Process != "auth-api" || !old.Start.Equal(at(0)) || !old.End.Equal(at(10)) || fresh.End != nil {
		t.Fatalf("old = %+v, new = %+v", old, fresh)
	}
	if !fresh.Start.Equal(at(20)) || !fresh.Ready.Equal(at(30)) || len(fresh.Checks) != 4 || fresh.Checks[0].Ready {
		t.Fatalf("new = %+v", fresh)
	}
}

func TestAPodThatNeverTurnedReadyHasNoReadyTime(t *testing.T) {
	w := rollout(t, &metrics{readiness: domain.Series{"radarr-566dd79995-jc5mr": {at(50): 0, at(55): 0, at(60): 0}}})
	if w.Pods[0].Ready != nil || w.Pods[0].Process != "radarr" {
		t.Fatalf("pod = %+v", w.Pods[0])
	}
}

func TestPodsAreByProcessThenWhenTheyStarted(t *testing.T) {
	w := rollout(t, &metrics{readiness: domain.Series{
		"b-0": {at(0): 1}, "a-2": {at(10): 1}, "a-1": {at(10): 1}, "a-0": {at(5): 1},
	}})
	var got []string
	for _, p := range w.Pods {
		got = append(got, p.Pod)
	}
	if want := []string{"a-0", "a-1", "a-2", "b-0"}; !slices.Equal(got, want) {
		t.Fatalf("pods = %v", got)
	}
}

func TestAPodsProcessIsItsNameWithoutWhatKubernetesAndFlaggerAdd(t *testing.T) {
	for pod, process := range map[string]string{
		"notes-primary-6d9f7c5b8-2hxlq": "notes",
		"radarr-566dd79995-jc5mr":       "radarr",
		"postgres-0":                    "postgres",
		"auth-migration-up-3-4-2-h3j4k": "auth-migration-up-3-4-2",
		"standalone":                    "standalone",
	} {
		t.Run(pod, func(t *testing.T) {
			if got := domain.ProcessOf(pod); got != process {
				t.Fatalf("ProcessOf(%q) = %q", pod, got)
			}
		})
	}
}

func TestACanarysWeightIsEachChangeFlaggerMade(t *testing.T) {
	w := rollout(t, &metrics{weights: domain.Series{
		"auth-api": {at(0): 0, at(5): 0, at(10): 20, at(15): 20, at(20): 40},
		"auth-ui":  {at(10): 0},
	}})
	var got []string
	for _, c := range w.Weights {
		got = append(got, c.Process+" "+strconv.Itoa(c.Weight)+"@"+strconv.Itoa(c.At.Second()))
	}
	if want := []string{"auth-api 0@0", "auth-api 20@10", "auth-ui 0@10", "auth-api 40@20"}; !slices.Equal(got, want) {
		t.Fatalf("weights = %v", got)
	}
}

func TestAWindowThatEndsBeforeItStartsOrLastsTooLongIsRefused(t *testing.T) {
	s := app.New(&metrics{}, time.Now)
	for name, end := range map[string]time.Time{"backwards": at(0).Add(-time.Second), "too long": at(0).Add(3 * time.Hour)} {
		if _, err := s.Rollout(t.Context(), "auth", at(0), end); !errors.Is(err, app.ErrBadWindow) {
			t.Fatalf("%s: error = %v", name, err)
		}
	}
	if _, err := s.Rollout(t.Context(), "_x--/", at(0), at(60)); !errors.Is(err, app.ErrNotAProject) {
		t.Fatalf("error = %v", err)
	}
}

func TestMetricsThatDoNotAnswerHaveNoRollout(t *testing.T) {
	if _, err := app.New(&metrics{err: domain.ErrNoMetrics}, time.Now).Rollout(t.Context(), "auth", at(0), at(60)); !errors.Is(err, domain.ErrNoMetrics) {
		t.Fatalf("error = %v", err)
	}
}

func TestARolloutCarriesTheRunsOfItsJobsInTheWindow(t *testing.T) {
	w := rollout(t, &metrics{jobs: []domain.JobPod{
		{Pod: "vault-policy-1a2b-x7k2p", Job: "vault-policy-1a2b", Phases: map[time.Time]string{at(10): "Running", at(15): "Succeeded"}},
		{Pod: "vault-policy-29877720-q8w2n", Job: "vault-policy-29877720", CronJob: "vault-policy", Phases: map[time.Time]string{at(0): "Succeeded"}},
	}})
	if len(w.Runs) != 1 || w.Runs[0].Job != "vault-policy-1a2b" || w.Runs[0].Outcome != domain.RunSucceeded || !w.Runs[0].End.Equal(at(15)) {
		t.Fatalf("runs = %+v", w.Runs)
	}
}
