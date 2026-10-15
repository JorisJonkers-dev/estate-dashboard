// Package web is the logs context's inbound HTTP adapter. It maps the domain's reads of Loki to
// the contract's types.
package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/logs/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/httpx"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/oas"
)

// UseCases is what the adapter needs from the application layer.
type UseCases interface {
	Streams(ctx context.Context, project string, since time.Duration) ([]domain.Stream, error)
	Lines(ctx context.Context, project, pod, container string, since time.Duration, limit int) ([]domain.Line, error)
}

// Logs implements the logs operations.
type Logs struct {
	uc     UseCases
	logger *slog.Logger
}

// New returns a Logs calling uc, logging to logger why Loki did not answer.
func New(uc UseCases, logger *slog.Logger) *Logs {
	return &Logs{uc: uc, logger: logger}
}

// unavailable is the 503 for Loki not answering, or a dashboard without it.
func (h *Logs) unavailable(ctx context.Context, err error) oas.ProblemStatusCode {
	detail := "Loki did not answer."
	if errors.Is(err, domain.ErrNoStore) {
		detail = "The dashboard runs without Loki."
	} else {
		h.logger.ErrorContext(ctx, "read the logs", "error", err)
	}
	return oas.ProblemStatusCode{StatusCode: http.StatusServiceUnavailable, Response: httpx.Problem(http.StatusServiceUnavailable, detail)}
}

func hours(o oas.OptInt32) time.Duration { return time.Duration(o.Or(0)) * time.Hour }

// ListLogStreams implements listLogStreams.
func (h *Logs) ListLogStreams(ctx context.Context, params oas.ListLogStreamsParams) (oas.ListLogStreamsRes, error) {
	streams, err := h.uc.Streams(ctx, params.Project, hours(params.Hours))
	switch {
	case errors.Is(err, domain.ErrNotAName):
		return &oas.StreamList{Items: []oas.Stream{}}, nil
	case err != nil:
		down := oas.ListLogStreamsServiceUnavailable(h.unavailable(ctx, err))
		return &down, nil
	}
	items := make([]oas.Stream, 0, len(streams))
	for _, s := range streams {
		items = append(items, oas.Stream{Pod: s.Pod, Container: s.Container})
	}
	return &oas.StreamList{Items: items}, nil
}

// ListLogLines implements listLogLines.
func (h *Logs) ListLogLines(ctx context.Context, params oas.ListLogLinesParams) (oas.ListLogLinesRes, error) {
	lines, err := h.uc.Lines(ctx, params.Project, params.Pod, params.Container, hours(params.Hours), int(params.Limit.Or(0)))
	switch {
	case errors.Is(err, domain.ErrNotAName):
		return &oas.LogLines{Items: []oas.LogLine{}}, nil
	case err != nil:
		down := oas.ListLogLinesServiceUnavailable(h.unavailable(ctx, err))
		return &down, nil
	}
	items := make([]oas.LogLine, 0, len(lines))
	for _, l := range lines {
		items = append(items, oas.LogLine{At: l.At, Text: l.Text})
	}
	return &oas.LogLines{Items: items}, nil
}
