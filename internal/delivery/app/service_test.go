package app_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/app"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/domain"
)

type cluster struct {
	sources   []domain.Source
	units     []domain.Unit
	releases  []domain.Release
	pods      map[string][]domain.Pod
	truncated bool
	err       error
}

func (c cluster) Pods(_ context.Context, namespace string) (domain.Page[domain.Pod], error) {
	return domain.Page[domain.Pod]{Items: c.pods[namespace]}, c.err
}

func (c cluster) Sources(context.Context) (domain.Page[domain.Source], error) {
	return domain.Page[domain.Source]{Items: c.sources, Truncated: c.truncated}, c.err
}

func (c cluster) Units(context.Context) (domain.Page[domain.Unit], error) {
	return domain.Page[domain.Unit]{Items: c.units}, c.err
}

func (c cluster) Releases(context.Context) (domain.Page[domain.Release], error) {
	return domain.Page[domain.Release]{Items: c.releases}, c.err
}

func TestEveryReadComesInAStableOrder(t *testing.T) {
	s := app.New(cluster{
		sources:  []domain.Source{{Name: "project-b"}, {Name: "estate"}, {Name: "project-a"}},
		units:    []domain.Unit{{Name: "estate-vso-secrets"}, {Name: "apps-b"}, {Name: "apps-a"}},
		releases: []domain.Release{{Namespace: "b", Application: "x"}, {Namespace: "a", Application: "y"}, {Namespace: "a", Application: "x"}},
	})
	sources, err := s.Sources(t.Context())
	var names []string
	for _, x := range sources.Items {
		names = append(names, x.Name)
	}
	if err != nil || !slices.Equal(names, []string{"estate", "project-a", "project-b"}) {
		t.Fatalf("sources %v, %v", names, err)
	}
	units, err := s.Units(t.Context())
	names = nil
	for _, x := range units.Items {
		names = append(names, x.Name)
	}
	if err != nil || !slices.Equal(names, []string{"apps-a", "apps-b", "estate-vso-secrets"}) {
		t.Fatalf("units %v, %v", names, err)
	}
	releases, err := s.Releases(t.Context())
	names = nil
	for _, x := range releases.Items {
		names = append(names, x.Namespace+"/"+x.Application)
	}
	if err != nil || !slices.Equal(names, []string{"a/x", "a/y", "b/x"}) {
		t.Fatalf("releases %v, %v", names, err)
	}
}

func TestATruncatedReadSaysSoAfterSorting(t *testing.T) {
	got, err := app.New(cluster{sources: []domain.Source{{Name: "b"}, {Name: "a"}}, truncated: true}).Sources(t.Context())
	if err != nil || !got.Truncated || got.Items[0].Name != "a" {
		t.Fatalf("sources = %+v, %v", got, err)
	}
}

func TestEveryReadReportsTheClustersFailure(t *testing.T) {
	s := app.New(cluster{err: domain.ErrNoCluster})
	_, e1 := s.Sources(t.Context())
	_, e2 := s.Units(t.Context())
	_, e3 := s.Releases(t.Context())
	for _, err := range []error{e1, e2, e3} {
		if !errors.Is(err, domain.ErrNoCluster) {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestAProjectsPodsAreReadFromItsNamespaceByProcessThenName(t *testing.T) {
	s := app.New(cluster{pods: map[string][]domain.Pod{
		"arr-system":      {{Name: "sonarr-1", Process: "sonarr"}, {Name: "radarr-2", Process: "radarr"}, {Name: "radarr-1", Process: "radarr"}},
		"grimoire-system": {{Name: "grimoire-1", Process: "grimoire"}},
	}})
	pods, err := s.Pods(t.Context(), "arr")
	var names []string
	for _, p := range pods.Items {
		names = append(names, p.Name)
	}
	if err != nil || !slices.Equal(names, []string{"radarr-1", "radarr-2", "sonarr-1"}) {
		t.Fatalf("pods %v, %v", names, err)
	}
}

func TestAProjectThatCannotBeOneReadsNoPods(t *testing.T) {
	if _, err := app.New(cluster{}).Pods(t.Context(), "Not/A-Project"); !errors.Is(err, app.ErrNotAProject) {
		t.Fatalf("error = %v", err)
	}
}

func TestReadingPodsReportsTheClustersFailure(t *testing.T) {
	if _, err := app.New(cluster{err: domain.ErrNoCluster}).Pods(t.Context(), "arr"); !errors.Is(err, domain.ErrNoCluster) {
		t.Fatalf("error = %v", err)
	}
}
