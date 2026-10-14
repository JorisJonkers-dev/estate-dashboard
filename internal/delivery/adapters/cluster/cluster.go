// Package cluster implements the delivery context's Cluster port on the Kubernetes API, through a
// ServiceAccount that may get, list and watch Flux's OCIRepositories and Kustomizations and
// Flagger's Canaries, and nothing else. Each gated Application's release-gate inputs and the
// Release Gate's record of it are the gate's to read: the dashboard asks the gate for both, at
// GET /releases, and holds no ConfigMap access (JorisJonkers-dev/estate-dashboard#15).
package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
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
	// releasesPath is where the Release Gate serves what it holds of every gated Application.
	releasesPath = "/releases"
	// maxGateAnswer bounds what one answer of the gate is read up to: a thousand Applications
	// of a few hundred bytes each.
	maxGateAnswer = 4 << 20
)

// Kube reads the cluster, and the Release Gate in it.
type Kube struct {
	Dynamic dynamic.Interface
	// Gate is the Release Gate's address, http://<host>:<port>; nil where the dashboard knows
	// none, and then no release can be read.
	Gate *url.URL
	HTTP *http.Client
}

// ErrNoGate is returned where the dashboard reads a cluster but knows no Release Gate to ask.
var ErrNoGate = errors.New("no Release Gate is configured")

var _ domain.Cluster = Kube{}

// maxListed is the most objects one read lists: what the contract's lists carry. A cluster that
// holds more answers with the first page, so no request grows with what anyone can create.
const maxListed = 1000

// maxReleases is the most Applications one read of the releases carries: what the contract's
// list carries, so a cluster full of Canaries never turns one answer into thousands.
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

// gateRelease is one gated Application as the Release Gate serves it at GET /releases.
type gateRelease struct {
	Namespace   string   `json:"namespace"`
	Application string   `json:"application"`
	Members     []string `json:"members"`
	Migration   *struct {
		Identity         string `json:"identity"`
		TestedAgainst    string `json:"testedAgainst"`
		NonTransactional bool   `json:"nonTransactional"`
	} `json:"migration"`
	Serving    string     `json:"serving"`
	Pinned     string     `json:"pinned"`
	Since      *time.Time `json:"since"`
	Unreadable string     `json:"unreadable"`
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
// render wrote, whose webhooks name their Application; each one's inputs and the gate's record of
// it are what the gate serves, read in one request. Its members are the Canaries already listed.
// One Application whose data cannot be read is reported as such and keeps no other from being read.
func (k Kube) Releases(ctx context.Context) (domain.Page[domain.Release], error) {
	items, more, err := k.list(ctx, Canaries)
	if err != nil {
		return domain.Page[domain.Release]{}, err
	}
	served, err := k.gate(ctx)
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
		releases = append(releases, release(at[0], at[1], served[at], byApplication[at]))
	}
	return domain.Page[domain.Release]{Items: releases, Truncated: more}, nil
}

// gate asks the Release Gate what it holds of every gated Application. A gate that does not
// answer, or answers with something else, is the cluster's failure, not a release's data.
func (k Kube) gate(ctx context.Context) (map[[2]string]*gateRelease, error) {
	if k.Gate == nil {
		return nil, ErrNoGate
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.Gate.JoinPath(releasesPath).String(), nil)
	if err != nil {
		return nil, fmt.Errorf("ask the Release Gate: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := k.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ask the Release Gate: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ask the Release Gate: %s", resp.Status)
	}
	var answer struct {
		Releases []gateRelease `json:"releases"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxGateAnswer)).Decode(&answer); err != nil {
		return nil, fmt.Errorf("read the Release Gate's answer: %w", err)
	}
	served := make(map[[2]string]*gateRelease, len(answer.Releases))
	for i := range answer.Releases {
		r := &answer.Releases[i]
		served[[2]string{r.Namespace, r.Application}] = r
	}
	return served, nil
}

// release is one Application as the gate serves it, its members joined with their Canaries. An
// Application the gate does not serve has no release-gate inputs it could read.
func release(namespace, application string, served *gateRelease, canaries map[string]domain.Member) domain.Release {
	release := domain.Release{Namespace: namespace, Application: application}
	switch {
	case served == nil:
		release.Unreadable = "no release-gate inputs"
		return release
	case served.Unreadable != "":
		release.Unreadable = served.Unreadable
		return release
	case len(served.Members) > maxMembers:
		release.Unreadable = "more members than an Application has"
		return release
	}
	if m := served.Migration; m != nil {
		release.Migration = &domain.Migration{Identity: m.Identity, TestedAgainst: m.TestedAgainst, NonTransactional: m.NonTransactional}
	}
	for _, process := range served.Members {
		member, seen := canaries[process]
		if !seen {
			// Flagger has not made its Canary yet, or it names another Application.
			member = domain.Member{Process: process}
		}
		release.Members = append(release.Members, member)
	}
	release.Serving, release.Pinned = served.Serving, served.Pinned
	if served.Since != nil {
		since := served.Since.UTC()
		release.Since = &since
	}
	return release
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
// and none where there is neither: then every read says so. gate is the Release Gate's address,
// or nil.
func Connect(inCluster func() (*rest.Config, error), kubeconfig string, gate *url.URL, client *http.Client) (domain.Cluster, error) {
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
	dyn, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("reach the cluster: %w", err)
	}
	return Kube{Dynamic: dyn, Gate: gate, HTTP: client}, nil
}
