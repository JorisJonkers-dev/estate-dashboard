package github_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/estate/adapters/github"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/estate/domain"
)

const record = `{"refused": false, "toolkit": "0.26.1", "snapshotAt": "2026-10-21T09:44:34Z",
 "lock": "sha256:9c41e07b2a1d", "moved": ["delivery"],
 "fragments": [
  {"project": "delivery", "version": "0.4.0", "outcome": "moved", "reason": null, "published": "2026-10-21T09:00:00Z"},
  {"project": "media", "version": null, "outcome": "stale", "reason": "E_PARTICIPANT_MISSING", "published": null},
  {"project": "odd", "version": "1.0.0", "outcome": "exploded", "reason": null, "published": null}]}`

// runs is a fake GitHub serving compose.yml's runs and their artifacts, whose downloads redirect
// to a second server, as GitHub's do to its blob storage.
type runs struct {
	mu        sync.Mutex
	listed    string
	artifacts map[string]string
	zips      map[string][]byte
	status    int
	seen      []string
	storeAuth []string
}

func (f *runs) serve(t *testing.T) *github.Estate {
	t.Helper()
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.storeAuth = append(f.storeAuth, r.Header.Get("Authorization"))
		_, _ = w.Write(f.zips[r.URL.Path])
	}))
	t.Cleanup(store.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.seen = append(f.seen, r.URL.Path+"?"+r.URL.RawQuery)
		path := strings.TrimPrefix(r.URL.Path, "/api/repos/JorisJonkers-dev/estate/actions")
		switch {
		case f.status != 0:
			w.WriteHeader(f.status)
		case path == "/workflows/compose.yml/runs":
			_, _ = io.WriteString(w, f.listed)
		case strings.HasSuffix(path, "/artifacts"):
			_, _ = io.WriteString(w, f.artifacts[path])
		case strings.HasSuffix(path, "/zip"):
			w.Header().Set("Location", store.URL+path)
			w.WriteHeader(http.StatusFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(api.Close)
	e, err := github.New(api.URL+"/api", "JorisJonkers-dev/estate", "t0ken", api.Client())
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func archive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(f, body)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func run(id int, title string) string {
	return fmt.Sprintf(`{"id": %d, "display_title": %q, "conclusion": "success", "run_started_at": "2026-10-21T10:06:04Z", "html_url": "https://github.com/JorisJonkers-dev/estate/actions/runs/%d"}`, id, title, id)
}

func artifact(id int, expired bool) string {
	return fmt.Sprintf(`{"artifacts": [{"id": %d, "name": "composition", "expired": %t}]}`, id, expired)
}

func TestEachCompositionRunIsReadFromTheRecordItsArtifactHolds(t *testing.T) {
	f := &runs{
		listed:    `{"workflow_runs": [` + run(7, "Deliver to production: delivery 0.4.0 released") + `]}`,
		artifacts: map[string]string{"/runs/7/artifacts": artifact(70, false)},
	}
	f.zips = map[string][]byte{"/artifacts/70/zip": archive(t, map[string]string{"run.json": record, "lock.json": "{}"})}
	got, err := f.serve(t).CompositionRuns(t.Context(), 5)
	want := []domain.CompositionRun{{
		ID: 7, At: time.Date(2026, 10, 21, 10, 6, 4, 0, time.UTC), Trigger: "delivery 0.4.0 released", Conclusion: "success",
		URL: "https://github.com/JorisJonkers-dev/estate/actions/runs/7", Toolkit: "0.26.1",
		SnapshotAt: time.Date(2026, 10, 21, 9, 44, 34, 0, time.UTC), Lock: "sha256:9c41e07b2a1d", Moved: []string{"delivery"},
		Fragments: []domain.CompositionFragment{
			{Project: "delivery", Version: "0.4.0", Outcome: "moved", Published: time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)},
			{Project: "media", Outcome: "stale", Reason: "E_PARTICIPANT_MISSING"},
		},
	}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("CompositionRuns = %+v, %v\nwant %+v", got, err, want)
	}
	if f.seen[0] != "/api/repos/JorisJonkers-dev/estate/actions/workflows/compose.yml/runs?per_page=5&status=completed" {
		t.Fatalf("listed the runs as %q", f.seen[0])
	}
}

func TestTheTokenIsNeverSentToWhereGitHubRedirectsADownload(t *testing.T) {
	f := &runs{
		listed:    `{"workflow_runs": [` + run(7, "Deliver to production: nightly safety net") + `]}`,
		artifacts: map[string]string{"/runs/7/artifacts": artifact(70, false)},
	}
	f.zips = map[string][]byte{"/artifacts/70/zip": archive(t, map[string]string{"run.json": record})}
	if _, err := f.serve(t).CompositionRuns(t.Context(), 5); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.storeAuth, []string{""}) {
		t.Fatalf("the storage host was sent %q", f.storeAuth)
	}
}

func TestARunWithNoRecordToReadIsLeftOut(t *testing.T) {
	f := &runs{
		listed: `{"workflow_runs": [` + strings.Join([]string{
			run(1, "Deliver to production: before runs recorded"),
			run(2, "Deliver to production: expired"),
			run(3, "Deliver to production: no run.json"),
			run(4, "Deliver to production: not a record"),
			run(5, "Deliver to production: not a zip"),
		}, ",") + `]}`,
		artifacts: map[string]string{
			"/runs/1/artifacts": `{"artifacts": []}`,
			"/runs/2/artifacts": artifact(20, true),
			"/runs/3/artifacts": artifact(30, false),
			"/runs/4/artifacts": artifact(40, false),
			"/runs/5/artifacts": artifact(50, false),
		},
	}
	f.zips = map[string][]byte{
		"/artifacts/20/zip": archive(t, map[string]string{"run.json": record}),
		"/artifacts/30/zip": archive(t, map[string]string{"lock.json": "{}"}),
		"/artifacts/40/zip": archive(t, map[string]string{"run.json": "not json"}),
		"/artifacts/50/zip": []byte("not a zip"),
	}
	got, err := f.serve(t).CompositionRuns(t.Context(), 10)
	if err != nil || len(got) != 0 {
		t.Fatalf("CompositionRuns = %+v, %v", got, err)
	}
}

func TestARunIsDownloadedOnceAndReadFromMemoryAfter(t *testing.T) {
	f := &runs{
		listed:    `{"workflow_runs": [` + run(7, "Deliver to production: delivery 0.4.0 released") + `]}`,
		artifacts: map[string]string{"/runs/7/artifacts": artifact(70, false)},
	}
	f.zips = map[string][]byte{"/artifacts/70/zip": archive(t, map[string]string{"run.json": record})}
	e := f.serve(t)
	for range 3 {
		if got, err := e.CompositionRuns(t.Context(), 5); err != nil || len(got) != 1 {
			t.Fatalf("CompositionRuns = %+v, %v", got, err)
		}
	}
	if len(f.storeAuth) != 1 || len(f.seen) != 5 {
		t.Fatalf("downloaded %d times, asked GitHub %v", len(f.storeAuth), f.seen)
	}
}

func TestGitHubRefusingTheRunsIsAnError(t *testing.T) {
	f := &runs{status: http.StatusForbidden}
	if _, err := f.serve(t).CompositionRuns(t.Context(), 5); err == nil {
		t.Fatal("a refused read was not an error")
	}
}
