package prometheus_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/health/adapters/prometheus"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/health/domain"
)

// fake is a Prometheus that answers each range query by which metric it names.
type fake struct {
	status  int
	answers map[string]string
	queries []string
}

func (f *fake) serve(t *testing.T) *prometheus.Prometheus {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" || r.URL.Query().Get("step") != "30" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query().Get("query")
		f.queries = append(f.queries, q)
		w.WriteHeader(f.status)
		for metric, answer := range f.answers {
			if strings.Contains(q, metric) {
				_, _ = w.Write([]byte(answer))
				return
			}
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
	}))
	t.Cleanup(srv.Close)
	p, err := prometheus.New(srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func series(values string) string {
	return `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[` + values + `]}]}}`
}

var (
	start = time.Unix(1760961600, 0).UTC()
	end   = start.Add(30 * time.Second)
)

func TestASampleIsWhatEachOfTheThreeSeriesSaysOfTheStep(t *testing.T) {
	f := &fake{status: http.StatusOK, answers: map[string]string{
		"kube_pod_status_ready":          series(`[1760961600,"0.5"],[1760961630,"1"]`),
		"gotk_resource_ready":            series(`[1760961600,"1"]`),
		"gatus_results_endpoint_success": series(`[1760961630,"0"]`),
	}}
	got, err := f.serve(t).Samples(t.Context(), "arr", []string{"home-media_sonarr"}, start, end, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	first, second := got[start], got[end]
	if *first.PodsReady != 0.5 || *first.UnitReady != 1 || first.ChecksUp != nil || *second.PodsReady != 1 || *second.ChecksUp != 0 || second.UnitReady != nil {
		t.Fatalf("samples = %+v %+v", first, second)
	}
}

func TestTheSeriesAreThoseOfTheProjectsNamespaceUnitAndChecks(t *testing.T) {
	f := &fake{status: http.StatusOK}
	if _, err := f.serve(t).Samples(t.Context(), "arr", []string{"home-media_sonarr", "home-media_radarr"}, start, end, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(f.queries, "\n")
	for _, want := range []string{`namespace="arr-system"`, `name="apps-arr"`, `key=~"home-media_sonarr|home-media_radarr"`} {
		if !strings.Contains(joined, want) {
			t.Fatalf("queries %v do not name %s", f.queries, want)
		}
	}
}

func TestACheckKeyThatCouldWidenTheQueryIsLeftOut(t *testing.T) {
	f := &fake{status: http.StatusOK}
	if _, err := f.serve(t).Samples(t.Context(), "arr", []string{`.*"}`}, start, end, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	if len(f.queries) != 2 {
		t.Fatalf("queries = %v", f.queries)
	}
}

func TestAValueThatIsNotANumberIsNoData(t *testing.T) {
	f := &fake{status: http.StatusOK, answers: map[string]string{"kube_pod_status_ready": series(`[1760961600,"NaN?"],["x","1"]`)}}
	got, err := f.serve(t).Samples(t.Context(), "arr", nil, start, end, 30*time.Second)
	if err != nil || got[start].PodsReady != nil {
		t.Fatalf("samples = %+v, %v", got, err)
	}
}

func TestAPrometheusThatRefusesIsAnError(t *testing.T) {
	for name, f := range map[string]*fake{
		"a refusal":             {status: http.StatusBadGateway},
		"an answer not JSON":    {status: http.StatusOK, answers: map[string]string{"kube_pod": "<html>"}},
		"an answer of an error": {status: http.StatusOK, answers: map[string]string{"gotk": `{"status":"error"}`}},
		"a checks query error":  {status: http.StatusOK, answers: map[string]string{"gatus": `{"status":"error"}`}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := f.serve(t).Samples(t.Context(), "arr", []string{"k"}, start, end, 30*time.Second); err == nil {
				t.Fatal("an answer that is not one read as samples")
			}
		})
	}
}

func TestAPrometheusThatIsGoneIsAnError(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	p, err := prometheus.New(gone.URL, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Samples(t.Context(), "arr", nil, start, end, 30*time.Second); err == nil {
		t.Fatal("samples from nowhere")
	}
}

func TestOnlyAnHTTPAddressIsAPrometheus(t *testing.T) {
	for _, base := range []string{"", "prometheus:9090", "ftp://prometheus"} {
		if _, err := prometheus.New(base, http.DefaultClient); err == nil {
			t.Fatalf("%q was taken for a Prometheus", base)
		}
	}
}

func TestWithoutAPrometheusEveryReadSaysSo(t *testing.T) {
	if _, err := (prometheus.Absent{}).Samples(t.Context(), "arr", nil, start, end, time.Second); !errors.Is(err, domain.ErrNoMetrics) {
		t.Fatalf("error = %v", err)
	}
}

func TestARolloutIsEachPodsReadinessAndEachCanarysWeight(t *testing.T) {
	f := &fake{status: http.StatusOK, answers: map[string]string{
		"kube_pod_status_ready": `{"status":"success","data":{"result":[{"metric":{"pod":"auth-api-1"},"values":[[1760961600,"1"]]},{"metric":{"pod":"auth-api-2"},"values":[[1760961630,"0"]]}]}}`,
		"flagger_canary_weight": `{"status":"success","data":{"result":[{"metric":{"name":"auth-api"},"values":[[1760961600,"20"]]}]}}`,
	}}
	readiness, weights, err := f.serve(t).Rollout(t.Context(), "auth", start, end, 30*time.Second)
	if err != nil || readiness["auth-api-1"][start] != 1 || readiness["auth-api-2"][end] != 0 || weights["auth-api"][start] != 20 {
		t.Fatalf("readiness %v, weights %v, %v", readiness, weights, err)
	}
	if !strings.Contains(strings.Join(f.queries, "\n"), `namespace="auth-system"`) {
		t.Fatalf("queries = %v", f.queries)
	}
}

func TestARolloutLeavesOutThePodsAJobRan(t *testing.T) {
	f := &fake{status: http.StatusOK, answers: map[string]string{}}
	if _, _, err := f.serve(t).Rollout(t.Context(), "jellyfin", start, end, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	want := `kube_pod_status_ready{namespace="jellyfin-system",condition="true"} unless on(namespace, pod) kube_pod_owner{namespace="jellyfin-system",owner_kind="Job"}`
	if f.queries[0] != want {
		t.Fatalf("readiness query = %q, want %q", f.queries[0], want)
	}
}

func TestARolloutPrometheusRefusesIsAnError(t *testing.T) {
	for name, f := range map[string]*fake{
		"the readiness": {status: http.StatusOK, answers: map[string]string{"kube_pod": `{"status":"error"}`}},
		"the weights":   {status: http.StatusOK, answers: map[string]string{"flagger": `{"status":"error"}`}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := f.serve(t).Rollout(t.Context(), "auth", start, end, 30*time.Second); err == nil {
				t.Fatal("an answer that is not one read as a rollout")
			}
		})
	}
	if _, _, err := (prometheus.Absent{}).Rollout(t.Context(), "auth", start, end, time.Second); !errors.Is(err, domain.ErrNoMetrics) {
		t.Fatalf("error = %v", err)
	}
}
