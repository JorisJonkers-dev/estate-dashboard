package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/pg/queries"
)

// Silences implements the silences mirror port on the sqlc-generated queries.
type Silences struct {
	q queries.Querier
}

var _ domain.Silences = (*Silences)(nil)

// NewSilences returns the mirror over q.
func NewSilences(q queries.Querier) *Silences {
	return &Silences{q: q}
}

// Record mirrors a silence the dashboard created.
func (s *Silences) Record(ctx context.Context, silence domain.Silence) error {
	params := queries.RecordSilenceParams{
		ID:          silence.ID,
		Fingerprint: silence.Fingerprint,
		CreatedBy:   silence.CreatedBy,
		CreatedAt:   silence.CreatedAt,
	}
	if silence.EndsAt != nil {
		params.EndsAt = pgtype.Timestamptz{Time: *silence.EndsAt, Valid: true}
	}
	if err := s.q.RecordSilence(ctx, params); err != nil {
		return fmt.Errorf("insert silence %s: %w", silence.ID, err)
	}
	return nil
}

// UntilResolved returns the open silences of fingerprint that last until it resolves.
func (s *Silences) UntilResolved(ctx context.Context, fingerprint string) ([]domain.Silence, error) {
	rows, err := s.q.ListUntilResolved(ctx, fingerprint)
	if err != nil {
		return nil, fmt.Errorf("select the silences of %s: %w", fingerprint, err)
	}
	out := make([]domain.Silence, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Silence{
			ID:          row.ID,
			Fingerprint: row.Fingerprint,
			CreatedBy:   row.CreatedBy,
			CreatedAt:   row.CreatedAt.UTC(),
		})
	}
	return out, nil
}

// Expired records that a silence no longer holds.
func (s *Silences) Expired(ctx context.Context, id string, at time.Time) error {
	if err := s.q.ExpireSilence(ctx, queries.ExpireSilenceParams{ID: id, ExpiredAt: pgtype.Timestamptz{Time: at, Valid: true}}); err != nil {
		return fmt.Errorf("expire silence %s: %w", id, err)
	}
	return nil
}
