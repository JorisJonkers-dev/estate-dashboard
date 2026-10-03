// Package app holds the alerts context's use cases: the only entry points the inbound adapters call.
package app

import (
	"context"
	"fmt"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/domain"
)

const (
	// DefaultLimit is how many events History returns when the caller names no limit.
	DefaultLimit = 50
	// MaxLimit is the most events one History returns.
	MaxLimit = 100
)

// Service runs the alerts use cases against a repository.
type Service struct {
	repo domain.Repository
}

// New returns a Service backed by repo.
func New(repo domain.Repository) *Service {
	return &Service{repo: repo}
}

// History returns up to limit events, newest first. A limit below 1 means DefaultLimit.
func (s *Service) History(ctx context.Context, limit int) ([]domain.Event, error) {
	switch {
	case limit < 1:
		limit = DefaultLimit
	case limit > MaxLimit:
		limit = MaxLimit
	}
	events, err := s.repo.History(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("read the alert history: %w", err)
	}
	return events, nil
}
