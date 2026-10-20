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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		cluster.Pods: "PodList", cluster.ReplicaSets: "ReplicaSetList",
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
	auth := object(cluster.Kustomizations, "Kustomization", "flux-system", "apps-auth", rendered,
		map[string]any{
			"sourceRef": map[string]any{"kind": "OCIRepository", "name": "project-auth"}, "path": "./apps/auth",
			"dependsOn": []any{map[string]any{"name": "apps-data"}, "not a reference", map[string]any{"name": int64(7)}, map[string]any{"name": "estate-vso-secrets"}},
		},
		func() map[string]any {
			s := ready("False", "DependencyNotReady", "apps-data is not ready")
			s["lastAppliedRevision"] = "sha256:1"
			return s
		}())
	auth.SetAnnotations(map[string]string{
		"event.toolkit.fluxcd.io/version": "3.4.2", "event.toolkit.fluxcd.io/repository": "JorisJonkers-dev/auth-api",
		"event.toolkit.fluxcd.io/sha": "8c1d40e",
	})
	k := kube(t, "", auth)
	page, err := k.Units(t.Context())
	got := page.Items
	want := []domain.Unit{{
		Name: "apps-auth", Source: "project-auth", Path: "./apps/auth", DependsOn: []string{"apps-data", "estate-vso-secrets"}, Applied: "sha256:1",
		Version: "3.4.2", Repository: "JorisJonkers-dev/auth-api",
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
	_, e4 := k.AllPods(t.Context())
	_, e5 := k.Revisions(t.Context())
	for _, err := range []error{e1, e2, e3, e4, e5} {
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
	_, e4 := c.AllPods(t.Context())
	_, e5 := c.Revisions(t.Context())
	for _, err := range []error{e1, e2, e3, e4, e5} {
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

// pod is a pod as the API serves it: the render's labels, a spec of containers and a status.
func pod(namespace, name string, labels map[string]string, spec, status map[string]any) *unstructured.Unstructured {
	o := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]any{"name": name, "namespace": namespace},
		"spec":     spec, "status": status,
	}}
	o.SetLabels(labels)
	return o
}

func at(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return &t
}

var radarrLabels = map[string]string{
	"app.kubernetes.io/managed-by": "deploy-kit", "app.kubernetes.io/name": "radarr",
	"app.kubernetes.io/part-of": "arr", "app.kubernetes.io/version": "1.0.4",
}

func radarr() *unstructured.Unstructured {
	p := pod("arr-system", "radarr-566dd79995-jc5mr", radarrLabels,
		map[string]any{
			"nodeName":       "enschede-1",
			"initContainers": []any{map[string]any{"name": "config-restore", "image": "restic:1"}},
			"containers": []any{map[string]any{
				"name": "radarr", "image": "ghcr.io/x/radarr:5",
				"readinessProbe": map[string]any{"httpGet": map[string]any{"path": "/ping"}},
				"livenessProbe":  map[string]any{"httpGet": map[string]any{"path": "/ping"}},
			}},
		},
		map[string]any{
			"phase": "Running", "startTime": "2026-10-19T20:00:00Z",
			"conditions": []any{
				map[string]any{"type": "PodScheduled", "status": "True"},
				map[string]any{"type": "Ready", "status": "True"},
			},
			"initContainerStatuses": []any{map[string]any{
				"name": "config-restore", "ready": true, "restartCount": int64(0),
				"state": map[string]any{"terminated": map[string]any{"reason": "Completed", "exitCode": int64(0), "finishedAt": "2026-10-19T20:00:05Z"}},
			}},
			"containerStatuses": []any{map[string]any{
				"name": "radarr", "ready": true, "restartCount": int64(2),
				"state":     map[string]any{"running": map[string]any{"startedAt": "2026-10-19T20:01:00Z"}},
				"lastState": map[string]any{"terminated": map[string]any{"reason": "OOMKilled", "exitCode": int64(137), "finishedAt": "2026-10-19T20:00:50Z"}},
			}},
		})
	p.SetOwnerReferences([]metav1.OwnerReference{{Kind: "ReplicaSet", Name: "radarr-566dd79995", APIVersion: "apps/v1"}})
	return p
}

func TestAPodIsWhatItsSpecDeclaresAndWhatTheKubeletSaysOfIt(t *testing.T) {
	k := kube(t, "", radarr())
	page, err := k.Pods(t.Context(), "arr-system")
	want := []domain.Pod{{
		Namespace: "arr-system", Name: "radarr-566dd79995-jc5mr", Process: "radarr", Application: "arr", Version: "1.0.4",
		Owner: "ReplicaSet", Node: "enschede-1", Phase: "Running", Started: at("2026-10-19T20:00:00Z"), Managed: true, Ready: true,
		Containers: []domain.Container{
			{Name: "config-restore", Image: "restic:1", Init: true, VersionSource: domain.FromNowhere, Ready: true, State: domain.Terminated, Reason: "Completed", Since: at("2026-10-19T20:00:05Z")},
			{
				Name: "radarr", Image: "ghcr.io/x/radarr:5", VersionSource: domain.FromNowhere, Ready: true, State: domain.Running, Since: at("2026-10-19T20:01:00Z"), Restarts: 2,
				Last:   &domain.Termination{Reason: "OOMKilled", ExitCode: 137, At: at("2026-10-19T20:00:50Z")},
				Probes: domain.Probes{Readiness: true, Liveness: true},
			},
		},
	}}
	if err != nil || !reflect.DeepEqual(page.Items, want) {
		t.Fatalf("pods = %+v, %v", page.Items, err)
	}
}

func TestAContainerThatWaitsSaysWhy(t *testing.T) {
	waiting := pod("arr-system", "sonarr-1", radarrLabels,
		map[string]any{"containers": []any{map[string]any{"name": "sonarr", "image": "s:1"}}},
		map[string]any{
			"phase": "Running",
			"containerStatuses": []any{map[string]any{
				"name": "sonarr", "ready": false, "restartCount": int64(9),
				"state": map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff", "message": "back-off 5m0s restarting failed container"}},
			}},
		})
	k := kube(t, "", waiting)
	page, err := k.Pods(t.Context(), "arr-system")
	if err != nil || len(page.Items) != 1 || len(page.Items[0].Containers) != 1 {
		t.Fatalf("pods = %+v, %v", page.Items, err)
	}
	got := page.Items[0].Containers[0]
	want := domain.Container{Name: "sonarr", Image: "s:1", VersionSource: domain.FromNowhere, State: domain.Waiting, Reason: "CrashLoopBackOff", Message: "back-off 5m0s restarting failed container", Restarts: 9}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("container = %+v, want %+v", got, want)
	}
}

func TestOnlyTheRendersPodsInTheNamespaceAreRead(t *testing.T) {
	other := pod("grimoire-system", "grimoire-1", radarrLabels, map[string]any{}, map[string]any{})
	unrendered := pod("arr-system", "debug", map[string]string{"run": "debug"}, map[string]any{}, map[string]any{})
	k := kube(t, "", radarr(), other, unrendered)
	page, err := k.Pods(t.Context(), "arr-system")
	if err != nil || len(page.Items) != 1 || page.Items[0].Name != "radarr-566dd79995-jc5mr" {
		t.Fatalf("pods = %+v, %v", page.Items, err)
	}
}

func TestAPodShuttingDownSaysSo(t *testing.T) {
	leaving := radarr()
	now := metav1.Now()
	leaving.SetDeletionTimestamp(&now)
	k := kube(t, "", leaving)
	page, err := k.Pods(t.Context(), "arr-system")
	if err != nil || len(page.Items) != 1 || !page.Items[0].Deleting {
		t.Fatalf("pods = %+v, %v", page.Items, err)
	}
}

// versioned is a pod of the render with a Process, a sidecar and the version annotations it writes.
func versioned() *unstructured.Unstructured {
	p := pod("arr-system", "sonarr-1", radarrLabels,
		map[string]any{"containers": []any{
			map[string]any{"name": "backup", "image": "ghcr.io/x/file-backup@sha256:b"},
			map[string]any{"name": "radarr", "image": "ghcr.io/x/radarr@sha256:a"},
			map[string]any{"name": "exporter", "image": "exporter:v0.17.1"},
		}},
		map[string]any{"containerStatuses": []any{
			map[string]any{"name": "radarr", "imageID": "ghcr.io/x/radarr@sha256:c"},
		}})
	p.SetAnnotations(map[string]string{"image.jorisjonkers.dev/radarr": "6.3.0.10514-ls314", "image.jorisjonkers.dev/backup": "0.9.2"})
	return p
}

func TestAContainersVersionIsTheOneTheRenderAnnotatedOrItsTag(t *testing.T) {
	page, err := kube(t, "", versioned()).Pods(t.Context(), "arr-system")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("pods = %+v, %v", page.Items, err)
	}
	var got []string
	for _, c := range page.Items[0].Containers {
		got = append(got, c.Name+" "+c.Version+" "+string(c.VersionSource))
	}
	if want := []string{"backup 0.9.2 lock", "radarr 6.3.0.10514-ls314 lock", "exporter v0.17.1 tag"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("versions = %v", got)
	}
}

func TestEveryContainerButThePodsProcessIsASidecar(t *testing.T) {
	page, err := kube(t, "", versioned(), radarr()).Pods(t.Context(), "arr-system")
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("pods = %+v, %v", page.Items, err)
	}
	var got []string
	for _, p := range page.Items {
		for _, c := range p.Containers {
			got = append(got, c.Name+" "+strconv.FormatBool(c.Sidecar))
		}
	}
	if want := []string{"config-restore false", "radarr false", "backup true", "radarr false", "exporter true"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sidecars = %v", got)
	}
}

func TestAPodNoContainerOfIsNamedForItsProcessHasItsFirstForItsProcess(t *testing.T) {
	gatus := pod("observability", "gatus-1", map[string]string{"app": "gatus"},
		map[string]any{"containers": []any{map[string]any{"name": "gatus", "image": "twinproduction/gatus:v5.36.0"}, map[string]any{"name": "reloader", "image": "r:1"}}},
		map[string]any{})
	page, err := kube(t, "", gatus).AllPods(t.Context())
	if err != nil || len(page.Items) != 1 || page.Items[0].Containers[0].Sidecar || !page.Items[0].Containers[1].Sidecar {
		t.Fatalf("pods = %+v, %v", page.Items, err)
	}
}

func TestAContainersRunningDigestIsTheOneTheKubeletReports(t *testing.T) {
	page, err := kube(t, "", versioned()).Pods(t.Context(), "arr-system")
	if err != nil || page.Items[0].Containers[1].RunningDigest != "sha256:c" || page.Items[0].Containers[0].RunningDigest != "" {
		t.Fatalf("pods = %+v, %v", page.Items, err)
	}
}

func TestTheImagesTheEstateRunsAreReadFromEveryNamespaceRenderedOrNot(t *testing.T) {
	platform := pod("observability", "alloy-1", map[string]string{"app.kubernetes.io/name": "alloy"},
		map[string]any{"containers": []any{map[string]any{"name": "alloy", "image": "grafana/alloy:v1.12.2"}}}, map[string]any{})
	page, err := kube(t, "", radarr(), platform).AllPods(t.Context())
	managed := map[string]bool{}
	for _, p := range page.Items {
		managed[p.Name] = p.Managed
	}
	if err != nil || len(page.Items) != 2 || !managed["radarr-566dd79995-jc5mr"] || managed["alloy-1"] {
		t.Fatalf("pods = %+v, %v", page.Items, err)
	}
}

func replicaSet(name, owner, release, created string, annotations map[string]string, containers ...map[string]any) *unstructured.Unstructured {
	template := map[string]any{"spec": map[string]any{"containers": func() []any {
		out := make([]any, 0, len(containers))
		for _, c := range containers {
			out = append(out, c)
		}
		return out
	}()}}
	meta := map[string]any{}
	if release != "" {
		meta["labels"] = map[string]any{"app.kubernetes.io/version": release}
	}
	if annotations != nil {
		values := map[string]any{}
		for k, v := range annotations {
			values[k] = v
		}
		meta["annotations"] = values
	}
	template["metadata"] = meta
	o := object(cluster.ReplicaSets, "ReplicaSet", "arr-system", name, nil, map[string]any{"template": template}, nil)
	stamp, _ := time.Parse(time.RFC3339, created)
	o.SetCreationTimestamp(metav1.NewTime(stamp))
	if owner != "" {
		o.SetOwnerReferences([]metav1.OwnerReference{{Kind: owner, Name: "sonarr", APIVersion: "apps/v1"}})
	}
	return o
}

func TestADeploymentsRevisionIsTheTemplateItsReplicaSetHolds(t *testing.T) {
	k := kube(t, "",
		replicaSet("sonarr-1", "Deployment", "1.0.4", "2026-10-20T09:00:00Z", map[string]string{"image.jorisjonkers.dev/sonarr": "4.0.19"},
			map[string]any{"name": "sonarr", "image": "ghcr.io/x/sonarr@sha256:1"}),
		replicaSet("loose", "", "", "2026-10-20T09:00:00Z", nil, map[string]any{"name": "x", "image": "x:1.0.0"}),
	)
	page, err := k.Revisions(t.Context())
	want := []domain.Revision{{
		Namespace: "arr-system", Deployment: "sonarr", Release: "1.0.4", Created: time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC),
		Containers: []domain.Container{{Name: "sonarr", Image: "ghcr.io/x/sonarr@sha256:1", Version: "4.0.19", VersionSource: domain.FromLock}},
	}}
	if err != nil || !reflect.DeepEqual(page.Items, want) {
		t.Fatalf("revisions = %+v, %v", page.Items, err)
	}
}
