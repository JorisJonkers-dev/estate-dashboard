package gatus_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/checks/adapters/gatus"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/checks/domain"
)

func serve(t *testing.T, status int, body string) *gatus.Gatus {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/endpoints/statuses" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	g, err := gatus.New(srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return g
}

const radarr = `[{"name":"radarr","group":"home-media","key":"home-media_radarr","results":[
 {"status":500,"hostname":"radarr.arr-system.svc.cluster.local","duration":1000000,"success":false,"timestamp":"2026-10-19T21:10:18Z","errors":["timeout"]},
 {"status":200,"hostname":"radarr.arr-system.svc.cluster.local","duration":206608089,"success":true,"timestamp":"2026-10-19T21:11:18.54912736Z",
  "conditionResults":[{"condition":"[STATUS] == 200","success":true},{"condition":"[RESPONSE_TIME] < 1500","success":true}]}]}]`

func TestACheckIsItsLatestResultAndWhetherEachRecentOneSucceeded(t *testing.T) {
	got, err := serve(t, http.StatusOK, radarr).Checks(t.Context())
	at := time.Date(2026, 10, 19, 21, 11, 18, 549127360, time.UTC)
	want := []domain.Check{{
		Key: "home-media_radarr", Name: "radarr", Group: "home-media", Host: "radarr.arr-system.svc.cluster.local",
		At: &at, Up: true, Status: 200, Duration: 206608089,
		Conditions: []domain.Condition{{Condition: "[STATUS] == 200", Success: true}, {Condition: "[RESPONSE_TIME] < 1500", Success: true}},
		Recent:     []bool{false, true},
	}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("checks = %+v, %v", got, err)
	}
}

func TestAFailedProbeSaysWhy(t *testing.T) {
	failing := `[{"name":"jellyfin","group":"home-media","key":"home-media_jellyfin","results":[
	 {"status":0,"hostname":"jellyfin.jorisjonkers.dev","duration":5000000000,"success":false,"timestamp":"2026-10-19T21:11:18Z","errors":["context deadline exceeded"]}]}]`
	got, err := serve(t, http.StatusOK, failing).Checks(t.Context())
	if err != nil || len(got) != 1 || got[0].Up || got[0].Error != "context deadline exceeded" {
		t.Fatalf("checks = %+v, %v", got, err)
	}
}

func TestAnEndpointNotYetProbedIsNotUp(t *testing.T) {
	got, err := serve(t, http.StatusOK, `[{"name":"new","group":"g","key":"g_new","results":[]}]`).Checks(t.Context())
	if err != nil || len(got) != 1 || got[0].Up || got[0].At != nil || len(got[0].Recent) != 0 {
		t.Fatalf("checks = %+v, %v", got, err)
	}
}

func TestAMonitorThatRefusesIsAnError(t *testing.T) {
	if _, err := serve(t, http.StatusBadGateway, "").Checks(t.Context()); err == nil {
		t.Fatal("a refusal read as checks")
	}
	if _, err := serve(t, http.StatusOK, "not json").Checks(t.Context()); err == nil {
		t.Fatal("an answer that is not one read as checks")
	}
}

func TestOnlyAnHTTPAddressIsAMonitor(t *testing.T) {
	for _, base := range []string{"", "gatus:80", "ftp://gatus"} {
		if _, err := gatus.New(base, http.DefaultClient); err == nil {
			t.Fatalf("%q was taken for a monitor", base)
		}
	}
}

func TestWithoutAMonitorEveryReadSaysSo(t *testing.T) {
	if _, err := (gatus.Absent{}).Checks(t.Context()); !errors.Is(err, domain.ErrNoMonitor) {
		t.Fatalf("error = %v", err)
	}
}
