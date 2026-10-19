// Package domain is the delivery context's pure core: what the cluster says about the estate's
// delivery, read-only. Flux's sources and units, and each gated Application's release. It imports
// no transport, Kubernetes client or generated code (enforced by depguard).
package domain

import (
	"context"
	"errors"
	"time"
)

// Condition is Flux's or Flagger's word on one object: ready or not, and why.
type Condition struct {
	Ready   bool
	Reason  string
	Message string
}

// Source is one Flux OCIRepository the render's pin sources declare: an artifact by digest.
type Source struct {
	Name string
	URL  string
	// Digest is the digest the pin names; Revision is what Flux last fetched.
	Digest   string
	Revision string
	Condition
}

// Unit is one Flux Kustomization: one Reconcile Unit, applied from a source after the units it
// depends on.
type Unit struct {
	Name      string
	Source    string
	Path      string
	DependsOn []string
	// Applied is the source revision Flux last applied.
	Applied string
	// Version and Repository are the release the unit's pin names and where it came from. They
	// come from the unit's annotations and are empty if composition wrote none.
	Version    string
	Repository string
	Condition
}

// Member is one Process of a gated Application, as Flagger switches it.
type Member struct {
	Process string
	// Phase is Flagger's phase of its Canary; Revision the Application revision its webhooks carry.
	Phase      string
	Revision   string
	Iterations int
}

// Migration is what the gate starts and may undo for a release.
type Migration struct {
	Identity         string
	TestedAgainst    string
	NonTransactional bool
}

// Release is one gated Application: its members, its migration, and what the Release Gate
// recorded it serves and saw pinned.
type Release struct {
	Namespace   string
	Application string
	Members     []Member
	Migration   *Migration
	// Serving, Pinned and Since are the gate's record; empty where it has recorded nothing.
	Serving string
	Pinned  string
	Since   *time.Time
	// Unreadable says why the rest could not be read, where it could not: inputs or a record that
	// do not parse, or more members than an Application has. One such release is shown as such,
	// and never keeps the others from being read.
	Unreadable string
}

// Probes says which checks the kubelet runs on a container.
type Probes struct {
	Readiness bool
	Liveness  bool
	Startup   bool
}

// ContainerState is what the kubelet says a container is doing now.
type ContainerState string

// The states a container is in.
const (
	Running    ContainerState = "running"
	Waiting    ContainerState = "waiting"
	Terminated ContainerState = "terminated"
)

// Termination is how a container last ended.
type Termination struct {
	Reason   string
	ExitCode int
	At       *time.Time
}

// Container is one container of a pod, as its spec declares it and the kubelet reports it.
type Container struct {
	Name  string
	Image string
	// Init is an init container's; it runs to completion before the others start.
	Init  bool
	Ready bool
	State ContainerState
	// Reason and Message say why it waits or ended, where the kubelet says.
	Reason   string
	Message  string
	Since    *time.Time
	Restarts int
	// Last is how it ended before its latest start, where it restarted.
	Last   *Termination
	Probes Probes
}

// Pod is one pod the render's workloads run, with what its labels say of it.
type Pod struct {
	Namespace string
	Name      string
	// Process and Application are the pod's app.kubernetes.io/name and part-of labels.
	Process     string
	Application string
	// Version is the release it runs, from its app.kubernetes.io/version label; empty without one.
	Version string
	// Owner is the kind of the object that made it: ReplicaSet, StatefulSet, Job, DaemonSet.
	Owner   string
	Node    string
	Phase   string
	Started *time.Time
	// Ready is the pod's Ready condition; Deleting, that it is shutting down.
	Ready      bool
	Deleting   bool
	Containers []Container
}

// ErrNoCluster is returned where the dashboard runs without a cluster to read.
var ErrNoCluster = errors.New("no cluster is configured")

// Page is what one read returns: the items, and whether there were more than one read returns.
// A read is bounded, so a cluster holding more says so rather than leaving the rest out unseen.
type Page[T any] struct {
	Items     []T
	Truncated bool
}

// Cluster is the port the cluster reader implements. It only ever reads.
type Cluster interface {
	Sources(ctx context.Context) (Page[Source], error)
	Units(ctx context.Context) (Page[Unit], error)
	Releases(ctx context.Context) (Page[Release], error)
	// Pods returns every pod the render's workloads run in namespace.
	Pods(ctx context.Context, namespace string) (Page[Pod], error)
}
