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
	// SourceID filters by the source_id column (app ID, check ID, component ID, etc.).
	SourceID string
	// SourceType filters by the source_type column: "app", "physical_host",
	// "virtual_host", "docker_engine", "monitor_check", "system".
	SourceType string
	// SourceTypes filters by multiple source_type values (IN clause). Takes
	// precedence over SourceType when non-empty.
	SourceTypes []string
	// SourceName filters events whose source_name contains this substring (case-insensitive).
	SourceName string
	// Search filters events whose title contains the given substring (case-insensitive).
	Search string
	// Level filters by one or more level values: debug, info, warn, error, critical.
	Level  []string
	Since  *time.Time
	Until  *time.Time
	Limit  int
	Offset int
	// Sort controls ordering: "newest" (default), "oldest", "level_desc", "level_asc"
	Sort string
}

// CategoryFilter defines criteria for matching events to a digest category.
// Empty strings are ignored (not applied to the query).
type CategoryFilter struct {
	SourceIDs  []string  // empty = all sources
	MatchField string    // json_extract(payload, '$.{MatchField}') = MatchValue
	MatchValue string
	MatchLevel string    // level = MatchLevel
	Since      time.Time // inclusive lower bound
	Until      time.Time // inclusive upper bound
}

// EventTypeCount is a grouped count row returned by GroupByTypeAndLevel.
type EventTypeCount struct {
	EventType string `db:"event_type"`
	Level     string `db:"level"`
	Count     int    `db:"count"`
}

// EventMetrics holds per-source event statistics for a time window.
type EventMetrics struct {
	EventsPerHour   int
	AvgPayloadBytes int
	PeakPerMinute   int
}

// AppEventCount is a per-app event count row returned by CountPerApp.
type AppEventCount struct {
	AppID   string `db:"source_id"`
	AppName string `db:"source_name"`
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
	// Get returns a single event by ID, including payload.
	Get(ctx context.Context, id string) (*models.Event, error)
	// Timeseries returns event counts grouped by time bucket over [since, until].
	// granularity is "hour" or "day". sourceID and level may be empty to include all.
	Timeseries(ctx context.Context, since, until time.Time, granularity, sourceID, level string) ([]TimeseriesBucket, error)
	// CountForCategory returns the number of events matching f.
	CountForCategory(ctx context.Context, f CategoryFilter) (int, error)
	// SparklineBuckets returns exactly 7 event counts, one per time bucket.
	// The window starts at startTime and each bucket covers bucketDur.
	SparklineBuckets(ctx context.Context, f CategoryFilter, startTime time.Time, bucketDur time.Duration) ([7]int, error)
	// LatestPerApp returns the most recent event per app source, keyed by source_id.
	LatestPerApp(ctx context.Context, appIDs []string) (map[string]*models.Event, error)
	// DeleteByLevelBefore deletes events with the given level older than before
	// and returns the number of rows deleted.
	DeleteByLevelBefore(ctx context.Context, level string, before time.Time) (int64, error)
	// GroupByTypeAndLevel returns event counts grouped by event_type (from the
	// payload JSON column) and level for a specific source and time range.
	GroupByTypeAndLevel(ctx context.Context, sourceID string, since, until time.Time) ([]EventTypeCount, error)
	// MetricsForApp computes event count, average payload size, and peak
	// per-minute rate for a single app source over [since, until).
	MetricsForApp(ctx context.Context, appID string, since, until time.Time) (EventMetrics, error)
	// CountPerApp returns the event count grouped by app source for the window [since, now].
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
		INSERT INTO events (id, level, source_name, source_type, source_id, title, payload, created_at)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, NULLIF(?, ''), ?)`,
		event.ID, event.Level, event.SourceName, event.SourceType,
		event.SourceID, event.Title, event.Payload, event.CreatedAt)
	if err != nil {
		return fmt.Errorf("create event: %w", err)
	}
	return nil
}

// buildWhere constructs the WHERE clause and argument slice from a ListFilter.
// Time comparisons use datetime() so sub-second precision in stored values is handled correctly.
func buildWhere(f ListFilter) (clause string, args []interface{}) {
	var parts []string

	if f.SourceID != "" {
		parts = append(parts, "e.source_id = ?")
		args = append(args, f.SourceID)
	}

	if len(f.SourceTypes) > 0 {
		placeholders := strings.Repeat("?,", len(f.SourceTypes))
		placeholders = placeholders[:len(placeholders)-1]
		parts = append(parts, "e.source_type IN ("+placeholders+")")
		for _, st := range f.SourceTypes {
			args = append(args, st)
		}
	} else if f.SourceType != "" {
		parts = append(parts, "e.source_type = ?")
		args = append(args, f.SourceType)
	}

	if f.SourceName != "" {
		parts = append(parts, "e.source_name LIKE ?")
		args = append(args, "%"+f.SourceName+"%")
	}

	if f.Search != "" {
		parts = append(parts, "e.title LIKE ?")
		args = append(args, "%"+f.Search+"%")
	}

	if len(f.Level) > 0 {
		ph := strings.TrimRight(strings.Repeat("?,", len(f.Level)), ",")
		parts = append(parts, "e.level IN ("+ph+")")
		for _, l := range f.Level {
			args = append(args, l)
		}
	}

	if f.Since != nil {
		parts = append(parts, "datetime(e.created_at) >= datetime(?)")
		args = append(args, f.Since.UTC().Format(time.RFC3339))
	}

	if f.Until != nil {
		parts = append(parts, "datetime(e.created_at) <= datetime(?)")
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
	const levOrder = `CASE e.level WHEN 'critical' THEN 5 WHEN 'error' THEN 4 WHEN 'warn' THEN 3 WHEN 'info' THEN 2 WHEN 'debug' THEN 1 ELSE 0 END`
	orderBy := " ORDER BY e.created_at DESC"
	switch f.Sort {
	case "oldest":
		orderBy = " ORDER BY e.created_at ASC"
	case "level_desc":
		orderBy = " ORDER BY " + levOrder + " DESC, e.created_at DESC"
	case "level_asc":
		orderBy = " ORDER BY " + levOrder + " ASC, e.created_at DESC"
	}

	// Fetch the page. payload is excluded from list results.
	pageQ := `
		SELECT e.id, e.level, e.source_name, e.source_type,
		       COALESCE(e.source_id, '') AS source_id, e.title,
		       '' AS payload, e.created_at
		FROM events e` +
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
		SELECT e.id, e.level, e.source_name, e.source_type,
		       COALESCE(e.source_id, '') AS source_id, e.title,
		       COALESCE(e.payload, '') AS payload, e.created_at
		FROM events e
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

	if len(f.SourceIDs) > 0 {
		ph := strings.TrimRight(strings.Repeat("?,", len(f.SourceIDs)), ",")
		parts = append(parts, "source_id IN ("+ph+")")
		for _, id := range f.SourceIDs {
			args = append(args, id)
		}
	}

	if f.MatchField != "" && f.MatchValue != "" {
		parts = append(parts, "json_extract(payload, '$."+f.MatchField+"') = ?")
		args = append(args, f.MatchValue)
	}

	if f.MatchLevel != "" {
		parts = append(parts, "level = ?")
		args = append(args, f.MatchLevel)
	}

	if !f.Since.IsZero() {
		parts = append(parts, "datetime(created_at) >= datetime(?)")
		args = append(args, f.Since.UTC().Format(time.RFC3339))
	}

	if !f.Until.IsZero() {
		parts = append(parts, "datetime(created_at) <= datetime(?)")
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

func (r *sqliteEventRepo) DeleteByLevelBefore(ctx context.Context, level string, before time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM events WHERE level = ? AND datetime(created_at) < datetime(?)`,
		level, before.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("delete events by level: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *sqliteEventRepo) GroupByTypeAndLevel(ctx context.Context, sourceID string, since, until time.Time) ([]EventTypeCount, error) {
	var rows []EventTypeCount
	err := r.db.SelectContext(ctx, &rows, `
		SELECT
			COALESCE(json_extract(payload, '$.event_type'), '') AS event_type,
			level,
			COUNT(*) AS count
		FROM events
		WHERE source_id = ?
		  AND source_type = 'app'
		  AND datetime(created_at) >= datetime(?)
		  AND datetime(created_at) < datetime(?)
		GROUP BY event_type, level`,
		sourceID,
		since.UTC().Format(time.RFC3339),
		until.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("group events by type and level: %w", err)
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
		SELECT COUNT(*), COALESCE(AVG(LENGTH(payload)), 0)
		FROM events
		WHERE source_id = ?
		  AND source_type = 'app'
		  AND datetime(created_at) >= datetime(?)
		  AND datetime(created_at) < datetime(?)`,
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
			WHERE source_id = ?
			  AND source_type = 'app'
			  AND datetime(created_at) >= datetime(?)
			  AND datetime(created_at) < datetime(?)
			GROUP BY strftime('%Y-%m-%dT%H:%M', created_at)
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
		SELECT e.id, e.level, e.source_name, e.source_type,
		       COALESCE(e.source_id, '') AS source_id, e.title,
		       '' AS payload, e.created_at
		FROM events e
		WHERE e.source_id IN (` + ph + `)
		  AND e.source_type = 'app'
		  AND e.created_at = (
		        SELECT MAX(e2.created_at) FROM events e2
		        WHERE e2.source_id = e.source_id AND e2.source_type = 'app'
		      )`

	var events []models.Event
	if err := r.db.SelectContext(ctx, &events, q, args...); err != nil {
		return nil, fmt.Errorf("latest per app: %w", err)
	}

	result := make(map[string]*models.Event, len(events))
	for i := range events {
		result[events[i].SourceID] = &events[i]
	}
	return result, nil
}

func (r *sqliteEventRepo) Timeseries(ctx context.Context, since, until time.Time, granularity, sourceID, level string) ([]TimeseriesBucket, error) {
	// fmtStr is controlled, not user-provided — safe to interpolate.
	fmtStr := "%Y-%m-%d"
	if granularity == "hour" {
		fmtStr = "%Y-%m-%dT%H:00:00Z"
	}

	var parts []string
	var args []interface{}
	parts = append(parts, "datetime(created_at) >= datetime(?)")
	args = append(args, since.UTC().Format(time.RFC3339))
	parts = append(parts, "datetime(created_at) <= datetime(?)")
	args = append(args, until.UTC().Format(time.RFC3339))
	if sourceID != "" {
		parts = append(parts, "source_id = ?")
		args = append(args, sourceID)
	}
	if level != "" {
		parts = append(parts, "level = ?")
		args = append(args, level)
	}
	where := " WHERE " + strings.Join(parts, " AND ")

	q := fmt.Sprintf(
		"SELECT strftime('%s', created_at) AS time, COUNT(*) AS count FROM events%s GROUP BY strftime('%s', created_at) ORDER BY time",
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
		SELECT e.source_id, COALESCE(e.source_name, e.source_id) AS source_name, COUNT(*) AS count
		FROM events e
		WHERE e.source_type = 'app'
		  AND e.source_id IS NOT NULL
		  AND datetime(e.created_at) >= datetime(?)
		GROUP BY e.source_id
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
