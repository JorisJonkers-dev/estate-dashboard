package cluster_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/adapters/cluster"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/domain"
)

var rendered = map[string]string{"app.kubernetes.io/managed-by": "deploy-kit"}

func object(gvr schema.GroupVersionResource, kind, namespace, name string, labels map[string]string, spec, status map[string]any) *unstructured.Unstructured {
	o := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": gvr.Group + "/" + gvr.Version, "kind": kind,
		"metadata": map[string]any{"name": name, "namespace": namespace},
		"spec":     spec,
	}}
	if labels != nil {
		o.SetLabels(labels)
	}
	if status != nil {
		o.Object["status"] = status
	}
	return o
}

func ready(status, reason, message string) map[string]any {
	return map[string]any{"conditions": []any{
		map[string]any{"type": "Reconciling", "status": "False"},
		map[string]any{"type": "Ready", "status": status, "reason": reason, "message": message},
	}}
}

// gate is a Release Gate that answers GET /releases with what it is handed, and counts the asks.
type gate struct {
	*httptest.Server
	asked atomic.Int32
}

func serveGate(t *testing.T, status int, body string) *gate {
	t.Helper()
	g := &gate{}
	g.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.asked.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/releases" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(g.Close)
	return g
}

// kube is a cluster holding custom, whose Release Gate answers releases.
func kube(t *testing.T, releases string, custom ...runtime.Object) cluster.Kube {
	t.Helper()
	g := serveGate(t, http.StatusOK, `{"releases":[`+releases+`],"truncated":false}`)
	return kubeAsking(g, custom...)
}

func kubeAsking(g *gate, custom ...runtime.Object) cluster.Kube {
	kinds := map[schema.GroupVersionResource]string{
		cluster.OCIRepositories: "OCIRepositoryList", cluster.Kustomizations: "KustomizationList", cluster.Canaries: "CanaryList",
	}
	address, _ := url.Parse(g.URL)
	return cluster.Kube{
		Dynamic: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, custom...),
		Gate:    address, HTTP: g.Client(),
	}
}

func TestTheSourcesAreTheRendersOCIRepositoriesWithWhatFluxSaysOfThem(t *testing.T) {
	stored := ready("True", "Succeeded", "stored artifact")
	stored["artifact"] = map[string]any{"revision": "sha256:1"}
	k := kube(t, "",
		object(cluster.OCIRepositories, "OCIRepository", "flux-system", "project-auth", rendered,
			map[string]any{"url": "oci://r/auth", "ref": map[string]any{"digest": "sha256:1"}}, stored),
		object(cluster.OCIRepositories, "OCIRepository", "flux-system", "project-new", rendered,
			map[string]any{"url": "oci://r/new", "ref": map[string]any{"digest": "sha256:0"}}, nil),
		// Flux's own bootstrap source is not the render's.
		object(cluster.OCIRepositories, "OCIRepository", "flux-system", "flux-system", nil, map[string]any{}, nil),
	)
	page, err := k.Sources(t.Context())
	got := page.Items
	want := map[string]domain.Source{
		"project-auth": {Name: "project-auth", URL: "oci://r/auth", Digest: "sha256:1", Revision: "sha256:1", Condition: domain.Condition{Ready: true, Reason: "Succeeded", Message: "stored artifact"}},
		"project-new":  {Name: "project-new", URL: "oci://r/new", Digest: "sha256:0"},
	}
	if err != nil || len(got) != len(want) {
		t.Fatalf("sources = %+v, %v", got, err)
	}
	for _, s := range got {
		if !reflect.DeepEqual(s, want[s.Name]) {
			t.Fatalf("source %+v, want %+v", s, want[s.Name])
		}
	}
}

func TestTheUnitsAreTheRendersKustomizations(t *testing.T) {
	k := kube(t, "", object(cluster.Kustomizations, "Kustomization", "flux-system", "apps-auth", rendered,
		map[string]any{
			"sourceRef": map[string]any{"kind": "OCIRepository", "name": "project-auth"}, "path": "./apps/auth",
			"dependsOn": []any{map[string]any{"name": "apps-data"}, "not a reference", map[string]any{"name": int64(7)}, map[string]any{"name": "estate-vso-secrets"}},
		},
		func() map[string]any {
			s := ready("False", "DependencyNotReady", "apps-data is not ready")
			s["lastAppliedRevision"] = "sha256:1"
			return s
		}()))
	page, err := k.Units(t.Context())
	got := page.Items
	want := []domain.Unit{{
		Name: "apps-auth", Source: "project-auth", Path: "./apps/auth", DependsOn: []string{"apps-data", "estate-vso-secrets"}, Applied: "sha256:1",
		Condition: domain.Condition{Reason: "DependencyNotReady", Message: "apps-data is not ready"},
	}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("units = %+v, %v", got, err)
	}
}

// canary is a member's Canary as the render writes it, its webhooks naming the Application.
func canary(namespace, application, process, revision string, status map[string]any) *unstructured.Unstructured {
	webhooks := []any{}
	if application != "" {
		webhooks = append(webhooks, map[string]any{"name": "may-start", "metadata": map[string]any{"application": application, "revision": revision}})
	}
	return object(cluster.Canaries, "Canary", namespace, process, rendered, map[string]any{"analysis": map[string]any{"webhooks": webhooks}}, status)
}

// authServed is auth as the gate serves it: its inputs' members and migration, and its record.
const authServed = `{"namespace":"auth-system","application":"auth","members":["auth-api","auth-ui"],` +
	`"migration":{"identity":"auth-migration","testedAgainst":"sha256:s","nonTransactional":true},` +
	`"serving":"sha256:s","pinned":"sha256:p","since":"2026-10-03T09:00:00+02:00"}`

func byApplication(t *testing.T, k cluster.Kube) map[string]domain.Release {
	t.Helper()
	got, err := k.Releases(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]domain.Release{}
	for _, r := range got.Items {
		out[r.Namespace+"/"+r.Application] = r
	}
	return out
}

func TestAReleaseIsWhatTheGateServesJoinedWithItsMembersCanaries(t *testing.T) {
	k := kube(t, authServed+`,{"namespace":"mail-system","application":"mail","members":["mail"]}`+
		// An Application the gate serves and no Canary names is none the dashboard shows.
		`,{"namespace":"idle-system","application":"idle","members":["idle"]}`,
		canary("auth-system", "auth", "auth-api", "sha256:p", map[string]any{"phase": "Progressing", "iterations": int64(2)}),
		canary("mail-system", "mail", "mail", "sha256:m", nil),
		// A Canary whose webhooks name no Application is none of a release's.
		canary("other", "", "loose", "", nil),
		// A Canary the render did not write is not read at all.
		object(cluster.Canaries, "Canary", "auth-system", "stray", nil, map[string]any{}, nil),
	)
	since := time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)
	want := map[string]domain.Release{
		"auth-system/auth": {
			Namespace: "auth-system", Application: "auth", Serving: "sha256:s", Pinned: "sha256:p", Since: &since,
			// auth-ui's Canary is not made yet: a member with no phase.
			Members:   []domain.Member{{Process: "auth-api", Phase: "Progressing", Revision: "sha256:p", Iterations: 2}, {Process: "auth-ui"}},
			Migration: &domain.Migration{Identity: "auth-migration", TestedAgainst: "sha256:s", NonTransactional: true},
		},
		"mail-system/mail": {Namespace: "mail-system", Application: "mail", Members: []domain.Member{{Process: "mail", Revision: "sha256:m"}}},
	}
	if got := byApplication(t, k); !reflect.DeepEqual(got, want) {
		t.Fatalf("releases = %+v", got)
	}
}

func TestOneReleaseThatDoesNotReadKeepsNoOtherFromBeingRead(t *testing.T) {
	many := `"members":[` + strings.Repeat(`"p",`, 64) + `"p"]`
	k := kube(t, authServed+
		`,{"namespace":"broken","application":"broken","unreadable":"release-gate inputs that do not parse"}`+
		`,{"namespace":"crowded","application":"crowded",`+many+`}`+
		`,{"namespace":"forged","application":"forged","members":["f"],"unreadable":"a release record the gate did not write for it"}`,
		canary("auth-system", "auth", "auth-api", "sha256:p", nil),
		canary("broken", "broken", "b", "", nil),
		canary("crowded", "crowded", "c", "", nil),
		canary("forged", "forged", "f", "", nil),
		canary("orphan", "orphan", "o", "", nil),
	)
	got := byApplication(t, k)
	for at, why := range map[string]string{
		"broken/broken":   "release-gate inputs that do not parse",
		"crowded/crowded": "more members than an Application has",
		"forged/forged":   "a release record the gate did not write for it",
		"orphan/orphan":   "no release-gate inputs",
	} {
		if r := got[at]; r.Unreadable != why || r.Serving != "" || len(r.Members) != 0 {
			t.Errorf("%s = %+v", at, r)
		}
	}
	if auth := got["auth-system/auth"]; auth.Unreadable != "" || len(auth.Members) != 2 {
		t.Fatalf("auth = %+v", auth)
	}
}

func TestNoReadGrowsWithWhatAnyoneCanCreate(t *testing.T) {
	var canaries []runtime.Object
	for i := range 300 {
		app := "app" + strconv.Itoa(i)
		canaries = append(canaries, canary("ns", app, app, "", nil))
	}
	g := serveGate(t, http.StatusOK, `{"releases":[],"truncated":false}`)
	got, err := kubeAsking(g, canaries...).Releases(t.Context())
	if err != nil || len(got.Items) != 256 || g.asked.Load() != 1 || !got.Truncated {
		t.Fatalf("read %d releases with %d asks, truncated %v, %v", len(got.Items), g.asked.Load(), got.Truncated, err)
	}
	// A cluster holding more than one page says so.
	more := kube(t, "")
	more.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
		list := &unstructured.UnstructuredList{Object: map[string]any{"apiVersion": "v1", "kind": "List"}}
		list.SetContinue("next")
		return true, list, nil
	})
	sources, e1 := more.Sources(t.Context())
	units, e2 := more.Units(t.Context())
	releases, e3 := more.Releases(t.Context())
	if errors.Join(e1, e2, e3) != nil || !sources.Truncated || !units.Truncated || !releases.Truncated {
		t.Fatalf("a cluster holding more: %v %v %v", sources.Truncated, units.Truncated, releases.Truncated)
	}
	// An answer of the gate is read up to a bound, so a gate cannot make the dashboard hold more.
	huge := serveGate(t, http.StatusOK, `{"releases":[`+strings.Repeat(authServed+",", 40000)+authServed+`]}`)
	if _, err := kubeAsking(huge, canary("auth-system", "auth", "auth-api", "", nil)).Releases(t.Context()); err == nil {
		t.Fatal("an answer past the bound was read whole")
	}
}

func TestAClusterOrAGateThatRefusesIsAnError(t *testing.T) {
	refuse := func(k8stesting.Action) (bool, runtime.Object, error) { return true, nil, errors.New("forbidden") }
	k := kube(t, "")
	k.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("*", "*", refuse)
	_, e1 := k.Sources(t.Context())
	_, e2 := k.Units(t.Context())
	_, e3 := k.Releases(t.Context())
	for _, err := range []error{e1, e2, e3} {
		if err == nil {
			t.Fatal("a refusal read as an answer")
		}
	}
	// What the gate will not hand over is the cluster's failure, not a release's data.
	gone := serveGate(t, http.StatusOK, "")
	gone.Close()
	for name, g := range map[string]*gate{
		"a 503":               serveGate(t, http.StatusServiceUnavailable, "the gate could not read the cluster"),
		"an answer not JSON":  serveGate(t, http.StatusOK, "<html>"),
		"a gate that is gone": gone,
	} {
		if _, err := kubeAsking(g, canary("auth-system", "auth", "auth-api", "", nil)).Releases(t.Context()); err == nil {
			t.Errorf("%s read as an answer", name)
		}
	}
	// A dashboard that knows no gate reads no release.
	none := kube(t, "")
	none.Gate = nil
	if _, err := none.Releases(t.Context()); !errors.Is(err, cluster.ErrNoGate) {
		t.Fatalf("without a gate: %v", err)
	}
	// An address that does not make a request is the dashboard's failure, read as one.
	broken := kube(t, "")
	broken.Gate = &url.URL{Scheme: "http", Host: "bad host"}
	if _, err := broken.Releases(t.Context()); err == nil {
		t.Fatal("a gate address that is not one was asked")
	}
}

func TestWithoutAClusterEveryReadSaysSo(t *testing.T) {
	notIn := func() (*rest.Config, error) { return nil, rest.ErrNotInCluster }
	c, err := cluster.Connect(notIn, "", nil, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	_, e1 := c.Sources(t.Context())
	_, e2 := c.Units(t.Context())
	_, e3 := c.Releases(t.Context())
	for _, err := range []error{e1, e2, e3} {
		if !errors.Is(err, domain.ErrNoCluster) {
			t.Fatalf("error = %v", err)
		}
	}
	if _, err := cluster.Connect(notIn, "/nonexistent/kubeconfig", nil, http.DefaultClient); err == nil {
		t.Fatal("a kubeconfig that is not there connected")
	}
	if _, err := cluster.Connect(func() (*rest.Config, error) { return nil, errors.New("no token") }, "", nil, http.DefaultClient); err == nil {
		t.Fatal("a broken in-cluster configuration connected")
	}
	in, err := cluster.Connect(func() (*rest.Config, error) { return &rest.Config{Host: "https://kubernetes.default.svc"}, nil }, "", nil, http.DefaultClient)
	if _, isKube := in.(cluster.Kube); err != nil || !isKube {
		t.Fatalf("in a cluster = %T, %v", in, err)
	}
}
