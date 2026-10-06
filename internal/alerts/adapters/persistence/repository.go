// Package persistence implements the alerts Repository port on the sqlc-generated queries.
package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/pg/queries"
)

// Repository maps rows to domain events, so the domain never sees a generated type.
type Repository struct {
	q queries.Querier
}

var _ domain.Repository = (*Repository)(nil)

// New returns a Repository over q.
func New(q queries.Querier) *Repository {
	return &Repository{q: q}
}

// History returns at most limit events, newest first.
func (r *Repository) History(ctx context.Context, limit int) ([]domain.Event, error) {
	if limit < 0 || limit > math.MaxInt32 {
		return nil, fmt.Errorf("select alert history: limit %d out of range", limit)
	}
	rows, err := r.q.ListAlertHistory(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("select alert history: %w", err)
	}
	events := make([]domain.Event, 0, len(rows))
	for _, row := range rows {
		event, err := toDomain(row)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

// Record keeps one event; the same state seen twice is kept once.
func (r *Repository) Record(ctx context.Context, e domain.Event) error {
	checked, err := domain.NewEvent(e)
	if err != nil {
		return err
	}
	labels := checked.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	encoded, err := json.Marshal(labels)
	if err != nil {
		return fmt.Errorf("encode the labels of alert %s: %w", checked.Fingerprint, err)
	}
	params := queries.RecordAlertEventParams{
		Fingerprint: checked.Fingerprint,
		Name:        checked.Name,
		Status:      string(checked.Status),
		Labels:      encoded,
		StartsAt:    checked.StartsAt,
		ObservedAt:  checked.ObservedAt,
	}
	if checked.EndsAt != nil {
		params.EndsAt = pgtype.Timestamptz{Time: *checked.EndsAt, Valid: true}
	}
	if err := r.q.RecordAlertEvent(ctx, params); err != nil {
		return fmt.Errorf("insert alert event %s: %w", checked.Fingerprint, err)
	}
	return nil
}

// Open returns every firing with no resolution recorded.
func (r *Repository) Open(ctx context.Context) ([]domain.Event, error) {
	rows, err := r.q.ListOpenFirings(ctx)
	if err != nil {
		return nil, fmt.Errorf("select open firings: %w", err)
	}
	events := make([]domain.Event, 0, len(rows))
	for _, row := range rows {
		event, err := toDomain(queries.ListAlertHistoryRow(row))
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

// toDomain re-checks the row against the domain's rules (the CHECKs hold them too), and hands out
// times in UTC whatever the connection's time zone.
func toDomain(row queries.ListAlertHistoryRow) (domain.Event, error) {
	event := domain.Event{
		ID:          row.ID,
		Fingerprint: row.Fingerprint,
		Name:        row.Name,
		Status:      domain.Status(row.Status),
		StartsAt:    row.StartsAt.UTC(),
		ObservedAt:  row.ObservedAt.UTC(),
	}
	if row.EndsAt.Valid {
		ends := row.EndsAt.Time.UTC()
		event.EndsAt = &ends
	}
	checked, err := domain.NewEvent(event)
	if err != nil {
		return domain.Event{}, fmt.Errorf("alert history row %d: %w", row.ID, err)
	}
	return checked, nil
}
