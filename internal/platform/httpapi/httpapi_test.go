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

func serve(t *testing.T, repo *memory) *httptest.Server {
	t.Helper()
	h, err := httpapi.New(slog.New(slog.DiscardHandler), gate{}, app.New(repo))
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

func TestTheSessionIsTheAdminTheCookieBelongsTo(t *testing.T) {
	srv := serve(t, &memory{})
	got := call(t, srv, http.MethodGet, "/api/v1/session", "", true)
	if got.code != http.StatusOK || got.body["subject"] != "user-1" || got.body["name"] != "joris" || len(got.body) != 2 {
		t.Fatalf("GET /api/v1/session = %d %v", got.code, got.body)
	}
}

func TestOnlyALiveAdminSessionReachesAnOperation(t *testing.T) {
	srv := serve(t, &memory{})
	for _, path := range []string{"/api/v1/session", "/api/v1/alerts/history"} {
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
