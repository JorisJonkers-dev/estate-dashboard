// Package web is the delivery context's inbound HTTP adapter: the three reads of the cluster,
// mapped from the domain to the contract's types.
package web

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/app"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/httpx"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/oas"
)

// UseCases is what the adapter needs from the application layer.
type UseCases interface {
	Sources(ctx context.Context) (domain.Page[domain.Source], error)
	Units(ctx context.Context) (domain.Page[domain.Unit], error)
	Releases(ctx context.Context) (domain.Page[domain.Release], error)
	Pods(ctx context.Context, project string) (domain.Page[domain.Pod], error)
}

// Delivery implements the delivery operations.
type Delivery struct {
	uc     UseCases
	logger *slog.Logger
}

// New returns a Delivery calling uc, logging to logger what kept the cluster from answering.
func New(uc UseCases, logger *slog.Logger) *Delivery {
	return &Delivery{uc: uc, logger: logger}
}

// unavailable is the 503 for a cluster that did not answer, or a dashboard without one.
func (h *Delivery) unavailable(ctx context.Context, what string, err error) oas.ProblemStatusCode {
	detail := "The cluster did not answer."
	if errors.Is(err, domain.ErrNoCluster) {
		detail = "The dashboard runs without a cluster to read."
	} else {
		h.logger.ErrorContext(ctx, "read the cluster", "what", what, "error", err)
	}
	return oas.ProblemStatusCode{StatusCode: http.StatusServiceUnavailable, Response: httpx.Problem(http.StatusServiceUnavailable, detail)}
}

func opt(s string) oas.OptString {
	if s == "" {
		return oas.OptString{}
	}
	return oas.NewOptString(s)
}

// ListSources implements listSources.
func (h *Delivery) ListSources(ctx context.Context) (oas.ListSourcesRes, error) {
	sources, err := h.uc.Sources(ctx)
	if err != nil {
		down := oas.ListSourcesServiceUnavailable(h.unavailable(ctx, "sources", err))
		return &down, nil //nolint:nilerr // the cause is logged; the contract's 503 is the answer
	}
	items := make([]oas.Source, 0, len(sources.Items))
	for _, s := range sources.Items {
		items = append(items, oas.Source{
			Name: s.Name, URL: s.URL, Digest: s.Digest, Revision: opt(s.Revision),
			Ready: s.Ready, Reason: opt(s.Reason), Message: opt(s.Message),
		})
	}
	return &oas.SourceList{Items: items, Truncated: sources.Truncated}, nil
}

// ListUnits implements listUnits.
func (h *Delivery) ListUnits(ctx context.Context) (oas.ListUnitsRes, error) {
	units, err := h.uc.Units(ctx)
	if err != nil {
		down := oas.ListUnitsServiceUnavailable(h.unavailable(ctx, "units", err))
		return &down, nil //nolint:nilerr // the cause is logged; the contract's 503 is the answer
	}
	items := make([]oas.Unit, 0, len(units.Items))
	for _, u := range units.Items {
		items = append(items, oas.Unit{
			Name: u.Name, Source: u.Source, Path: u.Path, DependsOn: append([]string{}, u.DependsOn...),
			Applied: opt(u.Applied), Version: opt(u.Version), Repository: opt(u.Repository), Ready: u.Ready, Reason: opt(u.Reason), Message: opt(u.Message),
		})
	}
	return &oas.UnitList{Items: items, Truncated: units.Truncated}, nil
}

// ListReleases implements listReleases.
func (h *Delivery) ListReleases(ctx context.Context) (oas.ListReleasesRes, error) {
	releases, err := h.uc.Releases(ctx)
	if err != nil {
		down := oas.ListReleasesServiceUnavailable(h.unavailable(ctx, "releases", err))
		return &down, nil //nolint:nilerr // the cause is logged; the contract's 503 is the answer
	}
	items := make([]oas.Release, 0, len(releases.Items))
	for _, r := range releases.Items {
		items = append(items, toRelease(r))
	}
	return &oas.ReleaseList{Items: items, Truncated: releases.Truncated}, nil
}

func toRelease(r domain.Release) oas.Release {
	out := oas.Release{Namespace: r.Namespace, Application: r.Application, Members: make([]oas.Member, 0, len(r.Members)), Serving: opt(r.Serving), Pinned: opt(r.Pinned)}
	for _, m := range r.Members {
		out.Members = append(out.Members, oas.Member{
			Process: m.Process, Phase: opt(m.Phase), Revision: opt(m.Revision),
			Iterations: int32(min(max(m.Iterations, 0), math.MaxInt32)), //nolint:gosec // clamped to the int32 range
		})
	}
	if m := r.Migration; m != nil {
		out.Migration = oas.NewOptReleaseMigration(oas.ReleaseMigration{Identity: m.Identity, TestedAgainst: opt(m.TestedAgainst), NonTransactional: m.NonTransactional})
	}
	if r.Since != nil {
		out.Since = oas.NewOptDateTime(*r.Since)
	}
	out.Unreadable = opt(r.Unreadable)
	return out
}

// ListPods implements listPods. A name that cannot be a Project's runs no pods.
func (h *Delivery) ListPods(ctx context.Context, params oas.ListPodsParams) (oas.ListPodsRes, error) {
	pods, err := h.uc.Pods(ctx, params.Project)
	switch {
	case errors.Is(err, app.ErrNotAProject):
		return &oas.PodList{Items: []oas.Pod{}}, nil
	case err != nil:
		down := oas.ListPodsServiceUnavailable(h.unavailable(ctx, "pods", err))
		return &down, nil
	}
	items := make([]oas.Pod, 0, len(pods.Items))
	for _, p := range pods.Items {
		items = append(items, toPod(p))
	}
	return &oas.PodList{Items: items, Truncated: pods.Truncated}, nil
}

func toPod(p domain.Pod) oas.Pod {
	out := oas.Pod{
		Name: p.Name, Process: p.Process, Application: opt(p.Application), Version: opt(p.Version), Owner: opt(p.Owner),
		Node: opt(p.Node), Phase: opt(p.Phase), Started: optTime(p.Started), Ready: p.Ready, Deleting: p.Deleting,
		Containers: make([]oas.PodContainer, 0, len(p.Containers)),
	}
	for _, c := range p.Containers {
		container := oas.PodContainer{
			Name: c.Name, Image: c.Image, Init: c.Init, Ready: c.Ready, Reason: opt(c.Reason), Message: opt(c.Message),
			Since: optTime(c.Since), Restarts: clamp(c.Restarts),
			Probes: oas.Probes{Readiness: c.Probes.Readiness, Liveness: c.Probes.Liveness, Startup: c.Probes.Startup},
		}
		if c.State != "" {
			container.State = oas.NewOptPodContainerState(oas.PodContainerState(c.State))
		}
		if l := c.Last; l != nil {
			container.Last = oas.NewOptTermination(oas.Termination{Reason: opt(l.Reason), ExitCode: clamp(l.ExitCode), At: optTime(l.At)})
		}
		out.Containers = append(out.Containers, container)
	}
	return out
}

func optTime(t *time.Time) oas.OptDateTime {
	if t == nil {
		return oas.OptDateTime{}
	}
	return oas.NewOptDateTime(*t)
}

// clamp is n in the int32 range.
func clamp(n int) int32 {
	return int32(min(max(n, math.MinInt32), math.MaxInt32)) //nolint:gosec // clamped to the int32 range
}
