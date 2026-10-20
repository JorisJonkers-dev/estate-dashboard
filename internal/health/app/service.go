// Package app holds the health context's use cases.
package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/health/domain"
)

const (
	// Step is how long one block lasts.
	Step = 30 * time.Second
	// Blocks is how many blocks a timeline holds: the last hour.
	Blocks = 120
)

// ErrNotAProject is returned for a Project name that cannot be one.
var ErrNotAProject = errors.New("not a project")

// project is what a Project may be called.
var project = regexp.MustCompile(`^[a-z0-9_][a-z0-9_-]{0,62}$`)

// Service reads the metrics.
type Service struct {
	metrics domain.Metrics
	now     func() time.Time
}

// New returns a Service over metrics, at the time now says.
func New(metrics domain.Metrics, now func() time.Time) *Service {
	return &Service{metrics: metrics, now: now}
}

// Timeline returns the last hour of a Project, oldest block first: each the worst of its pods,
// the uptime checks named, and its unit.
func (s *Service) Timeline(ctx context.Context, proj string, checks []string) ([]domain.Block, error) {
	if !project.MatchString(proj) {
		return nil, fmt.Errorf("%w: %q", ErrNotAProject, proj)
	}
	end := s.now().UTC().Truncate(Step)
	start := end.Add(-(Blocks - 1) * Step)
	samples, err := s.metrics.Samples(ctx, proj, checks, start, end, Step)
	if err != nil {
		return nil, fmt.Errorf("read the health of %s: %w", proj, err)
	}
	blocks := make([]domain.Block, 0, Blocks)
	for at := start; !at.After(end); at = at.Add(Step) {
		blocks = append(blocks, domain.BlockOf(at, samples[at]))
	}
	return blocks, nil
}

const (
	// RolloutStep is how often a pod's readiness is read over a release's window.
	RolloutStep = 5 * time.Second
	// MaxRollout is the longest window one read covers.
	MaxRollout = 2 * time.Hour
)

// ErrBadWindow is returned for a window that ends before it starts, or lasts too long.
var ErrBadWindow = errors.New("not a window")

// Rollout returns what a Project's pods and canaries did from start to end.
func (s *Service) Rollout(ctx context.Context, proj string, start, end time.Time) (domain.Window, error) {
	if !project.MatchString(proj) {
		return domain.Window{}, fmt.Errorf("%w: %q", ErrNotAProject, proj)
	}
	if !end.After(start) || end.Sub(start) > MaxRollout {
		return domain.Window{}, fmt.Errorf("%w: %s to %s", ErrBadWindow, start, end)
	}
	readiness, weights, err := s.metrics.Rollout(ctx, proj, start, end, RolloutStep)
	if err != nil {
		return domain.Window{}, fmt.Errorf("read the rollout of %s: %w", proj, err)
	}
	return domain.WindowOf(readiness, weights, end, RolloutStep), nil
}
