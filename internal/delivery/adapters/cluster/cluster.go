// Package cluster implements the delivery context's Cluster port on the Kubernetes API, through a
// ServiceAccount that may get, list and watch Flux's OCIRepositories and Kustomizations and
// Flagger's Canaries, and get a ConfigMap by name, and nothing else: each gated Application's
// release-gate inputs and the Release Gate's record of it are read by name, never listed.
package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/domain"
)

//nolint:gochecknoglobals // constants in all but type
var (
	// OCIRepositories, Kustomizations and Canaries are the custom kinds read.
	OCIRepositories = schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "ocirepositories"}
	Kustomizations  = schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}
	Canaries        = schema.GroupVersionResource{Group: "flagger.app", Version: "v1beta1", Resource: "canaries"}
)

const (
	// rendered selects what the render wrote (deploy-kit spec/v1/10-project-intent.md#the-label-set).
	rendered = "app.kubernetes.io/managed-by=deploy-kit"
	partOf   = "app.kubernetes.io/part-of"
	// inputsSuffix and inputsKey name an Application's release-gate inputs.
	inputsSuffix = "-release-gate"
	inputsKey    = "releaseGate.json"
	// recordNamespace, recordSuffix and recordKey name the Release Gate's record of one
	// Application: `<namespace>.<application>-release-record` in the gate's own namespace.
	recordNamespace = "delivery-system"
	recordSuffix    = "-release-record"
	recordKey       = "record.json"
)

// Kube reads the cluster.
type Kube struct {
	Client  kubernetes.Interface
	Dynamic dynamic.Interface
}

var _ domain.Cluster = Kube{}

// maxListed is the most objects one read lists: what the contract's lists carry. A cluster that
// holds more answers with the first page, so no request grows with what anyone can create.
const maxListed = 1000

// maxReleases is the most Applications one read of the releases reads the inputs and record of:
// two reads each, so a cluster full of Canaries never turns one request into thousands.
const maxReleases = 256

// list reads one page of what the render wrote of a kind, and whether the cluster holds more.
func (k Kube) list(ctx context.Context, gvr schema.GroupVersionResource) ([]unstructured.Unstructured, bool, error) {
	list, err := k.Dynamic.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: rendered, Limit: maxListed})
	if err != nil {
		return nil, false, fmt.Errorf("list %s: %w", gvr.Resource, err)
	}
	return list.Items, list.GetContinue() != "", nil
}

// readyOf reads the Ready condition Flux writes on its objects.
func readyOf(o unstructured.Unstructured) domain.Condition {
	conditions, _, _ := unstructured.NestedSlice(o.Object, "status", "conditions")
	for _, c := range conditions {
		fields, _ := c.(map[string]any)
		if fields["type"] != "Ready" {
			continue
		}
		reason, _ := fields["reason"].(string)
		message, _ := fields["message"].(string)
		return domain.Condition{Ready: fields["status"] == "True", Reason: reason, Message: message}
	}
	return domain.Condition{}
}

func str(o unstructured.Unstructured, path ...string) string {
	v, _, _ := unstructured.NestedString(o.Object, path...)
	return v
}

// Sources implements domain.Cluster: every OCIRepository the render wrote.
func (k Kube) Sources(ctx context.Context) (domain.Page[domain.Source], error) {
	items, more, err := k.list(ctx, OCIRepositories)
	if err != nil {
		return domain.Page[domain.Source]{}, err
	}
	sources := make([]domain.Source, 0, len(items))
	for _, o := range items {
		sources = append(sources, domain.Source{
			Name:      o.GetName(),
			URL:       str(o, "spec", "url"),
			Digest:    str(o, "spec", "ref", "digest"),
			Revision:  str(o, "status", "artifact", "revision"),
			Condition: readyOf(o),
		})
	}
	return domain.Page[domain.Source]{Items: sources, Truncated: more}, nil
}

// Units implements domain.Cluster: every Kustomization the render wrote.
func (k Kube) Units(ctx context.Context) (domain.Page[domain.Unit], error) {
	items, more, err := k.list(ctx, Kustomizations)
	if err != nil {
		return domain.Page[domain.Unit]{}, err
	}
	units := make([]domain.Unit, 0, len(items))
	for _, o := range items {
		var after []string
		dependsOn, _, _ := unstructured.NestedSlice(o.Object, "spec", "dependsOn")
		for _, d := range dependsOn {
			if fields, ok := d.(map[string]any); ok {
				if name, ok := fields["name"].(string); ok {
					after = append(after, name)
				}
			}
		}
		units = append(units, domain.Unit{
			Name:      o.GetName(),
			Source:    str(o, "spec", "sourceRef", "name"),
			Path:      str(o, "spec", "path"),
			DependsOn: after,
			Applied:   str(o, "status", "lastAppliedRevision"),
			Condition: readyOf(o),
		})
	}
	return domain.Page[domain.Unit]{Items: units, Truncated: more}, nil
}

// inputs is as much of the rendered release-gate inputs as a screen shows.
type inputs struct {
	Members []struct {
		Process string `json:"process"`
	} `json:"members"`
	Migration *struct {
		Identity         string  `json:"identity"`
		TestedAgainst    *string `json:"testedAgainst"`
		NonTransactional bool    `json:"nonTransactional"`
	} `json:"migration"`
}

// record is the Release Gate's record of one Application.
type record struct {
	Namespace string    `json:"namespace"`
	Serving   string    `json:"serving"`
	Pinned    string    `json:"pinned"`
	Since     time.Time `json:"since"`
}

// maxMembers is the most members one Application's inputs may name: what the contract carries.
const maxMembers = 64

// canaryOf is what a screen shows of one Canary, and the Application its webhooks name.
type canaryOf struct {
	application string
	member      domain.Member
}

func readCanary(o unstructured.Unstructured) canaryOf {
	var application, revision string
	webhooks, _, _ := unstructured.NestedSlice(o.Object, "spec", "analysis", "webhooks")
	if len(webhooks) > 0 {
		if fields, ok := webhooks[0].(map[string]any); ok {
			application, _, _ = unstructured.NestedString(fields, "metadata", "application")
			revision, _, _ = unstructured.NestedString(fields, "metadata", "revision")
		}
	}
	iterations, _, _ := unstructured.NestedInt64(o.Object, "status", "iterations")
	return canaryOf{application: application, member: domain.Member{
		Process: o.GetName(), Phase: str(o, "status", "phase"), Revision: revision, Iterations: int(iterations),
	}}
}

// Releases implements domain.Cluster. The gated Applications are found from the Canaries the
// render wrote, whose webhooks name their Application, so no ConfigMap is ever listed: each
// Application's inputs and the gate's record of it are read by name. Its members are the Canaries
// already listed. One Application whose data cannot be read is reported as such and keeps no other
// from being read.
func (k Kube) Releases(ctx context.Context) (domain.Page[domain.Release], error) {
	items, more, err := k.list(ctx, Canaries)
	if err != nil {
		return domain.Page[domain.Release]{}, err
	}
	byApplication := map[[2]string]map[string]domain.Member{}
	var order [][2]string
	for _, o := range items {
		c := readCanary(o)
		if c.application == "" {
			continue
		}
		at := [2]string{o.GetNamespace(), c.application}
		if byApplication[at] == nil {
			if len(order) == maxReleases {
				more = true
				continue
			}
			byApplication[at] = map[string]domain.Member{}
			order = append(order, at)
		}
		byApplication[at][c.member.Process] = c.member
	}
	releases := make([]domain.Release, 0, len(order))
	for _, at := range order {
		release, err := k.release(ctx, at[0], at[1], byApplication[at])
		if err != nil {
			return domain.Page[domain.Release]{}, err
		}
		releases = append(releases, release)
	}
	return domain.Page[domain.Release]{Items: releases, Truncated: more}, nil
}

// release reads one Application's inputs and the gate's record of it. An error is the cluster's;
// data that does not read is the release's own Unreadable.
func (k Kube) release(ctx context.Context, namespace, application string, canaries map[string]domain.Member) (domain.Release, error) {
	release := domain.Release{Namespace: namespace, Application: application}
	cm, err := k.Client.CoreV1().ConfigMaps(namespace).Get(ctx, application+inputsSuffix, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		release.Unreadable = "no release-gate inputs"
		return release, nil
	}
	if err != nil {
		return domain.Release{}, fmt.Errorf("read the release-gate inputs of %s/%s: %w", namespace, application, err)
	}
	var in inputs
	switch {
	case json.Unmarshal([]byte(cm.Data[inputsKey]), &in) != nil:
		release.Unreadable = "release-gate inputs that do not parse"
		return release, nil //nolint:nilerr // data that does not parse is the release's own state, not the cluster's failure
	case len(in.Members) > maxMembers:
		release.Unreadable = "more members than an Application has"
		return release, nil
	}
	if m := in.Migration; m != nil {
		release.Migration = &domain.Migration{Identity: m.Identity, NonTransactional: m.NonTransactional}
		if m.TestedAgainst != nil {
			release.Migration.TestedAgainst = *m.TestedAgainst
		}
	}
	for _, m := range in.Members {
		member, seen := canaries[m.Process]
		if !seen {
			// Flagger has not made its Canary yet, or it names another Application.
			member = domain.Member{Process: m.Process}
		}
		release.Members = append(release.Members, member)
	}
	return release, k.recorded(ctx, &release)
}

// recorded adds the Release Gate's record of the Application, where there is one that names the
// Application's own namespace, and marks the release unreadable where there is one that does not.
func (k Kube) recorded(ctx context.Context, release *domain.Release) error {
	name := release.Namespace + "." + release.Application + recordSuffix
	cm, err := k.Client.CoreV1().ConfigMaps(recordNamespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read the release record of %s/%s: %w", release.Namespace, release.Application, err)
	}
	var r record
	if err := json.Unmarshal([]byte(cm.Data[recordKey]), &r); err != nil || r.Namespace != release.Namespace {
		release.Unreadable = "a release record the gate did not write for it"
		return nil //nolint:nilerr // a record that does not read is the release's own state, not the cluster's failure
	}
	release.Serving, release.Pinned = r.Serving, r.Pinned
	if !r.Since.IsZero() {
		since := r.Since.UTC()
		release.Since = &since
	}
	return nil
}

// Absent is the cluster of a dashboard that runs without one: every read is domain.ErrNoCluster.
type Absent struct{}

var _ domain.Cluster = Absent{}

// Sources implements domain.Cluster.
func (Absent) Sources(context.Context) (domain.Page[domain.Source], error) {
	return domain.Page[domain.Source]{}, domain.ErrNoCluster
}

// Units implements domain.Cluster.
func (Absent) Units(context.Context) (domain.Page[domain.Unit], error) {
	return domain.Page[domain.Unit]{}, domain.ErrNoCluster
}

// Releases implements domain.Cluster.
func (Absent) Releases(context.Context) (domain.Page[domain.Release], error) {
	return domain.Page[domain.Release]{}, domain.ErrNoCluster
}

// Connect reads the cluster the dashboard runs in, or the one KUBECONFIG names on a local run,
// and none where there is neither: then every read says so.
func Connect(inCluster func() (*rest.Config, error), kubeconfig string) (domain.Cluster, error) {
	config, err := inCluster()
	if errors.Is(err, rest.ErrNotInCluster) {
		if kubeconfig == "" {
			return Absent{}, nil
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	if err != nil {
		return nil, fmt.Errorf("reach the cluster: %w", err)
	}
	client, typed := kubernetes.NewForConfig(config)
	dyn, untyped := dynamic.NewForConfig(config)
	if err := errors.Join(typed, untyped); err != nil {
		return nil, fmt.Errorf("reach the cluster: %w", err)
	}
	return Kube{Client: client, Dynamic: dyn}, nil
}
