package github_test

import (
	"encoding/base64"
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

// contents is what the contents API returns for a text file.
func contents(text string) string {
	raw, _ := json.Marshal(map[string]string{"encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(text))})
	return string(raw)
}

func TestTheHandoverLedgerIsThePlatformDocumentsOwnNamesOnly(t *testing.T) {
	f := &fake{status: http.StatusOK, rest: contents("apiVersion: intent.jorisjonkers.dev/v1\nhandover:\n  retireBy: 2027-03-31\n  legacy: [data, auth, \"../x\"]\n  estate:\n    - delivery\n    - Not-A-Name\n")}
	e := f.serve(t)
	got, err := e.Handover(t.Context())
	want := domain.Handover{Estate: []string{"delivery"}, Legacy: []string{"data", "auth"}}
	if err != nil || !reflect.DeepEqual(got, want) || f.seen[0] != "GET /api/repos/JorisJonkers-dev/estate/contents/platform.intent.yml?" {
		t.Fatalf("handover = %+v, %v, asked %v", got, err, f.seen)
	}
	f.rest = contents("apiVersion: intent.jorisjonkers.dev/v1\n")
	if got, err := e.Handover(t.Context()); err != nil || len(got.Legacy) != 0 || got.Legacy == nil {
		t.Fatalf("a document without a ledger = %#v, %v", got, err)
	}
	for name, reply := range map[string]string{
		"another encoding": `{"encoding":"none","content":""}`,
		"not base64":       `{"encoding":"base64","content":"!!"}`,
		"not YAML":         contents("handover: ["),
	} {
		f.rest = reply
		if _, err := e.Handover(t.Context()); err == nil {
			t.Errorf("%s read as a ledger", name)
		}
	}
	f.status = http.StatusNotFound
	if _, err := e.Handover(t.Context()); err == nil {
		t.Fatal("a refused read was a ledger")
	}
}

func TestARolloutIsADeploymentCompositionOpenedForTheProject(t *testing.T) {
	f := &fake{status: http.StatusOK, graphql: `{"data":{"repository":{"deployments":{"nodes":[
		{"databaseId":7,"createdAt":"2026-10-15T08:39:50Z","commitOid":"2a44","payload":"{\"project\":\"estate-dashboard\",\"version\":\"0.2.0\",\"run\":\"https://github.com/x/runs/1\",\"artifact\":\"ghcr.io/r@sha256:a\"}",
		 "statuses":{"nodes":[{"state":"SUCCESS","description":"estate-dashboard 0.2.0 promoted","logUrl":"https://github.com/x/runs/1","createdAt":"2026-10-15T08:42:42Z"}]}},
		{"databaseId":8,"createdAt":"2026-10-15T08:00:00Z","commitOid":"1","payload":"{\"project\":\"other\"}","statuses":{"nodes":[]}},
		{"databaseId":9,"createdAt":"2026-10-15T08:00:00Z","commitOid":"1","payload":"not json","statuses":{"nodes":[]}},
		{"databaseId":10,"createdAt":"2026-10-15T07:00:00Z","commitOid":"0","payload":"\"{\\\"project\\\":\\\"estate-dashboard\\\",\\\"version\\\":\\\"0.1.0\\\"}\"","statuses":{"nodes":[]}}
	]}}}}`}
	e := f.serve(t)
	got, err := e.Rollouts(t.Context(), "JorisJonkers-dev/estate-dashboard", "estate-dashboard", 5)
	want := []domain.Rollout{{
		ID: 7, Version: "0.2.0", Commit: "2a44", Run: "https://github.com/x/runs/1", Artifact: "ghcr.io/r@sha256:a", At: time.Date(2026, 10, 15, 8, 39, 50, 0, time.UTC),
		Stages: []domain.Stage{{State: "success", Description: "estate-dashboard 0.2.0 promoted", Log: "https://github.com/x/runs/1", At: time.Date(2026, 10, 15, 8, 42, 42, 0, time.UTC)}},
	}, {ID: 10, Version: "0.1.0", Commit: "0", At: time.Date(2026, 10, 15, 7, 0, 0, 0, time.UTC), Stages: []domain.Stage{}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("rollouts = %+v, %v", got, err)
	}
	for _, repository := range []string{"someone-else/repo", "no-slash", "a/b/c"} {
		if _, err := e.Rollouts(t.Context(), repository, "estate-dashboard", 5); !errors.Is(err, domain.ErrForeignRepository) {
			t.Errorf("Rollouts(%q) = %v", repository, err)
		}
	}
	if _, err := e.Rollouts(t.Context(), "JorisJonkers-dev/x", "Not A Name", 5); !errors.Is(err, domain.ErrNoProject) {
		t.Errorf("a name that is no Project = %v", err)
	}
	f.graphql = `{"data":{"repository":null}}`
	if got, err := e.Rollouts(t.Context(), "JorisJonkers-dev/gone", "x", 5); err != nil || len(got) != 0 || got == nil {
		t.Fatalf("a repository GitHub does not show = %#v, %v", got, err)
	}
	f.graphql = `{"errors":[{"message":"nope"}]}`
	if _, err := e.Rollouts(t.Context(), "JorisJonkers-dev/x", "x", 5); err == nil {
		t.Fatal("a refused query read as rollouts")
	}
	f.status = http.StatusBadGateway
	if _, err := e.Rollouts(t.Context(), "JorisJonkers-dev/x", "x", 5); err == nil {
		t.Fatal("a failed read was rollouts")
	}
}

func TestTheIssuesAreTheOpenOnesWithoutPullRequests(t *testing.T) {
	f := &fake{status: http.StatusOK, rest: `[
		{"number":7,"title":"auth is held","html_url":"https://github.com/x/7","updated_at":"2026-10-03T09:00:00+02:00","labels":[{"name":"held"}],"body":"auth is held.\n\n<!-- estate-condition: auth/held -->"},
		{"number":9,"title":"a feature","html_url":"https://github.com/x/9","updated_at":"2026-10-03T07:00:00Z","labels":[],"body":"<!-- estate-condition: ../x/held -->"},
		{"number":8,"title":"a pull request","html_url":"https://github.com/x/8","updated_at":"2026-10-03T07:00:00Z","labels":[],"pull_request":{}}
	]`}
	e := f.serve(t)
	got, err := e.Issues(t.Context())
	want := []domain.Issue{
		{Number: 7, Title: "auth is held", URL: "https://github.com/x/7", Labels: []string{"held"}, UpdatedAt: time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC), Project: "auth", Condition: "held"},
		{Number: 9, Title: "a feature", URL: "https://github.com/x/9", Labels: []string{}, UpdatedAt: time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)},
	}
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
	_, e4 := a.Handover(t.Context())
	_, e5 := a.Rollouts(t.Context(), "a/b", "x", 1)
	for _, err := range []error{e1, e2, e3, e4, e5} {
		if !errors.Is(err, domain.ErrNoRepository) {
			t.Fatalf("error = %v", err)
		}
	}
}
