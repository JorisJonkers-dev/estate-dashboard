package gateway_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/adapters/gateway"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/domain"
)

// alertmanagerImage is the Alertmanager the acceptance case runs against.
const alertmanagerImage = "prom/alertmanager:v0.28.1"

var start = time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)

func TestNewTakesOnlyAnHTTPURL(t *testing.T) {
	for _, base := range []string{"", "alertmanager:9093", "ftp://alertmanager", "http://", "::"} {
		if _, err := gateway.New(base, http.DefaultClient); err == nil {
			t.Errorf("New(%q) accepted", base)
		}
	}
	if _, err := gateway.New("https://alertmanager.observability-system.svc:9093", http.DefaultClient); err != nil {
		t.Fatal(err)
	}
}

// fake is an Alertmanager that answers from canned replies, and records what it was sent.
type fake struct {
	status int
	reply  string
	paths  []string
	bodies []string
}

func (f *fake) serve(t *testing.T) *gateway.Alertmanager {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		f.paths = append(f.paths, r.Method+" "+r.URL.Path)
		f.bodies = append(f.bodies, string(raw))
		w.WriteHeader(f.status)
		_, _ = io.WriteString(w, f.reply)
	}))
	t.Cleanup(srv.Close)
	am, err := gateway.New(srv.URL+"/base", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return am
}

func TestTheAlertsAreReadWithTheirSilences(t *testing.T) {
	f := &fake{status: http.StatusOK, reply: `[{"fingerprint":"9f2c","labels":{"alertname":"ReleaseHeld","alert_class":"page"},"annotations":{"summary":"held"},"startsAt":"2026-10-03T09:00:00+02:00","status":{"state":"suppressed","silencedBy":["s-1"]}}]`}
	got, err := f.serve(t).Alerts(t.Context())
	want := []domain.Alert{{Fingerprint: "9f2c", Name: "ReleaseHeld", Class: domain.Page, Summary: "held", StartsAt: start, Labels: map[string]string{"alertname": "ReleaseHeld", "alert_class": "page"}, SilencedBy: []string{"s-1"}}}
	if err != nil || !reflect.DeepEqual(got, want) || got[0].StartsAt.Location() != time.UTC {
		t.Fatalf("alerts = %+v, %v", got, err)
	}
	if f.paths[0] != "GET /base/api/v2/alerts" {
		t.Fatalf("asked %v", f.paths)
	}
}

func TestAReplyTheGatewayCannotReadIsAnError(t *testing.T) {
	for name, f := range map[string]*fake{
		"a failure":        {status: http.StatusInternalServerError, reply: "[]"},
		"not JSON":         {status: http.StatusOK, reply: "<html>"},
		"an alert unnamed": {status: http.StatusOK, reply: `[{"fingerprint":"1","labels":{},"startsAt":"2026-10-03T07:00:00Z"}]`},
	} {
		if _, err := f.serve(t).Alerts(t.Context()); err == nil {
			t.Errorf("%s: read as alerts", name)
		}
	}
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	am, _ := gateway.New(gone.URL, http.DefaultClient)
	if _, err := am.Alerts(t.Context()); err == nil {
		t.Fatal("an Alertmanager that is gone answered")
	}
	if _, err := am.Silence(t.Context(), domain.SilenceRequest{}); err == nil {
		t.Fatal("an Alertmanager that is gone set a silence")
	}
}

func TestASilenceIsOneExactMatcherPerLabelInNameOrder(t *testing.T) {
	f := &fake{status: http.StatusOK, reply: `{"silenceID":"s-1"}`}
	am := f.serve(t)
	id, err := am.Silence(t.Context(), domain.SilenceRequest{
		Matchers: map[string]string{"namespace": "auth-system", "alertname": "ReleaseHeld"},
		StartsAt: start, EndsAt: start.Add(time.Hour), CreatedBy: "joris", Comment: "why",
	})
	if err != nil || id != "s-1" || f.paths[0] != "POST /base/api/v2/silences" {
		t.Fatalf("silence = %q, %v, %v", id, err, f.paths)
	}
	want := `{"matchers":[{"name":"alertname","value":"ReleaseHeld","isRegex":false,"isEqual":true},{"name":"namespace","value":"auth-system","isRegex":false,"isEqual":true}],"startsAt":"2026-10-03T07:00:00Z","endsAt":"2026-10-03T08:00:00Z","createdBy":"joris","comment":"why"}`
	if f.bodies[0] != want {
		t.Fatalf("sent %s", f.bodies[0])
	}
	f.reply = `{}`
	if _, err := am.Silence(t.Context(), domain.SilenceRequest{}); err == nil {
		t.Fatal("a reply without an id was read as a silence")
	}
	f.status = http.StatusBadRequest
	if _, err := am.Silence(t.Context(), domain.SilenceRequest{}); err == nil {
		t.Fatal("a refused silence was read as set")
	}
}

func TestExpiringASilenceDeletesIt(t *testing.T) {
	f := &fake{status: http.StatusOK}
	if err := f.serve(t).Expire(t.Context(), "s-1"); err != nil || f.paths[0] != "DELETE /base/api/v2/silence/s-1" {
		t.Fatalf("expire = %v, %v", err, f.paths)
	}
	f.status = http.StatusNotFound
	if err := f.serve(t).Expire(t.Context(), "s-1"); err == nil {
		t.Fatal("a silence Alertmanager does not know was read as expired")
	}
}

// TestASilenceSetHereIsOneAlertmanagerHolds is the acceptance case of
// JorisJonkers-dev/estate-dashboard#3: a real Alertmanager, an alert in it, a silence set through
// the gateway, and the alert silenced by it; then the silence ended.
func TestASilenceSetHereIsOneAlertmanagerHolds(t *testing.T) {
	if testing.Short() {
		t.Skip("needs Docker; skipped with -short")
	}
	ctx := context.Background()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        alertmanagerImage,
			ExposedPorts: []string{"9093/tcp"},
			WaitingFor:   wait.ForHTTP("/-/ready").WithPort("9093/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })
	endpoint, err := c.PortEndpoint(ctx, "9093/tcp", "http")
	if err != nil {
		t.Fatal(err)
	}
	labels := map[string]string{"alertname": "ReleaseHeld", "alert_class": "urgent", "namespace": "auth-system"}
	fire, _ := json.Marshal([]map[string]any{{"labels": labels, "annotations": map[string]string{"summary": "held"}, "startsAt": time.Now().UTC().Add(-time.Minute)}})
	post, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/api/v2/alerts", bytes.NewReader(fire))
	post.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(post)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("fire an alert: %v %v", res, err)
	}
	_ = res.Body.Close()

	am, err := gateway.New(endpoint, &http.Client{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	firing := waitFor(t, func() (domain.Alert, bool) {
		alerts, err := am.Alerts(ctx)
		return first(alerts), err == nil && len(alerts) == 1
	})
	if firing.Name != "ReleaseHeld" || firing.Class != domain.Urgent || len(firing.SilencedBy) != 0 {
		t.Fatalf("firing = %+v", firing)
	}

	now := time.Now().UTC()
	id, err := am.Silence(ctx, domain.SilenceRequest{Matchers: firing.Labels, StartsAt: now, EndsAt: now.Add(time.Hour), CreatedBy: "joris", Comment: "from the test"})
	if err != nil {
		t.Fatal(err)
	}
	silenced := waitFor(t, func() (domain.Alert, bool) {
		alerts, err := am.Alerts(ctx)
		a := first(alerts)
		return a, err == nil && len(a.SilencedBy) == 1
	})
	if silenced.SilencedBy[0] != id {
		t.Fatalf("silenced by %v, set %s", silenced.SilencedBy, id)
	}

	if err := am.Expire(ctx, id); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() (domain.Alert, bool) {
		alerts, err := am.Alerts(ctx)
		a := first(alerts)
		return a, err == nil && len(a.SilencedBy) == 0 && strings.HasPrefix(a.Name, "Release")
	})
}

func first(alerts []domain.Alert) domain.Alert {
	if len(alerts) == 0 {
		return domain.Alert{}
	}
	return alerts[0]
}

// waitFor polls until done holds: Alertmanager applies an alert and a silence a moment later.
func waitFor(t *testing.T, done func() (domain.Alert, bool)) domain.Alert {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		a, ok := done()
		if ok {
			return a
		}
		if time.Now().After(deadline) {
			t.Fatalf("still %+v", a)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
