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

// Handover is the Platform document's handover ledger: which delivery path each Project is on
// (deploy-kit spec/v1/14-platform-intent.md#handover-ledger).
type Handover struct {
	// Estate are the Projects delivered by their pins; Legacy those fleet-infra still delivers.
	Estate []string
	Legacy []string
}

// Issue is one open issue of the Estate repository.
type Issue struct {
	Number    int
	Title     string
	URL       string
	Labels    []string
	UpdatedAt time.Time
	// Project and Condition are the condition composition keeps the issue for, from the marker
	// its body ends with (`<!-- estate-condition: <project>/<condition> -->`); empty on an issue
	// a person opened.
	Project   string
	Condition string
}

// Stage is one step of a rollout, as the Reporter wrote it on the GitHub Deployment.
type Stage struct {
	State       string
	Description string
	// Log is the run that did it, where there is one.
	Log string
	At  time.Time
}

// Rollout is one release of a Project reaching production: the GitHub Deployment composition
// opened in the Project's own repository, and the stages it went through, newest first.
type Rollout struct {
	ID       int64
	Version  string
	Commit   string
	Run      string
	Artifact string
	At       time.Time
	Stages   []Stage
}

// ErrForeignRepository is returned for a repository outside the Estate repository's owner: the
// dashboard reads the estate's own repositories and no others.
var ErrForeignRepository = errors.New("not one of the estate's repositories")

// ErrNoRepository is returned where the dashboard runs without the Estate repository to read.
var ErrNoRepository = errors.New("no Estate repository is configured")

// ErrNoProject is returned for a Project the Estate repository pins nothing for.
var ErrNoProject = errors.New("no such Project")

// Repository is the port the Estate repository reader implements. It only ever reads.
type Repository interface {
	Pins(ctx context.Context) ([]Pin, error)
	// Handover returns the Platform document's ledger; none, where it holds no ledger.
	Handover(ctx context.Context) (Handover, error)
	// Deploys returns at most limit commits that moved the Project's pin, newest first.
	Deploys(ctx context.Context, project string, limit int) ([]Deploy, error)
	Issues(ctx context.Context) ([]Issue, error)
	// Rollouts returns at most limit of the Project's rollouts in repository, newest first.
	Rollouts(ctx context.Context, repository, project string, limit int) ([]Rollout, error)
}
