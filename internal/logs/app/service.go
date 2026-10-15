// Package app holds the logs context's use cases.
package app

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/logs/domain"
)

const (
	// DefaultSince is how far back a read looks when the caller does not say.
	DefaultSince = 24 * time.Hour
	// MaxSince is the furthest back a read looks.
	MaxSince = 30 * 24 * time.Hour
	// DefaultLines is how many lines a read returns when the caller does not say.
	DefaultLines = 500
	// MaxLines is the most lines one read returns.
	MaxLines = 5000
)

var (
	// project is what a Project may be called.
	project = regexp.MustCompile(`^[a-z0-9_][a-z0-9_-]{0,62}$`)
	// name is what a pod or a container may be called in Kubernetes.
	name = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)
)

// Service reads a Project's logs. A Project's containers run in `<project>-system`.
type Service struct {
	store domain.Store
	now   func() time.Time
}

// New returns a Service over store.
func New(store domain.Store, now func() time.Time) *Service {
	return &Service{store: store, now: now}
}

// window is the start of a read looking back since, clamped to (0, MaxSince].
func (s *Service) window(since time.Duration) time.Time {
	switch {
	case since <= 0:
		since = DefaultSince
	case since > MaxSince:
		since = MaxSince
	}
	return s.now().Add(-since)
}

// Streams returns every container of the Project that wrote logs in the last since.
func (s *Service) Streams(ctx context.Context, proj string, since time.Duration) ([]domain.Stream, error) {
	if !project.MatchString(proj) {
		return nil, fmt.Errorf("%w: %q", domain.ErrNotAName, proj)
	}
	streams, err := s.store.Streams(ctx, proj+"-system", s.window(since))
	if err != nil {
		return nil, fmt.Errorf("read the log streams of %s: %w", proj, err)
	}
	return streams, nil
}

// Lines returns up to limit lines one container of the Project wrote in the last since.
func (s *Service) Lines(ctx context.Context, proj, pod, container string, since time.Duration, limit int) ([]domain.Line, error) {
	if !project.MatchString(proj) || !name.MatchString(pod) || !name.MatchString(container) {
		return nil, fmt.Errorf("%w: %q %q %q", domain.ErrNotAName, proj, pod, container)
	}
	switch {
	case limit < 1:
		limit = DefaultLines
	case limit > MaxLines:
		limit = MaxLines
	}
	lines, err := s.store.Lines(ctx, proj+"-system", domain.Stream{Pod: pod, Container: container}, s.window(since), limit)
	if err != nil {
		return nil, fmt.Errorf("read the logs of %s/%s: %w", pod, container, err)
	}
	return lines, nil
}
