package api

import (
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
	apps     repo.AppRepo
	events   repo.EventRepo
	checks   repo.CheckRepo
	rollups  repo.RollupRepo
	profiler apptemplate.Loader
}

// NewDashboardHandler creates a DashboardHandler with the given dependencies.
func NewDashboardHandler(apps repo.AppRepo, events repo.EventRepo, checks repo.CheckRepo, rollups repo.RollupRepo, profiler apptemplate.Loader) *DashboardHandler {
	return &DashboardHandler{
		apps:     apps,
		events:   events,
		checks:   checks,
		rollups:  rollups,
		profiler: profiler,
	}
}

// Routes registers dashboard endpoints on r.
func (h *DashboardHandler) Routes(r chi.Router) {
	r.Get("/dashboard/summary", h.Summary)
	r.Get("/dashboard/digest/{period}", h.Digest)
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
	Label     string `json:"label"`
	Count     int    `json:"count"`
	Sub       string `json:"sub"`
	Sparkline [7]int `json:"sparkline"`
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
	Sparkline     [7]int    `json:"sparkline"`
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

	// --- 2. Load all checks ---
	checks, err := h.checks.List(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load checks")
		return
	}

	// --- 3. Build summary_bar via profile digest aggregation ---

	// categoryEntry accumulates cross-app data for one label.
	type categoryEntry struct {
		cats    []apptemplate.DigestCategory // all matching category definitions
		perApp  map[string]int           // appID → count
		total   int
		sparkline [7]int
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
				MatchField:    cat.MatchField,
				MatchValue:    cat.MatchValue,
				MatchLevel: cat.MatchSeverity,
				Since:         pc.since,
				Until:         pc.until,
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

	// Build sparklines and sub-strings, only for categories with count > 0
	summaryBar := make([]summaryBarItem, 0, len(catOrder))
	for _, label := range catOrder {
		entry := catMap[label]
		if entry.total == 0 {
			continue
		}

		// Use the first category definition's match criteria for sparkline (they share a label)
		cat := entry.cats[0]
		sf := repo.CategoryFilter{
			MatchField:    cat.MatchField,
			MatchValue:    cat.MatchValue,
			MatchLevel: cat.MatchSeverity,
		}
		// Collect all app IDs that contribute to this label
		for appID := range entry.perApp {
			sf.SourceIDs = append(sf.SourceIDs, appID)
		}
		sparkline, err := h.events.SparklineBuckets(ctx, sf, pc.since, pc.bucketDur)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to build sparkline")
			return
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
			Label:     label,
			Count:     entry.total,
			Sub:       strings.Join(subParts, " · "),
			Sparkline: sparkline,
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

	appSummaries := make([]appSummary, 0, len(apps))
	for _, a := range apps {
		status := "online"
		if s := appCheckStatus[a.ID]; s == "down" {
			status = "down"
		} else if s == "warn" {
			status = "warn"
		}

		// Per-app sparkline: all events for this app in the period
		appSparkline, err := h.events.SparklineBuckets(ctx, repo.CategoryFilter{
			SourceIDs: []string{a.ID},
			Since:  pc.since,
			Until:  pc.until,
		}, pc.since, pc.bucketDur)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to build app sparkline")
			return
		}

		// Per-app stats from profile digest categories
		var stats []appStat
		if a.ProfileID != "" {
			p, err := h.profiler.Get(a.ProfileID)
			if err == nil && p != nil {
				for _, cat := range p.Digest.Categories {
					f := repo.CategoryFilter{
						SourceIDs:  []string{a.ID},
						MatchField:    cat.MatchField,
						MatchValue:    cat.MatchValue,
						MatchLevel: cat.MatchSeverity,
						Since:         pc.since,
						Until:         pc.until,
					}
					n, err := h.events.CountForCategory(ctx, f)
					if err != nil {
						writeError(w, http.StatusInternalServerError, "failed to count app category events")
						return
					}
					if n > 0 {
						stats = append(stats, appStat{
							Label: cat.Label,
							Value: strconv.Itoa(n),
						})
					}
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

		sum := appSummary{
			ID:            a.ID,
			Name:          a.Name,
			ProfileID:     a.ProfileID,
			Status:        status,
			LastEventAt:   lastEventAt,
			LastEventText: lastEventText,
			Stats:         stats,
			Sparkline:     appSparkline,
		}
		if a.ProfileID != "" {
			sum.IconURL = "/api/v1/icons/" + a.ProfileID
			if p, err := h.profiler.Get(a.ProfileID); err == nil && p != nil {
				sum.Capability = p.Meta.Capability
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
