package loki_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
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

// counted answers the line count with counts and each one-line read with lines[direction].
func counted(t *testing.T, counts string, lines map[string]string, asked *[]string) *loki.Loki {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*asked = append(*asked, r.URL.RawQuery)
		q := r.URL.Query()
		if strings.Contains(q.Get("query"), "count_over_time") {
			_, _ = io.WriteString(w, counts)
			return
		}
		line, ok := lines[q.Get("direction")]
		if !ok {
			_, _ = io.WriteString(w, `{"data":{"result":[]}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"result":[{"values":[["`+line+`","x"]]}]}}`)
	}))
	t.Cleanup(srv.Close)
	l, err := loki.New(srv.URL, srv.Client(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestStreamsAreEachPodAndContainerWithWhenTheyWrote(t *testing.T) {
	var asked []string
	l := counted(t, `{"data":{"result":[
		{"metric":{"pod":"auth-migration-x","container":"liquibase","version":"3.5.0"},"values":[[1792062600,"4"],[1792062660,"1"]]},
		{"metric":{"pod":"auth-api-1","container":"auth-api"},"values":[[1792063200,"9"]]},
		{"metric":{"pod":"","container":"x"},"values":[[1792063200,"1"]]},
		{"metric":{"pod":"empty","container":"x"},"values":[]}
	]}}`, map[string]string{"forward": "1792062570000000000", "backward": "1792065590000000000"}, &asked)
	got, err := l.Streams(t.Context(), "auth-system", now.Add(-time.Hour))
	first, last := time.Unix(0, 1792062570000000000).UTC(), time.Unix(0, 1792065590000000000).UTC()
	want := []domain.Stream{{Pod: "auth-api-1", Container: "auth-api", First: first, Last: last}, {Pod: "auth-migration-x", Container: "liquibase", Version: "3.5.0", First: first, Last: last}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("streams = %+v, %v", got, err)
	}
	q, _ := url.ParseQuery(asked[0])
	if q.Get("query") != `sum by (pod, container, version) (count_over_time({namespace="auth-system"}[60s]))` || q.Get("step") != "60" || q.Get("start") != "1792062000000000000" {
		t.Fatalf("asked %v", asked[0])
	}
	// Each one-line read stays inside its step.
	q, _ = url.ParseQuery(asked[1])
	if end, _ := strconv.ParseInt(q.Get("end"), 10, 64); end-mustInt(t, q.Get("start")) != int64(time.Minute) {
		t.Fatalf("read %v", asked[1])
	}
}

func TestAStepWithoutALineKeepsTheStep(t *testing.T) {
	var asked []string
	l := counted(t, `{"data":{"result":[{"metric":{"pod":"p","container":"c"},"values":[[1792063200,"1"],["bad","1"]]}]}}`, nil, &asked)
	got, err := l.Streams(t.Context(), "a-system", now.Add(-time.Hour))
	if err != nil || len(got) != 0 {
		t.Fatalf("streams = %+v, %v", got, err)
	}
	l = counted(t, `{"data":{"result":[{"metric":{"pod":"p","container":"c"},"values":[[1792063200,"1"]]}]}}`, nil, &asked)
	got, err = l.Streams(t.Context(), "a-system", now.Add(-time.Hour))
	at := time.Unix(1792063200, 0).UTC()
	if err != nil || !reflect.DeepEqual(got, []domain.Stream{{Pod: "p", Container: "c", First: at, Last: at}}) {
		t.Fatalf("streams = %+v, %v", got, err)
	}
}

func TestAWeekIsCountedInStepsOfMinutes(t *testing.T) {
	var asked []string
	l := counted(t, `{"data":{"result":[]}}`, nil, &asked)
	if _, err := l.Streams(t.Context(), "a-system", now.Add(-7*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if q, _ := url.ParseQuery(asked[0]); q.Get("step") != "604" {
		t.Fatalf("step = %s", q.Get("step"))
	}
}

func mustInt(t *testing.T, s string) int64 {
	t.Helper()
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return n
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

func TestAOneLineReadThatFailsFailsTheStreams(t *testing.T) {
	for _, failing := range []string{"forward", "backward"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			switch {
			case strings.Contains(q.Get("query"), "count_over_time"):
				_, _ = io.WriteString(w, `{"data":{"result":[{"metric":{"pod":"p","container":"c"},"values":[[1792063200,"1"]]}]}}`)
			case q.Get("direction") == failing:
				w.WriteHeader(http.StatusBadGateway)
			default:
				_, _ = io.WriteString(w, `{"data":{"result":[]}}`)
			}
		}))
		l, err := loki.New(srv.URL, srv.Client(), func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		if _, err := l.Streams(t.Context(), "a-system", now.Add(-time.Hour)); err == nil {
			t.Errorf("a failing %s read passed", failing)
		}
		srv.Close()
	}
}
