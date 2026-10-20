// Package prometheus implements the health context's Metrics port on Prometheus's HTTP API: three
// range queries, one per source of a Project's health.
package prometheus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// Samples implements domain.Metrics. A check whose key is not one Gatus writes is left out.
func (p *Prometheus) Samples(ctx context.Context, project string, checks []string, start, end time.Time, step time.Duration) (map[time.Time]domain.Sample, error) {
	namespace := project + "-system"
	pods := fmt.Sprintf(`sum(kube_pod_status_ready{namespace=%q,condition="true"}) / count(kube_pod_status_ready{namespace=%q,condition="true"})`, namespace, namespace)
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
			Values [][2]any `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// series runs one range query whose answer is one series, by the second each value is at.
func (p *Prometheus) series(ctx context.Context, query string, start, end time.Time, step time.Duration) (map[time.Time]float64, error) {
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
	out := map[time.Time]float64{}
	for _, r := range answer.Data.Result {
		for _, pair := range r.Values {
			at, okAt := pair[0].(float64)
			text, okValue := pair[1].(string)
			v, err := strconv.ParseFloat(text, 64)
			if !okAt || !okValue || err != nil {
				continue
			}
			out[time.Unix(int64(at), 0).UTC()] = v
		}
	}
	return out, nil
}

// Absent is the metrics of a dashboard that runs without Prometheus: every read is
// domain.ErrNoMetrics.
type Absent struct{}

var _ domain.Metrics = Absent{}

// Samples implements domain.Metrics.
func (Absent) Samples(context.Context, string, []string, time.Time, time.Time, time.Duration) (map[time.Time]domain.Sample, error) {
	return nil, domain.ErrNoMetrics
}
