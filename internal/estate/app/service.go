// Package app holds the estate context's use cases.
package app

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/estate/domain"
)

const (
	// DefaultDeploys is how many deploys a Project's log returns when the caller names no limit.
	DefaultDeploys = 20
	// MaxDeploys is the most deploys one read returns.
	MaxDeploys = 100
)

// Service reads the Estate repository.
type Service struct {
	repo domain.Repository
}

// New returns a Service over repo.
func New(repo domain.Repository) *Service {
	return &Service{repo: repo}
}

// Pins returns every Project's pin, by Project.
func (s *Service) Pins(ctx context.Context) ([]domain.Pin, error) {
	pins, err := s.repo.Pins(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the pins: %w", err)
	}
	slices.SortFunc(pins, func(a, b domain.Pin) int { return strings.Compare(a.Project, b.Project) })
	return pins, nil
}

// Handover returns each Project's delivery path, with both lists sorted.
func (s *Service) Handover(ctx context.Context) (domain.Handover, error) {
	ledger, err := s.repo.Handover(ctx)
	if err != nil {
		return domain.Handover{}, fmt.Errorf("read the handover ledger: %w", err)
	}
	slices.Sort(ledger.Estate)
	slices.Sort(ledger.Legacy)
	return ledger, nil
}

// Deploys returns up to limit commits that moved a Project's pin, newest first. A limit below 1
// means DefaultDeploys.
func (s *Service) Deploys(ctx context.Context, project string, limit int) ([]domain.Deploy, error) {
	switch {
	case limit < 1:
		limit = DefaultDeploys
	case limit > MaxDeploys:
		limit = MaxDeploys
	}
	deploys, err := s.repo.Deploys(ctx, project, limit)
	if err != nil {
		return nil, fmt.Errorf("read the deploys of %s: %w", project, err)
	}
	return deploys, nil
}

// DefaultRollouts is how many rollouts come back when the caller gives no limit.
const DefaultRollouts = 10

// Rollouts returns up to limit of a Project's rollouts in repository, newest first. A limit
// below 1 means DefaultRollouts. A limit above MaxDeploys is capped at MaxDeploys.
func (s *Service) Rollouts(ctx context.Context, repository, project string, limit int) ([]domain.Rollout, error) {
	switch {
	case limit < 1:
		limit = DefaultRollouts
	case limit > MaxDeploys:
		limit = MaxDeploys
	}
	rollouts, err := s.repo.Rollouts(ctx, repository, project, limit)
	if err != nil {
		return nil, fmt.Errorf("read the rollouts of %s: %w", project, err)
	}
	return rollouts, nil
}

// Issues returns the open issues, the most recently updated first.
func (s *Service) Issues(ctx context.Context) ([]domain.Issue, error) {
	issues, err := s.repo.Issues(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the issues: %w", err)
	}
	slices.SortStableFunc(issues, func(a, b domain.Issue) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return issues, nil
}
