package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/httpx"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/oas"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/oidc"
)

// LiveUseCases is what the adapter needs to read and write Alertmanager.
type LiveUseCases interface {
	Alerts(ctx context.Context) ([]domain.Alert, error)
	Silence(ctx context.Context, by, fingerprint string, length domain.Length) (domain.Silence, error)
}

// Live implements the operations that reach Alertmanager.
type Live struct {
	uc     LiveUseCases
	logger *slog.Logger
}

// NewLive returns the handler over uc, logging what made Alertmanager unreachable to logger.
func NewLive(uc LiveUseCases, logger *slog.Logger) *Live {
	return &Live{uc: uc, logger: logger}
}

// unavailable is the problem for an Alertmanager that did not answer: its cause is the log's.
func unavailable() oas.ProblemStatusCode {
	return oas.ProblemStatusCode{StatusCode: http.StatusServiceUnavailable, Response: httpx.Problem(http.StatusServiceUnavailable, "Alertmanager did not answer.")}
}

// ListAlerts implements listAlerts.
func (h *Live) ListAlerts(ctx context.Context) (oas.ListAlertsRes, error) {
	alerts, err := h.uc.Alerts(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "list alerts", "error", err)
		down := oas.ListAlertsServiceUnavailable(unavailable())
		return &down, nil //nolint:nilerr // the cause is logged; the contract's 503 is the answer
	}
	items := make([]oas.Alert, 0, len(alerts))
	for _, a := range alerts {
		items = append(items, toAlert(a))
	}
	return &oas.AlertList{Items: items}, nil
}

func toAlert(a domain.Alert) oas.Alert {
	out := oas.Alert{
		Fingerprint: a.Fingerprint,
		Name:        a.Name,
		StartsAt:    a.StartsAt,
		Labels:      oas.AlertLabels(a.Labels),
		SilencedBy:  append([]string{}, a.SilencedBy...),
	}
	if a.Class != domain.Unclassed {
		out.Class = oas.NewOptAlertClass(oas.AlertClass(a.Class))
	}
	if a.Summary != "" {
		out.Summary = oas.NewOptString(a.Summary)
	}
	return out
}

var errNoAdmin = errors.New("alerts: a silence reached the handler without an admin")

// SilenceAlert implements silenceAlert: as the signed-in admin, by name.
func (h *Live) SilenceAlert(ctx context.Context, req *oas.SilenceRequest, params oas.SilenceAlertParams) (oas.SilenceAlertRes, error) {
	who, ok := oidc.AdminFrom(ctx)
	if !ok {
		return nil, errNoAdmin
	}
	silence, err := h.uc.Silence(ctx, who.Name, params.Fingerprint, domain.Length(req.Length))
	switch {
	case errors.Is(err, domain.ErrNotFiring):
		missing := oas.SilenceAlertNotFound{StatusCode: http.StatusNotFound, Response: httpx.Problem(http.StatusNotFound, "The alert is not firing.")}
		return &missing, nil
	case errors.Is(err, domain.ErrUnknownLength):
		bad := oas.SilenceAlertBadRequest{StatusCode: http.StatusBadRequest, Response: httpx.Problem(http.StatusBadRequest, "A silence lasts 1h, 4h, 1d or until-resolved.")}
		return &bad, nil
	case err != nil:
		h.logger.ErrorContext(ctx, "silence an alert", "fingerprint", params.Fingerprint, "error", err)
		down := oas.SilenceAlertServiceUnavailable(unavailable())
		return &down, nil //nolint:nilerr // the cause is logged; the contract's 503 is the answer
	}
	out := &oas.Silence{ID: silence.ID, Fingerprint: silence.Fingerprint, CreatedBy: silence.CreatedBy, CreatedAt: silence.CreatedAt}
	if silence.EndsAt != nil {
		out.EndsAt = oas.NewOptDateTime(*silence.EndsAt)
	}
	return out, nil
}
