package github_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/estate/adapters/github"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/estate/domain"
)

const pinned = `# a comment
apiVersion: source.toolkit.fluxcd.io/v1
kind: OCIRepository
metadata:
  name: project-auth
  annotations:
    estate.jorisjonkers.dev/paused-by: joris
    estate.jorisjonkers.dev/paused-at: "2026-10-03T07:00:00Z"
    estate.jorisjonkers.dev/paused-reason: held
    estate.jorisjonkers.dev/rollback-version: 1.2.0
    estate.jorisjonkers.dev/rollback-fragment: sha256:f
spec:
  ref:
    digest: sha256:a
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: apps-auth
`

// fake answers as GitHub would, and records each request's method, path, query and token.
type fake struct {
	status  int
	graphql string
	rest    string
	seen    []string
	auth    []string
}

func (f *fake) serve(t *testing.T) *github.Estate {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		f.seen = append(f.seen, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		w.WriteHeader(f.status)
		if r.URL.Path == "/api/graphql" {
			_, _ = io.WriteString(w, f.graphql)
			return
		}
		_, _ = io.WriteString(w, f.rest)
	}))
	t.Cleanup(srv.Close)
	e, err := github.New(srv.URL+"/api", "JorisJonkers-dev/estate", "t0ken", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func tree(entries ...any) string {
	raw, _ := json.Marshal(map[string]any{"data": map[string]any{"repository": map[string]any{"object": map[string]any{"entries": entries}}}})
	return string(raw)
}

func dir(name string, files ...any) map[string]any {
	return map[string]any{"name": name, "type": "tree", "object": map[string]any{"entries": files}}
}

func file(name string, text any) map[string]any {
	return map[string]any{"name": name, "object": map[string]any{"text": text}}
}

func TestNewTakesAnHTTPURLAnOwnerNameAndAToken(t *testing.T) {
	for _, c := range [][3]string{
		{"", "a/b", "t"},
		{"ftp://x", "a/b", "t"},
		{"https://api.github.com", "ab", "t"},
		{"https://api.github.com", "a/b/c", "t"},
		{"https://api.github.com", "/b", "t"},
		{"https://api.github.com", "a/b", ""},
	} {
		if _, err := github.New(c[0], c[1], c[2], http.DefaultClient); err == nil {
			t.Errorf("New%v accepted", c)
		}
	}
}

func TestEveryPinIsReadInOneQueryWithWhatAPauseOrARollbackRecorded(t *testing.T) {
	f := &fake{status: http.StatusOK, graphql: tree(
		dir("auth", file("source.yaml", pinned), file("README.md", "x")),
		dir("plain", file("source.yaml", "kind: OCIRepository\nspec:\n  ref:\n    digest: sha256:p\n")),
		dir("broken", file("source.yaml", "kind: [")),
		dir("binary", file("source.yaml", nil)),
		map[string]any{"name": "README.md", "type": "blob", "object": map[string]any{}},
	)}
	got, err := f.serve(t).Pins(t.Context())
	want := []domain.Pin{
		{Project: "auth", Digest: "sha256:a", Paused: &domain.Pause{By: "joris", At: "2026-10-03T07:00:00Z", Reason: "held"}, RolledBack: &domain.Rollback{Version: "1.2.0", Fragment: "sha256:f"}},
		{Project: "plain", Digest: "sha256:p"},
		{Project: "broken"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("pins = %+v, %v", got, err)
	}
	if len(f.seen) != 1 || f.seen[0] != "POST /api/graphql?" || f.auth[0] != "Bearer t0ken" {
		t.Fatalf("asked %v with %v", f.seen, f.auth)
	}
}

func TestARepositoryWithoutProjectsHasNoPins(t *testing.T) {
	f := &fake{status: http.StatusOK, graphql: `{"data":{"repository":{"object":null}}}`}
	if got, err := f.serve(t).Pins(t.Context()); err != nil || len(got) != 0 || got == nil {
		t.Fatalf("pins = %#v, %v", got, err)
	}
}

func TestAPinsReplyGitHubRefusesIsAnError(t *testing.T) {
	for name, f := range map[string]*fake{
		"a GraphQL error": {status: http.StatusOK, graphql: `{"errors":[{"message":"bad credentials"}]}`},
		"a failure":       {status: http.StatusUnauthorized, graphql: `{}`},
		"not JSON":        {status: http.StatusOK, graphql: `<html>`},
	} {
		if _, err := f.serve(t).Pins(t.Context()); err == nil {
			t.Errorf("%s read as pins", name)
		}
	}
}

func TestADeployIsACommitThatTouchedThePinFile(t *testing.T) {
	f := &fake{status: http.StatusOK, rest: `[{"sha":"abc","commit":{"message":"auth 1.3.0\n\nbody","committer":{"date":"2026-10-03T09:00:00+02:00"}}}]`}
	e := f.serve(t)
	got, err := e.Deploys(t.Context(), "auth", 5)
	want := []domain.Deploy{{Commit: "abc", At: time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC), Message: "auth 1.3.0"}}
	if err != nil || !reflect.DeepEqual(got, want) || f.seen[0] != "GET /api/repos/JorisJonkers-dev/estate/commits?path=projects%2Fauth%2Fsource.yaml&per_page=5" {
		t.Fatalf("deploys = %+v, %v, asked %v", got, err, f.seen)
	}
	for _, name := range []string{"", "../x", "a/b", "Auth", "a b"} {
		if _, err := e.Deploys(t.Context(), name, 5); !errors.Is(err, domain.ErrNoProject) {
			t.Errorf("Deploys(%q) = %v", name, err)
		}
	}
	f.status = http.StatusNotFound
	if _, err := e.Deploys(t.Context(), "auth", 5); err == nil {
		t.Fatal("a refused read was a deploy log")
	}
}

func TestTheIssuesAreTheOpenOnesWithoutPullRequests(t *testing.T) {
	f := &fake{status: http.StatusOK, rest: `[
		{"number":7,"title":"auth is held","html_url":"https://github.com/x/7","updated_at":"2026-10-03T09:00:00+02:00","labels":[{"name":"held"}]},
		{"number":8,"title":"a pull request","html_url":"https://github.com/x/8","updated_at":"2026-10-03T07:00:00Z","labels":[],"pull_request":{}}
	]`}
	e := f.serve(t)
	got, err := e.Issues(t.Context())
	want := []domain.Issue{{Number: 7, Title: "auth is held", URL: "https://github.com/x/7", Labels: []string{"held"}, UpdatedAt: time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)}}
	if err != nil || !reflect.DeepEqual(got, want) || f.seen[0] != "GET /api/repos/JorisJonkers-dev/estate/issues?per_page=100&state=open" {
		t.Fatalf("issues = %+v, %v, asked %v", got, err, f.seen)
	}
	f.status = http.StatusForbidden
	if _, err := e.Issues(t.Context()); err == nil {
		t.Fatal("a refused read was a list of issues")
	}
}

func TestAGitHubThatIsGoneIsAnError(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	e, err := github.New(gone.URL, "a/b", "t", http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Pins(t.Context()); err == nil {
		t.Fatal("pins from nowhere")
	}
}

func TestWithoutARepositoryEveryReadSaysSo(t *testing.T) {
	var a github.Absent
	_, e1 := a.Pins(t.Context())
	_, e2 := a.Deploys(t.Context(), "auth", 1)
	_, e3 := a.Issues(t.Context())
	for _, err := range []error{e1, e2, e3} {
		if !errors.Is(err, domain.ErrNoRepository) {
			t.Fatalf("error = %v", err)
		}
	}
}
