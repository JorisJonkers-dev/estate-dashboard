// Package app holds the delivery context's use cases: what the screens read of the cluster, in a
// stable order.
package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/domain"
)

// Service reads the cluster.
type Service struct {
	cluster domain.Cluster
}

// New returns a Service over cluster.
func New(cluster domain.Cluster) *Service {
	return &Service{cluster: cluster}
}

// Sources returns every pin source, by name.
func (s *Service) Sources(ctx context.Context) (domain.Page[domain.Source], error) {
	sources, err := s.cluster.Sources(ctx)
	if err != nil {
		return domain.Page[domain.Source]{}, fmt.Errorf("read the sources: %w", err)
	}
	slices.SortFunc(sources.Items, func(a, b domain.Source) int { return strings.Compare(a.Name, b.Name) })
	return sources, nil
}

// Units returns every Reconcile Unit, by name.
func (s *Service) Units(ctx context.Context) (domain.Page[domain.Unit], error) {
	units, err := s.cluster.Units(ctx)
	if err != nil {
		return domain.Page[domain.Unit]{}, fmt.Errorf("read the units: %w", err)
	}
	slices.SortFunc(units.Items, func(a, b domain.Unit) int { return strings.Compare(a.Name, b.Name) })
	return units, nil
}

// Releases returns every gated Application, by namespace and then Application.
func (s *Service) Releases(ctx context.Context) (domain.Page[domain.Release], error) {
	releases, err := s.cluster.Releases(ctx)
	if err != nil {
		return domain.Page[domain.Release]{}, fmt.Errorf("read the releases: %w", err)
	}
	slices.SortFunc(releases.Items, func(a, b domain.Release) int {
		if c := strings.Compare(a.Namespace, b.Namespace); c != 0 {
			return c
		}
		return strings.Compare(a.Application, b.Application)
	})
	return releases, nil
}

// ErrNotAProject is returned for a Project name that cannot be one.
var ErrNotAProject = errors.New("not a project")

// project is what a Project may be called.
var project = regexp.MustCompile(`^[a-z0-9_][a-z0-9_-]{0,62}$`)

// Pods returns the pods a Project's workloads run, in `<project>-system`, by Process then name.
func (s *Service) Pods(ctx context.Context, proj string) (domain.Page[domain.Pod], error) {
	if !project.MatchString(proj) {
		return domain.Page[domain.Pod]{}, fmt.Errorf("%w: %q", ErrNotAProject, proj)
	}
	pods, err := s.cluster.Pods(ctx, proj+"-system")
	if err != nil {
		return domain.Page[domain.Pod]{}, fmt.Errorf("read the pods of %s: %w", proj, err)
	}
	slices.SortFunc(pods.Items, func(a, b domain.Pod) int {
		if c := strings.Compare(a.Process, b.Process); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	return pods, nil
}
