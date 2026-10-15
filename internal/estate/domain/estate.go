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

// Handover is the handover ledger from the Platform document
// (deploy-kit spec/v1/14-platform-intent.md#handover-ledger).
type Handover struct {
	// Estate lists Projects delivered by their pins. Legacy lists those fleet-infra still delivers.
	Estate []string
	Legacy []string
}

// Issue is one open issue in the Estate repository.
type Issue struct {
	Number    int
	Title     string
	URL       string
	Labels    []string
	UpdatedAt time.Time
	// Project and Condition come from the marker at the end of the body,
	// `<!-- estate-condition: <project>/<condition> -->`. Both are empty if a person opened the issue.
	Project   string
	Condition string
}

// Stage is one step of a rollout, as the Reporter wrote it on the GitHub Deployment.
type Stage struct {
	State       string
	Description string
	// Log is the run behind it, when there is one.
	Log string
	At  time.Time
}

// Rollout is one release of a Project that reached production. It is the GitHub Deployment
// composition opened in the Project's repository, with its stages, newest first.
type Rollout struct {
	ID       int64
	Version  string
	Commit   string
	Run      string
	Artifact string
	At       time.Time
	Stages   []Stage
}

// Notes are a release's notes, as its GitHub Release holds them: Markdown.
type Notes struct {
	Version     string
	Body        string
	URL         string
	PublishedAt time.Time
}

// ErrNoRelease is returned for a version its repository published no GitHub Release for.
var ErrNoRelease = errors.New("no such release")

// ErrForeignRepository is returned for a repository outside the estate's owner. The dashboard
// reads only the estate's own repositories.
var ErrForeignRepository = errors.New("not one of the estate's repositories")

// ErrNoRepository is returned where the dashboard runs without the Estate repository to read.
var ErrNoRepository = errors.New("no Estate repository is configured")

// ErrNoProject is returned for a Project the Estate repository pins nothing for.
var ErrNoProject = errors.New("no such Project")

// Repository is the port the Estate repository reader implements. It only ever reads.
type Repository interface {
	Pins(ctx context.Context) ([]Pin, error)
	// Handover returns the ledger, or an empty one if the document has none.
	Handover(ctx context.Context) (Handover, error)
	// Deploys returns at most limit commits that moved the Project's pin, newest first.
	Deploys(ctx context.Context, project string, limit int) ([]Deploy, error)
	Issues(ctx context.Context) ([]Issue, error)
	// Rollouts returns up to limit rollouts of the Project in repository, newest first.
	Rollouts(ctx context.Context, repository, project string, limit int) ([]Rollout, error)
	// Notes returns the notes of version's GitHub Release in repository.
	Notes(ctx context.Context, repository, version string) (Notes, error)
}
