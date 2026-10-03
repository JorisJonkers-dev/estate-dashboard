// Package web is the alerts context's inbound HTTP adapter: it implements the alerts operations
// of the generated ogen Handler and maps between the contract's types and the domain's.
package web

import (
	"context"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/oas"
)

// UseCases is what the adapter needs from the application layer.
type UseCases interface {
	History(ctx context.Context, limit int) ([]domain.Event, error)
}

// Handler implements the alerts operations. Business rules stay in the use cases.
type Handler struct {
	uc UseCases
}

// New returns a Handler calling uc.
func New(uc UseCases) *Handler {
	return &Handler{uc: uc}
}

// ListAlertHistory implements listAlertHistory.
func (h *Handler) ListAlertHistory(ctx context.Context, params oas.ListAlertHistoryParams) (oas.ListAlertHistoryRes, error) {
	events, err := h.uc.History(ctx, int(params.Limit.Or(0)))
	if err != nil {
		return nil, err
	}
	items := make([]oas.AlertEvent, 0, len(events))
	for _, e := range events {
		items = append(items, toAlertEvent(e))
	}
	return &oas.AlertHistory{Items: items}, nil
}

func toAlertEvent(e domain.Event) oas.AlertEvent {
	out := oas.AlertEvent{
		ID:          e.ID,
		Fingerprint: e.Fingerprint,
		Name:        e.Name,
		Status:      oas.AlertEventStatus(e.Status),
		StartsAt:    e.StartsAt,
		ObservedAt:  e.ObservedAt,
	}
	if e.EndsAt != nil {
		out.EndsAt = oas.NewOptDateTime(*e.EndsAt)
	}
	return out
}
