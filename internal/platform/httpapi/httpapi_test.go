package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/app"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/adapters/cluster"
	deliveryweb "github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/adapters/web"
	deliveryapp "github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/app"
	delivery "github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/estate/adapters/github"
	estateweb "github.com/JorisJonkers-dev/estate-dashboard/internal/estate/adapters/web"
	estateapp "github.com/JorisJonkers-dev/estate-dashboard/internal/estate/app"
	estatedomain "github.com/JorisJonkers-dev/estate-dashboard/internal/estate/domain"
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
	return serveAll(t, repo, l, deliveryapp.New(cluster.Absent{}))
}

func serveAll(t *testing.T, repo *memory, l *live, delivery deliveryweb.UseCases) *httptest.Server {
	t.Helper()
	return serveEstate(t, repo, l, delivery, estateapp.New(github.Absent{}))
}

func serveEstate(t *testing.T, repo *memory, l *live, delivery deliveryweb.UseCases, estate estateweb.UseCases) *httptest.Server {
	t.Helper()
	h, err := httpapi.New(slog.New(slog.DiscardHandler), gate{}, app.New(repo), l, delivery, estate)
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

// estate is a cluster read in memory.
type estate struct {
	sources  []delivery.Source
	units    []delivery.Unit
	releases []delivery.Release
	err      error
}

func (e estate) Sources(context.Context) (delivery.Page[delivery.Source], error) {
	return delivery.Page[delivery.Source]{Items: e.sources, Truncated: true}, e.err
}

func (e estate) Units(context.Context) (delivery.Page[delivery.Unit], error) {
	return delivery.Page[delivery.Unit]{Items: e.units}, e.err
}

func (e estate) Releases(context.Context) (delivery.Page[delivery.Release], error) {
	return delivery.Page[delivery.Release]{Items: e.releases}, e.err
}

func TestTheClusterIsServedInTheContractsShape(t *testing.T) {
	since := time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)
	srv := serveAll(t, &memory{}, &live{}, deliveryapp.New(estate{
		sources: []delivery.Source{
			{Name: "project-b", URL: "oci://r/b", Digest: "sha256:0"},
			{Name: "project-a", URL: "oci://r/a", Digest: "sha256:1", Revision: "sha256:1", Condition: delivery.Condition{Ready: true, Reason: "Succeeded", Message: "stored"}},
		},
		units: []delivery.Unit{{Name: "apps-a", Source: "project-a", Path: "./apps/a", DependsOn: []string{"estate-vso-secrets"}, Applied: "sha256:1", Condition: delivery.Condition{Ready: true}}},
		releases: []delivery.Release{{
			Namespace: "auth-system", Application: "auth", Serving: "sha256:s", Pinned: "sha256:p", Since: &since,
			Members:   []delivery.Member{{Process: "auth-api", Phase: "Progressing", Revision: "sha256:p", Iterations: 2}, {Process: "auth-ui"}},
			Migration: &delivery.Migration{Identity: "auth-migration", TestedAgainst: "sha256:s"},
		}},
	}))

	sources := call(t, srv, http.MethodGet, "/api/v1/delivery/sources", "", true)
	items, _ := sources.body["items"].([]any)
	if sources.code != http.StatusOK || len(items) != 2 || items[0].(map[string]any)["name"] != "project-a" || sources.body["truncated"] != true {
		t.Fatalf("sources = %d %v", sources.code, sources.body)
	}
	if bare := items[1].(map[string]any); bare["ready"] != false || len(bare) != 4 {
		t.Fatalf("a source Flux has not touched = %v", bare)
	}
	units := call(t, srv, http.MethodGet, "/api/v1/delivery/units", "", true)
	unit := units.body["items"].([]any)[0].(map[string]any)
	if units.code != http.StatusOK || units.body["truncated"] != false || unit["dependsOn"].([]any)[0] != "estate-vso-secrets" || unit["applied"] != "sha256:1" || unit["ready"] != true {
		t.Fatalf("units = %d %v", units.code, units.body)
	}
	releases := call(t, srv, http.MethodGet, "/api/v1/delivery/releases", "", true)
	release := releases.body["items"].([]any)[0].(map[string]any)
	members := release["members"].([]any)
	if releases.code != http.StatusOK || release["since"] != "2026-10-03T07:00:00Z" || release["migration"].(map[string]any)["testedAgainst"] != "sha256:s" || len(members) != 2 {
		t.Fatalf("releases = %d %v", releases.code, releases.body)
	}
	if api := members[0].(map[string]any); api["phase"] != "Progressing" || api["iterations"] != float64(2) {
		t.Fatalf("a member = %v", api)
	}
	if ui := members[1].(map[string]any); len(ui) != 2 {
		t.Fatalf("a member Flagger has not seen = %v", ui)
	}
}

func TestAClusterThatDoesNotAnswerOrIsNotThereIsA503(t *testing.T) {
	for want, uc := range map[string]deliveryweb.UseCases{
		"The cluster did not answer.":                   deliveryapp.New(estate{err: errors.New("forbidden")}),
		"The dashboard runs without a cluster to read.": deliveryapp.New(cluster.Absent{}),
	} {
		srv := serveAll(t, &memory{}, &live{}, uc)
		for _, path := range []string{"/api/v1/delivery/sources", "/api/v1/delivery/units", "/api/v1/delivery/releases"} {
			if got := call(t, srv, http.MethodGet, path, "", true); got.code != http.StatusServiceUnavailable || got.body["detail"] != want {
				t.Fatalf("%s = %d %v", path, got.code, got.body)
			}
		}
	}
}

// repository is the Estate repository read in memory; asked is each deploys read.
type repository struct {
	pins    []estatedomain.Pin
	deploys []estatedomain.Deploy
	issues  []estatedomain.Issue
	err     error
	asked   []string
}

func (r *repository) Pins(context.Context) ([]estatedomain.Pin, error) { return r.pins, r.err }

func (r *repository) Deploys(_ context.Context, project string, limit int) ([]estatedomain.Deploy, error) {
	r.asked = append(r.asked, project+" "+strconv.Itoa(limit))
	return r.deploys, r.err
}

func (r *repository) Issues(context.Context) ([]estatedomain.Issue, error) { return r.issues, r.err }

func TestTheEstateRepositoryIsServedInTheContractsShape(t *testing.T) {
	at := time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)
	repo := &repository{
		pins: []estatedomain.Pin{
			{Project: "data", Digest: "sha256:d"},
			{Project: "auth", Digest: "sha256:a", Paused: &estatedomain.Pause{By: "joris", At: "2026-10-03T07:00:00Z", Reason: "held"}, RolledBack: &estatedomain.Rollback{Version: "1.2.0", Fragment: "sha256:f"}},
		},
		deploys: []estatedomain.Deploy{{Commit: "abc", At: at, Message: "auth 1.3.0"}},
		issues:  []estatedomain.Issue{{Number: 7, Title: "auth is held", URL: "https://github.com/x/7", Labels: []string{"held"}, UpdatedAt: at}},
	}
	srv := serveEstate(t, &memory{}, &live{}, deliveryapp.New(cluster.Absent{}), estateapp.New(repo))

	pins := call(t, srv, http.MethodGet, "/api/v1/estate/pins", "", true)
	items, _ := pins.body["items"].([]any)
	auth, _ := items[0].(map[string]any)
	if pins.code != http.StatusOK || len(items) != 2 || auth["project"] != "auth" || auth["paused"].(map[string]any)["reason"] != "held" || auth["rolledBack"].(map[string]any)["version"] != "1.2.0" {
		t.Fatalf("pins = %d %v", pins.code, pins.body)
	}
	if data := items[1].(map[string]any); len(data) != 2 {
		t.Fatalf("a pin with nothing recorded = %v", data)
	}
	deploys := call(t, srv, http.MethodGet, "/api/v1/estate/projects/auth/deploys?limit=5", "", true)
	if deploys.code != http.StatusOK || deploys.body["items"].([]any)[0].(map[string]any)["at"] != "2026-10-03T07:00:00Z" || repo.asked[0] != "auth 5" {
		t.Fatalf("deploys = %d %v, asked %v", deploys.code, deploys.body, repo.asked)
	}
	issues := call(t, srv, http.MethodGet, "/api/v1/estate/issues", "", true)
	if issues.code != http.StatusOK || issues.body["items"].([]any)[0].(map[string]any)["number"] != float64(7) {
		t.Fatalf("issues = %d %v", issues.code, issues.body)
	}
	if got := call(t, srv, http.MethodGet, "/api/v1/estate/projects/NOT_A_NAME/deploys", "", true); got.code != http.StatusBadRequest {
		t.Fatalf("a name that is no Project = %d", got.code)
	}
}

func TestAnEstateRepositoryThatDoesNotAnswerOrIsNotThereIsA503(t *testing.T) {
	for want, uc := range map[string]estateweb.UseCases{
		"GitHub did not answer.":                                    estateapp.New(&repository{err: errors.New("rate limited")}),
		"The dashboard runs without the Estate repository to read.": estateapp.New(github.Absent{}),
	} {
		srv := serveEstate(t, &memory{}, &live{}, deliveryapp.New(cluster.Absent{}), uc)
		for _, path := range []string{"/api/v1/estate/pins", "/api/v1/estate/issues", "/api/v1/estate/projects/auth/deploys"} {
			if got := call(t, srv, http.MethodGet, path, "", true); got.code != http.StatusServiceUnavailable || got.body["detail"] != want {
				t.Fatalf("%s = %d %v", path, got.code, got.body)
			}
		}
	}
	none := serveEstate(t, &memory{}, &live{}, deliveryapp.New(cluster.Absent{}), estateapp.New(&repository{err: estatedomain.ErrNoProject}))
	if got := call(t, none, http.MethodGet, "/api/v1/estate/projects/auth/deploys", "", true); got.code != http.StatusOK || len(got.body["items"].([]any)) != 0 {
		t.Fatalf("a Project the repository does not know = %d %v", got.code, got.body)
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
	for _, path := range []string{"/api/v1/session", "/api/v1/alerts/history", "/api/v1/alerts", "/api/v1/delivery/sources", "/api/v1/delivery/units", "/api/v1/delivery/releases", "/api/v1/estate/pins", "/api/v1/estate/issues", "/api/v1/estate/projects/auth/deploys"} {
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
