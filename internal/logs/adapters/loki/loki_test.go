package loki_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/logs/adapters/loki"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/logs/domain"
)

var now = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

// serve is a Loki answering every request with status and body, recording what it was asked.
func serve(t *testing.T, status int, body string, asked *[]string) *loki.Loki {
	t.Helper()
	return serveEach(t, status, map[string]string{"": body}, asked)
}

// serveEach answers each path with its own body, and any other with the one keyed "".
func serveEach(t *testing.T, status int, bodies map[string]string, asked *[]string) *loki.Loki {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*asked = append(*asked, r.URL.Path+"?"+r.URL.RawQuery)
		w.WriteHeader(status)
		body, ok := bodies[r.URL.Path]
		if !ok {
			body = bodies[""]
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	l, err := loki.New(srv.URL, srv.Client(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestNewTakesOnlyAnHTTPURL(t *testing.T) {
	for _, base := range []string{"", "loki:3100", "ftp://loki", "http://"} {
		if _, err := loki.New(base, http.DefaultClient, time.Now); err == nil {
			t.Errorf("New(%q) accepted", base)
		}
	}
}

func TestStreamsAreEachPodAndContainerOnce(t *testing.T) {
	var asked []string
	l := serveEach(t, http.StatusOK, map[string]string{
		"/loki/api/v1/series": `{"data":[
		{"namespace":"auth-system","pod":"auth-api-1","container":"auth-api","level":"INFO"},
		{"namespace":"auth-system","pod":"auth-api-1","container":"auth-api","level":"WARN"},
		{"namespace":"auth-system","pod":"auth-migration-x","container":"liquibase","version":"3.5.0"},
		{"namespace":"auth-system","pod":"","container":"x"}
	]}`,
		"/loki/api/v1/query_range": `{"data":{"result":[{"values":[["1792065590000000000","x"]]}]}}`,
	}, &asked)
	got, err := l.Streams(t.Context(), "auth-system", now.Add(-time.Hour))
	at := time.Unix(0, 1792065590000000000).UTC()
	want := []domain.Stream{{Pod: "auth-api-1", Container: "auth-api", First: at, Last: at}, {Pod: "auth-migration-x", Container: "liquibase", Version: "3.5.0", First: at, Last: at}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("streams = %+v, %v", got, err)
	}
	if asked[0] != "/loki/api/v1/series?end=1792065600000000000&match%5B%5D=%7Bnamespace%3D%22auth-system%22%7D&start=1792062000000000000" {
		t.Fatalf("asked %v", asked)
	}
}

func TestLinesAreTheNewestOldestFirst(t *testing.T) {
	var asked []string
	l := serve(t, http.StatusOK, `{"data":{"result":[
		{"values":[["1792065590000000000","second"],["1792065580000000000","first"],["bad","skipped"]]},
		{"values":[["1792065595000000000","third"]]}
	]}}`, &asked)
	got, err := l.Lines(t.Context(), "auth-system", domain.Stream{Pod: "auth-api-1", Container: "auth-api"}, now.Add(-time.Hour), 2)
	if err != nil || len(got) != 2 || got[0].Text != "second" || got[1].Text != "third" || !got[1].At.Equal(time.Unix(0, 1792065595000000000)) {
		t.Fatalf("lines = %+v, %v", got, err)
	}
	if asked[0] != "/loki/api/v1/query_range?direction=backward&end=1792065600000000000&limit=2&query=%7Bnamespace%3D%22auth-system%22%2Cpod%3D%22auth-api-1%22%2Ccontainer%3D%22auth-api%22%7D&start=1792062000000000000" {
		t.Fatalf("asked %v", asked)
	}
}

func TestAValueCannotEscapeItsMatcher(t *testing.T) {
	var asked []string
	l := serve(t, http.StatusOK, `{"data":{"result":[]}}`, &asked)
	if _, err := l.Lines(t.Context(), "a-system", domain.Stream{Pod: `p",namespace=~".+`, Container: `c\`}, now, 1); err != nil {
		t.Fatal(err)
	}
	q, _ := url.ParseQuery(strings.SplitN(asked[0], "?", 2)[1])
	if got := q.Get("query"); got != `{namespace="a-system",pod="p\",namespace=~\".+",container="c\\"}` {
		t.Fatalf("query = %s", got)
	}
}

func TestALokiThatRefusesIsAnError(t *testing.T) {
	var asked []string
	for name, l := range map[string]*loki.Loki{
		"a failure": serve(t, http.StatusBadGateway, `{}`, &asked),
		"not JSON":  serve(t, http.StatusOK, `<html>`, &asked),
	} {
		if _, err := l.Streams(t.Context(), "a-system", now); err == nil {
			t.Errorf("%s read as streams", name)
		}
		if _, err := l.Lines(t.Context(), "a-system", domain.Stream{Pod: "p", Container: "c"}, now, 1); err == nil {
			t.Errorf("%s read as lines", name)
		}
	}
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	l, _ := loki.New(gone.URL, http.DefaultClient, time.Now)
	if _, err := l.Streams(t.Context(), "a-system", now); err == nil {
		t.Fatal("streams from nowhere")
	}
}

func TestWithoutLokiEveryReadSaysSo(t *testing.T) {
	var a loki.Absent
	_, e1 := a.Streams(t.Context(), "a", now)
	_, e2 := a.Lines(t.Context(), "a", domain.Stream{}, now, 1)
	if !errors.Is(e1, domain.ErrNoStore) || !errors.Is(e2, domain.ErrNoStore) {
		t.Fatalf("errors = %v %v", e1, e2)
	}
}
