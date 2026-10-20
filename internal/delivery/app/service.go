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
	skipped []string
}

// New returns a Service over cluster. The images the estate runs leave out the pods of the skipped
// namespaces: the cluster's own, which run none of the estate's software.
func New(cluster domain.Cluster, skipped ...string) *Service {
	return &Service{cluster: cluster, skipped: skipped}
}

// Containers returns every image the estate runs, with every container that runs it.
func (s *Service) Containers(ctx context.Context) (domain.Page[domain.Images], error) {
	pods, err := s.cluster.AllPods(ctx)
	if err != nil {
		return domain.Page[domain.Images]{}, fmt.Errorf("read the pods: %w", err)
	}
	kept := slices.DeleteFunc(pods.Items, func(p domain.Pod) bool { return slices.Contains(s.skipped, p.Namespace) })
	return domain.Page[domain.Images]{Items: domain.Group(kept), Truncated: pods.Truncated}, nil
}

// History returns every software version the image at repository ran, newest first.
func (s *Service) History(ctx context.Context, repository string) ([]domain.VersionChange, error) {
	revisions, err := s.cluster.Revisions(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the revisions: %w", err)
	}
	return domain.History(repository, revisions.Items), nil
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
