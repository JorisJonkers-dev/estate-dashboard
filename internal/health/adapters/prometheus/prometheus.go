// Package prometheus implements the health context's Metrics port on Prometheus's HTTP API: range
// queries over what kube-state-metrics, Flagger, Flux and Gatus write.
package prometheus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/health/domain"
)

// maxBody is the most of an answer read: three series of an hour at thirty seconds are far less.
const maxBody = 4 << 20

// Prometheus reads one Prometheus.
type Prometheus struct {
	base   *url.URL
	client *http.Client
}

var _ domain.Metrics = (*Prometheus)(nil)

// New returns a reader of the Prometheus at base, which must be an absolute http(s) URL.
func New(base string, client *http.Client) (*Prometheus, error) {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("prometheus: %q is not an http(s) URL", base)
	}
	return &Prometheus{base: u, client: client}, nil
}

var errStatus = errors.New("prometheus: unexpected status")

// running is each pod of the namespace that serves, by whether it is ready: every pod but those
// a Job ran, which end Completed and never ready.
func running(namespace string) string {
	return fmt.Sprintf(`kube_pod_status_ready{namespace=%q,condition="true"} unless on(namespace, pod) kube_pod_owner{namespace=%q,owner_kind="Job"}`, namespace, namespace)
}

// Samples implements domain.Metrics. A check whose key is not one Gatus writes is left out.
func (p *Prometheus) Samples(ctx context.Context, project string, checks []string, start, end time.Time, step time.Duration) (map[time.Time]domain.Sample, error) {
	namespace := project + "-system"
	pods := fmt.Sprintf(`sum(%s) / count(%s)`, running(namespace), running(namespace))
	unit := fmt.Sprintf(`min(gotk_resource_ready{customresource_kind="Kustomization",name=%q,type="Ready"})`, "apps-"+project)
	samples := map[time.Time]domain.Sample{}
	set := func(query string, into func(*domain.Sample, float64)) error {
		points, err := p.series(ctx, query, start, end, step)
		if err != nil {
			return err
		}
		for at, v := range points {
			s := samples[at]
			into(&s, v)
			samples[at] = s
		}
		return nil
	}
	if err := set(pods, func(s *domain.Sample, v float64) { s.PodsReady = &v }); err != nil {
		return nil, err
	}
	if err := set(unit, func(s *domain.Sample, v float64) { s.UnitReady = &v }); err != nil {
		return nil, err
	}
	if keys := checkKeys(checks); keys != "" {
		query := fmt.Sprintf(`min(gatus_results_endpoint_success{key=~%q})`, keys)
		if err := set(query, func(s *domain.Sample, v float64) { s.ChecksUp = &v }); err != nil {
			return nil, err
		}
	}
	return samples, nil
}

// gatusKey is what a key Gatus writes may hold, so a key never widens the query it is put in.
var gatusKey = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func checkKeys(checks []string) string {
	kept := make([]string, 0, len(checks))
	for _, c := range checks {
		if gatusKey.MatchString(c) {
			kept = append(kept, regexp.QuoteMeta(c))
		}
	}
	return strings.Join(kept, "|")
}

type rangeAnswer struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Values [][2]any          `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// series runs one range query whose answer is one series, by the second each value is at.
func (p *Prometheus) series(ctx context.Context, query string, start, end time.Time, step time.Duration) (map[time.Time]float64, error) {
	answer, err := p.query(ctx, query, start, end, step)
	if err != nil {
		return nil, err
	}
	return by(answer, "")[""], nil
}

// jobPods is each pod a Job ran in the namespace.
func jobPods(namespace string) string {
	return fmt.Sprintf(`kube_pod_owner{namespace=%q,owner_kind="Job"}`, namespace)
}

// eachContainer is a query of both kinds of container of the pods that serve, each series
// labelled with its kind, container or init: of names its kube_pod_container_ metric with %[1]s,
// and the namespace with %[2]q.
func eachContainer(namespace, of string) string {
	kind := func(family, kind string) string {
		return fmt.Sprintf(`label_replace(%s, "kind", %q, "", "")`, fmt.Sprintf(of, family, namespace), kind)
	}
	return fmt.Sprintf(`(%s or %s) unless on(pod) %s`, kind("kube_pod_container", "container"), kind("kube_pod_init_container", "init"), jobPods(namespace))
}

// Rollout implements domain.Metrics: each pod's readiness and containers, each canary's weight,
// and each pod a Job ran in the Project's namespace.
func (p *Prometheus) Rollout(ctx context.Context, project string, start, end time.Time, step time.Duration) (domain.Observed, error) {
	namespace := project + "-system"
	read := func(query string) ([]labelled, error) { return p.query(ctx, query, start, end, step) }
	// A Job's pod runs to completion and is never ready: it is no Process switching to a release,
	// but a run of its Job.
	readiness, err := read(running(namespace))
	if err != nil {
		return domain.Observed{}, err
	}
	weights, err := read(fmt.Sprintf(`flagger_canary_weight{namespace=%q}`, namespace))
	if err != nil {
		return domain.Observed{}, err
	}
	containers, err := containersIn(namespace, read)
	if err != nil {
		return domain.Observed{}, err
	}
	jobs, err := jobsIn(namespace, read)
	if err != nil {
		return domain.Observed{}, err
	}
	return domain.Observed{Readiness: by(readiness, "pod"), Weights: by(weights, "name"), Containers: containers, Jobs: jobs}, nil
}

// readAll runs each query in turn, its answer the series it returned.
func readAll(read func(string) ([]labelled, error), queries ...string) ([][]labelled, error) {
	out := make([][]labelled, 0, len(queries))
	for _, q := range queries {
		answer, err := read(q)
		if err != nil {
			return nil, err
		}
		out = append(out, answer)
	}
	return out, nil
}

// containersIn is each container of the pods that serve: whether it ran and was ready each step,
// the reason it had ended with, when it started, and whether an init container restarts with its
// pod.
func containersIn(namespace string, read func(string) ([]labelled, error)) ([]domain.ContainerSeries, error) {
	answers, err := readAll(read,
		eachContainer(namespace, `max by (pod, container) (%[1]s_status_running{namespace=%[2]q} * 2 + %[1]s_status_ready{namespace=%[2]q})`),
		eachContainer(namespace, `max by (pod, container, reason) (%[1]s_status_terminated_reason{namespace=%[2]q} == 1)`),
		eachContainer(namespace, `max by (pod, container) (%[1]s_state_started{namespace=%[2]q})`),
		fmt.Sprintf(`max by (pod, container) (kube_pod_init_container_info{namespace=%q,restart_policy="Always"})`, namespace),
	)
	if err != nil {
		return nil, err
	}
	all := map[string]*domain.ContainerSeries{}
	of := func(l map[string]string) *domain.ContainerSeries {
		key := l["pod"] + "/" + l["container"] + "/" + l["kind"]
		if all[key] == nil {
			all[key] = &domain.ContainerSeries{Pod: l["pod"], Name: l["container"], Init: l["kind"] == "init", Samples: map[time.Time]domain.ContainerSample{}}
		}
		return all[key]
	}
	for _, s := range answers[0] {
		c := of(s.labels)
		for at, v := range s.points {
			sample := c.Samples[at]
			sample.Running, sample.Ready = v >= 2, int(v)%2 == 1
			c.Samples[at] = sample
		}
	}
	for _, s := range answers[1] {
		c := of(s.labels)
		for at := range s.points {
			sample := c.Samples[at]
			sample.Ended = s.labels["reason"]
			c.Samples[at] = sample
		}
	}
	for _, s := range answers[2] {
		c := of(s.labels)
		c.Started = latest(s.points)
	}
	for _, s := range answers[3] {
		s.labels["kind"] = "init"
		of(s.labels).Restarts = true
	}
	out := make([]domain.ContainerSeries, 0, len(all))
	for _, c := range all {
		out = append(out, *c)
	}
	return out, nil
}

// jobsIn is each pod a Job ran: its phase each step, when it started and finished by its own
// record, and the CronJob that made its Job, where one did.
func jobsIn(namespace string, read func(string) ([]labelled, error)) ([]domain.JobPod, error) {
	answers, err := readAll(read,
		fmt.Sprintf(`max by (namespace, pod, phase) (kube_pod_status_phase{namespace=%q} == 1) * on(namespace, pod) group_left(owner_name) max by (namespace, pod, owner_name) (%s)`, namespace, jobPods(namespace)),
		fmt.Sprintf(`(label_replace(max by (pod) (kube_pod_start_time{namespace=%q}), "time", "start", "", "") or label_replace(max by (pod) (kube_pod_completion_time{namespace=%q}), "time", "end", "", "")) and on(pod) %s`, namespace, namespace, jobPods(namespace)),
		fmt.Sprintf(`max by (job_name, owner_name) (kube_job_owner{namespace=%q,owner_kind="CronJob"})`, namespace),
	)
	if err != nil {
		return nil, err
	}
	cronJobs := map[string]string{}
	for _, s := range answers[2] {
		cronJobs[s.labels["job_name"]] = s.labels["owner_name"]
	}
	pods := map[string]*domain.JobPod{}
	for _, s := range answers[0] {
		pod := s.labels["pod"]
		if pods[pod] == nil {
			job := s.labels["owner_name"]
			pods[pod] = &domain.JobPod{Pod: pod, Job: job, CronJob: cronJobs[job], Phases: map[time.Time]string{}}
		}
		for at := range s.points {
			pods[pod].Phases[at] = s.labels["phase"]
		}
	}
	for _, s := range answers[1] {
		if j := pods[s.labels["pod"]]; j != nil {
			if s.labels["time"] == "start" {
				j.Start = latest(s.points)
			} else {
				j.End = latest(s.points)
			}
		}
	}
	out := make([]domain.JobPod, 0, len(pods))
	for _, j := range pods {
		out = append(out, *j)
	}
	return out, nil
}

// latest is the moment the last sample of a timestamp metric names.
func latest(points map[time.Time]float64) *time.Time {
	var last time.Time
	var value float64
	for at, v := range points {
		if at.After(last) {
			last, value = at, v
		}
	}
	if last.IsZero() {
		return nil
	}
	t := time.Unix(int64(value), 0).UTC()
	return &t
}

// labelled is one series of an answer: its labels, and its values by when each was sampled.
type labelled struct {
	labels map[string]string
	points map[time.Time]float64
}

// by is the series of an answer by the value of label; every series under "" without one.
func by(answer []labelled, label string) domain.Series {
	out := domain.Series{}
	for _, s := range answer {
		key := s.labels[label]
		if out[key] == nil {
			out[key] = map[time.Time]float64{}
		}
		maps.Copy(out[key], s.points)
	}
	return out
}

// query runs one range query, its answer each series it returned.
func (p *Prometheus) query(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]labelled, error) {
	u := p.base.JoinPath("/api/v1/query_range")
	u.RawQuery = url.Values{
		"query": {query},
		"start": {strconv.FormatInt(start.Unix(), 10)},
		"end":   {strconv.FormatInt(end.Unix(), 10)},
		"step":  {strconv.Itoa(int(step.Seconds()))},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("prometheus: %w", err)
	}
	res, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prometheus: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", errStatus, res.StatusCode)
	}
	var answer rangeAnswer
	if err := json.NewDecoder(io.LimitReader(res.Body, maxBody)).Decode(&answer); err != nil || answer.Status != "success" {
		return nil, fmt.Errorf("prometheus: read the answer: %w", errors.Join(err, errStatus))
	}
	out := make([]labelled, 0, len(answer.Data.Result))
	for _, r := range answer.Data.Result {
		s := labelled{labels: r.Metric, points: map[time.Time]float64{}}
		if s.labels == nil {
			s.labels = map[string]string{}
		}
		for _, pair := range r.Values {
			at, okAt := pair[0].(float64)
			text, okValue := pair[1].(string)
			v, err := strconv.ParseFloat(text, 64)
			if !okAt || !okValue || err != nil {
				continue
			}
			s.points[time.Unix(int64(at), 0).UTC()] = v
		}
		out = append(out, s)
	}
	return out, nil
}

// Absent is the metrics of a dashboard that runs without Prometheus: every read is
// domain.ErrNoMetrics.
type Absent struct{}

var _ domain.Metrics = Absent{}

// Rollout implements domain.Metrics.
func (Absent) Rollout(context.Context, string, time.Time, time.Time, time.Duration) (domain.Observed, error) {
	return domain.Observed{}, domain.ErrNoMetrics
}

// Samples implements domain.Metrics.
func (Absent) Samples(context.Context, string, []string, time.Time, time.Time, time.Duration) (map[time.Time]domain.Sample, error) {
	return nil, domain.ErrNoMetrics
}
