// Package loki implements the logs context's Store on Loki's HTTP API: the series API for which
// containers wrote logs, and query_range for what they wrote. Streams are found by the namespace,
// pod and container labels Alloy puts on every pod's logs.
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

	"github.com/JorisJonkers-dev/estate-dashboard/internal/logs/domain"
)

// maxBody is the most of a reply the adapter reads.
const maxBody = 16 << 20

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

// selector is the LogQL stream selector for the labels given. Every value has already been held
// to a name's pattern, so none can carry a quote.
func selector(labels ...[2]string) string {
	parts := make([]string, 0, len(labels))
	for _, l := range labels {
		parts = append(parts, l[0]+`="`+l[1]+`"`)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func nanos(t time.Time) string { return strconv.FormatInt(t.UnixNano(), 10) }

// Streams implements domain.Store: one Stream per pod and container, by pod then container.
func (l *Loki) Streams(ctx context.Context, namespace string, since time.Time) ([]domain.Stream, error) {
	var reply struct {
		Data []map[string]string `json:"data"`
	}
	query := url.Values{"match[]": {selector([2]string{"namespace", namespace})}, "start": {nanos(since)}, "end": {nanos(l.now())}}
	if err := l.get(ctx, "/loki/api/v1/series", query, &reply); err != nil {
		return nil, fmt.Errorf("loki: read the streams of %s: %w", namespace, err)
	}
	seen := map[domain.Stream]bool{}
	streams := []domain.Stream{}
	for _, labels := range reply.Data {
		s := domain.Stream{Pod: labels["pod"], Container: labels["container"]}
		if s.Pod == "" || s.Container == "" || seen[s] {
			continue
		}
		seen[s] = true
		streams = append(streams, s)
	}
	slices.SortFunc(streams, func(a, b domain.Stream) int {
		if c := strings.Compare(a.Pod, b.Pod); c != 0 {
			return c
		}
		return strings.Compare(a.Container, b.Container)
	})
	return streams, nil
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
		"query":     {selector([2]string{"namespace", namespace}, [2]string{"pod", s.Pod}, [2]string{"container", s.Container})},
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
