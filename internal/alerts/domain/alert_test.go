package domain_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/domain"
)

func TestNewAlertReadsItsNameClassAndSummaryFromWhatItCarries(t *testing.T) {
	start := time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)
	labels := map[string]string{"alertname": "ReleaseHeld", "alert_class": "urgent", "namespace": "auth-system"}
	got, err := domain.NewAlert("9f2c", labels, map[string]string{"summary": "auth is held."}, start, []string{"s-1"})
	want := domain.Alert{Fingerprint: "9f2c", Name: "ReleaseHeld", Class: domain.Urgent, Summary: "auth is held.", StartsAt: start, Labels: labels, SilencedBy: []string{"s-1"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("NewAlert = %+v, %v", got, err)
	}
	// The alert keeps its own copy of the labels.
	labels["namespace"] = "elsewhere"
	if got.Labels["namespace"] != "auth-system" {
		t.Fatal("the alert's labels changed with the caller's map")
	}
	for class, want := range map[string]domain.Class{"page": domain.Page, "business-hours": domain.BusinessHours, "": domain.Unclassed, "critical": domain.Unclassed} {
		a, err := domain.NewAlert("1", map[string]string{"alertname": "A", "alert_class": class}, nil, start, nil)
		if err != nil || a.Class != want || a.Summary != "" {
			t.Errorf("class %q read as %q, %v", class, a.Class, err)
		}
	}
	for name, a := range map[string]struct {
		fingerprint string
		labels      map[string]string
		start       time.Time
	}{
		"no fingerprint": {"", map[string]string{"alertname": "A"}, start},
		"no name":        {"1", map[string]string{}, start},
		"no start":       {"1", map[string]string{"alertname": "A"}, time.Time{}},
	} {
		if _, err := domain.NewAlert(a.fingerprint, a.labels, nil, a.start, nil); !errors.Is(err, domain.ErrIncompleteAlert) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestALengthSaysWhenItsSilenceEnds(t *testing.T) {
	now := time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)
	for raw, want := range map[string]struct {
		after         time.Duration
		untilResolved bool
	}{
		"1h": {time.Hour, false}, "4h": {4 * time.Hour, false}, "1d": {24 * time.Hour, false},
		"until-resolved": {domain.UntilResolvedAtMost, true},
	} {
		length, err := domain.ParseLength(raw)
		if err != nil {
			t.Fatal(err)
		}
		ends, untilResolved := length.Ends(now)
		if !ends.Equal(now.Add(want.after)) || untilResolved != want.untilResolved {
			t.Errorf("%s ends %v, %v", raw, ends, untilResolved)
		}
	}
	if domain.UntilResolvedAtMost != 7*24*time.Hour {
		t.Fatalf("until resolved lasts at most %v", domain.UntilResolvedAtMost)
	}
	for _, raw := range []string{"", "2h", "1H", "forever"} {
		if _, err := domain.ParseLength(raw); !errors.Is(err, domain.ErrUnknownLength) {
			t.Errorf("ParseLength(%q) = %v", raw, err)
		}
	}
	if ends, untilResolved := domain.Length("2h").Ends(now); !ends.Equal(now) || untilResolved {
		t.Fatalf("a length that is none of the four ends at %v, %v", ends, untilResolved)
	}
}
