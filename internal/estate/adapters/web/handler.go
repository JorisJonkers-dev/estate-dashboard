// Package web is the estate context's inbound HTTP adapter: the three reads of the Estate
// repository, mapped from the domain to the contract's types.
package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/estate/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/httpx"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/oas"
)

// UseCases is what the adapter needs from the application layer.
type UseCases interface {
	Pins(ctx context.Context) ([]domain.Pin, error)
	Deploys(ctx context.Context, project string, limit int) ([]domain.Deploy, error)
	Issues(ctx context.Context) ([]domain.Issue, error)
}

// Estate implements the estate operations.
type Estate struct {
	uc     UseCases
	logger *slog.Logger
}

// New returns an Estate calling uc, logging to logger what kept GitHub from answering.
func New(uc UseCases, logger *slog.Logger) *Estate {
	return &Estate{uc: uc, logger: logger}
}

// unavailable is the 503 for GitHub not answering, or a dashboard without the Estate repository.
func (h *Estate) unavailable(ctx context.Context, what string, err error) oas.ProblemStatusCode {
	detail := "GitHub did not answer."
	if errors.Is(err, domain.ErrNoRepository) {
		detail = "The dashboard runs without the Estate repository to read."
	} else {
		h.logger.ErrorContext(ctx, "read the Estate repository", "what", what, "error", err)
	}
	return oas.ProblemStatusCode{StatusCode: http.StatusServiceUnavailable, Response: httpx.Problem(http.StatusServiceUnavailable, detail)}
}

// ListPins implements listPins.
func (h *Estate) ListPins(ctx context.Context) (oas.ListPinsRes, error) {
	pins, err := h.uc.Pins(ctx)
	if err != nil {
		down := oas.ListPinsServiceUnavailable(h.unavailable(ctx, "pins", err))
		return &down, nil //nolint:nilerr // the cause is logged; the contract's 503 is the answer
	}
	items := make([]oas.Pin, 0, len(pins))
	for _, p := range pins {
		pin := oas.Pin{Project: p.Project, Digest: p.Digest}
		if p.Paused != nil {
			pin.Paused = oas.NewOptPause(oas.Pause{By: p.Paused.By, At: p.Paused.At, Reason: p.Paused.Reason})
		}
		if p.RolledBack != nil {
			pin.RolledBack = oas.NewOptRollback(oas.Rollback{Version: p.RolledBack.Version, Fragment: p.RolledBack.Fragment})
		}
		items = append(items, pin)
	}
	return &oas.PinList{Items: items}, nil
}

// ListDeploys implements listDeploys. The contract's pattern admits only a Project name, so a
// name the domain refuses is a Project with no deploys.
func (h *Estate) ListDeploys(ctx context.Context, params oas.ListDeploysParams) (oas.ListDeploysRes, error) {
	deploys, err := h.uc.Deploys(ctx, params.Project, int(params.Limit.Or(0)))
	switch {
	case errors.Is(err, domain.ErrNoProject):
		return &oas.DeployList{Items: []oas.Deploy{}}, nil
	case err != nil:
		down := oas.ListDeploysServiceUnavailable(h.unavailable(ctx, "deploys", err))
		return &down, nil
	}
	items := make([]oas.Deploy, 0, len(deploys))
	for _, d := range deploys {
		items = append(items, oas.Deploy{Commit: d.Commit, At: d.At, Message: d.Message})
	}
	return &oas.DeployList{Items: items}, nil
}

// ListIssues implements listIssues.
func (h *Estate) ListIssues(ctx context.Context) (oas.ListIssuesRes, error) {
	issues, err := h.uc.Issues(ctx)
	if err != nil {
		down := oas.ListIssuesServiceUnavailable(h.unavailable(ctx, "issues", err))
		return &down, nil //nolint:nilerr // the cause is logged; the contract's 503 is the answer
	}
	items := make([]oas.Issue, 0, len(issues))
	for _, i := range issues {
		items = append(items, oas.Issue{Number: int32(min(max(i.Number, 1), 1<<31-1)), Title: i.Title, URL: i.URL, Labels: append([]string{}, i.Labels...), UpdatedAt: i.UpdatedAt}) //nolint:gosec // clamped to the int32 range
	}
	return &oas.IssueList{Items: items}, nil
}
