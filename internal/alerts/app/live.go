package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/domain"
)

// Live runs the use cases that read and write Alertmanager: what is firing now, silencing one
// alert, and the watch that records each firing and resolution and ends a silence that lasts
// until its alert resolves.
type Live struct {
	am       domain.Alertmanager
	history  domain.Repository
	silences domain.Silences
	now      func() time.Time
	logger   *slog.Logger
}

// NewLive returns the Alertmanager use cases.
func NewLive(am domain.Alertmanager, history domain.Repository, silences domain.Silences, now func() time.Time, logger *slog.Logger) *Live {
	return &Live{am: am, history: history, silences: silences, now: now, logger: logger}
}

// Alerts returns every alert firing now, the newest first.
func (l *Live) Alerts(ctx context.Context) ([]domain.Alert, error) {
	alerts, err := l.am.Alerts(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the alerts: %w", err)
	}
	slices.SortStableFunc(alerts, func(a, b domain.Alert) int {
		if c := b.StartsAt.Compare(a.StartsAt); c != 0 {
			return c
		}
		return strings.Compare(a.Fingerprint, b.Fingerprint)
	})
	return alerts, nil
}

// Silence silences the firing alert fingerprint for length, as by, and mirrors what it set.
func (l *Live) Silence(ctx context.Context, by, fingerprint string, length domain.Length) (domain.Silence, error) {
	if _, err := domain.ParseLength(string(length)); err != nil {
		return domain.Silence{}, err
	}
	alerts, err := l.am.Alerts(ctx)
	if err != nil {
		return domain.Silence{}, fmt.Errorf("read the alerts: %w", err)
	}
	i := slices.IndexFunc(alerts, func(a domain.Alert) bool { return a.Fingerprint == fingerprint })
	if i < 0 {
		return domain.Silence{}, fmt.Errorf("%w: %s", domain.ErrNotFiring, fingerprint)
	}
	alert := alerts[i]
	now := l.now().UTC()
	ends, untilResolved := length.Ends(now)
	id, err := l.am.Silence(ctx, domain.SilenceRequest{
		Matchers:  alert.Labels,
		StartsAt:  now,
		EndsAt:    ends,
		CreatedBy: by,
		Comment:   fmt.Sprintf("Silenced from the estate dashboard for %s.", length),
	})
	if err != nil {
		return domain.Silence{}, fmt.Errorf("silence %s: %w", fingerprint, err)
	}
	silence := domain.Silence{ID: id, Fingerprint: fingerprint, CreatedBy: by, CreatedAt: now}
	if !untilResolved {
		silence.EndsAt = &ends
	}
	// Alertmanager holds the silence whether or not the mirror does: a failure here is logged,
	// and the silence is still set.
	if err := l.silences.Record(ctx, silence); err != nil {
		l.logger.ErrorContext(ctx, "silence set in Alertmanager and not mirrored", "silence", id, "fingerprint", fingerprint, "error", err)
	}
	return silence, nil
}

// Watch looks at Alertmanager once: it records every alert firing now that is not recorded yet,
// records as resolved every recorded firing Alertmanager no longer holds, and expires each
// silence that lasted until such an alert resolved. One failure does not stop the rest.
func (l *Live) Watch(ctx context.Context) error {
	now := l.now().UTC()
	alerts, err := l.am.Alerts(ctx)
	if err != nil {
		return fmt.Errorf("read the alerts: %w", err)
	}
	open, err := l.history.Open(ctx)
	if err != nil {
		return fmt.Errorf("read the open firings: %w", err)
	}
	firing := map[string]bool{}
	var failed []error
	for _, a := range alerts {
		firing[key(a.Fingerprint, a.StartsAt)] = true
		e := domain.Event{Fingerprint: a.Fingerprint, Name: a.Name, Status: domain.Firing, StartsAt: a.StartsAt, ObservedAt: now, Labels: a.Labels}
		if err := l.history.Record(ctx, e); err != nil {
			failed = append(failed, err)
		}
	}
	for _, e := range open {
		if firing[key(e.Fingerprint, e.StartsAt)] {
			continue
		}
		resolved := domain.Event{Fingerprint: e.Fingerprint, Name: e.Name, Status: domain.Resolved, StartsAt: e.StartsAt, EndsAt: &now, ObservedAt: now}
		if err := l.history.Record(ctx, resolved); err != nil {
			failed = append(failed, err)
			continue
		}
		failed = append(failed, l.endSilences(ctx, e.Fingerprint, alerts, now)...)
	}
	return errors.Join(failed...)
}

// endSilences expires the silences of a resolved alert that lasted until it resolved, unless a
// later firing of the same alert is firing now.
func (l *Live) endSilences(ctx context.Context, fingerprint string, alerts []domain.Alert, now time.Time) []error {
	if slices.ContainsFunc(alerts, func(a domain.Alert) bool { return a.Fingerprint == fingerprint }) {
		return nil
	}
	silences, err := l.silences.UntilResolved(ctx, fingerprint)
	if err != nil {
		return []error{err}
	}
	var failed []error
	for _, s := range silences {
		if err := l.am.Expire(ctx, s.ID); err != nil {
			failed = append(failed, fmt.Errorf("expire silence %s: %w", s.ID, err))
			continue
		}
		if err := l.silences.Expired(ctx, s.ID, now); err != nil {
			failed = append(failed, err)
		}
	}
	return failed
}

func key(fingerprint string, startsAt time.Time) string {
	return fingerprint + "@" + startsAt.UTC().Format(time.RFC3339Nano)
}
