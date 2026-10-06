// Package domain is the estate context's pure core: what the Estate repository says, read-only.
// Each Project's pin and the Pause or Rollback recorded on it, the commits that moved a pin, and
// the open issues the composition keeps one per Project condition. It imports no transport and no
// generated code (enforced by depguard).
package domain

import (
	"context"
	"errors"
	"time"
)

// Pause is what a Pause or a Rollback recorded on a pin
// (deploy-kit spec/v1/55-delivery.md#pause-and-rollback).
type Pause struct {
	By     string
	At     string
	Reason string
}

// Rollback is the release a Rollback took the Project back to.
type Rollback struct {
	Version  string
	Fragment string
}

// Pin is one Project's pin: the artifact its source names by digest.
type Pin struct {
	Project string
	Digest  string
	// Paused and RolledBack are nil where nothing is recorded.
	Paused     *Pause
	RolledBack *Rollback
}

// Deploy is one commit that moved a Project's pin: the estate's deploy log.
type Deploy struct {
	Commit  string
	At      time.Time
	Message string
}

// Issue is one open issue of the Estate repository: one Project condition.
type Issue struct {
	Number    int
	Title     string
	URL       string
	Labels    []string
	UpdatedAt time.Time
}

// ErrNoRepository is returned where the dashboard runs without the Estate repository to read.
var ErrNoRepository = errors.New("no Estate repository is configured")

// ErrNoProject is returned for a Project the Estate repository pins nothing for.
var ErrNoProject = errors.New("no such Project")

// Repository is the port the Estate repository reader implements. It only ever reads.
type Repository interface {
	Pins(ctx context.Context) ([]Pin, error)
	// Deploys returns at most limit commits that moved the Project's pin, newest first.
	Deploys(ctx context.Context, project string, limit int) ([]Deploy, error)
	Issues(ctx context.Context) ([]Issue, error)
}
