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
		detail := "Prometheus did not answer."
		if errors.Is(err, domain.ErrNoMetrics) {
			detail = "The dashboard runs without Prometheus."
		} else {
			h.logger.ErrorContext(ctx, "read the metrics", "error", err)
		}
		down := oas.GetHealthServiceUnavailable(oas.ProblemStatusCode{StatusCode: http.StatusServiceUnavailable, Response: httpx.Problem(http.StatusServiceUnavailable, detail)})
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
