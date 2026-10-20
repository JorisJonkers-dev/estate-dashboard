// Package gatus implements the checks context's Monitor on Gatus's HTTP API.
package gatus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/checks/domain"
)

// maxBody is the most of an answer read: a few hundred endpoints of fifty results each.
const maxBody = 16 << 20

// recent is how many of the latest probes a check carries.
const recent = 60

// Gatus reads one Gatus.
type Gatus struct {
	base   *url.URL
	client *http.Client
}

var _ domain.Monitor = (*Gatus)(nil)

// New returns a reader of the Gatus at base, which must be an absolute http(s) URL.
func New(base string, client *http.Client) (*Gatus, error) {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("gatus: %q is not an http(s) URL", base)
	}
	return &Gatus{base: u, client: client}, nil
}

var errStatus = errors.New("gatus: unexpected status")

type endpoint struct {
	Name    string   `json:"name"`
	Group   string   `json:"group"`
	Key     string   `json:"key"`
	Results []result `json:"results"`
}

type result struct {
	Status     int       `json:"status"`
	Hostname   string    `json:"hostname"`
	Duration   int64     `json:"duration"`
	Success    bool      `json:"success"`
	Timestamp  time.Time `json:"timestamp"`
	Errors     []string  `json:"errors"`
	Conditions []struct {
		Condition string `json:"condition"`
		Success   bool   `json:"success"`
	} `json:"conditionResults"`
}

// Checks implements domain.Monitor: every endpoint Gatus probes, with its latest result.
func (g *Gatus) Checks(ctx context.Context) ([]domain.Check, error) {
	u := g.base.JoinPath("/api/v1/endpoints/statuses")
	u.RawQuery = url.Values{"page": {"1"}, "pageSize": {strconv.Itoa(recent)}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("gatus: %w", err)
	}
	res, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gatus: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", errStatus, res.StatusCode)
	}
	var endpoints []endpoint
	if err := json.NewDecoder(io.LimitReader(res.Body, maxBody)).Decode(&endpoints); err != nil {
		return nil, fmt.Errorf("gatus: read the statuses: %w", err)
	}
	checks := make([]domain.Check, 0, len(endpoints))
	for _, e := range endpoints {
		checks = append(checks, check(e))
	}
	return checks, nil
}

func check(e endpoint) domain.Check {
	c := domain.Check{Key: e.Key, Name: e.Name, Group: e.Group, Recent: []bool{}}
	results := e.Results[max(0, len(e.Results)-recent):]
	for _, r := range results {
		c.Recent = append(c.Recent, r.Success)
	}
	if len(results) == 0 {
		return c
	}
	last := results[len(results)-1]
	at := last.Timestamp.UTC()
	c.Host, c.At, c.Up, c.Status, c.Duration = last.Hostname, &at, last.Success, last.Status, time.Duration(last.Duration)
	if len(last.Errors) > 0 {
		c.Error = last.Errors[0]
	}
	for _, cond := range last.Conditions {
		c.Conditions = append(c.Conditions, domain.Condition{Condition: cond.Condition, Success: cond.Success})
	}
	return c
}

// Absent is the monitor of a dashboard that runs without one: every read is domain.ErrNoMonitor.
type Absent struct{}

var _ domain.Monitor = Absent{}

// Checks implements domain.Monitor.
func (Absent) Checks(context.Context) ([]domain.Check, error) { return nil, domain.ErrNoMonitor }
