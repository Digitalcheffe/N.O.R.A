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

// CategoryFilter defines criteria for matching events to a digest category.
// Empty strings are ignored (not applied to the query).
type CategoryFilter struct {
	AppIDs        []string  // empty = all apps
	MatchField    string    // json_extract(fields, '$.{MatchField}') = MatchValue
	MatchValue    string
	MatchSeverity string    // severity = MatchSeverity
	Since         time.Time // inclusive lower bound
	Until         time.Time // inclusive upper bound
}

// EventRepo defines read/write operations for the events table.
type EventRepo interface {
	// Create persists a new event.
	Create(ctx context.Context, event *models.Event) error
	// List returns a page of events matching f plus the total matching count.
	List(ctx context.Context, f ListFilter) (events []models.Event, total int, err error)
	// Get returns a single event by ID, including raw_payload.
	Get(ctx context.Context, id string) (*models.Event, error)
	// CountForCategory returns the number of events matching f.
	CountForCategory(ctx context.Context, f CategoryFilter) (int, error)
	// SparklineBuckets returns exactly 7 event counts, one per time bucket.
	// The window starts at startTime and each bucket covers bucketDur.
	SparklineBuckets(ctx context.Context, f CategoryFilter, startTime time.Time, bucketDur time.Duration) ([7]int, error)
	// LatestPerApp returns the most recent event per app, keyed by app ID.
	LatestPerApp(ctx context.Context, appIDs []string) (map[string]*models.Event, error)
}

type sqliteEventRepo struct {
	db *sqlx.DB
}

// NewEventRepo returns an EventRepo backed by the given SQLite database.
func NewEventRepo(db *sqlx.DB) EventRepo {
	return &sqliteEventRepo{db: db}
}

func (r *sqliteEventRepo) Create(ctx context.Context, event *models.Event) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO events (id, app_id, received_at, severity, display_text, raw_payload, fields)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.AppID, event.ReceivedAt, event.Severity,
		event.DisplayText, event.RawPayload, event.Fields)
	if err != nil {
		return fmt.Errorf("create event: %w", err)
	}
	return nil
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

// buildCategoryWhere builds the WHERE clause for a CategoryFilter.
func buildCategoryWhere(f CategoryFilter) (string, []interface{}) {
	var parts []string
	var args []interface{}

	if len(f.AppIDs) > 0 {
		ph := strings.TrimRight(strings.Repeat("?,", len(f.AppIDs)), ",")
		parts = append(parts, "app_id IN ("+ph+")")
		for _, id := range f.AppIDs {
			args = append(args, id)
		}
	}

	if f.MatchField != "" && f.MatchValue != "" {
		parts = append(parts, "json_extract(fields, '$."+f.MatchField+"') = ?")
		args = append(args, f.MatchValue)
	}

	if f.MatchSeverity != "" {
		parts = append(parts, "severity = ?")
		args = append(args, f.MatchSeverity)
	}

	if !f.Since.IsZero() {
		parts = append(parts, "datetime(received_at) >= datetime(?)")
		args = append(args, f.Since.UTC().Format(time.RFC3339))
	}

	if !f.Until.IsZero() {
		parts = append(parts, "datetime(received_at) <= datetime(?)")
		args = append(args, f.Until.UTC().Format(time.RFC3339))
	}

	if len(parts) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

func (r *sqliteEventRepo) CountForCategory(ctx context.Context, f CategoryFilter) (int, error) {
	where, args := buildCategoryWhere(f)
	q := "SELECT COUNT(*) FROM events" + where
	var count int
	if err := r.db.GetContext(ctx, &count, q, args...); err != nil {
		return 0, fmt.Errorf("count for category: %w", err)
	}
	return count, nil
}

func (r *sqliteEventRepo) SparklineBuckets(ctx context.Context, f CategoryFilter, startTime time.Time, bucketDur time.Duration) ([7]int, error) {
	var counts [7]int
	for i := 0; i < 7; i++ {
		bucketStart := startTime.Add(time.Duration(i) * bucketDur)
		bucketEnd := bucketStart.Add(bucketDur)
		bf := f
		bf.Since = bucketStart
		bf.Until = bucketEnd
		n, err := r.CountForCategory(ctx, bf)
		if err != nil {
			return counts, err
		}
		counts[i] = n
	}
	return counts, nil
}

func (r *sqliteEventRepo) LatestPerApp(ctx context.Context, appIDs []string) (map[string]*models.Event, error) {
	if len(appIDs) == 0 {
		return map[string]*models.Event{}, nil
	}

	ph := strings.TrimRight(strings.Repeat("?,", len(appIDs)), ",")
	args := make([]interface{}, len(appIDs))
	for i, id := range appIDs {
		args[i] = id
	}

	q := `
		SELECT e.id, e.app_id, COALESCE(a.name, '') AS app_name,
		       e.received_at, e.severity, e.display_text, e.fields
		FROM events e
		LEFT JOIN apps a ON a.id = e.app_id
		WHERE e.app_id IN (` + ph + `)
		  AND e.received_at = (
		        SELECT MAX(e2.received_at) FROM events e2 WHERE e2.app_id = e.app_id
		      )`

	var events []models.Event
	if err := r.db.SelectContext(ctx, &events, q, args...); err != nil {
		return nil, fmt.Errorf("latest per app: %w", err)
	}

	result := make(map[string]*models.Event, len(events))
	for i := range events {
		result[events[i].AppID] = &events[i]
	}
	return result, nil
}
