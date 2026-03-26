package ingest

import (
	"sync"
	"time"
)

// RateLimiter enforces per-app event rate limits using a 60-second sliding window.
type RateLimiter struct {
	mu      sync.Mutex
	windows map[string]*rateWindow
}

type rateWindow struct {
	count       int
	windowStart time.Time
}

// NewRateLimiter creates a RateLimiter ready for use.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		windows: make(map[string]*rateWindow),
	}
}

// Allow returns true if the event for appID should be accepted given limit events/minute.
// When the 60-second window has expired it resets, so this is a fixed-window-per-minute
// approximation rather than a true sliding window.
func (r *RateLimiter) Allow(appID string, limit int) bool {
	if limit <= 0 {
		limit = 100
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	w, ok := r.windows[appID]
	if !ok || now.Sub(w.windowStart) >= 60*time.Second {
		r.windows[appID] = &rateWindow{count: 1, windowStart: now}
		return true
	}
	w.count++
	return w.count <= limit
}
