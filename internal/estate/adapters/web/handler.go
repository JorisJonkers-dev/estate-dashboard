// Package web is the estate context's inbound HTTP adapter. It maps the domain's reads of the
// Estate repository to the contract's types.
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
	Handover(ctx context.Context) (domain.Handover, error)
	Deploys(ctx context.Context, project string, limit int) ([]domain.Deploy, error)
	Issues(ctx context.Context) ([]domain.Issue, error)
	Rollouts(ctx context.Context, repository, project string, limit int) ([]domain.Rollout, error)
	Notes(ctx context.Context, repository, version string) (domain.Notes, error)
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

// GetHandover implements getHandover.
func (h *Estate) GetHandover(ctx context.Context) (oas.GetHandoverRes, error) {
	ledger, err := h.uc.Handover(ctx)
	if err != nil {
		down := oas.GetHandoverServiceUnavailable(h.unavailable(ctx, "handover", err))
		return &down, nil //nolint:nilerr // already logged; the 503 is the answer
	}
	return &oas.Handover{Estate: append([]string{}, ledger.Estate...), Legacy: append([]string{}, ledger.Legacy...)}, nil
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

// ListRollouts implements listRollouts. A foreign repository or an unknown Project gets an
// empty list.
func (h *Estate) ListRollouts(ctx context.Context, params oas.ListRolloutsParams) (oas.ListRolloutsRes, error) {
	rollouts, err := h.uc.Rollouts(ctx, params.Repository, params.Project, int(params.Limit.Or(0)))
	switch {
	case errors.Is(err, domain.ErrNoProject), errors.Is(err, domain.ErrForeignRepository):
		return &oas.RolloutList{Items: []oas.Rollout{}}, nil
	case err != nil:
		down := oas.ListRolloutsServiceUnavailable(h.unavailable(ctx, "rollouts", err))
		return &down, nil
	}
	items := make([]oas.Rollout, 0, len(rollouts))
	for _, r := range rollouts {
		stages := make([]oas.Stage, 0, len(r.Stages))
		for _, s := range r.Stages {
			stages = append(stages, oas.Stage{State: s.State, Description: s.Description, Log: opt(s.Log), At: s.At})
		}
		items = append(items, oas.Rollout{ID: r.ID, Version: r.Version, Commit: r.Commit, Run: opt(r.Run), Artifact: opt(r.Artifact), At: r.At, Stages: stages})
	}
	return &oas.RolloutList{Items: items}, nil
}

// GetReleaseNotes implements getReleaseNotes. A release with no GitHub Release, or one outside the
// estate's repositories, is a 404.
func (h *Estate) GetReleaseNotes(ctx context.Context, params oas.GetReleaseNotesParams) (oas.GetReleaseNotesRes, error) {
	notes, err := h.uc.Notes(ctx, params.Repository, params.Version)
	switch {
	case errors.Is(err, domain.ErrNoRelease), errors.Is(err, domain.ErrForeignRepository):
		missing := oas.GetReleaseNotesNotFound{StatusCode: http.StatusNotFound, Response: httpx.Problem(http.StatusNotFound, "This release has no notes.")}
		return &missing, nil
	case err != nil:
		down := oas.GetReleaseNotesServiceUnavailable(h.unavailable(ctx, "notes", err))
		return &down, nil
	}
	return &oas.ReleaseNotes{Version: notes.Version, Body: notes.Body, URL: opt(notes.URL), PublishedAt: oas.NewOptDateTime(notes.PublishedAt)}, nil
}

// notYet is the 503 for a read the dashboard does not make yet.
func notYet(what string) oas.ProblemStatusCode {
	return oas.ProblemStatusCode{StatusCode: http.StatusServiceUnavailable, Response: httpx.Problem(http.StatusServiceUnavailable, what+" are not read yet.")}
}

// ListCompositionRuns implements listCompositionRuns. The runs are read from each run's artifact,
// which compose.yml does not upload yet (JorisJonkers-dev/estate-dashboard#30).
func (h *Estate) ListCompositionRuns(context.Context) (oas.ListCompositionRunsRes, error) {
	down := oas.ListCompositionRunsServiceUnavailable(notYet("Composition runs"))
	return &down, nil
}

// GetDeclaration implements getDeclaration. A declaration is read from the Project's fragment,
// which the dashboard does not read yet (JorisJonkers-dev/estate-dashboard#31).
func (h *Estate) GetDeclaration(context.Context, oas.GetDeclarationParams) (oas.GetDeclarationRes, error) {
	down := oas.GetDeclarationServiceUnavailable(notYet("Declarations"))
	return &down, nil
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
		items = append(items, oas.Issue{
			Number: int32(min(max(i.Number, 1), 1<<31-1)), //nolint:gosec // clamped to int32
			Title:  i.Title, URL: i.URL, Labels: append([]string{}, i.Labels...), UpdatedAt: i.UpdatedAt,
			Project: opt(i.Project), Condition: opt(i.Condition),
		})
	}
	return &oas.IssueList{Items: items}, nil
}

// opt returns s as an optional field, absent when s is empty.
func opt(s string) oas.OptString {
	if s == "" {
		return oas.OptString{}
	}
	return oas.NewOptString(s)
}
