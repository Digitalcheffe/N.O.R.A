// Package metrics provides MetricsScanner implementations for each
// infrastructure integration type supported by NORA.
//
// Each scanner is constructed with a *repo.Store and registered with the
// scanner.ScanScheduler in main.go. Scanners run every 2 minutes
// (scanner.MetricsInterval), write raw readings to resource_readings, and
// fire threshold-crossing events at most once per crossing (no spam).
//
// Design rules:
//   - Write to resource_readings ONLY — never to events on a successful reading.
//   - Fire an event when a threshold is first crossed; do not re-fire on
//     subsequent readings while the metric stays in breach.
//   - Fire a recovery event when the metric drops back below the threshold.
//   - Do NOT update infrastructure_components.last_status or meta columns.
//     Status / snapshot updates are the responsibility of SnapshotScanner
//     (REFACTOR-08) and the legacy pollers that are still running in parallel.
package metrics

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// thresholdLevel represents whether a metric is below its threshold,
// in warning territory, or in error territory.
type thresholdLevel int

const (
	levelNormal thresholdLevel = iota
	levelWarn
	levelError
)

func (l thresholdLevel) String() string {
	switch l {
	case levelWarn:
		return "warn"
	case levelError:
		return "error"
	default:
		return ""
	}
}

// ThresholdTracker tracks the last known threshold level per (entityID, metric)
// pair so that events are only emitted on transitions, not on every reading.
type ThresholdTracker struct {
	mu    sync.RWMutex
	state map[string]thresholdLevel // key: "entityID/metric"
}

func newThresholdTracker() ThresholdTracker {
	return ThresholdTracker{state: make(map[string]thresholdLevel)}
}

// key returns the map key for a (entityID, metric) pair.
func (t *ThresholdTracker) key(entityID, metric string) string {
	return entityID + "/" + metric
}

// load returns the last known threshold level for (entityID, metric).
func (t *ThresholdTracker) load(entityID, metric string) thresholdLevel {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state[t.key(entityID, metric)]
}

// store saves the new threshold level for (entityID, metric).
func (t *ThresholdTracker) store(entityID, metric string, level thresholdLevel) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state[t.key(entityID, metric)] = level
}

// CheckAndFire compares current to previous threshold level and writes an event
// if the level has changed. Returns the new level.
func (t *ThresholdTracker) CheckAndFire(
	ctx context.Context,
	store *repo.Store,
	entityID, entityName, sourceType, metric string,
	current thresholdLevel,
	eventTitle func(thresholdLevel) string,
) thresholdLevel {
	prev := t.load(entityID, metric)
	t.store(entityID, metric, current)

	if current == prev {
		return current
	}

	// Level changed — emit an event.
	var level, title string
	switch current {
	case levelError:
		level = "error"
		title = eventTitle(current)
	case levelWarn:
		level = "warn"
		title = eventTitle(current)
	case levelNormal:
		// Recovery from a prior breach.
		level = "info"
		title = eventTitle(current)
	}

	writeMetricsEvent(ctx, store, entityID, entityName, sourceType, level, title)
	return current
}

// cpuThreshold maps CPU% to a threshold level per the REFACTOR-07 spec.
// CPU > 90% → warn (no error level for CPU).
func cpuThreshold(pct float64) thresholdLevel {
	if pct > 90 {
		return levelWarn
	}
	return levelNormal
}

// memThreshold maps memory% to a threshold level.
// Memory > 90% → warn.
func memThreshold(pct float64) thresholdLevel {
	if pct > 90 {
		return levelWarn
	}
	return levelNormal
}

// tempThreshold maps temperature (Celsius) to a threshold level.
// > 90°C → error; > 80°C → warn.
func tempThreshold(tempC float64) thresholdLevel {
	switch {
	case tempC > 90:
		return levelError
	case tempC > 80:
		return levelWarn
	default:
		return levelNormal
	}
}

// writeMetricsEvent persists a single event for a threshold crossing.
func writeMetricsEvent(
	ctx context.Context,
	store *repo.Store,
	sourceID, sourceName, sourceType, level, title string,
) {
	payload := fmt.Sprintf(
		`{"bucket":"metrics","source_id":%q,"source_name":%q}`,
		sourceID, sourceName,
	)
	ev := &models.Event{
		ID:         uuid.New().String(),
		Level:      level,
		SourceName: sourceName,
		SourceType: sourceType,
		SourceID:   sourceID,
		Title:      title,
		Payload:    payload,
		CreatedAt:  time.Now().UTC(),
	}
	if err := store.Events.Create(ctx, ev); err != nil {
		log.Printf("metrics: write event for %s (%s): %v", sourceName, sourceID, err)
	}
}

// writeReading persists a single resource reading.
func writeReading(
	ctx context.Context,
	store *repo.Store,
	sourceID, sourceType, metric string,
	value float64,
	now time.Time,
) {
	r := &models.ResourceReading{
		ID:         uuid.New().String(),
		SourceID:   sourceID,
		SourceType: sourceType,
		Metric:     metric,
		Value:      value,
		RecordedAt: now,
	}
	if err := store.Resources.Create(ctx, r); err != nil {
		log.Printf("metrics: write reading %s/%s: %v", sourceID, metric, err)
	}
}
