// Package web is the checks context's inbound HTTP adapter, mapping the monitor's checks to the
// contract's types.
package web

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/checks/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/httpx"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/oas"
)

// UseCases is what the adapter needs from the application layer.
type UseCases interface {
	Checks(ctx context.Context) ([]domain.Check, error)
}

// Checks implements the checks operations.
type Checks struct {
	uc     UseCases
	logger *slog.Logger
}

// New returns a Checks calling uc, logging to logger why the monitor did not answer.
func New(uc UseCases, logger *slog.Logger) *Checks {
	return &Checks{uc: uc, logger: logger}
}

// ListChecks implements listChecks.
func (h *Checks) ListChecks(ctx context.Context) (oas.ListChecksRes, error) {
	checks, err := h.uc.Checks(ctx)
	if err != nil {
		detail := "The uptime monitor did not answer."
		if errors.Is(err, domain.ErrNoMonitor) {
			detail = "The dashboard runs without an uptime monitor."
		} else {
			h.logger.ErrorContext(ctx, "read the checks", "error", err)
		}
		down := oas.ListChecksServiceUnavailable(oas.ProblemStatusCode{
			StatusCode: http.StatusServiceUnavailable, Response: httpx.Problem(http.StatusServiceUnavailable, detail),
		})
		return &down, nil
	}
	items := make([]oas.Check, 0, len(checks))
	for _, c := range checks {
		items = append(items, toCheck(c))
	}
	return &oas.CheckList{Items: items}, nil
}

func toCheck(c domain.Check) oas.Check {
	out := oas.Check{
		Key: c.Key, Name: c.Name, Group: c.Group, Up: c.Up,
		Conditions: make([]oas.CheckCondition, 0, len(c.Conditions)), Recent: append([]bool{}, c.Recent...),
	}
	if c.Host != "" {
		out.Host = oas.NewOptString(c.Host)
	}
	if c.Error != "" {
		out.Error = oas.NewOptString(c.Error)
	}
	if c.At != nil {
		out.At = oas.NewOptDateTime(*c.At)
		out.DurationMs = oas.NewOptInt64(c.Duration.Milliseconds())
		if c.Status > 0 {
			out.Status = oas.NewOptInt32(int32(min(c.Status, math.MaxInt32))) //nolint:gosec // bounded above
		}
	}
	for _, cond := range c.Conditions {
		out.Conditions = append(out.Conditions, oas.CheckCondition{Condition: cond.Condition, Success: cond.Success})
	}
	return out
}
