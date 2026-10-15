// Package loki implements the logs context's Store on Loki's HTTP API.
package loki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/logs/domain"
)

// maxBody is the most of a reply the adapter reads.
const maxBody = 16 << 20

// spans is how many containers' first and latest lines are read at once.
const spans = 8

// maxSteps bounds the points of the line count: a week is steps of about ten minutes.
const maxSteps = 1000

// Loki reads one Loki.
type Loki struct {
	base   *url.URL
	client *http.Client
	now    func() time.Time
}

var _ domain.Store = (*Loki)(nil)

// New returns a reader of the Loki at base, which must be an absolute http(s) URL.
func New(base string, client *http.Client, now func() time.Time) (*Loki, error) {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("loki: %q is not an http(s) URL", base)
	}
	return &Loki{base: u, client: client, now: now}, nil
}

var errStatus = errors.New("loki: unexpected status")

func (l *Loki) get(ctx context.Context, path string, query url.Values, into any) error {
	u := l.base.JoinPath(path)
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	res, err := l.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: GET %s answered %d", errStatus, path, res.StatusCode)
	}
	return json.Unmarshal(raw, into)
}

// selector is the LogQL stream selector for the labels; each value is quoted so none can escape.
// quiet drops what the collector writes into a stream once its container is gone.
const quiet = ` != "unable to retrieve container logs for"`

func selector(labels ...[2]string) string {
	parts := make([]string, 0, len(labels))
	for _, l := range labels {
		parts = append(parts, l[0]+"="+strconv.Quote(l[1]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func nanos(t time.Time) string { return strconv.FormatInt(t.UnixNano(), 10) }

// Streams implements domain.Store: one Stream per pod and container, by pod then container.
// One line count finds them, then one-line reads inside two steps each say when they wrote.
func (l *Loki) Streams(ctx context.Context, namespace string, since time.Time) ([]domain.Stream, error) {
	end := l.now()
	step := max(time.Minute, end.Sub(since)/maxSteps).Truncate(time.Second)
	// Points fall at start, start+step, ...; a start a whole number of steps back puts the last one at end.
	start := end.Add(-step * time.Duration((end.Sub(since)+step-1)/step))
	streams, err := l.counted(ctx, namespace, start, end, step)
	if err != nil {
		return nil, err
	}
	if err := l.refine(ctx, namespace, streams, step); err != nil {
		return nil, err
	}
	slices.SortFunc(streams, func(a, b domain.Stream) int {
		if c := strings.Compare(a.Pod, b.Pod); c != 0 {
			return c
		}
		return strings.Compare(a.Container, b.Container)
	})
	return streams, nil
}

// counted is every container that wrote between since and end, First and Last at step ends.
func (l *Loki) counted(ctx context.Context, namespace string, since, end time.Time, step time.Duration) ([]domain.Stream, error) {
	var reply struct {
		Data struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Values [][2]any          `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	query := url.Values{
		"query": {fmt.Sprintf("sum by (pod, container, version) (count_over_time(%s%s[%ds]))", selector([2]string{"namespace", namespace}), quiet, int(step.Seconds()))},
		"start": {nanos(since)},
		"end":   {nanos(end)},
		"step":  {strconv.Itoa(int(step.Seconds()))},
	}
	if err := l.get(ctx, "/loki/api/v1/query_range", query, &reply); err != nil {
		return nil, fmt.Errorf("loki: read the streams of %s: %w", namespace, err)
	}
	streams := []domain.Stream{}
	for _, r := range reply.Data.Result {
		s := domain.Stream{Pod: r.Metric["pod"], Container: r.Metric["container"], Version: r.Metric["version"]}
		first, okFirst := stepAt(r.Values, 0)
		last, okLast := stepAt(r.Values, len(r.Values)-1)
		if s.Pod == "" || s.Container == "" || !okFirst || !okLast {
			continue
		}
		s.First, s.Last = first, last
		streams = append(streams, s)
	}
	return streams, nil
}

// refine narrows First and Last from step ends to when each line was written.
func (l *Loki) refine(ctx context.Context, namespace string, streams []domain.Stream, step time.Duration) error {
	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(spans)
	for i := range streams {
		group.Go(func() error {
			s := &streams[i]
			first, err := l.oneLine(ctx, namespace, *s, s.First.Add(-step), s.First, "forward")
			if err != nil {
				return fmt.Errorf("loki: read when %s/%s first wrote: %w", s.Pod, s.Container, err)
			}
			last, err := l.oneLine(ctx, namespace, *s, s.Last.Add(-step), s.Last, "backward")
			if err != nil {
				return fmt.Errorf("loki: read when %s/%s last wrote: %w", s.Pod, s.Container, err)
			}
			if !first.IsZero() {
				s.First = first
			}
			if !last.IsZero() {
				s.Last = last
			}
			return nil
		})
	}
	return group.Wait()
}

// stepAt is the time of a series' i-th point.
func stepAt(values [][2]any, i int) (time.Time, bool) {
	if i < 0 || i >= len(values) {
		return time.Time{}, false
	}
	seconds, ok := values[i][0].(float64)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(0, int64(seconds*float64(time.Second))).UTC(), true
}

// oneLine is when the first (forward) or latest (backward) line between from and to was written.
func (l *Loki) oneLine(ctx context.Context, namespace string, s domain.Stream, from, to time.Time, direction string) (time.Time, error) {
	var reply struct {
		Data struct {
			Result []struct {
				Values [][2]string `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	query := url.Values{
		"query":     {selector([2]string{"namespace", namespace}, [2]string{"pod", s.Pod}, [2]string{"container", s.Container}) + quiet},
		"start":     {nanos(from)},
		"end":       {nanos(to)},
		"limit":     {"1"},
		"direction": {direction},
	}
	if err := l.get(ctx, "/loki/api/v1/query_range", query, &reply); err != nil {
		return time.Time{}, err
	}
	for _, r := range reply.Data.Result {
		for _, v := range r.Values {
			if ns, err := strconv.ParseInt(v[0], 10, 64); err == nil {
				return time.Unix(0, ns).UTC(), nil
			}
		}
	}
	return time.Time{}, nil
}

// Lines implements domain.Store: the newest limit lines since then, returned oldest first.
func (l *Loki) Lines(ctx context.Context, namespace string, s domain.Stream, since time.Time, limit int) ([]domain.Line, error) {
	var reply struct {
		Data struct {
			Result []struct {
				Values [][2]string `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	query := url.Values{
		"query":     {selector([2]string{"namespace", namespace}, [2]string{"pod", s.Pod}, [2]string{"container", s.Container}) + quiet},
		"start":     {nanos(since)},
		"end":       {nanos(l.now())},
		"limit":     {strconv.Itoa(limit)},
		"direction": {"backward"},
	}
	if err := l.get(ctx, "/loki/api/v1/query_range", query, &reply); err != nil {
		return nil, fmt.Errorf("loki: read the logs of %s/%s: %w", s.Pod, s.Container, err)
	}
	lines := []domain.Line{}
	for _, r := range reply.Data.Result {
		for _, v := range r.Values {
			ns, err := strconv.ParseInt(v[0], 10, 64)
			if err != nil {
				continue
			}
			lines = append(lines, domain.Line{At: time.Unix(0, ns).UTC(), Text: v[1]})
		}
	}
	slices.SortStableFunc(lines, func(a, b domain.Line) int { return a.At.Compare(b.At) })
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return lines, nil
}

// Absent is the store of a dashboard that runs without one: every read is domain.ErrNoStore.
type Absent struct{}

var _ domain.Store = Absent{}

// Streams implements domain.Store.
func (Absent) Streams(context.Context, string, time.Time) ([]domain.Stream, error) {
	return nil, domain.ErrNoStore
}

// Lines implements domain.Store.
func (Absent) Lines(context.Context, string, domain.Stream, time.Time, int) ([]domain.Line, error) {
	return nil, domain.ErrNoStore
}
