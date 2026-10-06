// Package domain is the alerts context's pure core: its value objects, entities and ports. It
// imports no transport, persistence or generated code (enforced by depguard).
package domain

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Status is a state an alert was seen in.
type Status string

const (
	Firing   Status = "firing"
	Resolved Status = "resolved"
)

// ErrUnknownStatus is returned for a status that is neither firing nor resolved.
var ErrUnknownStatus = errors.New("an alert is firing or resolved")

// ErrInconsistentEvent is returned for an event whose times contradict its status.
var ErrInconsistentEvent = errors.New("a resolved alert has an end, at or after its start, and a firing one has none")

// ParseStatus reads a stored status. db/migrations holds the same rule as a CHECK.
func ParseStatus(raw string) (Status, error) {
	switch Status(raw) {
	case Firing, Resolved:
		return Status(raw), nil
	default:
		return "", fmt.Errorf("%w: got %q", ErrUnknownStatus, raw)
	}
}

// Event is one state an alert was seen in: it fired, or it resolved. An alert is its
// fingerprint, and one firing of it is the fingerprint with when it started.
type Event struct {
	ID          int64
	Fingerprint string
	Name        string
	Status      Status
	StartsAt    time.Time
	// EndsAt is set exactly when the alert resolved.
	EndsAt     *time.Time
	ObservedAt time.Time
	// Labels is the alert's label set when it was seen, kept with a firing; History does not
	// read it back.
	Labels map[string]string
}

// NewEvent checks the rules an event's status and times obey together.
func NewEvent(e Event) (Event, error) {
	if _, err := ParseStatus(string(e.Status)); err != nil {
		return Event{}, err
	}
	resolved := e.Status == Resolved
	if resolved != (e.EndsAt != nil) || (e.EndsAt != nil && e.EndsAt.Before(e.StartsAt)) {
		return Event{}, fmt.Errorf("%w: alert %s", ErrInconsistentEvent, e.Fingerprint)
	}
	return e, nil
}

// Repository is the port the persistence adapter implements.
type Repository interface {
	// History returns at most limit events, newest first.
	History(ctx context.Context, limit int) ([]Event, error)
	// Record keeps one event; an event already kept is kept once.
	Record(ctx context.Context, e Event) error
	// Open returns every firing recorded with no resolution recorded after it.
	Open(ctx context.Context) ([]Event, error)
}
