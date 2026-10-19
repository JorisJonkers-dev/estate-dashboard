// Package app holds the checks context's use cases.
package app

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/checks/domain"
)

// Service reads the uptime monitor.
type Service struct {
	monitor domain.Monitor
}

// New returns a Service over monitor.
func New(monitor domain.Monitor) *Service {
	return &Service{monitor: monitor}
}

// Checks returns every probed endpoint, by group then name.
func (s *Service) Checks(ctx context.Context) ([]domain.Check, error) {
	checks, err := s.monitor.Checks(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the checks: %w", err)
	}
	slices.SortFunc(checks, func(a, b domain.Check) int {
		if c := strings.Compare(a.Group, b.Group); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	return checks, nil
}
