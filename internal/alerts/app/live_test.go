package app_test

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/app"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/domain"
)

var (
	start = time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)
	now   = start.Add(5 * time.Minute)
)

// alertmanager is an in-memory Alertmanager: what it holds, and every silence set or ended.
type alertmanager struct {
	alerts   []domain.Alert
	err      error
	silenced []domain.SilenceRequest
	expired  []string
	// silenceErr and expireErr fail those calls alone.
	silenceErr, expireErr error
}

func (a *alertmanager) Alerts(context.Context) ([]domain.Alert, error) { return a.alerts, a.err }

func (a *alertmanager) Silence(_ context.Context, r domain.SilenceRequest) (string, error) {
	if a.silenceErr != nil {
		return "", a.silenceErr
	}
	a.silenced = append(a.silenced, r)
	return "s-1", nil
}

func (a *alertmanager) Expire(_ context.Context, id string) error {
	if a.expireErr != nil {
		return a.expireErr
	}
	a.expired = append(a.expired, id)
	return nil
}

// history is an in-memory history: what Record kept, and what Open answers.
type history struct {
	recorded           []domain.Event
	open               []domain.Event
	openErr, recordErr error
}

func (*history) History(context.Context, int) ([]domain.Event, error) { return nil, nil }

func (h *history) Record(_ context.Context, e domain.Event) error {
	if h.recordErr != nil {
		return h.recordErr
	}
	h.recorded = append(h.recorded, e)
	return nil
}

func (h *history) Open(context.Context) ([]domain.Event, error) { return h.open, h.openErr }

// mirror is an in-memory silences mirror.
type mirror struct {
	recorded      []domain.Silence
	untilResolved map[string][]domain.Silence
	expired       []string
	recordErr     error
	listErr       error
	expireErr     error
}

func (m *mirror) Record(_ context.Context, s domain.Silence) error {
	if m.recordErr != nil {
		return m.recordErr
	}
	m.recorded = append(m.recorded, s)
	return nil
}

func (m *mirror) UntilResolved(_ context.Context, fingerprint string) ([]domain.Silence, error) {
	return m.untilResolved[fingerprint], m.listErr
}

func (m *mirror) Expired(_ context.Context, id string, at time.Time) error {
	if m.expireErr != nil {
		return m.expireErr
	}
	if !at.Equal(now) {
		return errors.New("expired at another time than now")
	}
	m.expired = append(m.expired, id)
	return nil
}

func alert(fingerprint string, started time.Time) domain.Alert {
	return domain.Alert{Fingerprint: fingerprint, Name: "A" + fingerprint, StartsAt: started, Labels: map[string]string{"alertname": "A" + fingerprint, "job": "x"}}
}

func live(am *alertmanager, h *history, m *mirror, log *strings.Builder) *app.Live {
	logger := slog.New(slog.DiscardHandler)
	if log != nil {
		logger = slog.New(slog.NewTextHandler(log, nil))
	}
	return app.NewLive(am, h, m, func() time.Time { return now.In(time.FixedZone("CEST", 7200)) }, logger)
}

func TestAlertsAreTheNewestFirstAndTiesByFingerprint(t *testing.T) {
	am := &alertmanager{alerts: []domain.Alert{alert("b", start), alert("old", start.Add(-time.Hour)), alert("new", start.Add(time.Hour)), alert("a", start)}}
	got, err := live(am, &history{}, &mirror{}, nil).Alerts(t.Context())
	var order []string
	for _, a := range got {
		order = append(order, a.Fingerprint)
	}
	if err != nil || !slices.Equal(order, []string{"new", "a", "b", "old"}) {
		t.Fatalf("order = %v, %v", order, err)
	}
	if _, err := live(&alertmanager{err: errors.New("away")}, &history{}, &mirror{}, nil).Alerts(t.Context()); err == nil {
		t.Fatal("an Alertmanager that is away answered")
	}
}

func TestASilenceMatchesEveryLabelOfTheAlertFiringNowAndIsMirrored(t *testing.T) {
	am := &alertmanager{alerts: []domain.Alert{alert("other", start), alert("9f2c", start)}}
	m := &mirror{}
	got, err := live(am, &history{}, m, nil).Silence(t.Context(), "joris", "9f2c", domain.FourHours)

	ends := now.Add(4 * time.Hour)
	want := domain.Silence{ID: "s-1", Fingerprint: "9f2c", CreatedBy: "joris", CreatedAt: now, EndsAt: &ends}
	if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(m.recorded, []domain.Silence{want}) {
		t.Fatalf("silence = %+v, %v; mirrored %+v", got, err, m.recorded)
	}
	asked := domain.SilenceRequest{Matchers: alert("9f2c", start).Labels, StartsAt: now, EndsAt: ends, CreatedBy: "joris", Comment: "Silenced from the estate dashboard for 4h."}
	if !reflect.DeepEqual(am.silenced, []domain.SilenceRequest{asked}) || am.silenced[0].StartsAt.Location() != time.UTC {
		t.Fatalf("asked Alertmanager for %+v", am.silenced)
	}
}

func TestASilenceUntilResolvedHasNoEndOfItsOwn(t *testing.T) {
	am := &alertmanager{alerts: []domain.Alert{alert("9f2c", start)}}
	m := &mirror{}
	got, err := live(am, &history{}, m, nil).Silence(t.Context(), "joris", "9f2c", domain.UntilResolved)
	if err != nil || got.EndsAt != nil || m.recorded[0].EndsAt != nil || !am.silenced[0].EndsAt.Equal(now.Add(domain.UntilResolvedAtMost)) {
		t.Fatalf("silence = %+v, %v; asked %+v", got, err, am.silenced)
	}
}

func TestASilenceIsRefusedForWhatIsNotFiringOrNotALength(t *testing.T) {
	am := &alertmanager{alerts: []domain.Alert{alert("9f2c", start)}}
	if _, err := live(am, &history{}, &mirror{}, nil).Silence(t.Context(), "joris", "gone", domain.Hour); !errors.Is(err, domain.ErrNotFiring) {
		t.Fatalf("not firing = %v", err)
	}
	if _, err := live(am, &history{}, &mirror{}, nil).Silence(t.Context(), "joris", "9f2c", "2h"); !errors.Is(err, domain.ErrUnknownLength) {
		t.Fatalf("no length = %v", err)
	}
	if _, err := live(&alertmanager{err: errors.New("away")}, &history{}, &mirror{}, nil).Silence(t.Context(), "joris", "9f2c", domain.Hour); err == nil || errors.Is(err, domain.ErrNotFiring) {
		t.Fatalf("Alertmanager away = %v", err)
	}
	refuses := &alertmanager{alerts: am.alerts, silenceErr: errors.New("refused")}
	if _, err := live(refuses, &history{}, &mirror{}, nil).Silence(t.Context(), "joris", "9f2c", domain.Hour); err == nil {
		t.Fatal("a silence Alertmanager refused was reported as set")
	}
	if am.silenced != nil {
		t.Fatalf("a refused silence reached Alertmanager: %+v", am.silenced)
	}
}

func TestASilenceTheMirrorMissesIsStillSetAndLogged(t *testing.T) {
	am := &alertmanager{alerts: []domain.Alert{alert("9f2c", start)}}
	var log strings.Builder
	got, err := live(am, &history{}, &mirror{recordErr: errors.New("db away")}, &log).Silence(t.Context(), "joris", "9f2c", domain.Hour)
	if err != nil || got.ID != "s-1" || !strings.Contains(log.String(), "silence set in Alertmanager and not mirrored") {
		t.Fatalf("silence = %+v, %v; log %q", got, err, log.String())
	}
}

func TestTheWatchRecordsWhatFiresAndWhatResolved(t *testing.T) {
	am := &alertmanager{alerts: []domain.Alert{alert("new", start)}}
	h := &history{open: []domain.Event{
		{Fingerprint: "new", Name: "Anew", Status: domain.Firing, StartsAt: start},
		{Fingerprint: "gone", Name: "Agone", Status: domain.Firing, StartsAt: start},
	}}
	m := &mirror{untilResolved: map[string][]domain.Silence{"gone": {{ID: "s-9"}}, "new": {{ID: "s-8"}}}}

	if err := live(am, h, m, nil).Watch(t.Context()); err != nil {
		t.Fatal(err)
	}

	firing := domain.Event{Fingerprint: "new", Name: "Anew", Status: domain.Firing, StartsAt: start, ObservedAt: now, Labels: alert("new", start).Labels}
	resolved := domain.Event{Fingerprint: "gone", Name: "Agone", Status: domain.Resolved, StartsAt: start, EndsAt: &now, ObservedAt: now}
	if !reflect.DeepEqual(h.recorded, []domain.Event{firing, resolved}) {
		t.Fatalf("recorded %+v", h.recorded)
	}
	// Only the resolved alert's silence until resolved ends; the firing one's holds.
	if !slices.Equal(am.expired, []string{"s-9"}) || !slices.Equal(m.expired, []string{"s-9"}) {
		t.Fatalf("expired %v in Alertmanager, %v in the mirror", am.expired, m.expired)
	}
}

func TestALaterFiringKeepsTheSilenceOfAnEarlierOne(t *testing.T) {
	// The alert resolved and fired again between two looks: the earlier firing resolves, and its
	// silence until resolved holds, since the alert it matches is firing again.
	am := &alertmanager{alerts: []domain.Alert{alert("9f2c", start.Add(time.Minute))}}
	h := &history{open: []domain.Event{{Fingerprint: "9f2c", Name: "A9f2c", Status: domain.Firing, StartsAt: start}}}
	m := &mirror{untilResolved: map[string][]domain.Silence{"9f2c": {{ID: "s-1"}}}}

	if err := live(am, h, m, nil).Watch(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(h.recorded) != 2 || h.recorded[1].Status != domain.Resolved || am.expired != nil {
		t.Fatalf("recorded %+v, expired %v", h.recorded, am.expired)
	}
}

func TestTheWatchGoesOnPastOneFailureAndReportsEach(t *testing.T) {
	gone := []domain.Event{{Fingerprint: "a", Name: "Aa", Status: domain.Firing, StartsAt: start}, {Fingerprint: "b", Name: "Ab", Status: domain.Firing, StartsAt: start}}
	both := map[string][]domain.Silence{"a": {{ID: "s-a"}}, "b": {{ID: "s-b"}}}
	cases := map[string]struct {
		am   *alertmanager
		h    *history
		m    *mirror
		want int
	}{
		"Alertmanager away":              {&alertmanager{err: errors.New("away")}, &history{}, &mirror{}, 1},
		"the history away":               {&alertmanager{}, &history{openErr: errors.New("away")}, &mirror{}, 1},
		"a record that fails":            {&alertmanager{alerts: []domain.Alert{alert("x", start)}}, &history{open: gone, recordErr: errors.New("full")}, &mirror{untilResolved: both}, 3},
		"the mirror away":                {&alertmanager{}, &history{open: gone}, &mirror{untilResolved: both, listErr: errors.New("away")}, 2},
		"an expiry Alertmanager refuses": {&alertmanager{expireErr: errors.New("no")}, &history{open: gone}, &mirror{untilResolved: both}, 2},
		"an expiry the mirror misses":    {&alertmanager{}, &history{open: gone}, &mirror{untilResolved: both, expireErr: errors.New("away")}, 2},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := live(tc.am, tc.h, tc.m, nil).Watch(t.Context())
			var joined interface{ Unwrap() []error }
			count := 1
			if errors.As(err, &joined) {
				count = len(joined.Unwrap())
			}
			if err == nil || count != tc.want {
				t.Fatalf("watch = %v (%d errors), want %d", err, count, tc.want)
			}
		})
	}
}
