// Package web is the health context's inbound HTTP adapter, mapping a Project's timeline to the
// contract's types.
package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/health/app"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/health/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/httpx"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/oas"
)

// UseCases is what the adapter needs from the application layer.
type UseCases interface {
	Timeline(ctx context.Context, project string, checks []string) ([]domain.Block, error)
	Rollout(ctx context.Context, project string, start, end time.Time) (domain.Window, error)
}

// Health implements the health operations.
type Health struct {
	uc     UseCases
	logger *slog.Logger
}

// New returns a Health calling uc, logging to logger why Prometheus did not answer.
func New(uc UseCases, logger *slog.Logger) *Health {
	return &Health{uc: uc, logger: logger}
}

// GetHealth implements getHealth. A name that cannot be a Project's has no health to show.
func (h *Health) GetHealth(ctx context.Context, params oas.GetHealthParams) (oas.GetHealthRes, error) {
	blocks, err := h.uc.Timeline(ctx, params.Project, params.Check)
	switch {
	case errors.Is(err, app.ErrNotAProject):
		return &oas.HealthTimeline{Step: int32(app.Step / time.Second), Blocks: []oas.HealthBlock{}}, nil
	case err != nil:
		down := oas.GetHealthServiceUnavailable(h.unavailable(ctx, err))
		return &down, nil
	}
	items := make([]oas.HealthBlock, 0, len(blocks))
	for _, b := range blocks {
		block := oas.HealthBlock{At: b.At, State: oas.HealthBlockState(b.State)}
		if b.Detail != "" {
			block.Detail = oas.NewOptString(b.Detail)
		}
		items = append(items, block)
	}
	return &oas.HealthTimeline{Step: int32(app.Step / time.Second), Blocks: items}, nil
}

// GetRolloutWindow implements getRolloutWindow.
func (h *Health) GetRolloutWindow(ctx context.Context, params oas.GetRolloutWindowParams) (oas.GetRolloutWindowRes, error) {
	window, err := h.uc.Rollout(ctx, params.Project, params.From, params.To)
	switch {
	case errors.Is(err, app.ErrBadWindow), errors.Is(err, app.ErrNotAProject):
		bad := oas.GetRolloutWindowBadRequest(oas.ProblemStatusCode{StatusCode: http.StatusBadRequest, Response: httpx.Problem(http.StatusBadRequest, "The window must end after it starts, at most two hours later.")})
		return &bad, nil
	case err != nil:
		down := oas.GetRolloutWindowServiceUnavailable(h.unavailable(ctx, err))
		return &down, nil
	}
	out := &oas.RolloutWindow{Pods: make([]oas.PodSpan, 0, len(window.Pods)), Weights: make([]oas.CanaryWeight, 0, len(window.Weights)), Runs: make([]oas.Run, 0, len(window.Runs))}
	for _, p := range window.Pods {
		out.Pods = append(out.Pods, podSpan(p))
	}
	for _, w := range window.Weights {
		out.Weights = append(out.Weights, oas.CanaryWeight{Process: w.Process, At: w.At, Weight: int32(min(max(w.Weight, 0), 100))}) //nolint:gosec // clamped to 0..100
	}
	for _, r := range window.Runs {
		run := oas.Run{Pod: r.Pod, Job: r.Job, Start: r.Start, End: optTime(r.End), Outcome: oas.RunOutcome(r.Outcome)}
		if r.CronJob != "" {
			run.CronJob = oas.NewOptString(r.CronJob)
		}
		out.Runs = append(out.Runs, run)
	}
	return out, nil
}

func podSpan(p domain.PodSpan) oas.PodSpan {
	span := oas.PodSpan{
		Pod: p.Pod, Process: p.Process, Start: optTime(p.Start), Ready: optTime(p.Ready), End: optTime(p.End),
		Checks: make([]oas.ReadinessCheck, 0, len(p.Checks)), Containers: make([]oas.ContainerSpan, 0, len(p.Containers)),
	}
	for _, c := range p.Checks {
		span.Checks = append(span.Checks, oas.ReadinessCheck{At: c.At, Ready: c.Ready})
	}
	for _, c := range p.Containers {
		container := oas.ContainerSpan{Name: c.Name, Kind: oas.ContainerSpanKind(c.Kind), Changes: make([]oas.ContainerChange, 0, len(c.Changes))}
		for _, change := range c.Changes {
			container.Changes = append(container.Changes, oas.ContainerChange{At: change.At, State: oas.ContainerChangeState(change.State)})
		}
		span.Containers = append(span.Containers, container)
	}
	return span
}

func optTime(t *time.Time) oas.OptDateTime {
	if t == nil {
		return oas.OptDateTime{}
	}
	return oas.NewOptDateTime(*t)
}

// unavailable is the 503 for Prometheus not answering, or a dashboard without it.
func (h *Health) unavailable(ctx context.Context, err error) oas.ProblemStatusCode {
	detail := "Prometheus did not answer."
	if errors.Is(err, domain.ErrNoMetrics) {
		detail = "The dashboard runs without Prometheus."
	} else {
		h.logger.ErrorContext(ctx, "read the metrics", "error", err)
	}
	return oas.ProblemStatusCode{StatusCode: http.StatusServiceUnavailable, Response: httpx.Problem(http.StatusServiceUnavailable, detail)}
}
