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
}
