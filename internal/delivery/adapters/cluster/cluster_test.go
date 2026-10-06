package cluster_test

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
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

func kube(core []runtime.Object, custom ...runtime.Object) cluster.Kube {
	kinds := map[schema.GroupVersionResource]string{
		cluster.OCIRepositories: "OCIRepositoryList", cluster.Kustomizations: "KustomizationList", cluster.Canaries: "CanaryList",
	}
	return cluster.Kube{Client: fake.NewClientset(core...), Dynamic: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, custom...)}
}

func TestTheSourcesAreTheRendersOCIRepositoriesWithWhatFluxSaysOfThem(t *testing.T) {
	stored := ready("True", "Succeeded", "stored artifact")
	stored["artifact"] = map[string]any{"revision": "sha256:1"}
	k := kube(nil,
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
	k := kube(nil, object(cluster.Kustomizations, "Kustomization", "flux-system", "apps-auth", rendered,
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

func inputs(namespace, application, data string) *corev1.ConfigMap {
	return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: application + "-release-gate", Namespace: namespace}, Data: map[string]string{"releaseGate.json": data}}
}

func recordOf(namespace, application, data string) *corev1.ConfigMap {
	return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: namespace + "." + application + "-release-record", Namespace: "delivery-system"}, Data: map[string]string{"record.json": data}}
}

// canary is a member's Canary as the render writes it, its webhooks naming the Application.
func canary(namespace, application, process, revision string, status map[string]any) *unstructured.Unstructured {
	webhooks := []any{}
	if application != "" {
		webhooks = append(webhooks, map[string]any{"name": "may-start", "metadata": map[string]any{"application": application, "revision": revision}})
	}
	return object(cluster.Canaries, "Canary", namespace, process, rendered, map[string]any{"analysis": map[string]any{"webhooks": webhooks}}, status)
}

const authGate = `{"members":[{"process":"auth-api"},{"process":"auth-ui"}],"migration":{"identity":"auth-migration","testedAgainst":"sha256:s","nonTransactional":true}}`

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

func TestAReleaseIsTheGatesInputsItsMembersCanariesAndTheGatesRecord(t *testing.T) {
	k := kube([]runtime.Object{
		inputs("auth-system", "auth", authGate),
		recordOf("auth-system", "auth", `{"namespace":"auth-system","serving":"sha256:s","pinned":"sha256:p","since":"2026-10-03T09:00:00+02:00"}`),
		inputs("mail-system", "mail", `{"members":[{"process":"mail"}]}`),
	},
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
	many := `{"members":[` + strings.Repeat(`{"process":"p"},`, 64) + `{"process":"p"}]}`
	k := kube([]runtime.Object{
		inputs("auth-system", "auth", authGate),
		inputs("broken", "broken", "{"),
		inputs("crowded", "crowded", many),
		inputs("forged", "forged", `{"members":[]}`),
		recordOf("forged", "forged", `{"namespace":"elsewhere","serving":"sha256:x"}`),
		inputs("garbled", "garbled", `{"members":[]}`),
		recordOf("garbled", "garbled", `x`),
	},
		canary("auth-system", "auth", "auth-api", "sha256:p", nil),
		canary("broken", "broken", "b", "", nil),
		canary("crowded", "crowded", "c", "", nil),
		canary("forged", "forged", "f", "", nil),
		canary("garbled", "garbled", "g", "", nil),
		canary("orphan", "orphan", "o", "", nil),
	)
	got := byApplication(t, k)
	for at, why := range map[string]string{
		"broken/broken":   "release-gate inputs that do not parse",
		"crowded/crowded": "more members than an Application has",
		"forged/forged":   "a release record the gate did not write for it",
		"garbled/garbled": "a release record the gate did not write for it",
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
	k := kube(nil, canaries...)
	var gets int
	k.Client.(*fake.Clientset).PrependReactor("get", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
		gets++
		return false, nil, nil
	})
	got, err := k.Releases(t.Context())
	if err != nil || len(got.Items) != 256 || gets != 256 || !got.Truncated {
		t.Fatalf("read %d releases with %d gets, truncated %v, %v", len(got.Items), gets, got.Truncated, err)
	}
	// A cluster holding more than one page says so.
	more := kube(nil)
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
}

func TestAClusterThatRefusesIsAnError(t *testing.T) {
	refuse := func(k8stesting.Action) (bool, runtime.Object, error) { return true, nil, errors.New("forbidden") }
	k := kube(nil)
	k.Client.(*fake.Clientset).PrependReactor("*", "*", refuse)
	k.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("*", "*", refuse)
	_, e1 := k.Sources(t.Context())
	_, e2 := k.Units(t.Context())
	_, e3 := k.Releases(t.Context())
	for _, err := range []error{e1, e2, e3} {
		if err == nil {
			t.Fatal("a refusal read as an answer")
		}
	}
	// The inputs or the record the cluster will not hand over is the cluster's failure, not a
	// release's data.
	for name, failing := range map[string]func(k8stesting.Action) bool{
		"the inputs": func(a k8stesting.Action) bool { return a.GetNamespace() == "auth-system" },
		"the record": func(a k8stesting.Action) bool { return a.GetNamespace() == "delivery-system" },
	} {
		k := kube([]runtime.Object{inputs("auth-system", "auth", authGate)}, canary("auth-system", "auth", "auth-api", "", nil))
		k.Client.(*fake.Clientset).PrependReactor("get", "configmaps", func(a k8stesting.Action) (bool, runtime.Object, error) {
			if failing(a) {
				return true, nil, errors.New("forbidden")
			}
			return false, nil, nil
		})
		if _, err := k.Releases(t.Context()); err == nil {
			t.Errorf("%s refused read as an answer", name)
		}
	}
}

func TestWithoutAClusterEveryReadSaysSo(t *testing.T) {
	notIn := func() (*rest.Config, error) { return nil, rest.ErrNotInCluster }
	c, err := cluster.Connect(notIn, "")
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
	if _, err := cluster.Connect(notIn, "/nonexistent/kubeconfig"); err == nil {
		t.Fatal("a kubeconfig that is not there connected")
	}
	if _, err := cluster.Connect(func() (*rest.Config, error) { return nil, errors.New("no token") }, ""); err == nil {
		t.Fatal("a broken in-cluster configuration connected")
	}
	in, err := cluster.Connect(func() (*rest.Config, error) { return &rest.Config{Host: "https://kubernetes.default.svc"}, nil }, "")
	if _, isKube := in.(cluster.Kube); err != nil || !isKube {
		t.Fatalf("in a cluster = %T, %v", in, err)
	}
}
