package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// DashboardHandler holds dependencies for the dashboard endpoints.
type DashboardHandler struct {
	apps        repo.AppRepo
	events      repo.EventRepo
	checks      repo.CheckRepo
	rollups     repo.RollupRepo
	profiler    apptemplate.Loader
	registry    repo.DigestRegistryRepo
	appMetrics  repo.AppMetricSnapshotRepo
}

// NewDashboardHandler creates a DashboardHandler with the given dependencies.
// registry and appMetrics may be nil — when nil, widget rendering falls back to
// empty (categories continue to work off the profile loader for legacy tests).
func NewDashboardHandler(
	apps repo.AppRepo,
	events repo.EventRepo,
	checks repo.CheckRepo,
	rollups repo.RollupRepo,
	profiler apptemplate.Loader,
	registry repo.DigestRegistryRepo,
	appMetrics repo.AppMetricSnapshotRepo,
) *DashboardHandler {
	return &DashboardHandler{
		apps:       apps,
		events:     events,
		checks:     checks,
		rollups:    rollups,
		profiler:   profiler,
		registry:   registry,
		appMetrics: appMetrics,
	}
}

// Routes registers dashboard endpoints on r.
func (h *DashboardHandler) Routes(r chi.Router) {
	r.Get("/dashboard/summary", h.Summary)
	r.Get("/dashboard/digest/{period}", h.Digest)
}

// registryKey identifies a digest_registry row for the active-gate lookup.
type registryKey struct {
	profileID string
	entryType string // "category" or "widget"
	label     string
}

// loadActiveRegistry returns the set of active (profile, entryType, label)
// tuples from digest_registry. Returns nil when the registry dependency is
// not wired — callers treat nil as "gate disabled, show everything".
func (h *DashboardHandler) loadActiveRegistry(ctx context.Context) (map[registryKey]bool, error) {
	if h.registry == nil {
		return nil, nil
	}
	entries, err := h.registry.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[registryKey]bool, len(entries))
	for _, e := range entries {
		if !e.Active {
			continue
		}
		out[registryKey{profileID: e.ProfileID, entryType: e.EntryType, label: e.Label}] = true
	}
	return out, nil
}

// registryAllows reports whether (profileID, entryType, label) is active.
// Nil map = gate disabled, so everything passes.
func registryAllows(m map[registryKey]bool, profileID, entryType, label string) bool {
	if m == nil {
		return true
	}
	return m[registryKey{profileID: profileID, entryType: entryType, label: label}]
}

// widgetValue computes the rendered value string for a DigestWidget:
//
//   - source=webhook → count of events in [since, until] matching the widget's
//     field predicate (with optional and_field/and_value compound clause).
//     Returns "0" when nothing matches — callers render that as a zero card.
//   - source=api     → latest app_metric_snapshot.value for the referenced
//     metric name. Returns "—" when no snapshot exists yet (e.g. the profile
//     declares an API widget but api_polling hasn't populated it).
func (h *DashboardHandler) widgetValue(
	ctx context.Context,
	appID string,
	wg apptemplate.DigestWidget,
	since, until time.Time,
) (string, error) {
	switch wg.Source {
	case "webhook":
		f := repo.CategoryFilter{
			SourceIDs:  []string{appID},
			MatchField: wg.MatchField,
			MatchValue: wg.MatchValue,
			AndField:   wg.AndField,
			AndValue:   wg.AndValue,
			Since:      since,
			Until:      until,
		}
		n, err := h.events.CountForCategory(ctx, f)
		if err != nil {
			return "", err
		}
		return strconv.Itoa(n), nil
	case "api":
		if h.appMetrics == nil || wg.Metric == "" {
			return "—", nil
		}
		snap, err := h.appMetrics.GetByAppAndMetric(ctx, appID, wg.Metric)
		if err != nil || snap == nil {
			// A missing snapshot is not a handler-level failure — it just
			// means the API poller hasn't populated this metric yet.
			return "—", nil
		}
		return snap.Value, nil
	default:
		return "—", nil
	}
}

// --- period helpers ---

type periodConfig struct {
	since     time.Time
	until     time.Time
	bucketDur time.Duration
}

func parsePeriod(param string, now time.Time) periodConfig {
	switch param {
	case "day":
		// 7 × 4h buckets = 28h window
		bd := 4 * time.Hour
		return periodConfig{since: now.Add(-7 * bd), until: now, bucketDur: bd}
	case "month":
		// 7 × 4-day buckets = 28d window
		bd := 4 * 24 * time.Hour
		return periodConfig{since: now.Add(-7 * bd), until: now, bucketDur: bd}
	default: // "week"
		// 7 × 1-day buckets = 7d window
		bd := 24 * time.Hour
		return periodConfig{since: now.Add(-7 * bd), until: now, bucketDur: bd}
	}
}

// --- response types ---

type summaryBarItem struct {
	Label string `json:"label"`
	Count int    `json:"count"`
	Sub   string `json:"sub"`
}

type appStat struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Color string `json:"color,omitempty"`
}

type appSummary struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	ProfileID     string    `json:"profile_id"`
	IconURL       string    `json:"icon_url,omitempty"`
	Capability    string    `json:"capability,omitempty"`
	Status        string    `json:"status"`
	LastEventAt   *string   `json:"last_event_at"`
	LastEventText *string   `json:"last_event_text"`
	Stats         []appStat `json:"stats"`
	ChecksUp      int       `json:"checks_up"`
	ChecksTotal   int       `json:"checks_total"`
}

type checkSummary struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	Target        string  `json:"target"`
	Status        string  `json:"status"`
	UptimePct     float64 `json:"uptime_pct"`
	LastCheckedAt *string `json:"last_checked_at,omitempty"`
}

type sslCert struct {
	Domain        string `json:"domain"`
	DaysRemaining int    `json:"days_remaining"`
	ExpiresAt     string `json:"expires_at"`
	Status        string `json:"status"`
}

type summaryResponse struct {
	Status     string           `json:"status"`
	Period     string           `json:"period"`
	SummaryBar []summaryBarItem `json:"summary_bar"`
	Apps       []appSummary     `json:"apps"`
	Checks     []checkSummary   `json:"checks"`
	SSLCerts   []sslCert        `json:"ssl_certs"`
}

// statusToUptimePct converts a check's last_status into a 0–100 uptime proxy.
// This is used until a check_results history table exists for real uptime math.
//
//	up       → 100.0   (check passed on last poll)
//	warn     →  75.0   (check degraded: SSL near expiry, DNS drift, slow response)
//	down     →   0.0   (check failed on last poll)
//	critical →   0.0   (SSL expired / check hard failure)
//	unknown  →   0.0   (never polled yet — treat as unavailable, not as "fine")
func statusToUptimePct(status string) float64 {
	switch status {
	case "up":
		return 100.0
	case "warn":
		return 75.0
	default: // "down", "critical", "unknown", ""
		return 0.0
	}
}

// Summary handles GET /api/v1/dashboard/summary?period=week
func (h *DashboardHandler) Summary(w http.ResponseWriter, r *http.Request) {
	periodParam := r.URL.Query().Get("period")
	if periodParam == "" {
		periodParam = "week"
	}
	now := time.Now().UTC()
	pc := parsePeriod(periodParam, now)

	ctx := r.Context()

	// --- 1. Load all apps ---
	apps, err := h.apps.List(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load apps")
		return
	}

	// Load the digest registry once — rendering is gated on active entries so
	// deleting or deactivating a row drops the card from the UI. When the
	// handler is wired without a registry (older tests), everything is treated
	// as active to preserve behavior.
	registryActive, err := h.loadActiveRegistry(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load digest registry")
		return
	}

	// --- 2. Load all checks ---
	checks, err := h.checks.List(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load checks")
		return
	}

	// --- 3. Build summary_bar via profile digest aggregation ---

	// categoryEntry accumulates cross-app data for one label.
	type categoryEntry struct {
		cats   []apptemplate.DigestCategory // all matching category definitions
		perApp map[string]int               // appID → count
		total  int
	}

	// keyed by label
	catMap := map[string]*categoryEntry{}
	// preserve insertion order
	catOrder := []string{}

	appIDs := make([]string, 0, len(apps))
	for _, a := range apps {
		appIDs = append(appIDs, a.ID)
	}

	// Gather per-app latest events for status/last_event fields
	latestEvents, err := h.events.LatestPerApp(ctx, appIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load latest events")
		return
	}

	for _, a := range apps {
		if a.ProfileID == "" {
			continue
		}
		p, err := h.profiler.Get(a.ProfileID)
		if err != nil || p == nil {
			continue
		}

		for _, cat := range p.Digest.Categories {
			if !registryAllows(registryActive, a.ProfileID, "category", cat.Label) {
				continue
			}
			if _, exists := catMap[cat.Label]; !exists {
				catMap[cat.Label] = &categoryEntry{
					perApp: make(map[string]int),
				}
				catOrder = append(catOrder, cat.Label)
			}
			entry := catMap[cat.Label]
			entry.cats = append(entry.cats, cat)

			// Count events for this app+category in the period
			f := repo.CategoryFilter{
				SourceIDs:  []string{a.ID},
				MatchField: cat.MatchField,
				MatchValue: cat.MatchValue,
				AndField:   cat.AndField,
				AndValue:   cat.AndValue,
				MatchLevel: cat.MatchSeverity,
				Since:      pc.since,
				Until:      pc.until,
			}
			n, err := h.events.CountForCategory(ctx, f)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to count events")
				return
			}
			entry.perApp[a.ID] += n
			entry.total += n
		}
	}

	// Build sub-strings for all categories, including zero-count ones.
	summaryBar := make([]summaryBarItem, 0, len(catOrder))
	for _, label := range catOrder {
		entry := catMap[label]
		if entry.total == 0 {
			summaryBar = append(summaryBar, summaryBarItem{
				Label: label,
				Count: 0,
				Sub:   "",
			})
			continue
		}

		// Build sub string: "AppName N · AppName N" for apps with count > 0
		type appCount struct {
			name  string
			count int
		}
		var breakdown []appCount
		for _, a := range apps {
			if n := entry.perApp[a.ID]; n > 0 {
				breakdown = append(breakdown, appCount{name: a.Name, count: n})
			}
		}
		sort.Slice(breakdown, func(i, j int) bool { return breakdown[i].count > breakdown[j].count })
		subParts := make([]string, 0, len(breakdown))
		for _, ac := range breakdown {
			subParts = append(subParts, fmt.Sprintf("%s %d", ac.name, ac.count))
		}

		summaryBar = append(summaryBar, summaryBarItem{
			Label: label,
			Count: entry.total,
			Sub:   strings.Join(subParts, " · "),
		})
	}

	// --- 4. Build per-app summaries ---
	// Build a map from appID → check status using monitor_checks
	appCheckStatus := map[string]string{}
	for _, c := range checks {
		if c.AppID == "" {
			continue
		}
		if c.LastStatus == "down" {
			appCheckStatus[c.AppID] = "down"
		} else if c.LastStatus == "warn" && appCheckStatus[c.AppID] != "down" {
			appCheckStatus[c.AppID] = "warn"
		} else if appCheckStatus[c.AppID] == "" {
			appCheckStatus[c.AppID] = c.LastStatus
		}
	}

	// Build a map from appID → {up, total} for per-app check counts.
	type checkCounts struct{ up, total int }
	appCheckCounts := map[string]checkCounts{}
	for _, c := range checks {
		if c.AppID == "" || !c.Enabled {
			continue
		}
		cc := appCheckCounts[c.AppID]
		cc.total++
		if c.LastStatus == "up" {
			cc.up++
		}
		appCheckCounts[c.AppID] = cc
	}

	appSummaries := make([]appSummary, 0, len(apps))
	for _, a := range apps {
		status := "online"
		if s := appCheckStatus[a.ID]; s == "down" {
			status = "down"
		} else if s == "warn" {
			status = "warn"
		}

		// Per-app stats from profile digest entries, gated by digest_registry.
		// Both categories and widgets render here (widgets stay off the main
		// summary bar). Zero counts still produce a card so the dashboard
		// layout doesn't jump when counts hit zero for a period.
		var stats []appStat
		if a.ProfileID != "" {
			p, err := h.profiler.Get(a.ProfileID)
			if err == nil && p != nil {
				for _, cat := range p.Digest.Categories {
					if !registryAllows(registryActive, a.ProfileID, "category", cat.Label) {
						continue
					}
					f := repo.CategoryFilter{
						SourceIDs:  []string{a.ID},
						MatchField: cat.MatchField,
						MatchValue: cat.MatchValue,
						AndField:   cat.AndField,
						AndValue:   cat.AndValue,
						MatchLevel: cat.MatchSeverity,
						Since:      pc.since,
						Until:      pc.until,
					}
					n, err := h.events.CountForCategory(ctx, f)
					if err != nil {
						writeError(w, http.StatusInternalServerError, "failed to count app category events")
						return
					}
					stats = append(stats, appStat{
						Label: cat.Label,
						Value: strconv.Itoa(n),
					})
				}

				for _, wg := range p.Digest.Widgets {
					if !registryAllows(registryActive, a.ProfileID, "widget", wg.Label) {
						continue
					}
					value, err := h.widgetValue(ctx, a.ID, wg, pc.since, pc.until)
					if err != nil {
						writeError(w, http.StatusInternalServerError, "failed to build app widget value")
						return
					}
					stats = append(stats, appStat{
						Label: wg.Label,
						Value: value,
					})
				}
			}
		}

		var lastEventAt *string
		var lastEventText *string
		if ev, ok := latestEvents[a.ID]; ok {
			s := ev.CreatedAt.UTC().Format(time.RFC3339)
			lastEventAt = &s
			lastEventText = &ev.Title
		}

		cc := appCheckCounts[a.ID]
		sum := appSummary{
			ID:            a.ID,
			Name:          a.Name,
			ProfileID:     a.ProfileID,
			Status:        status,
			LastEventAt:   lastEventAt,
			LastEventText: lastEventText,
			Stats:         stats,
			ChecksUp:      cc.up,
			ChecksTotal:   cc.total,
		}
		if a.ProfileID != "" {
			sum.IconURL = "/api/v1/icons/" + a.ProfileID
			if p, err := h.profiler.Get(a.ProfileID); err == nil && p != nil {
				sum.Capability = apptemplate.InferCapability(p)
			}
		}
		appSummaries = append(appSummaries, sum)
	}

	// --- 5. Build check summaries ---
	checkSummaries := make([]checkSummary, 0)
	sslCerts := make([]sslCert, 0)

	for _, c := range checks {
		if !c.Enabled {
			continue
		}

		status := "unknown"
		if c.LastStatus != "" {
			status = c.LastStatus
		}

		var lastCheckedAt *string
		if c.LastCheckedAt != nil {
			s := c.LastCheckedAt.UTC().Format(time.RFC3339)
			lastCheckedAt = &s
		}

		if c.Type == "ssl" {
			// Parse expiry from last_result if available
			cert := sslCert{
				Domain: c.Target,
				Status: "unknown",
			}
			if c.LastResult != "" {
				var result map[string]interface{}
				if err := json.Unmarshal([]byte(c.LastResult), &result); err == nil {
					if exp, ok := result["expires_at"].(string); ok {
						cert.ExpiresAt = exp
						if t, err := time.Parse("2006-01-02", exp); err == nil {
							days := int(time.Until(t).Hours() / 24)
							cert.DaysRemaining = days
							switch {
							case days <= c.SSLCritDays:
								cert.Status = "critical"
							case days <= c.SSLWarnDays:
								cert.Status = "warn"
							default:
								cert.Status = "ok"
							}
						}
					}
					if days, ok := result["days_remaining"].(float64); ok {
						cert.DaysRemaining = int(days)
					}
				}
			}
			sslCerts = append(sslCerts, cert)
		}

		// Derive uptime from last_status. No check-result history table exists yet,
		// so this is a point-in-time proxy: up=100, warn=75, down/critical/unknown=0.
		// When a check has never run (status=="unknown") we treat it as 0 so it
		// doesn't inflate the average of a type group that has real failures.
		uptimePct := statusToUptimePct(status)

		checkSummaries = append(checkSummaries, checkSummary{
			ID:            c.ID,
			Name:          c.Name,
			Type:          c.Type,
			Target:        c.Target,
			Status:        status,
			UptimePct:     uptimePct,
			LastCheckedAt: lastCheckedAt,
		})
	}

	// Sort SSL certs by days remaining ascending (soonest expiry first)
	sort.Slice(sslCerts, func(i, j int) bool {
		return sslCerts[i].DaysRemaining < sslCerts[j].DaysRemaining
	})

	// --- 6. Compute global status ---
	globalStatus := "normal"
	for _, c := range checks {
		if !c.Enabled {
			continue
		}
		if c.LastStatus == "down" {
			globalStatus = "down"
			break
		}
		if c.LastStatus == "warn" && globalStatus != "down" {
			globalStatus = "warn"
		}
	}
	if globalStatus != "down" {
		for _, a := range appSummaries {
			if a.Status == "down" {
				globalStatus = "down"
				break
			}
			if a.Status == "warn" && globalStatus != "down" {
				globalStatus = "warn"
			}
		}
	}

	writeJSON(w, http.StatusOK, summaryResponse{
		Status:     globalStatus,
		Period:     periodParam,
		SummaryBar: summaryBar,
		Apps:       appSummaries,
		Checks:     checkSummaries,
		SSLCerts:   sslCerts,
	})
}

// --- digest types ---

type digestCategory struct {
	Label     string `json:"label"`
	Count     int    `json:"count"`
	Breakdown string `json:"breakdown,omitempty"`
	OK        *bool  `json:"ok,omitempty"`
}

type digestResponse struct {
	Period      string           `json:"period"`
	Categories  []digestCategory `json:"categories"`
	UptimePct   float64          `json:"uptime_pct"`
	ErrorCount  int              `json:"error_count"`
}

// Digest handles GET /api/v1/dashboard/digest/{period}
// period format: YYYY-MM
func (h *DashboardHandler) Digest(w http.ResponseWriter, r *http.Request) {
	periodStr := chi.URLParam(r, "period")
	parts := strings.SplitN(periodStr, "-", 2)
	if len(parts) != 2 {
		writeError(w, http.StatusBadRequest, "period must be YYYY-MM")
		return
	}
	year, err := strconv.Atoi(parts[0])
	if err != nil || year < 2000 {
		writeError(w, http.StatusBadRequest, "invalid year in period")
		return
	}
	month, err := strconv.Atoi(parts[1])
	if err != nil || month < 1 || month > 12 {
		writeError(w, http.StatusBadRequest, "invalid month in period")
		return
	}

	ctx := r.Context()

	rollups, err := h.rollups.ListByPeriod(ctx, year, month)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load rollups")
		return
	}

	// Load app names for breakdown strings
	apps, err := h.apps.List(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load apps")
		return
	}
	appNames := make(map[string]string, len(apps))
	for _, a := range apps {
		appNames[a.ID] = a.Name
	}

	// Aggregate rollups: group by event_type → sum counts, build breakdown
	type rollupGroup struct {
		perApp map[string]int
		total  int
	}
	groups := map[string]*rollupGroup{}
	groupOrder := []string{}
	errorCount := 0

	for _, rl := range rollups {
		if _, ok := groups[rl.EventType]; !ok {
			groups[rl.EventType] = &rollupGroup{perApp: make(map[string]int)}
			groupOrder = append(groupOrder, rl.EventType)
		}
		g := groups[rl.EventType]
		g.perApp[rl.AppID] += rl.Count
		g.total += rl.Count

		if rl.Severity == "error" || rl.Severity == "critical" {
			errorCount += rl.Count
		}
	}

	categories := make([]digestCategory, 0, len(groupOrder))
	for _, label := range groupOrder {
		g := groups[label]

		// Build "AppName N, AppName N" breakdown
		type appCount struct {
			name  string
			count int
		}
		var breakdown []appCount
		for appID, n := range g.perApp {
			name := appNames[appID]
			if name == "" {
				name = appID
			}
			breakdown = append(breakdown, appCount{name: name, count: n})
		}
		sort.Slice(breakdown, func(i, j int) bool { return breakdown[i].count > breakdown[j].count })
		parts := make([]string, 0, len(breakdown))
		for _, ac := range breakdown {
			parts = append(parts, fmt.Sprintf("%s %d", ac.name, ac.count))
		}

		ok := errorCount == 0
		categories = append(categories, digestCategory{
			Label:     label,
			Count:     g.total,
			Breakdown: strings.Join(parts, ", "),
			OK:        &ok,
		})
	}

	writeJSON(w, http.StatusOK, digestResponse{
		Period:     periodStr,
		Categories: categories,
		UptimePct:  100.0,
		ErrorCount: errorCount,
	})
}
