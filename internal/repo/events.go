package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// ListFilter constrains an event list query. Zero values mean "no filter".
type ListFilter struct {
	AppID    string
	Severity []string
	Since    *time.Time
	Until    *time.Time
	Limit    int
	Offset   int
}

// EventRepo defines read operations for the events table.
type EventRepo interface {
	// List returns a page of events matching f plus the total matching count.
	List(ctx context.Context, f ListFilter) (events []models.Event, total int, err error)
	// Get returns a single event by ID, including raw_payload.
	Get(ctx context.Context, id string) (*models.Event, error)
}

type sqliteEventRepo struct {
	db *sqlx.DB
}

// NewEventRepo returns an EventRepo backed by the given SQLite database.
func NewEventRepo(db *sqlx.DB) EventRepo {
	return &sqliteEventRepo{db: db}
}

// buildWhere constructs the WHERE clause and argument slice from a ListFilter.
// Time comparisons use datetime() so sub-second precision in stored values is handled correctly.
func buildWhere(f ListFilter) (clause string, args []interface{}) {
	var parts []string

	if f.AppID != "" {
		parts = append(parts, "e.app_id = ?")
		args = append(args, f.AppID)
	}

	if len(f.Severity) > 0 {
		ph := strings.TrimRight(strings.Repeat("?,", len(f.Severity)), ",")
		parts = append(parts, "e.severity IN ("+ph+")")
		for _, s := range f.Severity {
			args = append(args, s)
		}
	}

	if f.Since != nil {
		parts = append(parts, "datetime(e.received_at) >= datetime(?)")
		args = append(args, f.Since.UTC().Format(time.RFC3339))
	}

	if f.Until != nil {
		parts = append(parts, "datetime(e.received_at) <= datetime(?)")
		args = append(args, f.Until.UTC().Format(time.RFC3339))
	}

	if len(parts) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

func (r *sqliteEventRepo) List(ctx context.Context, f ListFilter) ([]models.Event, int, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 200 {
		f.Limit = 200
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	where, whereArgs := buildWhere(f)

	// Total matching rows (no LIMIT/OFFSET).
	countQ := "SELECT COUNT(*) FROM events e" + where
	var total int
	if err := r.db.GetContext(ctx, &total, countQ, whereArgs...); err != nil {
		return nil, 0, fmt.Errorf("count events: %w", err)
	}

	// Fetch the page. raw_payload is excluded from list results.
	pageQ := `
		SELECT e.id, e.app_id, COALESCE(a.name, '') AS app_name,
		       e.received_at, e.severity, e.display_text, e.fields
		FROM events e
		LEFT JOIN apps a ON a.id = e.app_id` +
		where +
		` ORDER BY e.received_at DESC
		 LIMIT ? OFFSET ?`

	pageArgs := append(whereArgs, f.Limit, f.Offset)
	var events []models.Event
	if err := r.db.SelectContext(ctx, &events, pageQ, pageArgs...); err != nil {
		return nil, 0, fmt.Errorf("list events: %w", err)
	}
	if events == nil {
		events = []models.Event{}
	}
	return events, total, nil
}

func (r *sqliteEventRepo) Get(ctx context.Context, id string) (*models.Event, error) {
	const q = `
		SELECT e.id, e.app_id, COALESCE(a.name, '') AS app_name,
		       e.received_at, e.severity, e.display_text, e.raw_payload, e.fields
		FROM events e
		LEFT JOIN apps a ON a.id = e.app_id
		WHERE e.id = ?`

	var ev models.Event
	if err := r.db.GetContext(ctx, &ev, q, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get event: %w", err)
	}
	return &ev, nil
}
