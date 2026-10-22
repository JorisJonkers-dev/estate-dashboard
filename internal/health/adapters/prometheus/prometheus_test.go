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
	got, err := f.serve(t).Rollout(t.Context(), "auth", start, end, 30*time.Second)
	if err != nil || got.Readiness["auth-api-1"][start] != 1 || got.Readiness["auth-api-2"][end] != 0 || got.Weights["auth-api"][start] != 20 {
		t.Fatalf("observed %+v, %v", got, err)
	}
	if !strings.Contains(strings.Join(f.queries, "\n"), `namespace="auth-system"`) {
		t.Fatalf("queries = %v", f.queries)
	}
}

// A Job's pod ends Completed and never ready, so counting it would mark every Project that runs
// a Job, its Vault policy job or a backup, as degraded while each of its Processes serves.
func TestASampleLeavesOutThePodsAJobRan(t *testing.T) {
	f := &fake{status: http.StatusOK, answers: map[string]string{}}
	if _, err := f.serve(t).Samples(t.Context(), "jellyfin", nil, start, end, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	running := `kube_pod_status_ready{namespace="jellyfin-system",condition="true"} unless on(namespace, pod) kube_pod_owner{namespace="jellyfin-system",owner_kind="Job"}`
	want := `sum(` + running + `) / count(` + running + `)`
	if f.queries[0] != want {
		t.Fatalf("pods query = %q, want %q", f.queries[0], want)
	}
}

func TestARolloutLeavesOutThePodsAJobRan(t *testing.T) {
	f := &fake{status: http.StatusOK, answers: map[string]string{}}
	if _, err := f.serve(t).Rollout(t.Context(), "jellyfin", start, end, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	want := `kube_pod_status_ready{namespace="jellyfin-system",condition="true"} unless on(namespace, pod) kube_pod_owner{namespace="jellyfin-system",owner_kind="Job"}`
	if f.queries[0] != want {
		t.Fatalf("readiness query = %q, want %q", f.queries[0], want)
	}
}

func TestARolloutPrometheusRefusesIsAnError(t *testing.T) {
	for name, f := range map[string]*fake{
		"the readiness":       {status: http.StatusOK, answers: map[string]string{"kube_pod": `{"status":"error"}`}},
		"the weights":         {status: http.StatusOK, answers: map[string]string{"flagger": `{"status":"error"}`}},
		"the containers":      {status: http.StatusOK, answers: map[string]string{"_status_running": `{"status":"error"}`}},
		"the native sidecars": {status: http.StatusOK, answers: map[string]string{"restart_policy": `{"status":"error"}`}},
		"the runs":            {status: http.StatusOK, answers: map[string]string{"kube_pod_status_phase": `{"status":"error"}`}},
		"the runs' CronJobs":  {status: http.StatusOK, answers: map[string]string{"kube_job_owner": `{"status":"error"}`}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := f.serve(t).Rollout(t.Context(), "auth", start, end, 30*time.Second); err == nil {
				t.Fatal("an answer that is not one read as a rollout")
			}
		})
	}
	if _, err := (prometheus.Absent{}).Rollout(t.Context(), "auth", start, end, time.Second); !errors.Is(err, domain.ErrNoMetrics) {
		t.Fatalf("error = %v", err)
	}
}

// A rollout reads, beside each pod's readiness and each canary's weight, the containers of the
// pods that serve and every pod a Job ran, leaving the Job's pods out of the containers.
func TestARolloutReadsTheContainersOfThePodsThatServeAndThePodsAJobRan(t *testing.T) {
	f := &fake{status: http.StatusOK, answers: map[string]string{}}
	if _, err := f.serve(t).Rollout(t.Context(), "auth", start, end, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	jobs := `kube_pod_owner{namespace="auth-system",owner_kind="Job"}`
	want := []string{
		`(label_replace(max by (pod, container) (kube_pod_container_status_running{namespace="auth-system"} * 2 + kube_pod_container_status_ready{namespace="auth-system"}), "kind", "container", "", "")` +
			` or label_replace(max by (pod, container) (kube_pod_init_container_status_running{namespace="auth-system"} * 2 + kube_pod_init_container_status_ready{namespace="auth-system"}), "kind", "init", "", "")) unless on(pod) ` + jobs,
		`(label_replace(max by (pod, container, reason) (kube_pod_container_status_terminated_reason{namespace="auth-system"} == 1), "kind", "container", "", "")` +
			` or label_replace(max by (pod, container, reason) (kube_pod_init_container_status_terminated_reason{namespace="auth-system"} == 1), "kind", "init", "", "")) unless on(pod) ` + jobs,
		`(label_replace(max by (pod, container) (kube_pod_container_state_started{namespace="auth-system"}), "kind", "container", "", "")` +
			` or label_replace(max by (pod, container) (kube_pod_init_container_state_started{namespace="auth-system"}), "kind", "init", "", "")) unless on(pod) ` + jobs,
		`max by (pod, container) (kube_pod_init_container_info{namespace="auth-system",restart_policy="Always"})`,
		`max by (namespace, pod, phase) (kube_pod_status_phase{namespace="auth-system"} == 1) * on(namespace, pod) group_left(owner_name) max by (namespace, pod, owner_name) (` + jobs + `)`,
		`(label_replace(max by (pod) (kube_pod_start_time{namespace="auth-system"}), "time", "start", "", "") or label_replace(max by (pod) (kube_pod_completion_time{namespace="auth-system"}), "time", "end", "", "")) and on(pod) ` + jobs,
		`max by (job_name, owner_name) (kube_job_owner{namespace="auth-system",owner_kind="CronJob"})`,
	}
	if len(f.queries) != 2+len(want) {
		t.Fatalf("queries = %v", f.queries)
	}
	for i, q := range want {
		if f.queries[2+i] != q {
			t.Fatalf("query %d = %q, want %q", 2+i, f.queries[2+i], q)
		}
	}
}

func TestEachContainerIsWhetherItRanAndWasReadyHowItEndedAndWhenItStarted(t *testing.T) {
	f := &fake{status: http.StatusOK, answers: map[string]string{
		"_status_running": `{"status":"success","data":{"result":[` +
			`{"metric":{"pod":"qbittorrent-1","container":"vpn","kind":"init"},"values":[[1760961600,"2"],[1760961630,"3"]]},` +
			`{"metric":{"pod":"auth-api-1","container":"vault-agent-init","kind":"init"},"values":[[1760961600,"0"],[1760961630,"1"]]}]}}`,
		"_status_terminated_reason": `{"status":"success","data":{"result":[{"metric":{"pod":"auth-api-1","container":"vault-agent-init","kind":"init","reason":"Completed"},"values":[[1760961630,"1"]]}]}}`,
		"_state_started":            `{"status":"success","data":{"result":[{"metric":{"pod":"auth-api-1","container":"vault-agent-init","kind":"init"},"values":[[1760961600,"1760961610"],[1760961630,"1760961611"]]},{"metric":{"pod":"qbittorrent-1","container":"vpn","kind":"init"},"values":[]}]}}`,
		"restart_policy":            `{"status":"success","data":{"result":[{"metric":{"pod":"qbittorrent-1","container":"vpn"},"values":[[1760961600,"1"]]}]}}`,
	}}
	got, err := f.serve(t).Rollout(t.Context(), "auth", start, end, 30*time.Second)
	if err != nil || len(got.Containers) != 2 {
		t.Fatalf("containers = %+v, %v", got.Containers, err)
	}
	byName := map[string]domain.ContainerSeries{}
	for _, c := range got.Containers {
		byName[c.Name] = c
	}
	vpn, initc := byName["vpn"], byName["vault-agent-init"]
	if !vpn.Init || !vpn.Restarts || vpn.Started != nil || vpn.Pod != "qbittorrent-1" || vpn.Samples[start] != (domain.ContainerSample{Running: true}) || vpn.Samples[end] != (domain.ContainerSample{Running: true, Ready: true}) {
		t.Fatalf("vpn = %+v", vpn)
	}
	if initc.Restarts || initc.Samples[start] != (domain.ContainerSample{}) || initc.Samples[end] != (domain.ContainerSample{Ready: true, Ended: "Completed"}) || !initc.Started.Equal(time.Unix(1760961611, 0)) {
		t.Fatalf("init = %+v", initc)
	}
}

func TestEachPodAJobRanIsItsPhaseEachStepWhenItRanAndItsCronJob(t *testing.T) {
	f := &fake{status: http.StatusOK, answers: map[string]string{
		"kube_pod_status_phase": `{"status":"success","data":{"result":[` +
			`{"metric":{"pod":"vault-policy-29877720-q8w2n","phase":"Running","owner_name":"vault-policy-29877720"},"values":[[1760961600,"1"]]},` +
			`{"metric":{"pod":"vault-policy-29877720-q8w2n","phase":"Succeeded","owner_name":"vault-policy-29877720"},"values":[[1760961630,"1"]]},` +
			`{"metric":{"pod":"auth-migration-up-h3j4k","phase":"Running","owner_name":"auth-migration-up"},"values":[[1760961630,"1"]]}]}}`,
		"kube_pod_completion_time": `{"status":"success","data":{"result":[` +
			`{"metric":{"pod":"vault-policy-29877720-q8w2n","time":"start"},"values":[[1760961600,"1760961598"]]},` +
			`{"metric":{"pod":"vault-policy-29877720-q8w2n","time":"end"},"values":[[1760961630,"1760961604"]]},` +
			`{"metric":{"pod":"gone-already","time":"end"},"values":[[1760961630,"1760961604"]]}]}}`,
		"kube_job_owner": `{"status":"success","data":{"result":[{"metric":{"job_name":"vault-policy-29877720","owner_name":"vault-policy"},"values":[[1760961600,"1"]]}]}}`,
	}}
	got, err := f.serve(t).Rollout(t.Context(), "auth", start, end, 30*time.Second)
	if err != nil || len(got.Jobs) != 2 {
		t.Fatalf("jobs = %+v, %v", got.Jobs, err)
	}
	byPod := map[string]domain.JobPod{}
	for _, j := range got.Jobs {
		byPod[j.Pod] = j
	}
	policy, migration := byPod["vault-policy-29877720-q8w2n"], byPod["auth-migration-up-h3j4k"]
	if policy.Job != "vault-policy-29877720" || policy.CronJob != "vault-policy" || policy.Phases[start] != "Running" || policy.Phases[end] != "Succeeded" ||
		!policy.Start.Equal(time.Unix(1760961598, 0)) || !policy.End.Equal(time.Unix(1760961604, 0)) {
		t.Fatalf("policy = %+v", policy)
	}
	if migration.CronJob != "" || migration.Start != nil || migration.End != nil || migration.Phases[end] != "Running" {
		t.Fatalf("migration = %+v", migration)
	}
}
