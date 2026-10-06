// Package web is the delivery context's inbound HTTP adapter: the three reads of the cluster,
// mapped from the domain to the contract's types.
package web

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/httpx"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/oas"
)

// UseCases is what the adapter needs from the application layer.
type UseCases interface {
	Sources(ctx context.Context) (domain.Page[domain.Source], error)
	Units(ctx context.Context) (domain.Page[domain.Unit], error)
	Releases(ctx context.Context) (domain.Page[domain.Release], error)
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
			Applied: opt(u.Applied), Ready: u.Ready, Reason: opt(u.Reason), Message: opt(u.Message),
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
