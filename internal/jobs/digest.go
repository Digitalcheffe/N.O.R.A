package jobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// digestScheduleKey is the settings table key for the digest schedule config.
const digestScheduleKey = "digest_schedule"

// smtpSettingsKey is the settings table key for SMTP configuration.
const smtpSettingsKey = "smtp"

// ── Data types ────────────────────────────────────────────────────────────────

// DigestData holds all data passed to the HTML templates.
type DigestData struct {
	// Header
	Title  string
	Period string

	// Narrative
	Headline     string   // "All systems healthy" | "3 items need your attention"
	HealthStatus string   // "healthy" | "warning" | "critical"
	Summary      []string // 2–4 plain-English sentences
	ActionItems  []DigestActionItem

	// Event severity totals for the period
	EventInfo     int
	EventWarn     int
	EventError    int
	EventCritical int

	// Monitor checks — one entry per check type (url/ssl/dns/ping)
	CheckGroups []DigestCheckGroup

	// App activity — per app with category labels from template (full / webhook_only / limited)
	AppSections []DigestAppSection

	// Services — monitor_only / docker_only apps (no webhook digest data)
	ServiceSections []DigestAppSection

	// Infrastructure component status summary
	InfraOnline  int
	InfraOffline int
	InfraRows    []DigestInfraRow

	// Container signals
	ContainersTotal   int
	ContainersRunning int
	UpdatesAvailable  int
	NewContainers     []string // container names new this period
	StoppedContainers []string // container names currently stopped

	// Resource warnings (high CPU / memory / disk over the period)
	ResourceWarnings []DigestResourceWarning

	// Legacy fields — kept so existing template paths compile.
	TotalErrors int
	AppRows     []DigestAppRow
}

// DigestActionItem is a single item the recipient should act on or review.
type DigestActionItem struct {
	Priority string // "urgent" | "review" | "info"
	Text     string
}

// DigestCheckGroup is a rolled-up view of one check type (url, ssl, dns, ping).
type DigestCheckGroup struct {
	Type      string
	Total     int
	Up        int
	NotUp     int
	AvgUptime float64
	Status    string // "healthy" | "warning" | "down" | "none"
}

// DigestAppSection groups the digest category counts for one app.
type DigestAppSection struct {
	AppName     string
	ProfileName string
	Categories  []DigestCategoryRow
	TotalEvents int
	HasIssues   bool
}

// DigestCategoryRow is one category (e.g. "Downloads", "Errors") for an app.
type DigestCategoryRow struct {
	Label   string
	Count   int
	IsError bool
}

// DigestInfraRow is one infrastructure component in the infra status section.
type DigestInfraRow struct {
	Name   string
	Type   string
	Status string
}

// DigestResourceWarning flags a component with sustained high resource usage.
type DigestResourceWarning struct {
	ComponentName string
	Metric        string  // "CPU" | "Memory" | "Disk"
	AvgPct        float64 // average over the period
	MaxPct        float64 // peak over the period
}

// DigestAppRow is the legacy per-app row (kept so existing tests compile).
type DigestAppRow struct {
	AppName   string
	EventType string
	Count     int
	HasErrors bool
}

// ── DigestJob ─────────────────────────────────────────────────────────────────

// DigestJob generates and sends the NORA digest email.
type DigestJob struct {
	store    *repo.Store
	config   *config.Config
	profiler apptemplate.Loader // optional; nil = no per-app category breakdown
}

// NewDigestJob creates a DigestJob.
// profiler may be nil — if absent, app category sections are omitted.
func NewDigestJob(store *repo.Store, cfg *config.Config, profiler apptemplate.Loader) *DigestJob {
	return &DigestJob{store: store, config: cfg, profiler: profiler}
}

// Run is called every hour. It reads the stored schedule and decides whether
// to send based on the configured frequency, day, and send_hour.
func (d *DigestJob) Run(ctx context.Context) error {
	var schedule models.DigestSchedule
	err := d.store.Settings.GetJSON(ctx, digestScheduleKey, &schedule)
	if errors.Is(err, repo.ErrNotFound) {
		h := 8
		schedule = models.DigestSchedule{Frequency: "monthly", DayOfWeek: 1, DayOfMonth: 1, SendHour: &h}
	} else if err != nil {
		return fmt.Errorf("digest: read schedule: %w", err)
	}

	now := time.Now()
	if now.Hour() != schedule.EffectiveSendHour() {
		return nil
	}
	if !d.ShouldSendToday(schedule, now) {
		log.Printf("digest: skipping — not scheduled for today (%s)", now.Format("2006-01-02"))
		return nil
	}

	period := periodLabel(schedule.Frequency, now)
	return d.Send(ctx, period)
}

// ShouldSendToday returns true if the digest should fire on the given date.
func (d *DigestJob) ShouldSendToday(schedule models.DigestSchedule, on time.Time) bool {
	switch schedule.Frequency {
	case "daily":
		return true
	case "weekly":
		return int(on.Weekday()) == schedule.DayOfWeek
	case "monthly":
		return on.Day() == schedule.DayOfMonth
	default:
		return false
	}
}

// Send generates and emails the digest for the given period label.
func (d *DigestJob) Send(ctx context.Context, period string) error {
	log.Printf("digest: generating for period %s", period)

	data, err := d.buildDigestData(ctx, period)
	if err != nil {
		return fmt.Errorf("digest: build data: %w", err)
	}

	html, err := d.RenderHTML(data)
	if err != nil {
		return fmt.Errorf("digest: render html: %w", err)
	}

	recipients, err := d.adminEmails(ctx)
	if err != nil {
		return fmt.Errorf("digest: get recipients: %w", err)
	}
	if len(recipients) == 0 {
		log.Printf("digest: no recipients found, skipping send")
		return nil
	}

	smtp, err := d.smtpSettings(ctx)
	if err != nil {
		log.Printf("digest: SMTP not configured (%v), skipping send", err)
		return nil
	}

	if err := SendMail(smtp.Host, smtp.Port, smtp.User, smtp.Pass, smtp.From,
		recipients, data.Title, html); err != nil {
		log.Printf("digest: smtp error: %v", err)
		return nil
	}

	log.Printf("digest: sent to %d recipients for period %s", len(recipients), period)
	return nil
}

// RenderHTML renders the digest email HTML from DigestData.
func (d *DigestJob) RenderHTML(data *DigestData) (string, error) {
	tmpl, err := template.New("digest").Parse(digestHTMLTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// GenerateReportHTML builds digest data for the given period and renders the
// print-friendly report HTML.
func (d *DigestJob) GenerateReportHTML(ctx context.Context, period string) (string, error) {
	data, err := d.buildDigestData(ctx, period)
	if err != nil {
		return "", fmt.Errorf("digest: build report data: %w", err)
	}
	tmpl, err := template.New("report").Parse(reportHTMLTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// SMTPConfigured returns true if SMTP is reachable.
func (d *DigestJob) SMTPConfigured(ctx context.Context) bool {
	_, err := d.smtpSettings(ctx)
	return err == nil
}

// ── buildDigestData ───────────────────────────────────────────────────────────

func (d *DigestJob) buildDigestData(ctx context.Context, period string) (*DigestData, error) {
	since, until := periodTimeRange(period)

	apps, err := d.store.Apps.List(ctx)
	if err != nil {
		return nil, err
	}
	appNames := make(map[string]string, len(apps))
	for _, a := range apps {
		appNames[a.ID] = a.Name
	}

	data := &DigestData{
		Title:  subjectLine(period),
		Period: period,
	}

	// ── 1. Event severity counts ───────────────────────────────────────────
	for _, level := range []string{"info", "warn", "error", "critical"} {
		n, err := d.store.Events.CountForCategory(ctx, repo.CategoryFilter{
			MatchLevel: level,
			Since:      since,
			Until:      until,
		})
		if err != nil {
			log.Printf("digest: count %s events: %v", level, err)
			continue
		}
		switch level {
		case "info":
			data.EventInfo = n
		case "warn":
			data.EventWarn = n
		case "error":
			data.EventError = n
			data.TotalErrors = n
		case "critical":
			data.EventCritical = n
			data.TotalErrors += n
		}
	}

	// ── 2. Monitor check rollup (current snapshot, same logic as dashboard) ──
	allCheckTypes := []string{"url", "ssl", "dns", "ping"}
	checks, err := d.store.Checks.List(ctx)
	if err != nil {
		log.Printf("digest: list checks: %v", err)
	}
	for _, ct := range allCheckTypes {
		var ofType []models.MonitorCheck
		for _, c := range checks {
			if c.Enabled && c.Type == ct {
				ofType = append(ofType, c)
			}
		}
		g := DigestCheckGroup{Type: ct, Total: len(ofType)}
		if len(ofType) == 0 {
			g.Status = "none"
			data.CheckGroups = append(data.CheckGroups, g)
			continue
		}
		var sumPct float64
		for _, c := range ofType {
			pct := statusToUptimePctDigest(c.LastStatus)
			sumPct += pct
			if c.LastStatus == "up" {
				g.Up++
			} else {
				g.NotUp++
			}
		}
		g.AvgUptime = sumPct / float64(len(ofType))
		switch {
		case g.AvgUptime >= 95:
			g.Status = "healthy"
		case g.AvgUptime >= 75:
			g.Status = "warning"
		default:
			g.Status = "down"
		}
		data.CheckGroups = append(data.CheckGroups, g)
	}

	// ── 3. App activity — per-app category breakdown ───────────────────────
	// monitor_only / docker_only apps have no webhook/digest categories — route
	// them to ServiceSections so the email can render a separate Services block.
	isServiceCapability := func(cap string) bool {
		return cap == "monitor_only" || cap == "docker_only"
	}

	if d.profiler != nil {
		for _, app := range apps {
			if app.ProfileID == "" {
				continue
			}
			profile, err := d.profiler.Get(app.ProfileID)
			if err != nil || profile == nil {
				continue
			}

			// Service apps: no digest categories — list them in ServiceSections.
			if isServiceCapability(profile.Meta.Capability) {
				data.ServiceSections = append(data.ServiceSections, DigestAppSection{
					AppName:     app.Name,
					ProfileName: profile.Meta.Name,
				})
				continue
			}

			if len(profile.Digest.Categories) == 0 {
				continue
			}

			section := DigestAppSection{
				AppName:     app.Name,
				ProfileName: profile.Meta.Name,
			}
			for _, cat := range profile.Digest.Categories {
				f := repo.CategoryFilter{
					SourceIDs:  []string{app.ID},
					MatchField: cat.MatchField,
					MatchValue: cat.MatchValue,
					MatchLevel: cat.MatchSeverity,
					Since:      since,
					Until:      until,
				}
				n, err := d.store.Events.CountForCategory(ctx, f)
				if err != nil {
					log.Printf("digest: count category %q for app %s: %v", cat.Label, app.Name, err)
					continue
				}
				isErr := cat.MatchSeverity == "error" || cat.MatchSeverity == "critical"
				section.Categories = append(section.Categories, DigestCategoryRow{
					Label:   cat.Label,
					Count:   n,
					IsError: isErr,
				})
				section.TotalEvents += n
				if isErr && n > 0 {
					section.HasIssues = true
				}
			}
			data.AppSections = append(data.AppSections, section)
		}
	} else {
		// Fallback: use live rollups (legacy behaviour) to populate AppRows.
		rollups, err := d.liveRollupsForWindow(ctx, apps, since, until)
		if err != nil {
			log.Printf("digest: live rollups: %v", err)
		}
		type appKey struct{ appID, eventType string }
		counts := map[appKey]int{}
		for _, r := range rollups {
			counts[appKey{r.AppID, r.EventType}] += r.Count
		}
		for k, count := range counts {
			data.AppRows = append(data.AppRows, DigestAppRow{
				AppName:   appNames[k.appID],
				EventType: k.eventType,
				Count:     count,
				HasErrors: k.eventType == "error" || k.eventType == "critical",
			})
		}
		sort.Slice(data.AppRows, func(i, j int) bool {
			if data.AppRows[i].AppName != data.AppRows[j].AppName {
				return data.AppRows[i].AppName < data.AppRows[j].AppName
			}
			return data.AppRows[i].EventType < data.AppRows[j].EventType
		})
	}

	// ── 4. Infrastructure status ───────────────────────────────────────────
	var infra []models.InfrastructureComponent
	if d.store.InfraComponents != nil {
		infra, err = d.store.InfraComponents.List(ctx)
		if err != nil {
			log.Printf("digest: list infra: %v", err)
		}
	}
	for _, c := range infra {
		if !c.Enabled {
			continue
		}
		row := DigestInfraRow{
			Name:   c.Name,
			Type:   infraTypeLabel(c.Type),
			Status: c.LastStatus,
		}
		data.InfraRows = append(data.InfraRows, row)
		if c.LastStatus == "online" {
			data.InfraOnline++
		} else {
			data.InfraOffline++
		}
	}

	// ── 5. Container signals ───────────────────────────────────────────────
	var containers []*models.DiscoveredContainer
	if d.store.DiscoveredContainers != nil {
		containers, err = d.store.DiscoveredContainers.ListAllDiscoveredContainers(ctx)
		if err != nil {
			log.Printf("digest: list containers: %v", err)
		}
	}
	for _, c := range containers {
		data.ContainersTotal++
		if c.Status == "running" {
			data.ContainersRunning++
		} else if c.Status == "stopped" || c.Status == "exited" {
			data.StoppedContainers = append(data.StoppedContainers, trimContainerName(c.ContainerName))
		}
		if c.ImageUpdateAvailable == 1 {
			data.UpdatesAvailable++
		}
		// New containers created within the digest period
		if c.CreatedAt.After(since) && c.CreatedAt.Before(until) {
			data.NewContainers = append(data.NewContainers, trimContainerName(c.ContainerName))
		}
	}
	// Cap display lists
	data.StoppedContainers = capList(data.StoppedContainers, 8)
	data.NewContainers = capList(data.NewContainers, 8)

	// ── 6. Resource warnings (high CPU / memory / disk over the period) ────
	var aggregates []repo.ResourceAggregate
	if d.store.ResourceRollups != nil {
		aggregates, err = d.store.ResourceRollups.AggregateHourlyRollups(ctx, since, until)
		if err != nil {
			log.Printf("digest: resource rollups: %v", err)
		}
	}
	// Build a component name map from the infra list.
	compNameMap := make(map[string]string, len(infra))
	for _, c := range infra {
		compNameMap[c.ID] = c.Name
	}
	for _, agg := range aggregates {
		name := compNameMap[agg.SourceID]
		if name == "" {
			name = agg.SourceID
		}
		var threshold float64
		var label string
		switch agg.Metric {
		case "cpu":
			threshold, label = 80.0, "CPU"
		case "mem":
			threshold, label = 85.0, "Memory"
		case "disk":
			threshold, label = 90.0, "Disk"
		default:
			continue
		}
		if agg.Avg >= threshold || agg.Max >= 95.0 {
			data.ResourceWarnings = append(data.ResourceWarnings, DigestResourceWarning{
				ComponentName: name,
				Metric:        label,
				AvgPct:        agg.Avg,
				MaxPct:        agg.Max,
			})
		}
	}

	// ── 7. Build narrative ─────────────────────────────────────────────────
	data.Headline, data.HealthStatus, data.Summary, data.ActionItems = d.buildNarrative(data)

	return data, nil
}

// ── Narrative builder ─────────────────────────────────────────────────────────

// buildNarrative generates the headline, health status, plain-English summary
// sentences, and prioritised action items from the collected digest data.
func (d *DigestJob) buildNarrative(data *DigestData) (headline, healthStatus string, summary []string, actions []DigestActionItem) {
	var urgentItems, reviewItems, infoItems []DigestActionItem

	// ── Check issues ───────────────────────────────────────────────────────
	var downChecks, warnChecks int
	for _, g := range data.CheckGroups {
		if g.Status == "down" {
			downChecks++
			urgentItems = append(urgentItems, DigestActionItem{
				Priority: "urgent",
				Text:     fmt.Sprintf("%s checks are down (%.1f%% avg uptime, %d not up)", strings.ToUpper(g.Type), g.AvgUptime, g.NotUp),
			})
		} else if g.Status == "warning" {
			warnChecks++
			reviewItems = append(reviewItems, DigestActionItem{
				Priority: "review",
				Text:     fmt.Sprintf("%s checks degraded — %d of %d not fully up (%.1f%% avg)", strings.ToUpper(g.Type), g.NotUp, g.Total, g.AvgUptime),
			})
		}
	}

	// ── Event severity issues ──────────────────────────────────────────────
	if data.EventCritical > 0 {
		urgentItems = append(urgentItems, DigestActionItem{
			Priority: "urgent",
			Text:     fmt.Sprintf("%d critical event%s recorded — review the Events log", data.EventCritical, plural(data.EventCritical)),
		})
	}
	if data.EventError > 0 {
		reviewItems = append(reviewItems, DigestActionItem{
			Priority: "review",
			Text:     fmt.Sprintf("%d error event%s recorded this period", data.EventError, plural(data.EventError)),
		})
	}

	// ── App issues ─────────────────────────────────────────────────────────
	for _, s := range data.AppSections {
		if s.HasIssues {
			reviewItems = append(reviewItems, DigestActionItem{
				Priority: "review",
				Text:     fmt.Sprintf("%s had error-level activity this period", s.AppName),
			})
		}
	}

	// ── Infra issues ───────────────────────────────────────────────────────
	for _, r := range data.InfraRows {
		if r.Status != "online" {
			reviewItems = append(reviewItems, DigestActionItem{
				Priority: "review",
				Text:     fmt.Sprintf("Infrastructure component %q is %s", r.Name, r.Status),
			})
		}
	}

	// ── Container signals ──────────────────────────────────────────────────
	if data.UpdatesAvailable > 0 {
		reviewItems = append(reviewItems, DigestActionItem{
			Priority: "review",
			Text:     fmt.Sprintf("%d container image update%s available — consider pulling latest", data.UpdatesAvailable, plural(data.UpdatesAvailable)),
		})
	}
	if len(data.NewContainers) > 0 {
		infoItems = append(infoItems, DigestActionItem{
			Priority: "info",
			Text:     fmt.Sprintf("%d new container%s appeared this period: %s", len(data.NewContainers), plural(len(data.NewContainers)), strings.Join(data.NewContainers, ", ")),
		})
	}
	if len(data.StoppedContainers) > 0 {
		infoItems = append(infoItems, DigestActionItem{
			Priority: "info",
			Text:     fmt.Sprintf("%d container%s currently stopped: %s", len(data.StoppedContainers), plural(len(data.StoppedContainers)), strings.Join(data.StoppedContainers, ", ")),
		})
	}

	// ── Resource warnings ──────────────────────────────────────────────────
	for _, rw := range data.ResourceWarnings {
		reviewItems = append(reviewItems, DigestActionItem{
			Priority: "review",
			Text:     fmt.Sprintf("%s on %s averaged %.1f%% (peak %.1f%%) — consider investigating", rw.Metric, rw.ComponentName, rw.AvgPct, rw.MaxPct),
		})
	}

	// ── Assemble action items in priority order ────────────────────────────
	actions = append(urgentItems, append(reviewItems, infoItems...)...)

	// ── Health status and headline ─────────────────────────────────────────
	totalIssues := len(urgentItems) + len(reviewItems)
	switch {
	case len(urgentItems) > 0:
		healthStatus = "critical"
		headline = fmt.Sprintf("%d item%s need immediate attention", len(urgentItems)+len(reviewItems), plural(totalIssues))
	case len(reviewItems) > 0:
		healthStatus = "warning"
		headline = fmt.Sprintf("%d item%s worth reviewing", len(reviewItems), plural(len(reviewItems)))
	default:
		healthStatus = "healthy"
		headline = "All systems look healthy"
	}

	// ── Summary sentences — flowing prose paragraphs ─────────────────────
	var sb []string

	// ── Sentence 1: Events overview ───────────────────────────────────────
	totalSevere := data.EventError + data.EventCritical
	totalEvents := data.EventInfo + data.EventWarn + totalSevere
	if totalEvents == 0 {
		if healthStatus == "healthy" {
			sb = append(sb, "No events were recorded this period — everything has been quiet.")
		}
	} else if totalSevere == 0 {
		sb = append(sb, fmt.Sprintf(
			"NORA recorded %d event%s this period with no errors or critical alerts — a clean run.",
			totalEvents, plural(totalEvents)))
	} else {
		var parts []string
		if data.EventInfo > 0 {
			parts = append(parts, fmt.Sprintf("%d info", data.EventInfo))
		}
		if data.EventWarn > 0 {
			parts = append(parts, fmt.Sprintf("%d warning", data.EventWarn))
		}
		if data.EventError > 0 {
			parts = append(parts, fmt.Sprintf("%d error", data.EventError))
		}
		if data.EventCritical > 0 {
			parts = append(parts, fmt.Sprintf("%d critical", data.EventCritical))
		}
		sb = append(sb, fmt.Sprintf(
			"NORA recorded %d event%s this period (%s) — the error and critical events are worth reviewing.",
			totalEvents, plural(totalEvents), strings.Join(parts, ", ")))
	}

	// ── Sentence 2: Monitor checks ────────────────────────────────────────
	totalChecks := 0
	for _, g := range data.CheckGroups {
		totalChecks += g.Total
	}
	if totalChecks > 0 {
		var checkDescs []string
		for _, g := range data.CheckGroups {
			if g.Total == 0 {
				continue
			}
			switch g.Status {
			case "healthy":
				checkDescs = append(checkDescs, fmt.Sprintf("%s (%d up)", strings.ToUpper(g.Type), g.Total))
			case "warning":
				checkDescs = append(checkDescs, fmt.Sprintf("%s at %.0f%% avg with %d of %d not fully up",
					strings.ToUpper(g.Type), g.AvgUptime, g.NotUp, g.Total))
			case "down":
				checkDescs = append(checkDescs, fmt.Sprintf("%s at %.0f%% avg — %d of %d down",
					strings.ToUpper(g.Type), g.AvgUptime, g.NotUp, g.Total))
			}
		}
		if len(checkDescs) > 0 {
			if downChecks == 0 && warnChecks == 0 {
				sb = append(sb, fmt.Sprintf("Monitor checks are all healthy — %s.", strings.Join(checkDescs, ", ")))
			} else {
				sb = append(sb, fmt.Sprintf("Monitor checks need attention: %s.", strings.Join(checkDescs, "; ")))
			}
		}
	}

	// ── Sentence 3: Infrastructure ────────────────────────────────────────
	if len(data.InfraRows) > 0 {
		var offlineNames []string
		for _, r := range data.InfraRows {
			if r.Status != "online" {
				offlineNames = append(offlineNames, r.Name)
			}
		}
		if len(offlineNames) == 0 {
			sb = append(sb, fmt.Sprintf(
				"All %d infrastructure component%s reported online throughout the period.",
				len(data.InfraRows), plural(len(data.InfraRows))))
		} else {
			sb = append(sb, fmt.Sprintf(
				"%d of %d infrastructure component%s online — %s %s offline and should be investigated.",
				data.InfraOnline, len(data.InfraRows), plural(len(data.InfraRows)),
				strings.Join(offlineNames, ", "), isAre(len(offlineNames))))
		}
	}

	// ── Sentence 4: Containers ────────────────────────────────────────────
	if data.ContainersTotal > 0 {
		if data.UpdatesAvailable > 0 {
			sb = append(sb, fmt.Sprintf(
				"%d of %d container%s running with %d image update%s available to pull.",
				data.ContainersRunning, data.ContainersTotal, plural(data.ContainersTotal),
				data.UpdatesAvailable, plural(data.UpdatesAvailable)))
		} else {
			sb = append(sb, fmt.Sprintf(
				"%d of %d container%s running with no image updates pending.",
				data.ContainersRunning, data.ContainersTotal, plural(data.ContainersTotal)))
		}
	}

	// ── Sentence 5 (optional): App highlights ────────────────────────────
	if len(data.AppSections) > 0 {
		// Find the most active app
		var topApp *DigestAppSection
		for i := range data.AppSections {
			if topApp == nil || data.AppSections[i].TotalEvents > topApp.TotalEvents {
				topApp = &data.AppSections[i]
			}
		}
		if topApp != nil && topApp.TotalEvents > 0 {
			var catParts []string
			for _, c := range topApp.Categories {
				if c.Count > 0 {
					catParts = append(catParts, fmt.Sprintf("%d %s", c.Count, strings.ToLower(c.Label)))
				}
			}
			if len(catParts) > 0 {
				if len(data.AppSections) == 1 {
					sb = append(sb, fmt.Sprintf("%s logged %s this period.",
						topApp.AppName, strings.Join(catParts, ", ")))
				} else {
					others := len(data.AppSections) - 1
					sb = append(sb, fmt.Sprintf(
						"App highlights: %s logged %s — see App Activity below for all %d app%s.",
						topApp.AppName, strings.Join(catParts, ", "), len(data.AppSections), plural(len(data.AppSections))))
					_ = others
				}
			}
		}
	}

	summary = sb
	return
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// statusToUptimePctDigest mirrors dashboard.go's statusToUptimePct.
func statusToUptimePctDigest(status string) float64 {
	switch status {
	case "up":
		return 100.0
	case "warn":
		return 75.0
	default:
		return 0.0
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func nounPlural(noun string, n int) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}

func isAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

func trimContainerName(name string) string {
	name = strings.TrimPrefix(name, "/")
	if len(name) > 32 {
		return name[:32] + "…"
	}
	return name
}

func capList(s []string, max int) []string {
	if len(s) <= max {
		return s
	}
	return append(s[:max], fmt.Sprintf("…and %d more", len(s)-max))
}

var infraTypeLabelMap = map[string]string{
	"proxmox_node":  "Proxmox Node",
	"synology":      "Synology NAS",
	"vm":            "VM",
	"lxc":           "LXC",
	"bare_metal":    "Bare Metal",
	"linux_host":    "Linux Host",
	"windows_host":  "Windows Host",
	"generic_host":  "Generic Host",
	"docker_engine": "Docker Engine",
	"traefik":       "Traefik",
	"portainer":     "Portainer",
}

func infraTypeLabel(t string) string {
	if l, ok := infraTypeLabelMap[t]; ok {
		return l
	}
	return t
}

// liveRollupsForWindow queries the events table for all apps in [since, until).
// Used as a fallback when the profiler is unavailable.
func (d *DigestJob) liveRollupsForWindow(ctx context.Context, apps []models.App, since, until time.Time) ([]models.Rollup, error) {
	year := since.Year()
	month := int(since.Month())
	var result []models.Rollup
	for _, app := range apps {
		rows, err := d.store.Events.GroupByTypeAndLevel(ctx, app.ID, since, until)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			result = append(result, models.Rollup{
				AppID:     app.ID,
				Year:      year,
				Month:     month,
				EventType: row.EventType,
				Severity:  row.Level,
				Count:     row.Count,
			})
		}
	}
	return result, nil
}

// ── Period helpers ────────────────────────────────────────────────────────────

func isMonthlyPeriod(period string) bool {
	_, err := time.Parse("2006-01", period)
	return err == nil
}

func periodTimeRange(period string) (since, until time.Time) {
	if t, err := time.Parse("2006-01-02", period); err == nil {
		since = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		until = since.AddDate(0, 0, 1)
		return
	}
	var year, week int
	if n, _ := fmt.Sscanf(period, "%d-W%d", &year, &week); n == 2 {
		jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
		_, w := jan4.ISOWeek()
		monday := jan4.AddDate(0, 0, (week-w)*7-int(jan4.Weekday())+1)
		since = monday
		until = monday.AddDate(0, 0, 7)
		return
	}
	if t, err := time.Parse("2006-01", period); err == nil {
		since = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		until = since.AddDate(0, 1, 0)
		return
	}
	now := time.Now().UTC()
	since = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	until = since.AddDate(0, 1, 0)
	return
}

func periodYearMonth(period string) (int, int) {
	if t, err := time.Parse("2006-01-02", period); err == nil {
		return t.Year(), int(t.Month())
	}
	var year, week int
	if n, _ := fmt.Sscanf(period, "%d-W%d", &year, &week); n == 2 {
		jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
		_, w := jan4.ISOWeek()
		monday := jan4.AddDate(0, 0, (week-w)*7-int(jan4.Weekday())+1)
		return monday.Year(), int(monday.Month())
	}
	if t, err := time.Parse("2006-01", period); err == nil {
		return t.Year(), int(t.Month())
	}
	now := time.Now()
	return now.Year(), int(now.Month())
}

// adminEmails returns the digest recipient list.
func (d *DigestJob) adminEmails(ctx context.Context) ([]string, error) {
	smtp, err := d.smtpSettings(ctx)
	if err != nil {
		return nil, err
	}
	if smtp.To != "" {
		return []string{smtp.To}, nil
	}
	if smtp.From != "" {
		return []string{smtp.From}, nil
	}
	return nil, nil
}

// smtpSettings reads SMTP config from the settings table or environment.
func (d *DigestJob) smtpSettings(ctx context.Context) (*models.SMTPSettings, error) {
	var s models.SMTPSettings
	err := d.store.Settings.GetJSON(ctx, smtpSettingsKey, &s)
	if errors.Is(err, repo.ErrNotFound) {
		if d.config.SMTPHost == "" {
			return nil, fmt.Errorf("smtp not configured")
		}
		return &models.SMTPSettings{
			Host: d.config.SMTPHost,
			Port: d.config.SMTPPort,
			User: d.config.SMTPUser,
			Pass: d.config.SMTPPass,
			From: d.config.SMTPFrom,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	if s.Host == "" {
		return nil, fmt.Errorf("smtp not configured")
	}
	return &s, nil
}

func periodLabel(frequency string, t time.Time) string {
	switch frequency {
	case "daily":
		return t.Format("2006-01-02")
	case "weekly":
		_, week := t.ISOWeek()
		return fmt.Sprintf("%d-W%02d", t.Year(), week)
	default:
		return t.Format("2006-01")
	}
}

func subjectLine(period string) string {
	if t, err := time.Parse("2006-01-02", period); err == nil {
		return "NORA Digest — " + t.Format("Monday, January 2")
	}
	var year, week int
	if n, _ := fmt.Sscanf(period, "%d-W%d", &year, &week); n == 2 {
		jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
		_, w := jan4.ISOWeek()
		monday := jan4.AddDate(0, 0, (week-w)*7-int(jan4.Weekday())+1)
		sunday := monday.AddDate(0, 0, 6)
		return fmt.Sprintf("NORA Digest — Week of %s–%d", monday.Format("January 2"), sunday.Day())
	}
	if t, err := time.Parse("2006-01", period); err == nil {
		return "NORA Digest — " + t.Format("January 2006")
	}
	return "NORA Digest"
}

// StartDigestJob waits until the next whole hour, then runs every hour.
func StartDigestJob(ctx context.Context, job *DigestJob) {
	delay := durationUntilNextHour()
	log.Printf("digest: job waiting %s until next hour boundary", delay.Round(time.Minute))
	select {
	case <-ctx.Done():
		return
	case <-time.After(delay):
	}
	run := func() {
		if err := job.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("digest: job error: %v", err)
		}
	}
	run()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func durationUntilNextHour() time.Duration {
	now := time.Now()
	next := now.Truncate(time.Hour).Add(time.Hour)
	return next.Sub(now)
}

// ── HTML Templates ────────────────────────────────────────────────────────────

// statusDot returns an inline coloured circle character for a given status string.
// Used in both email and report templates via template functions (not needed —
// the templates handle this via {{if}} blocks instead, which is email-safe).

var digestHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title>
</head>
<body style="margin:0;padding:0;background:#0a0c0f;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;color:#c8d4e0;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#0a0c0f;">
  <tr><td align="center" style="padding:24px 16px;">
  <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%;">

    <!-- Header -->
    <tr><td style="background:#0f1215;border:1px solid #1e2530;border-radius:8px 8px 0 0;padding:20px 28px;border-bottom:none;">
      <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
        <tr>
          <td><span style="font-size:14px;font-weight:700;letter-spacing:0.1em;color:#3b82f6;font-family:monospace;">NORA</span></td>
          <td align="right"><span style="font-size:12px;color:#445566;font-family:monospace;">{{.Period}}</span></td>
        </tr>
      </table>
      <p style="margin:8px 0 0;font-size:20px;font-weight:600;color:#f1f5f9;">{{.Title}}</p>
    </td></tr>

    <!-- Narrative / Health banner -->
    <tr><td style="padding:0;">
      {{if eq .HealthStatus "healthy"}}
      <div style="background:#14532d;border-left:4px solid #22c55e;padding:16px 28px;border-right:1px solid #1e2530;">
        <p style="margin:0;font-size:15px;font-weight:600;color:#22c55e;">✓ {{.Headline}}</p>
      </div>
      {{else if eq .HealthStatus "warning"}}
      <div style="background:#713f12;border-left:4px solid #eab308;padding:16px 28px;border-right:1px solid #1e2530;">
        <p style="margin:0;font-size:15px;font-weight:600;color:#eab308;">⚠ {{.Headline}}</p>
      </div>
      {{else}}
      <div style="background:#7f1d1d;border-left:4px solid #ef4444;padding:16px 28px;border-right:1px solid #1e2530;">
        <p style="margin:0;font-size:15px;font-weight:600;color:#ef4444;">✗ {{.Headline}}</p>
      </div>
      {{end}}
    </td></tr>

    <!-- Summary sentences -->
    {{if .Summary}}
    <tr><td style="background:#0f1215;border:1px solid #1e2530;border-top:none;border-bottom:none;padding:16px 28px;">
      {{range .Summary}}
      <p style="margin:0 0 6px;font-size:14px;color:#7a8fa8;line-height:1.6;">{{.}}</p>
      {{end}}
    </td></tr>
    {{end}}

    <!-- Action items -->
    {{if .ActionItems}}
    <tr><td style="background:#0f1215;border:1px solid #1e2530;border-top:none;border-bottom:none;padding:4px 28px 16px;">
      <p style="margin:0 0 10px;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.08em;color:#445566;font-family:monospace;">Action Items</p>
      {{range .ActionItems}}
      <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin-bottom:6px;">
        <tr>
          <td width="8" style="vertical-align:top;padding-top:2px;">
            {{if eq .Priority "urgent"}}<span style="color:#ef4444;font-size:12px;">●</span>
            {{else if eq .Priority "review"}}<span style="color:#eab308;font-size:12px;">●</span>
            {{else}}<span style="color:#3b82f6;font-size:12px;">●</span>{{end}}
          </td>
          <td style="padding-left:8px;font-size:13px;color:#c8d4e0;line-height:1.5;">{{.Text}}</td>
        </tr>
      </table>
      {{end}}
    </td></tr>
    {{end}}

    <!-- Divider -->
    <tr><td style="background:#0f1215;border-left:1px solid #1e2530;border-right:1px solid #1e2530;padding:0 28px;">
      <div style="height:1px;background:#1e2530;"></div>
    </td></tr>

    <!-- Event counts -->
    <tr><td style="background:#0f1215;border:1px solid #1e2530;border-top:none;border-bottom:none;padding:16px 28px;">
      <p style="margin:0 0 12px;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.08em;color:#445566;font-family:monospace;">Events This Period</p>
      <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
        <tr>
          <td align="center" style="padding:0 6px;">
            <div style="font-size:26px;font-weight:700;color:#3b82f6;font-family:monospace;">{{.EventInfo}}</div>
            <div style="font-size:10px;color:#445566;text-transform:uppercase;letter-spacing:0.08em;font-family:monospace;margin-top:2px;">Info</div>
          </td>
          <td align="center" style="padding:0 6px;">
            <div style="font-size:26px;font-weight:700;{{if gt .EventWarn 0}}color:#eab308;{{else}}color:#445566;{{end}}font-family:monospace;">{{.EventWarn}}</div>
            <div style="font-size:10px;color:#445566;text-transform:uppercase;letter-spacing:0.08em;font-family:monospace;margin-top:2px;">Warn</div>
          </td>
          <td align="center" style="padding:0 6px;">
            <div style="font-size:26px;font-weight:700;{{if gt .EventError 0}}color:#ef4444;{{else}}color:#445566;{{end}}font-family:monospace;">{{.EventError}}</div>
            <div style="font-size:10px;color:#445566;text-transform:uppercase;letter-spacing:0.08em;font-family:monospace;margin-top:2px;">Error</div>
          </td>
          <td align="center" style="padding:0 6px;">
            <div style="font-size:26px;font-weight:700;{{if gt .EventCritical 0}}color:#ef4444;{{else}}color:#445566;{{end}}font-family:monospace;">{{.EventCritical}}</div>
            <div style="font-size:10px;color:#445566;text-transform:uppercase;letter-spacing:0.08em;font-family:monospace;margin-top:2px;">Critical</div>
          </td>
        </tr>
      </table>
    </td></tr>

    <!-- Monitor checks -->
    <tr><td style="background:#0f1215;border:1px solid #1e2530;border-top:none;border-bottom:none;padding:16px 28px;">
      <p style="margin:0 0 12px;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.08em;color:#445566;font-family:monospace;">Monitor Checks</p>
      <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
        {{range .CheckGroups}}
        <tr style="border-bottom:1px solid #1e2530;">
          <td style="padding:8px 0;font-size:12px;font-weight:600;font-family:monospace;color:#7a8fa8;text-transform:uppercase;width:50px;">{{.Type}}</td>
          <td style="padding:8px 0;font-size:18px;font-weight:700;font-family:monospace;width:80px;
            {{if eq .Status "healthy"}}color:#22c55e;
            {{else if eq .Status "warning"}}color:#eab308;
            {{else if eq .Status "down"}}color:#ef4444;
            {{else}}color:#445566;{{end}}">
            {{if eq .Status "none"}}—{{else}}{{printf "%.1f" .AvgUptime}}%{{end}}
          </td>
          <td style="padding:8px 0;font-size:12px;color:#445566;font-family:monospace;">
            {{if eq .Status "none"}}no checks configured
            {{else}}{{.Total}} check{{if ne .Total 1}}s{{end}}{{if gt .NotUp 0}} · {{.NotUp}} not up{{end}}{{end}}
          </td>
          <td align="right" style="padding:8px 0;">
            {{if eq .Status "healthy"}}<span style="color:#22c55e;font-size:14px;">●</span>
            {{else if eq .Status "warning"}}<span style="color:#eab308;font-size:14px;">▲</span>
            {{else if eq .Status "down"}}<span style="color:#ef4444;font-size:14px;">✗</span>
            {{else}}<span style="color:#1e2530;font-size:14px;">○</span>{{end}}
          </td>
        </tr>
        {{end}}
      </table>
    </td></tr>

    <!-- App activity -->
    {{if .AppSections}}
    <tr><td style="background:#0f1215;border:1px solid #1e2530;border-top:none;border-bottom:none;padding:16px 28px;">
      <p style="margin:0 0 12px;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.08em;color:#445566;font-family:monospace;">App Activity</p>
      {{range .AppSections}}
      <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin-bottom:14px;">
        <tr>
          <td colspan="2" style="padding:6px 0 4px;border-bottom:1px solid #1e2530;">
            <span style="font-size:13px;font-weight:600;color:#c8d4e0;">{{.AppName}}</span>
            {{if .ProfileName}}<span style="font-size:11px;color:#445566;margin-left:6px;font-family:monospace;">{{.ProfileName}}</span>{{end}}
          </td>
        </tr>
        {{range .Categories}}
        <tr>
          <td style="padding:5px 0 5px 12px;font-size:13px;color:#7a8fa8;">{{.Label}}</td>
          <td align="right" style="padding:5px 0;font-size:14px;font-weight:600;font-family:monospace;{{if and .IsError (gt .Count 0)}}color:#ef4444;{{else if gt .Count 0}}color:#3b82f6;{{else}}color:#445566;{{end}}">{{.Count}}</td>
        </tr>
        {{end}}
        {{if eq .TotalEvents 0}}
        <tr><td colspan="2" style="padding:5px 0 5px 12px;font-size:12px;color:#445566;font-style:italic;">No activity this period</td></tr>
        {{end}}
      </table>
      {{end}}
    </td></tr>
    {{end}}

    <!-- Services (monitor_only / docker_only — no digest data) -->
    {{if .ServiceSections}}
    <tr><td style="background:#0f1215;border:1px solid #1e2530;border-top:none;border-bottom:none;padding:16px 28px;">
      <p style="margin:0 0 12px;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.08em;color:#445566;font-family:monospace;">Services</p>
      <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
        {{range .ServiceSections}}
        <tr>
          <td style="padding:5px 0;font-size:13px;color:#c8d4e0;">{{.AppName}}</td>
          <td style="padding:5px 0;font-size:11px;color:#445566;font-family:monospace;">{{.ProfileName}}</td>
          <td align="right" style="padding:5px 0;font-size:11px;font-family:monospace;color:#445566;">monitor only</td>
        </tr>
        {{end}}
      </table>
      <p style="margin:10px 0 0;font-size:11px;color:#445566;font-style:italic;">These services are monitored for uptime but do not emit webhook events.</p>
    </td></tr>
    {{end}}

    <!-- Infrastructure -->
    {{if .InfraRows}}
    <tr><td style="background:#0f1215;border:1px solid #1e2530;border-top:none;border-bottom:none;padding:16px 28px;">
      <p style="margin:0 0 12px;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.08em;color:#445566;font-family:monospace;">Infrastructure
        <span style="font-weight:400;color:#22c55e;margin-left:8px;">{{.InfraOnline}} online</span>
        {{if gt .InfraOffline 0}}<span style="font-weight:400;color:#ef4444;margin-left:8px;">{{.InfraOffline}} offline</span>{{end}}
      </p>
      <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
        {{range .InfraRows}}
        <tr>
          <td style="padding:5px 0;font-size:13px;color:#c8d4e0;">{{.Name}}</td>
          <td style="padding:5px 0;font-size:11px;color:#445566;font-family:monospace;">{{.Type}}</td>
          <td align="right" style="padding:5px 0;font-size:12px;font-family:monospace;
            {{if eq .Status "online"}}color:#22c55e;
            {{else if eq .Status "degraded"}}color:#eab308;
            {{else}}color:#ef4444;{{end}}">{{.Status}}</td>
        </tr>
        {{end}}
      </table>
    </td></tr>
    {{end}}

    <!-- Containers -->
    {{if gt .ContainersTotal 0}}
    <tr><td style="background:#0f1215;border:1px solid #1e2530;border-top:none;border-bottom:none;padding:16px 28px;">
      <p style="margin:0 0 10px;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.08em;color:#445566;font-family:monospace;">Containers</p>
      <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
        <tr>
          <td style="padding:4px 0;font-size:13px;color:#7a8fa8;">Running</td>
          <td align="right" style="padding:4px 0;font-size:14px;font-weight:600;font-family:monospace;color:#22c55e;">{{.ContainersRunning}} / {{.ContainersTotal}}</td>
        </tr>
        <tr>
          <td style="padding:4px 0;font-size:13px;color:#7a8fa8;">Image updates available</td>
          <td align="right" style="padding:4px 0;font-size:14px;font-weight:600;font-family:monospace;{{if gt .UpdatesAvailable 0}}color:#eab308;{{else}}color:#445566;{{end}}">{{.UpdatesAvailable}}</td>
        </tr>
        {{if .NewContainers}}
        <tr>
          <td colspan="2" style="padding:6px 0 2px;font-size:12px;color:#445566;font-family:monospace;">New this period: {{range $i, $n := .NewContainers}}{{if $i}}, {{end}}{{$n}}{{end}}</td>
        </tr>
        {{end}}
      </table>
    </td></tr>
    {{end}}

    <!-- Resource warnings -->
    {{if .ResourceWarnings}}
    <tr><td style="background:#0f1215;border:1px solid #1e2530;border-top:none;border-bottom:none;padding:16px 28px;">
      <p style="margin:0 0 10px;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.08em;color:#445566;font-family:monospace;">Resource Warnings</p>
      <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
        {{range .ResourceWarnings}}
        <tr>
          <td style="padding:5px 0;font-size:13px;color:#c8d4e0;">{{.ComponentName}}</td>
          <td style="padding:5px 0;font-size:11px;color:#7a8fa8;font-family:monospace;">{{.Metric}}</td>
          <td align="right" style="padding:5px 0;font-size:12px;font-family:monospace;color:#eab308;">avg {{printf "%.0f" .AvgPct}}% · peak {{printf "%.0f" .MaxPct}}%</td>
        </tr>
        {{end}}
      </table>
    </td></tr>
    {{end}}

    <!-- Footer -->
    <tr><td style="background:#0a0c0f;border:1px solid #1e2530;border-top:none;border-radius:0 0 8px 8px;padding:14px 28px;">
      <p style="margin:0;font-size:12px;color:#1e2530;text-align:center;font-family:monospace;">
        NORA · Nexus Operations Recon &amp; Alerts
      </p>
    </td></tr>

  </table>
  </td></tr>
</table>
</body>
</html>`

// reportHTMLTemplate is the browser/print-friendly report template.
var reportHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title>
<style>
  *{box-sizing:border-box;margin:0;padding:0;}
  body{background:#0a0c0f;color:#c8d4e0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;padding:32px 16px;}
  .report{max-width:720px;margin:0 auto;}
  .header{display:flex;align-items:baseline;justify-content:space-between;padding-bottom:16px;border-bottom:1px solid #1e2530;margin-bottom:24px;}
  .brand{font-size:13px;font-weight:700;letter-spacing:0.1em;color:#3b82f6;font-family:monospace;}
  .title{font-size:22px;font-weight:600;color:#f1f5f9;margin-top:4px;}
  .period{font-size:12px;color:#445566;font-family:monospace;}
  .banner{border-radius:6px;padding:14px 20px;margin-bottom:20px;}
  .banner.healthy{background:#14532d;border-left:4px solid #22c55e;}
  .banner.warning{background:#713f12;border-left:4px solid #eab308;}
  .banner.critical{background:#7f1d1d;border-left:4px solid #ef4444;}
  .banner-headline{font-size:15px;font-weight:600;margin-bottom:8px;}
  .banner.healthy .banner-headline{color:#22c55e;}
  .banner.warning .banner-headline{color:#eab308;}
  .banner.critical .banner-headline{color:#ef4444;}
  .summary-text{font-size:13px;color:#7a8fa8;line-height:1.7;margin-bottom:4px;}
  .section{margin-bottom:28px;}
  .section-label{font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.08em;color:#445566;font-family:monospace;margin-bottom:12px;}
  .action-list{list-style:none;}
  .action-list li{display:flex;gap:10px;align-items:flex-start;margin-bottom:8px;font-size:13px;line-height:1.5;}
  .dot-urgent{color:#ef4444;flex-shrink:0;}
  .dot-review{color:#eab308;flex-shrink:0;}
  .dot-info{color:#3b82f6;flex-shrink:0;}
  .event-grid{display:grid;grid-template-columns:repeat(4,1fr);gap:12px;}
  .event-cell{background:#0f1215;border:1px solid #1e2530;border-radius:8px;padding:14px;text-align:center;}
  .event-value{font-size:28px;font-weight:700;font-family:monospace;line-height:1;}
  .event-label{font-size:10px;text-transform:uppercase;letter-spacing:0.08em;color:#445566;font-family:monospace;margin-top:4px;}
  /* ── Check cards ── */
  .check-grid{display:grid;grid-template-columns:repeat(4,1fr);gap:12px;}
  .check-card{background:#0f1215;border:1px solid #1e2530;border-radius:8px;padding:16px 14px;border-top:3px solid;}
  .check-card.healthy{border-top-color:#22c55e;}
  .check-card.warning{border-top-color:#eab308;}
  .check-card.down{border-top-color:#ef4444;}
  .check-card.none{border-top-color:#3b82f6;}
  .check-card-type{font-size:11px;font-weight:700;text-transform:uppercase;letter-spacing:0.08em;color:#445566;font-family:monospace;margin-bottom:8px;}
  .check-card-uptime{font-size:24px;font-weight:700;font-family:monospace;line-height:1;margin-bottom:6px;}
  .check-card.healthy .check-card-uptime{color:#22c55e;}
  .check-card.warning .check-card-uptime{color:#eab308;}
  .check-card.down   .check-card-uptime{color:#ef4444;}
  .check-card.none   .check-card-uptime{color:#445566;}
  .check-card-meta{font-size:11px;color:#445566;font-family:monospace;line-height:1.4;}
  .check-card-notup{color:#ef4444;}
  .check-card.warning .check-card-notup{color:#eab308;}
  /* ── App widgets ── */
  .app-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:12px;}
  .app-widget{background:#0f1215;border:1px solid #1e2530;border-radius:8px;overflow:hidden;}
  .app-widget-header{padding:10px 14px;border-bottom:1px solid #1e2530;display:flex;align-items:center;justify-content:space-between;gap:8px;}
  .app-widget-name{font-weight:600;font-size:13px;color:#c8d4e0;}
  .app-widget-badge{font-size:10px;font-family:monospace;color:#445566;background:#141820;border:1px solid #1e2530;border-radius:4px;padding:1px 5px;}
  .cat-row{display:flex;justify-content:space-between;align-items:center;padding:7px 14px;border-bottom:1px solid #1e2530;}
  .cat-row:last-child{border-bottom:none;}
  .cat-label{font-size:12px;color:#7a8fa8;}
  .cat-count{font-family:monospace;font-size:13px;font-weight:600;}
  .cat-count.has-errors{color:#ef4444;}
  .cat-count.has-activity{color:#3b82f6;}
  .cat-count.none{color:#445566;}
  /* ── Infra cards ── */
  .infra-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:10px;}
  .infra-card{background:#0f1215;border:1px solid #1e2530;border-radius:8px;padding:12px 14px;display:flex;align-items:center;justify-content:space-between;gap:8px;}
  .infra-card-left{}
  .infra-card-name{font-size:13px;font-weight:600;color:#c8d4e0;margin-bottom:3px;}
  .infra-card-type{font-size:11px;font-family:monospace;color:#445566;}
  .infra-dot{width:8px;height:8px;border-radius:50%;flex-shrink:0;}
  .infra-dot.online{background:#22c55e;}
  .infra-dot.degraded{background:#eab308;}
  .infra-dot.offline,.infra-dot.unknown{background:#ef4444;}
  /* ── Containers ── */
  .container-grid{display:grid;grid-template-columns:1fr 1fr;gap:12px;}
  .container-cell{background:#0f1215;border:1px solid #1e2530;border-radius:8px;padding:14px;}
  .container-value{font-size:22px;font-weight:700;font-family:monospace;}
  .container-label{font-size:11px;text-transform:uppercase;letter-spacing:0.08em;color:#445566;font-family:monospace;margin-top:4px;}
  .container-sub{font-size:11px;color:#445566;font-family:monospace;margin-top:6px;line-height:1.5;}
  /* ── Resource warnings ── */
  .warn-table{width:100%;border-collapse:collapse;}
  .warn-table td{padding:8px 0;border-bottom:1px solid #1e2530;font-size:13px;}
  /* ── Print button ── */
  .print-btn{display:inline-flex;align-items:center;gap:8px;padding:8px 18px;background:#1c2028;border:1px solid #1e2530;border-radius:6px;color:#c8d4e0;font-size:13px;cursor:pointer;margin-bottom:24px;}
  .print-btn:hover{background:#252d38;}
  @media print{
    body{background:#fff;color:#000;padding:0;}
    .print-btn{display:none;}
    .banner.healthy{background:#d1fae5;border-left-color:#16a34a;}
    .banner.warning{background:#fef9c3;border-left-color:#ca8a04;}
    .banner.critical{background:#fee2e2;border-left-color:#dc2626;}
    .banner.healthy .banner-headline{color:#16a34a;}
    .banner.warning .banner-headline{color:#ca8a04;}
    .banner.critical .banner-headline{color:#dc2626;}
    .check-card,.app-widget,.infra-card,.container-cell,.event-cell{border-color:#ddd;background:#fff;}
    .section-label,.check-card-meta,.infra-card-type,.container-label,.app-widget-badge{color:#666;}
    .summary-text,.cat-label,.infra-card-name{color:#333;}
    .cat-count.none,.check-card.none .check-card-uptime{color:#999;}
  }
</style>
</head>
<body>
<div class="report">
  <button class="print-btn" onclick="window.print()">⎙ Print / Save as PDF</button>

  <div class="header">
    <div>
      <div class="brand">NORA</div>
      <div class="title">{{.Title}}</div>
    </div>
    <div class="period">{{.Period}}</div>
  </div>

  <!-- Health banner + narrative -->
  <div class="banner {{.HealthStatus}}">
    <div class="banner-headline">{{if eq .HealthStatus "healthy"}}✓{{else if eq .HealthStatus "warning"}}⚠{{else}}✗{{end}} {{.Headline}}</div>
    {{range .Summary}}<p class="summary-text">{{.}}</p>{{end}}
  </div>

  <!-- Action items -->
  {{if .ActionItems}}
  <div class="section">
    <div class="section-label">Action Items</div>
    <ul class="action-list">
      {{range .ActionItems}}
      <li>
        {{if eq .Priority "urgent"}}<span class="dot-urgent">●</span>
        {{else if eq .Priority "review"}}<span class="dot-review">●</span>
        {{else}}<span class="dot-info">●</span>{{end}}
        <span>{{.Text}}</span>
      </li>
      {{end}}
    </ul>
  </div>
  {{end}}

  <!-- Event counts -->
  <div class="section">
    <div class="section-label">Events This Period</div>
    <div class="event-grid">
      <div class="event-cell">
        <div class="event-value" style="color:#3b82f6;">{{.EventInfo}}</div>
        <div class="event-label">Info</div>
      </div>
      <div class="event-cell">
        <div class="event-value" style="{{if gt .EventWarn 0}}color:#eab308;{{else}}color:#445566;{{end}}">{{.EventWarn}}</div>
        <div class="event-label">Warn</div>
      </div>
      <div class="event-cell">
        <div class="event-value" style="{{if gt .EventError 0}}color:#ef4444;{{else}}color:#445566;{{end}}">{{.EventError}}</div>
        <div class="event-label">Error</div>
      </div>
      <div class="event-cell">
        <div class="event-value" style="{{if gt .EventCritical 0}}color:#ef4444;{{else}}color:#445566;{{end}}">{{.EventCritical}}</div>
        <div class="event-label">Critical</div>
      </div>
    </div>
  </div>

  <!-- Monitor checks — 4-card grid -->
  <div class="section">
    <div class="section-label">Monitor Checks</div>
    <div class="check-grid">
      {{range .CheckGroups}}
      <div class="check-card {{.Status}}">
        <div class="check-card-type">{{.Type}}</div>
        <div class="check-card-uptime">{{if eq .Status "none"}}—{{else}}{{printf "%.1f" .AvgUptime}}%{{end}}</div>
        <div class="check-card-meta">
          {{if eq .Status "none"}}no checks configured
          {{else}}{{.Total}} check{{if ne .Total 1}}s{{end}}{{if gt .NotUp 0}}<br><span class="check-card-notup">{{.NotUp}} not up</span>{{end}}
          {{end}}
        </div>
      </div>
      {{end}}
    </div>
  </div>

  <!-- App activity — widget grid -->
  {{if .AppSections}}
  <div class="section">
    <div class="section-label">App Activity</div>
    <div class="app-grid">
      {{range .AppSections}}
      <div class="app-widget">
        <div class="app-widget-header">
          <span class="app-widget-name">{{.AppName}}</span>
          {{if .ProfileName}}<span class="app-widget-badge">{{.ProfileName}}</span>{{end}}
        </div>
        {{range .Categories}}
        <div class="cat-row">
          <span class="cat-label">{{.Label}}</span>
          <span class="cat-count {{if and .IsError (gt .Count 0)}}has-errors{{else if gt .Count 0}}has-activity{{else}}none{{end}}">{{.Count}}</span>
        </div>
        {{end}}
        {{if eq .TotalEvents 0}}
        <div class="cat-row"><span class="cat-label" style="font-style:italic;font-size:11px;">No activity this period</span></div>
        {{end}}
      </div>
      {{end}}
    </div>
  </div>
  {{end}}

  <!-- Services — monitor_only / docker_only -->
  {{if .ServiceSections}}
  <div class="section">
    <div class="section-label">Services</div>
    <div class="app-grid">
      {{range .ServiceSections}}
      <div class="app-widget">
        <div class="app-widget-header">
          <span class="app-widget-name">{{.AppName}}</span>
          {{if .ProfileName}}<span class="app-widget-badge">{{.ProfileName}}</span>{{end}}
        </div>
        <div class="cat-row">
          <span class="cat-label" style="font-style:italic;">Monitor only — no event data</span>
        </div>
      </div>
      {{end}}
    </div>
  </div>
  {{end}}

  <!-- Infrastructure — card grid -->
  {{if .InfraRows}}
  <div class="section">
    <div class="section-label">Infrastructure — {{.InfraOnline}} online{{if gt .InfraOffline 0}}, {{.InfraOffline}} offline{{end}}</div>
    <div class="infra-grid">
      {{range .InfraRows}}
      <div class="infra-card">
        <div class="infra-card-left">
          <div class="infra-card-name">{{.Name}}</div>
          <div class="infra-card-type">{{.Type}}</div>
        </div>
        <div class="infra-dot {{.Status}}"></div>
      </div>
      {{end}}
    </div>
  </div>
  {{end}}

  <!-- Containers -->
  {{if gt .ContainersTotal 0}}
  <div class="section">
    <div class="section-label">Containers</div>
    <div class="container-grid">
      <div class="container-cell">
        <div class="container-value" style="color:#22c55e;">{{.ContainersRunning}}<span style="font-size:14px;color:#445566;"> / {{.ContainersTotal}}</span></div>
        <div class="container-label">Running</div>
      </div>
      <div class="container-cell">
        <div class="container-value" style="{{if gt .UpdatesAvailable 0}}color:#eab308;{{else}}color:#445566;{{end}}">{{.UpdatesAvailable}}</div>
        <div class="container-label">Updates Available</div>
        {{if .NewContainers}}<div class="container-sub">New: {{range $i,$n := .NewContainers}}{{if $i}}, {{end}}{{$n}}{{end}}</div>{{end}}
      </div>
    </div>
  </div>
  {{end}}

  <!-- Resource warnings -->
  {{if .ResourceWarnings}}
  <div class="section">
    <div class="section-label">Resource Warnings</div>
    <table class="warn-table">
      {{range .ResourceWarnings}}
      <tr>
        <td style="color:#c8d4e0;">{{.ComponentName}}</td>
        <td style="font-family:monospace;font-size:11px;color:#7a8fa8;">{{.Metric}}</td>
        <td align="right" style="font-family:monospace;font-size:12px;color:#eab308;">avg {{printf "%.0f" .AvgPct}}% · peak {{printf "%.0f" .MaxPct}}%</td>
      </tr>
      {{end}}
    </table>
  </div>
  {{end}}

</div>
</body>
</html>`
