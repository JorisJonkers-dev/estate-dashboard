package domain_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/domain"
)

func TestParseStatus(t *testing.T) {
	for _, raw := range []string{"firing", "resolved"} {
		got, err := domain.ParseStatus(raw)
		if err != nil || string(got) != raw {
			t.Errorf("ParseStatus(%q) = %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"", "Firing", "suppressed"} {
		if _, err := domain.ParseStatus(raw); !errors.Is(err, domain.ErrUnknownStatus) {
			t.Errorf("ParseStatus(%q) error = %v, want ErrUnknownStatus", raw, err)
		}
	}
}

func TestNewEvent(t *testing.T) {
	start := time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)
	later, earlier := start.Add(time.Hour), start.Add(-time.Hour)

	accepted := map[string]domain.Event{
		"a firing alert":                         {Fingerprint: "a", Status: domain.Firing, StartsAt: start},
		"a resolved alert":                       {Fingerprint: "a", Status: domain.Resolved, StartsAt: start, EndsAt: &later},
		"an alert resolved the instant it fired": {Fingerprint: "a", Status: domain.Resolved, StartsAt: start, EndsAt: &start},
	}
	for name, event := range accepted {
		got, err := domain.NewEvent(event)
		if err != nil || !reflect.DeepEqual(got, event) {
			t.Errorf("%s: NewEvent = %+v, %v", name, got, err)
		}
	}

	refused := map[string]struct {
		event domain.Event
		want  error
	}{
		"a status that is not one":            {domain.Event{Fingerprint: "a", Status: "pending", StartsAt: start}, domain.ErrUnknownStatus},
		"a firing alert with an end":          {domain.Event{Fingerprint: "a", Status: domain.Firing, StartsAt: start, EndsAt: &later}, domain.ErrInconsistentEvent},
		"a resolved alert with no end":        {domain.Event{Fingerprint: "a", Status: domain.Resolved, StartsAt: start}, domain.ErrInconsistentEvent},
		"an alert that ends before it starts": {domain.Event{Fingerprint: "a", Status: domain.Resolved, StartsAt: start, EndsAt: &earlier}, domain.ErrInconsistentEvent},
	}
	for name, c := range refused {
		if _, err := domain.NewEvent(c.event); !errors.Is(err, c.want) {
			t.Errorf("%s: error = %v, want %v", name, err, c.want)
		}
	}
}
