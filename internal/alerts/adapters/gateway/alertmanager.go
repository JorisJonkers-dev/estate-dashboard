// Package gateway implements the Alertmanager port on Alertmanager's v2 HTTP API: the alerts it
// holds, and the silences the dashboard sets and ends, which are the dashboard's one write.
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/domain"
)

// maxBody is the most of an Alertmanager reply the gateway reads.
const maxBody = 8 << 20

// Alertmanager talks to one Alertmanager.
type Alertmanager struct {
	base   *url.URL
	client *http.Client
}

var _ domain.Alertmanager = (*Alertmanager)(nil)

// New returns a gateway to the Alertmanager at base, which must be an absolute http(s) URL.
func New(base string, client *http.Client) (*Alertmanager, error) {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("alertmanager: %q is not an http(s) URL", base)
	}
	return &Alertmanager{base: u, client: client}, nil
}

var errStatus = errors.New("alertmanager: unexpected status")

func (a *Alertmanager) do(ctx context.Context, method, path string, body any, want int, into any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base.JoinPath(path).String(), reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	res, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		return err
	}
	if res.StatusCode != want {
		return fmt.Errorf("%w: %s %s answered %d", errStatus, method, path, res.StatusCode)
	}
	if into == nil {
		return nil
	}
	return json.Unmarshal(raw, into)
}

type wireAlert struct {
	Fingerprint string            `json:"fingerprint"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	Status      struct {
		State      string   `json:"state"`
		SilencedBy []string `json:"silencedBy"`
	} `json:"status"`
}

// Alerts implements domain.Alertmanager: every alert it holds, silenced or not.
func (a *Alertmanager) Alerts(ctx context.Context) ([]domain.Alert, error) {
	var wire []wireAlert
	if err := a.do(ctx, http.MethodGet, "/api/v2/alerts", nil, http.StatusOK, &wire); err != nil {
		return nil, fmt.Errorf("alertmanager: list alerts: %w", err)
	}
	alerts := make([]domain.Alert, 0, len(wire))
	for _, w := range wire {
		alert, err := domain.NewAlert(w.Fingerprint, w.Labels, w.Annotations, w.StartsAt.UTC(), slices.Clone(w.Status.SilencedBy))
		if err != nil {
			return nil, fmt.Errorf("alertmanager: %w", err)
		}
		alerts = append(alerts, alert)
	}
	return alerts, nil
}

type wireMatcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
	IsEqual bool   `json:"isEqual"`
}

type wireSilence struct {
	Matchers  []wireMatcher `json:"matchers"`
	StartsAt  time.Time     `json:"startsAt"`
	EndsAt    time.Time     `json:"endsAt"`
	CreatedBy string        `json:"createdBy"`
	Comment   string        `json:"comment"`
}

// Silence implements domain.Alertmanager: one exact matcher per label, in name order.
func (a *Alertmanager) Silence(ctx context.Context, r domain.SilenceRequest) (string, error) {
	names := make([]string, 0, len(r.Matchers))
	for name := range r.Matchers {
		names = append(names, name)
	}
	slices.Sort(names)
	matchers := make([]wireMatcher, 0, len(names))
	for _, name := range names {
		matchers = append(matchers, wireMatcher{Name: name, Value: r.Matchers[name], IsEqual: true})
	}
	var created struct {
		SilenceID string `json:"silenceID"`
	}
	body := wireSilence{Matchers: matchers, StartsAt: r.StartsAt, EndsAt: r.EndsAt, CreatedBy: r.CreatedBy, Comment: r.Comment}
	if err := a.do(ctx, http.MethodPost, "/api/v2/silences", body, http.StatusOK, &created); err != nil {
		return "", fmt.Errorf("alertmanager: create silence: %w", err)
	}
	if created.SilenceID == "" {
		return "", errors.New("alertmanager: create silence: no id in the reply")
	}
	return created.SilenceID, nil
}

// Expire implements domain.Alertmanager.
func (a *Alertmanager) Expire(ctx context.Context, id string) error {
	if err := a.do(ctx, http.MethodDelete, "/api/v2/silence/"+id, nil, http.StatusOK, nil); err != nil {
		return fmt.Errorf("alertmanager: expire silence %s: %w", id, err)
	}
	return nil
}
