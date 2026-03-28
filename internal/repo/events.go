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
	// Sort controls ordering: "newest" (default), "oldest", "severity_desc", "severity_asc"
	Sort string
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

// EventTypeCount is a grouped count row returned by GroupByTypeAndSeverity.
type EventTypeCount struct {
	EventType string `db:"event_type"`
	Severity  string `db:"severity"`
	Count     int    `db:"count"`
}

// EventMetrics holds per-app event statistics for a time window.
type EventMetrics struct {
	EventsPerHour   int
	AvgPayloadBytes int
	PeakPerMinute   int
}

// AppEventCount is a per-app event count row returned by CountPerApp.
type AppEventCount struct {
	AppID   string `db:"app_id"`
	AppName string `db:"app_name"`
	Count   int    `db:"count"`
}

// TimeseriesBucket holds a single time bucket in a timeseries query.
type TimeseriesBucket struct {
	Time  string `db:"time"  json:"time"`
	Count int    `db:"count" json:"count"`
}

// EventRepo defines read/write operations for the events table.
type EventRepo interface {
	// Create persists a new event.
	Create(ctx context.Context, event *models.Event) error
	// List returns a page of events matching f plus the total matching count.
	List(ctx context.Context, f ListFilter) (events []models.Event, total int, err error)
	// Get returns a single event by ID, including raw_payload.
	Get(ctx context.Context, id string) (*models.Event, error)
	// Timeseries returns event counts grouped by time bucket over [since, until].
	// granularity is "hour" or "day". appID and severity may be empty to include all.
	Timeseries(ctx context.Context, since, until time.Time, granularity, appID, severity string) ([]TimeseriesBucket, error)
	// CountForCategory returns the number of events matching f.
	CountForCategory(ctx context.Context, f CategoryFilter) (int, error)
	// SparklineBuckets returns exactly 7 event counts, one per time bucket.
	// The window starts at startTime and each bucket covers bucketDur.
	SparklineBuckets(ctx context.Context, f CategoryFilter, startTime time.Time, bucketDur time.Duration) ([7]int, error)
	// LatestPerApp returns the most recent event per app, keyed by app ID.
	LatestPerApp(ctx context.Context, appIDs []string) (map[string]*models.Event, error)
	// DeleteBySeverityBefore deletes events with the given severity older than
	// before and returns the number of rows deleted.
	DeleteBySeverityBefore(ctx context.Context, severity string, before time.Time) (int64, error)
	// GroupByTypeAndSeverity returns event counts grouped by event_type (from
	// the fields JSON column) and severity for a specific app and time range.
	GroupByTypeAndSeverity(ctx context.Context, appID string, since, until time.Time) ([]EventTypeCount, error)
	// MetricsForApp computes event count, average payload size, and peak
	// per-minute rate for a single app over [since, until).
	MetricsForApp(ctx context.Context, appID string, since, until time.Time) (EventMetrics, error)
	// CountPerApp returns the event count grouped by app for the window [since, now].
	// Results are ordered by count descending.
	CountPerApp(ctx context.Context, since time.Time) ([]AppEventCount, error)
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
		VALUES (?, NULLIF(?, ''), ?, ?, ?, ?, ?)`,
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
	if f.Limit > 500 {
		f.Limit = 500
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

	// Dynamic ORDER BY based on Sort field.
	const sevOrder = `CASE e.severity WHEN 'critical' THEN 5 WHEN 'error' THEN 4 WHEN 'warn' THEN 3 WHEN 'info' THEN 2 WHEN 'debug' THEN 1 ELSE 0 END`
	orderBy := " ORDER BY e.received_at DESC"
	switch f.Sort {
	case "oldest":
		orderBy = " ORDER BY e.received_at ASC"
	case "severity_desc":
		orderBy = " ORDER BY " + sevOrder + " DESC, e.received_at DESC"
	case "severity_asc":
		orderBy = " ORDER BY " + sevOrder + " ASC, e.received_at DESC"
	}

	// Fetch the page. raw_payload is excluded from list results.
	pageQ := `
		SELECT e.id, COALESCE(e.app_id, '') AS app_id, COALESCE(a.name, '') AS app_name,
		       e.received_at, e.severity, e.display_text, e.fields
		FROM events e
		LEFT JOIN apps a ON a.id = e.app_id` +
		where + orderBy +
		` LIMIT ? OFFSET ?`

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
		SELECT e.id, COALESCE(e.app_id, '') AS app_id, COALESCE(a.name, '') AS app_name,
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

func (r *sqliteEventRepo) DeleteBySeverityBefore(ctx context.Context, severity string, before time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM events WHERE severity = ? AND datetime(received_at) < datetime(?)`,
		severity, before.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("delete events by severity: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *sqliteEventRepo) GroupByTypeAndSeverity(ctx context.Context, appID string, since, until time.Time) ([]EventTypeCount, error) {
	var rows []EventTypeCount
	err := r.db.SelectContext(ctx, &rows, `
		SELECT
			COALESCE(json_extract(fields, '$.event_type'), '') AS event_type,
			severity,
			COUNT(*) AS count
		FROM events
		WHERE app_id = ?
		  AND datetime(received_at) >= datetime(?)
		  AND datetime(received_at) < datetime(?)
		GROUP BY event_type, severity`,
		appID,
		since.UTC().Format(time.RFC3339),
		until.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("group events by type and severity: %w", err)
	}
	if rows == nil {
		rows = []EventTypeCount{}
	}
	return rows, nil
}

func (r *sqliteEventRepo) MetricsForApp(ctx context.Context, appID string, since, until time.Time) (EventMetrics, error) {
	sinceStr := since.UTC().Format(time.RFC3339)
	untilStr := until.UTC().Format(time.RFC3339)

	var count int
	var avgBytes float64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(AVG(LENGTH(raw_payload)), 0)
		FROM events
		WHERE app_id = ?
		  AND datetime(received_at) >= datetime(?)
		  AND datetime(received_at) < datetime(?)`,
		appID, sinceStr, untilStr,
	).Scan(&count, &avgBytes)
	if err != nil {
		return EventMetrics{}, fmt.Errorf("metrics count/avg: %w", err)
	}

	var peak int
	err = r.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(cnt), 0)
		FROM (
			SELECT COUNT(*) AS cnt
			FROM events
			WHERE app_id = ?
			  AND datetime(received_at) >= datetime(?)
			  AND datetime(received_at) < datetime(?)
			GROUP BY strftime('%Y-%m-%dT%H:%M', received_at)
		)`,
		appID, sinceStr, untilStr,
	).Scan(&peak)
	if err != nil {
		return EventMetrics{}, fmt.Errorf("metrics peak: %w", err)
	}

	return EventMetrics{
		EventsPerHour:   count,
		AvgPayloadBytes: int(avgBytes),
		PeakPerMinute:   peak,
	}, nil
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
		SELECT e.id, COALESCE(e.app_id, '') AS app_id, COALESCE(a.name, '') AS app_name,
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

func (r *sqliteEventRepo) Timeseries(ctx context.Context, since, until time.Time, granularity, appID, severity string) ([]TimeseriesBucket, error) {
	// fmtStr is controlled, not user-provided — safe to interpolate.
	fmtStr := "%Y-%m-%d"
	if granularity == "hour" {
		fmtStr = "%Y-%m-%dT%H:00:00Z"
	}

	var parts []string
	var args []interface{}
	parts = append(parts, "datetime(received_at) >= datetime(?)")
	args = append(args, since.UTC().Format(time.RFC3339))
	parts = append(parts, "datetime(received_at) <= datetime(?)")
	args = append(args, until.UTC().Format(time.RFC3339))
	if appID != "" {
		parts = append(parts, "app_id = ?")
		args = append(args, appID)
	}
	if severity != "" {
		parts = append(parts, "severity = ?")
		args = append(args, severity)
	}
	where := " WHERE " + strings.Join(parts, " AND ")

	q := fmt.Sprintf(
		"SELECT strftime('%s', received_at) AS time, COUNT(*) AS count FROM events%s GROUP BY strftime('%s', received_at) ORDER BY time",
		fmtStr, where, fmtStr,
	)

	var rows []TimeseriesBucket
	if err := r.db.SelectContext(ctx, &rows, q, args...); err != nil {
		return nil, fmt.Errorf("timeseries: %w", err)
	}
	if rows == nil {
		rows = []TimeseriesBucket{}
	}
	return rows, nil
}

func (r *sqliteEventRepo) CountPerApp(ctx context.Context, since time.Time) ([]AppEventCount, error) {
	var rows []AppEventCount
	err := r.db.SelectContext(ctx, &rows, `
		SELECT e.app_id, COALESCE(a.name, e.app_id) AS app_name, COUNT(*) AS count
		FROM events e
		LEFT JOIN apps a ON a.id = e.app_id
		WHERE e.app_id IS NOT NULL
		  AND datetime(e.received_at) >= datetime(?)
		GROUP BY e.app_id
		ORDER BY count DESC`,
		since.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("count per app: %w", err)
	}
	if rows == nil {
		rows = []AppEventCount{}
	}
	return rows, nil
}
