package repo

import (
	"context"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// EventFilter constrains event list queries.
type EventFilter struct {
	AppID    string
	Severity string
	From     *time.Time
	To       *time.Time
	Limit    int
	Offset   int
}

// EventRepository defines data access for events.
type EventRepository interface {
	Create(ctx context.Context, event *models.Event) error
	GetByID(ctx context.Context, id string) (*models.Event, error)
	List(ctx context.Context, f EventFilter) ([]*models.Event, error)
	CountBySeverity(ctx context.Context, appID string) (map[string]int, error)
	DeleteOlderThan(ctx context.Context, cutoff time.Time) error
}

type sqliteEventRepo struct{ db *sqlx.DB }

func (r *sqliteEventRepo) Create(ctx context.Context, event *models.Event) error {
	event.ID = uuid.NewString()
	event.ReceivedAt = time.Now().UTC()
	if event.RawPayload == nil {
		event.RawPayload = models.RawMessage("{}")
	}
	if event.Fields == nil {
		event.Fields = models.RawMessage("{}")
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO events (id, app_id, received_at, severity, display_text, raw_payload, fields)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.AppID, event.ReceivedAt, event.Severity,
		event.DisplayText, event.RawPayload, event.Fields,
	)
	return err
}

func (r *sqliteEventRepo) GetByID(ctx context.Context, id string) (*models.Event, error) {
	var e models.Event
	err := r.db.GetContext(ctx, &e, `SELECT * FROM events WHERE id = ?`, id)
	return &e, err
}

func (r *sqliteEventRepo) List(ctx context.Context, f EventFilter) ([]*models.Event, error) {
	q := `SELECT * FROM events WHERE 1=1`
	args := []interface{}{}

	if f.AppID != "" {
		q += ` AND app_id = ?`
		args = append(args, f.AppID)
	}
	if f.Severity != "" {
		q += ` AND severity = ?`
		args = append(args, f.Severity)
	}
	if f.From != nil {
		q += ` AND received_at >= ?`
		args = append(args, f.From.UTC())
	}
	if f.To != nil {
		q += ` AND received_at <= ?`
		args = append(args, f.To.UTC())
	}
	q += ` ORDER BY received_at DESC`
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
		if f.Offset > 0 {
			q += ` OFFSET ?`
			args = append(args, f.Offset)
		}
	}

	var events []*models.Event
	err := r.db.SelectContext(ctx, &events, q, args...)
	return events, err
}

func (r *sqliteEventRepo) CountBySeverity(ctx context.Context, appID string) (map[string]int, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT severity, COUNT(*) FROM events WHERE app_id = ? GROUP BY severity`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var sev string
		var n int
		if err := rows.Scan(&sev, &n); err != nil {
			return nil, err
		}
		counts[sev] = n
	}
	return counts, rows.Err()
}

func (r *sqliteEventRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM events WHERE received_at < ?`, cutoff.UTC())
	return err
}
