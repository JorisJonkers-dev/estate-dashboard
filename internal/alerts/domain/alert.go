package domain

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"
)

// Class is how loudly an alert wakes someone (deploy-kit, spec/v1, an Application's
// observability.alertClass), as the alert rules put it on the alert's `alert_class` label.
type Class string

const (
	BusinessHours Class = "business-hours"
	Urgent        Class = "urgent"
	Page          Class = "page"
	// Unclassed is an alert whose rule carries no class the dashboard knows.
	Unclassed Class = ""
)

// ClassLabel is the label an alert carries its class in.
const ClassLabel = "alert_class"

// classOf reads an alert's class, and an unknown one as Unclassed rather than as an error: an
// alert the dashboard cannot sort is still an alert.
func classOf(labels map[string]string) Class {
	switch c := Class(labels[ClassLabel]); c {
	case BusinessHours, Urgent, Page:
		return c
	case Unclassed:
		return Unclassed
	default:
		return Unclassed
	}
}

// Alert is one alert Alertmanager holds now: firing, and perhaps silenced.
type Alert struct {
	Fingerprint string
	Name        string
	Class       Class
	// Summary is the rule's `summary` annotation, where it carries one.
	Summary  string
	StartsAt time.Time
	// Labels is every label of the alert: what a silence of it matches on.
	Labels map[string]string
	// SilencedBy is the id of every silence that holds the alert quiet now.
	SilencedBy []string
}

// ErrIncompleteAlert is returned for an alert with no fingerprint, no name or no start.
var ErrIncompleteAlert = errors.New("an alert has a fingerprint, an alertname label and a start")

// NewAlert reads an alert as Alertmanager reports it: its name and class are labels.
func NewAlert(fingerprint string, labels, annotations map[string]string, startsAt time.Time, silencedBy []string) (Alert, error) {
	name := labels["alertname"]
	if fingerprint == "" || name == "" || startsAt.IsZero() {
		return Alert{}, fmt.Errorf("%w: %q", ErrIncompleteAlert, fingerprint)
	}
	return Alert{
		Fingerprint: fingerprint,
		Name:        name,
		Class:       classOf(labels),
		Summary:     annotations["summary"],
		StartsAt:    startsAt,
		Labels:      maps.Clone(labels),
		SilencedBy:  silencedBy,
	}, nil
}

// Length is how long an admin silences an alert for: the four choices the dashboard offers.
type Length string

const (
	Hour          Length = "1h"
	FourHours     Length = "4h"
	Day           Length = "1d"
	UntilResolved Length = "until-resolved"
)

// UntilResolvedAtMost is how long a silence until resolved lasts in Alertmanager, which wants an
// end: the dashboard expires it as soon as it sees the alert resolve, and this bounds a silence
// the dashboard was not running to expire.
const UntilResolvedAtMost = 7 * 24 * time.Hour

// ErrUnknownLength is returned for a length that is none of the four.
var ErrUnknownLength = errors.New("a silence lasts 1h, 4h, 1d or until-resolved")

// ParseLength reads a length.
func ParseLength(raw string) (Length, error) {
	switch Length(raw) {
	case Hour, FourHours, Day, UntilResolved:
		return Length(raw), nil
	default:
		return "", fmt.Errorf("%w: got %q", ErrUnknownLength, raw)
	}
}

// Ends says when a silence of this length, started at now, ends in Alertmanager, and whether the
// dashboard ends it sooner, when the alert resolves.
func (l Length) Ends(now time.Time) (time.Time, bool) {
	switch l {
	case Hour:
		return now.Add(time.Hour), false
	case FourHours:
		return now.Add(4 * time.Hour), false
	case Day:
		return now.Add(24 * time.Hour), false
	case UntilResolved:
		return now.Add(UntilResolvedAtMost), true
	default:
		return now, false
	}
}

// Silence is one silence the dashboard set in Alertmanager: its one write.
type Silence struct {
	// ID is Alertmanager's id of the silence.
	ID          string
	Fingerprint string
	CreatedBy   string
	CreatedAt   time.Time
	// EndsAt is nil for a silence that lasts until the alert resolves.
	EndsAt *time.Time
}

// SilenceRequest is what Alertmanager is asked to hold quiet: every label of the alert, matched
// exactly, from now until the end.
type SilenceRequest struct {
	Matchers  map[string]string
	StartsAt  time.Time
	EndsAt    time.Time
	CreatedBy string
	Comment   string
}

// ErrNotFiring is returned when the alert to silence is not one Alertmanager holds now.
var ErrNotFiring = errors.New("the alert is not firing")

// Alertmanager is the port the Alertmanager gateway implements.
type Alertmanager interface {
	// Alerts returns every alert Alertmanager holds now.
	Alerts(ctx context.Context) ([]Alert, error)
	// Silence creates a silence and returns its id.
	Silence(ctx context.Context, r SilenceRequest) (string, error)
	// Expire ends a silence now.
	Expire(ctx context.Context, id string) error
}

// Silences is the port the silences mirror implements.
type Silences interface {
	// Record mirrors a silence the dashboard created.
	Record(ctx context.Context, s Silence) error
	// UntilResolved returns the silences of fingerprint that last until it resolves and are not
	// expired yet.
	UntilResolved(ctx context.Context, fingerprint string) ([]Silence, error)
	// Expired records that a silence no longer holds, as of at.
	Expired(ctx context.Context, id string, at time.Time) error
}
