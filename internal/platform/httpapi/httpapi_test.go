package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/app"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/httpapi"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/oidc"
)

// signedIn is the session cookie value the gate below admits; any other is nobody's.
const signedIn = "sealed-session"

type gate struct{}

func (gate) Admin(_ context.Context, cookie string) (oidc.Admin, bool) {
	return oidc.Admin{Sub: "user-1", Name: "joris"}, cookie == signedIn
}

// memory is an in-memory domain.Repository, so these tests exercise the contract, not Postgres.
type memory struct {
	events []domain.Event
	err    error
}

func (m *memory) History(_ context.Context, limit int) ([]domain.Event, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.events[:min(limit, len(m.events))], nil
}

func (*memory) Record(context.Context, domain.Event) error { return nil }

func (*memory) Open(context.Context) ([]domain.Event, error) { return nil, nil }

// live stands in for the Alertmanager use cases: what they return, and what they were asked.
type live struct {
	alerts  []domain.Alert
	silence domain.Silence
	err     error
	// asked is who silenced which alert for how long.
	asked []string
}

func (l *live) Alerts(context.Context) ([]domain.Alert, error) { return l.alerts, l.err }

func (l *live) Silence(_ context.Context, by, fingerprint string, length domain.Length) (domain.Silence, error) {
	l.asked = append(l.asked, by+" "+fingerprint+" "+string(length))
	return l.silence, l.err
}

func serve(t *testing.T, repo *memory) *httptest.Server {
	t.Helper()
	return serveLive(t, repo, &live{})
}

func serveLive(t *testing.T, repo *memory, l *live) *httptest.Server {
	t.Helper()
	h, err := httpapi.New(slog.New(slog.DiscardHandler), gate{}, app.New(repo), l)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

type response struct {
	code   int
	header http.Header
	body   map[string]any
}

func call(t *testing.T, srv *httptest.Server, method, path, body string, identified bool) response {
	t.Helper()
	cookie := ""
	if identified {
		cookie = signedIn
	}
	return callWith(t, srv, method, path, body, cookie)
}

// callWith sends cookie as the session cookie, or none when it is empty.
func callWith(t *testing.T, srv *httptest.Server, method, path, body, cookie string) response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		req.Header.Set("Cookie", oidc.SessionCookie+"="+cookie)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	out := response{code: resp.StatusCode, header: resp.Header, body: map[string]any{}}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out.body); err != nil {
			t.Fatalf("%s %s: body is not JSON: %s", method, path, raw)
		}
	}
	return out
}

func TestHistoryIsServedInTheContractsShape(t *testing.T) {
	start := time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	srv := serve(t, &memory{events: []domain.Event{
		{ID: 2, Fingerprint: "9f2c", Name: "CollectorStopped", Status: domain.Resolved, StartsAt: start, EndsAt: &end, ObservedAt: end},
		{ID: 1, Fingerprint: "9f2c", Name: "CollectorStopped", Status: domain.Firing, StartsAt: start, ObservedAt: start},
	}})

	listed := call(t, srv, http.MethodGet, "/api/v1/alerts/history?limit=10", "", true)
	items, _ := listed.body["items"].([]any)
	if listed.code != http.StatusOK || len(items) != 2 {
		t.Fatalf("list = %d %v", listed.code, listed.body)
	}
	resolved, _ := items[0].(map[string]any)
	firing, _ := items[1].(map[string]any)
	if resolved["status"] != "resolved" || resolved["endsAt"] != "2026-10-03T07:30:00Z" || resolved["startsAt"] != "2026-10-03T07:00:00Z" {
		t.Fatalf("the resolved event = %v", resolved)
	}
	if _, has := firing["endsAt"]; has || firing["status"] != "firing" || firing["id"] != float64(1) {
		t.Fatalf("the firing event = %v", firing)
	}

	one := call(t, srv, http.MethodGet, "/api/v1/alerts/history?limit=1", "", true)
	if items, _ := one.body["items"].([]any); len(items) != 1 {
		t.Fatalf("limit=1 returned %v", one.body)
	}
}

func TestTheAlertsFiringNowAreServedInTheContractsShape(t *testing.T) {
	start := time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)
	srv := serveLive(t, &memory{}, &live{alerts: []domain.Alert{
		{Fingerprint: "9f2c", Name: "ReleaseHeld", Class: domain.Urgent, Summary: "auth is held.", StartsAt: start, Labels: map[string]string{"alertname": "ReleaseHeld"}, SilencedBy: []string{"s-1"}},
		{Fingerprint: "1a2b", Name: "Odd", StartsAt: start, Labels: map[string]string{"alertname": "Odd"}},
	}})

	got := call(t, srv, http.MethodGet, "/api/v1/alerts", "", true)
	items, _ := got.body["items"].([]any)
	if got.code != http.StatusOK || len(items) != 2 {
		t.Fatalf("list = %d %v", got.code, got.body)
	}
	held, _ := items[0].(map[string]any)
	odd, _ := items[1].(map[string]any)
	if held["class"] != "urgent" || held["summary"] != "auth is held." || held["startsAt"] != "2026-10-03T07:00:00Z" || len(held["silencedBy"].([]any)) != 1 {
		t.Fatalf("a classed, silenced alert = %v", held)
	}
	_, hasClass := odd["class"]
	_, hasSummary := odd["summary"]
	if hasClass || hasSummary || len(odd["silencedBy"].([]any)) != 0 || odd["labels"].(map[string]any)["alertname"] != "Odd" {
		t.Fatalf("an unclassed alert = %v", odd)
	}
}

func TestAnAlertmanagerThatDoesNotAnswerIsA503(t *testing.T) {
	srv := serveLive(t, &memory{}, &live{err: errors.New("connection refused")})
	for _, req := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/alerts", ""},
		{http.MethodPost, "/api/v1/alerts/9f2c/silences", `{"length":"1h"}`},
	} {
		got := call(t, srv, req.method, req.path, req.body, true)
		if got.code != http.StatusServiceUnavailable || got.body["detail"] != "Alertmanager did not answer." || got.header.Get("Content-Type") != "application/problem+json" {
			t.Fatalf("%s %s = %d %v", req.method, req.path, got.code, got.body)
		}
	}
}

func TestAnAdminSilencesAnAlertAsThemselves(t *testing.T) {
	created := time.Date(2026, 10, 3, 7, 5, 0, 0, time.UTC)
	ends := created.Add(4 * time.Hour)
	l := &live{silence: domain.Silence{ID: "s-1", Fingerprint: "9f2c", CreatedBy: "joris", CreatedAt: created, EndsAt: &ends}}
	srv := serveLive(t, &memory{}, l)

	got := call(t, srv, http.MethodPost, "/api/v1/alerts/9f2c/silences", `{"length":"4h"}`, true)
	if got.code != http.StatusCreated || got.body["id"] != "s-1" || got.body["endsAt"] != "2026-10-03T11:05:00Z" || got.body["createdBy"] != "joris" {
		t.Fatalf("silence = %d %v", got.code, got.body)
	}
	if len(l.asked) != 1 || l.asked[0] != "joris 9f2c 4h" {
		t.Fatalf("asked %v", l.asked)
	}

	l.silence.EndsAt = nil
	untilResolved := call(t, srv, http.MethodPost, "/api/v1/alerts/9f2c/silences", `{"length":"until-resolved"}`, true)
	if _, has := untilResolved.body["endsAt"]; untilResolved.code != http.StatusCreated || has {
		t.Fatalf("until resolved = %d %v", untilResolved.code, untilResolved.body)
	}
}

func TestASilenceOfNothingFiringOrOfNoLengthIsRefused(t *testing.T) {
	srv := serveLive(t, &memory{}, &live{err: domain.ErrNotFiring})
	if got := call(t, srv, http.MethodPost, "/api/v1/alerts/9f2c/silences", `{"length":"1d"}`, true); got.code != http.StatusNotFound || got.body["detail"] != "The alert is not firing." {
		t.Fatalf("not firing = %d %v", got.code, got.body)
	}
	unknown := serveLive(t, &memory{}, &live{err: domain.ErrUnknownLength})
	if got := call(t, unknown, http.MethodPost, "/api/v1/alerts/9f2c/silences", `{"length":"1d"}`, true); got.code != http.StatusBadRequest {
		t.Fatalf("a length the use case refuses = %d %v", got.code, got.body)
	}
	l := &live{}
	strict := serveLive(t, &memory{}, l)
	for _, body := range []string{`{"length":"2h"}`, `{}`, `{"length":"1h","by":"x"}`} {
		if got := call(t, strict, http.MethodPost, "/api/v1/alerts/9f2c/silences", body, true); got.code != http.StatusBadRequest {
			t.Fatalf("%s = %d %v", body, got.code, got.body)
		}
	}
	if got := call(t, strict, http.MethodPost, "/api/v1/alerts/NOT-HEX/silences", `{"length":"1h"}`, true); got.code != http.StatusBadRequest {
		t.Fatalf("a fingerprint that is no fingerprint = %d", got.code)
	}
	if l.asked != nil {
		t.Fatalf("a refused request reached the use case: %v", l.asked)
	}
}

func TestTheSessionIsTheAdminTheCookieBelongsTo(t *testing.T) {
	srv := serve(t, &memory{})
	got := call(t, srv, http.MethodGet, "/api/v1/session", "", true)
	if got.code != http.StatusOK || got.body["subject"] != "user-1" || got.body["name"] != "joris" || len(got.body) != 2 {
		t.Fatalf("GET /api/v1/session = %d %v", got.code, got.body)
	}
}

func TestOnlyALiveAdminSessionReachesAnOperation(t *testing.T) {
	srv := serve(t, &memory{})
	for _, path := range []string{"/api/v1/session", "/api/v1/alerts/history", "/api/v1/alerts"} {
		for name, cookie := range map[string]string{"no cookie": "", "a cookie the gate does not admit": "someone-elses"} {
			got := callWith(t, srv, http.MethodGet, path, "", cookie)
			if got.code != http.StatusUnauthorized || got.body["status"] != float64(http.StatusUnauthorized) || got.body["detail"] != nil {
				t.Fatalf("%s with %s = %d %v, want a bare 401 problem", path, name, got.code, got.body)
			}
			if ct := got.header.Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("Content-Type = %q", ct)
			}
		}
	}
}

func TestProblems(t *testing.T) {
	srv := serve(t, &memory{})
	cases := []struct {
		name, method, path, body string
		identified               bool
		want                     int
	}{
		{"limit below the range", http.MethodGet, "/api/v1/alerts/history?limit=0", "", true, http.StatusBadRequest},
		{"limit above the range", http.MethodGet, "/api/v1/alerts/history?limit=101", "", true, http.StatusBadRequest},
		{"limit that is not a number", http.MethodGet, "/api/v1/alerts/history?limit=all", "", true, http.StatusBadRequest},
		{"no identity", http.MethodGet, "/api/v1/alerts/history", "", false, http.StatusUnauthorized},
		{"unknown path", http.MethodGet, "/api/v1/nope", "", true, http.StatusNotFound},
		{"wrong method", http.MethodDelete, "/api/v1/alerts/history", "", true, http.StatusMethodNotAllowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := call(t, srv, tc.method, tc.path, tc.body, tc.identified)
			if got.code != tc.want || got.body["status"] != float64(tc.want) {
				t.Fatalf("%s %s = %d %v, want a %d problem", tc.method, tc.path, got.code, got.body, tc.want)
			}
			if ct := got.header.Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("Content-Type = %q", ct)
			}
		})
	}
}

func TestFailureIsAnOpaque500(t *testing.T) {
	srv := serve(t, &memory{err: errors.New("connection refused to 10.0.0.7")})
	got := call(t, srv, http.MethodGet, "/api/v1/alerts/history", "", true)
	if got.code != http.StatusInternalServerError || got.body["detail"] != nil {
		t.Fatalf("GET = %d %v, want a 500 without detail", got.code, got.body)
	}
}
