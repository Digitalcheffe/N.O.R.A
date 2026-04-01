package jobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"sort"
	"time"

	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// digestScheduleKey is the settings table key for the digest schedule config.
const digestScheduleKey = "digest_schedule"

// smtpSettingsKey is the settings table key for SMTP configuration.
const smtpSettingsKey = "smtp"

// DigestData holds all data passed to the HTML template.
type DigestData struct {
	Title      string
	Period     string
	TotalDownloads int
	TotalBackups   int
	TotalUpdates   int
	TotalErrors    int
	UptimePct      float64
	AppRows    []DigestAppRow
}

// DigestAppRow is a per-app summary line in the email.
type DigestAppRow struct {
	AppName    string
	EventType  string
	Count      int
	HasErrors  bool
}

// DigestJob generates and sends the NORA digest email.
type DigestJob struct {
	store  *repo.Store
	config *config.Config
}

// NewDigestJob creates a DigestJob.
func NewDigestJob(store *repo.Store, cfg *config.Config) *DigestJob {
	return &DigestJob{store: store, config: cfg}
}

// Run is called every hour. It reads the stored schedule and decides whether
// to send based on the configured frequency, day, and send_hour.
func (d *DigestJob) Run(ctx context.Context) error {
	var schedule models.DigestSchedule
	err := d.store.Settings.GetJSON(ctx, digestScheduleKey, &schedule)
	if errors.Is(err, repo.ErrNotFound) {
		// No schedule stored — use the default (monthly, 1st, 08:00).
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
// Period formats:
//
//	daily   → "2026-03-27"
//	weekly  → "2026-W13"
//	monthly → "2026-03"
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
		log.Printf("digest: no admin users found, skipping send")
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
		// Do not return error — a failed send should not crash the job.
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
// print-friendly report HTML. Used by the report export endpoint.
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

// SMTPConfigured returns true if SMTP is configured either in the settings
// table or via environment-level config.
func (d *DigestJob) SMTPConfigured(ctx context.Context) bool {
	_, err := d.smtpSettings(ctx)
	return err == nil
}

// buildDigestData queries rollups and assembles DigestData for the given period.
//
// For monthly periods it tries the rollups table first (pre-aggregated data from
// past months) and falls back to querying events live when the month is still in
// progress. Daily and weekly periods always query events live because the rollup
// table only stores monthly aggregates.
func (d *DigestJob) buildDigestData(ctx context.Context, period string) (*DigestData, error) {
	// Fetch app list — needed regardless of data source.
	apps, err := d.store.Apps.List(ctx)
	if err != nil {
		return nil, err
	}

	appNames := map[string]string{}
	for _, a := range apps {
		appNames[a.ID] = a.Name
	}

	var rollups []models.Rollup

	// Monthly periods can use pre-rolled-up data; daily/weekly must go live.
	isMonthly := isMonthlyPeriod(period)
	if isMonthly {
		year, month := periodYearMonth(period)
		rollups, err = d.store.Rollups.ListByPeriod(ctx, year, month)
		if err != nil {
			return nil, err
		}
	}

	// No rollup data? Query events directly for the exact period window.
	if len(rollups) == 0 {
		since, until := periodTimeRange(period)
		rollups, err = d.liveRollupsForWindow(ctx, apps, since, until)
		if err != nil {
			return nil, err
		}
	}

	// Aggregate per (app, event_type).
	type appKey struct {
		appID     string
		eventType string
	}
	counts := map[appKey]int{}
	var totalErrors int

	for _, r := range rollups {
		counts[appKey{r.AppID, r.EventType}] += r.Count
		if r.Severity == "error" || r.Severity == "critical" {
			totalErrors += r.Count
		}
	}

	var rows []DigestAppRow
	for k, count := range counts {
		rows = append(rows, DigestAppRow{
			AppName:   appNames[k.appID],
			EventType: k.eventType,
			Count:     count,
			HasErrors: k.eventType == "error" || k.eventType == "critical",
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].AppName != rows[j].AppName {
			return rows[i].AppName < rows[j].AppName
		}
		return rows[i].EventType < rows[j].EventType
	})

	title := subjectLine(period)
	return &DigestData{
		Title:       title,
		Period:      period,
		TotalErrors: totalErrors,
		AppRows:     rows,
	}, nil
}

// liveRollupsForWindow queries the events table for all apps over [since, until)
// and returns rollup-equivalent rows. Used when the rollups table has no data.
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

// isMonthlyPeriod returns true if the period string is in YYYY-MM format.
func isMonthlyPeriod(period string) bool {
	_, err := time.Parse("2006-01", period)
	return err == nil
}

// periodTimeRange returns the [since, until) time bounds for any period label.
//
//	daily   "2026-04-01"  → [2026-04-01 00:00 UTC, 2026-04-02 00:00 UTC)
//	weekly  "2026-W14"    → [Monday 00:00 UTC, next Monday 00:00 UTC)
//	monthly "2026-04"     → [2026-04-01 00:00 UTC, 2026-05-01 00:00 UTC)
func periodTimeRange(period string) (since, until time.Time) {
	// Daily
	if t, err := time.Parse("2006-01-02", period); err == nil {
		since = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		until = since.AddDate(0, 0, 1)
		return
	}
	// Weekly "2026-W14" → Monday of that ISO week
	var year, week int
	if n, _ := fmt.Sscanf(period, "%d-W%d", &year, &week); n == 2 {
		jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
		_, w := jan4.ISOWeek()
		monday := jan4.AddDate(0, 0, (week-w)*7-int(jan4.Weekday())+1)
		since = monday
		until = monday.AddDate(0, 0, 7)
		return
	}
	// Monthly
	if t, err := time.Parse("2006-01", period); err == nil {
		since = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		until = since.AddDate(0, 1, 0)
		return
	}
	// Fallback: current month
	now := time.Now().UTC()
	since = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	until = since.AddDate(0, 1, 0)
	return
}

// adminEmails returns the digest recipient list.
// Uses the dedicated "to" address when set, falling back to "from".
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

// smtpSettings reads SMTP config from the settings table, falling back to
// environment-level config values.
func (d *DigestJob) smtpSettings(ctx context.Context) (*models.SMTPSettings, error) {
	var s models.SMTPSettings
	err := d.store.Settings.GetJSON(ctx, smtpSettingsKey, &s)
	if errors.Is(err, repo.ErrNotFound) {
		// Fall back to env-configured values.
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

// periodLabel returns the period label string for the given frequency and date.
func periodLabel(frequency string, t time.Time) string {
	switch frequency {
	case "daily":
		return t.Format("2006-01-02")
	case "weekly":
		_, week := t.ISOWeek()
		return fmt.Sprintf("%d-W%02d", t.Year(), week)
	default: // monthly
		return t.Format("2006-01")
	}
}

// periodYearMonth extracts year + month from a period label.
// Supports all three formats: "2026-03-27", "2026-W13", "2026-03".
func periodYearMonth(period string) (int, int) {
	// Try daily: "2026-03-27"
	if t, err := time.Parse("2006-01-02", period); err == nil {
		return t.Year(), int(t.Month())
	}
	// Try weekly: "2026-W13" → find the Monday of that week.
	var year, week int
	if n, _ := fmt.Sscanf(period, "%d-W%d", &year, &week); n == 2 {
		// Monday of ISO week.
		jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
		_, w := jan4.ISOWeek()
		monday := jan4.AddDate(0, 0, (week-w)*7-int(jan4.Weekday())+1)
		return monday.Year(), int(monday.Month())
	}
	// Monthly: "2026-03"
	if t, err := time.Parse("2006-01", period); err == nil {
		return t.Year(), int(t.Month())
	}
	// Fallback to current month.
	now := time.Now()
	return now.Year(), int(now.Month())
}

// subjectLine returns the email subject for a given period label.
func subjectLine(period string) string {
	// Try daily: "2026-03-27"
	if t, err := time.Parse("2006-01-02", period); err == nil {
		return "Your Homelab — " + t.Format("Monday, January 2")
	}
	// Try weekly: "2026-W13"
	var year, week int
	if n, _ := fmt.Sscanf(period, "%d-W%d", &year, &week); n == 2 {
		jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
		_, w := jan4.ISOWeek()
		monday := jan4.AddDate(0, 0, (week-w)*7-int(jan4.Weekday())+1)
		sunday := monday.AddDate(0, 0, 6)
		return fmt.Sprintf("Your Homelab — Week of %s–%d",
			monday.Format("January 2"), sunday.Day())
	}
	// Monthly: "2026-03"
	if t, err := time.Parse("2006-01", period); err == nil {
		return "Your Homelab — " + t.Format("January 2006")
	}
	return "Your Homelab Digest"
}

// StartDigestJob waits until the next whole hour boundary, then calls
// DigestJob.Run every hour. Run reads the stored send_hour and decides
// whether this is the right time to fire.
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

// durationUntilNextHour returns the duration from now until the next whole hour (HH:00:00).
func durationUntilNextHour() time.Duration {
	now := time.Now()
	next := now.Truncate(time.Hour).Add(time.Hour)
	return next.Sub(now)
}

// digestHTMLTemplate is the inline-CSS HTML email template.
var digestHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title>
</head>
<body style="margin:0;padding:0;background-color:#0a0c0f;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:#0a0c0f;">
  <tr>
    <td align="center" style="padding:24px 16px;">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%;">

        <!-- Header -->
        <tr>
          <td style="background-color:#111318;border-radius:8px 8px 0 0;padding:24px 32px;border-bottom:1px solid #1e2330;">
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
              <tr>
                <td>
                  <span style="font-size:18px;font-weight:700;color:#e2e8f0;letter-spacing:0.05em;">NORA</span>
                </td>
                <td align="right">
                  <span style="font-size:13px;color:#64748b;">{{.Period}}</span>
                </td>
              </tr>
            </table>
            <p style="margin:8px 0 0;font-size:20px;font-weight:600;color:#f1f5f9;">{{.Title}}</p>
          </td>
        </tr>

        <!-- Summary row -->
        <tr>
          <td style="background-color:#111318;padding:20px 32px;border-bottom:1px solid #1e2330;">
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
              <tr>
                {{if .TotalErrors}}
                <td align="center" style="padding:0 8px;">
                  <div style="font-size:22px;font-weight:700;color:#f87171;">{{.TotalErrors}}</div>
                  <div style="font-size:11px;color:#94a3b8;margin-top:2px;text-transform:uppercase;letter-spacing:0.08em;">Errors</div>
                </td>
                {{end}}
                <td align="center" style="padding:0 8px;">
                  <div style="font-size:22px;font-weight:700;color:#38bdf8;">{{len .AppRows}}</div>
                  <div style="font-size:11px;color:#94a3b8;margin-top:2px;text-transform:uppercase;letter-spacing:0.08em;">Events</div>
                </td>
              </tr>
            </table>
          </td>
        </tr>

        <!-- Per-app breakdown -->
        {{if .AppRows}}
        <tr>
          <td style="background-color:#111318;padding:20px 32px;border-bottom:1px solid #1e2330;">
            <p style="margin:0 0 12px;font-size:13px;font-weight:600;color:#94a3b8;text-transform:uppercase;letter-spacing:0.08em;">Activity</p>
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
              {{range .AppRows}}
              <tr>
                <td style="padding:6px 0;border-bottom:1px solid #1e2330;">
                  <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
                    <tr>
                      <td style="font-size:14px;color:#e2e8f0;">{{.AppName}}</td>
                      <td style="font-size:12px;color:#64748b;text-align:center;">{{.EventType}}</td>
                      <td align="right">
                        <span style="font-size:14px;font-weight:600;{{if .HasErrors}}color:#f87171;{{else}}color:#38bdf8;{{end}}">{{.Count}}</span>
                      </td>
                    </tr>
                  </table>
                </td>
              </tr>
              {{end}}
            </table>
          </td>
        </tr>
        {{else}}
        <tr>
          <td style="background-color:#111318;padding:20px 32px;border-bottom:1px solid #1e2330;">
            <p style="margin:0;font-size:14px;color:#64748b;text-align:center;">No activity recorded for this period.</p>
          </td>
        </tr>
        {{end}}

        <!-- Footer -->
        <tr>
          <td style="background-color:#0d1117;border-radius:0 0 8px 8px;padding:16px 32px;">
            <p style="margin:0;font-size:12px;color:#475569;text-align:center;">
              Sent by NORA &middot;
              <a href="#" style="color:#38bdf8;text-decoration:none;">Manage digest settings</a>
            </p>
          </td>
        </tr>

      </table>
    </td>
  </tr>
</table>
</body>
</html>`

// reportHTMLTemplate is the browser/print-friendly report template.
// It includes print CSS and a screen-only print button.
var reportHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { background: #0a0c0f; color: #e2e8f0; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; padding: 32px 16px; }
  .report { max-width: 680px; margin: 0 auto; }
  .report-header { display: flex; align-items: baseline; justify-content: space-between; padding-bottom: 16px; border-bottom: 1px solid #1e2330; margin-bottom: 24px; }
  .report-brand { font-size: 16px; font-weight: 700; letter-spacing: 0.08em; color: #64748b; }
  .report-title { font-size: 22px; font-weight: 600; color: #f1f5f9; margin-top: 4px; }
  .report-period { font-size: 13px; color: #64748b; }
  .section { margin-bottom: 24px; }
  .section-label { font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.08em; color: #64748b; margin-bottom: 12px; }
  .summary-row { display: flex; gap: 24px; margin-bottom: 24px; }
  .summary-cell { text-align: center; }
  .summary-value { font-size: 28px; font-weight: 700; }
  .summary-value.errors { color: #f87171; }
  .summary-value.events { color: #38bdf8; }
  .summary-caption { font-size: 11px; color: #94a3b8; text-transform: uppercase; letter-spacing: 0.08em; margin-top: 2px; }
  table.activity { width: 100%; border-collapse: collapse; }
  table.activity td { padding: 9px 0; border-bottom: 1px solid #1e2330; font-size: 14px; }
  table.activity td.app { color: #e2e8f0; }
  table.activity td.type { color: #64748b; text-align: center; font-size: 12px; }
  table.activity td.count { text-align: right; font-weight: 600; }
  td.count.err { color: #f87171; }
  td.count.ok  { color: #38bdf8; }
  .empty { color: #64748b; font-size: 14px; padding: 12px 0; }
  .print-btn { display: inline-flex; align-items: center; gap: 8px; padding: 8px 18px; background: #1e2330; border: 1px solid #334155; border-radius: 6px; color: #e2e8f0; font-size: 13px; font-weight: 500; cursor: pointer; margin-bottom: 24px; }
  .print-btn:hover { background: #263044; }
  @media print {
    body { background: #fff; color: #000; padding: 0; }
    .report-header { border-bottom-color: #ccc; }
    .report-brand, .report-period { color: #666; }
    .report-title { color: #000; }
    .section-label { color: #666; }
    table.activity td { border-bottom-color: #ddd; }
    table.activity td.app { color: #000; }
    table.activity td.type { color: #666; }
    td.count.err { color: #c0392b; }
    td.count.ok  { color: #2563eb; }
    .print-btn { display: none; }
    .summary-value.errors { color: #c0392b; }
    .summary-value.events { color: #2563eb; }
  }
</style>
</head>
<body>
<div class="report">
  <button class="print-btn" onclick="window.print()">⎙ Print / Save as PDF</button>

  <div class="report-header">
    <div>
      <div class="report-brand">NORA</div>
      <div class="report-title">{{.Title}}</div>
    </div>
    <div class="report-period">{{.Period}}</div>
  </div>

  <div class="section">
    <div class="section-label">Summary</div>
    <div class="summary-row">
      {{if .TotalErrors}}
      <div class="summary-cell">
        <div class="summary-value errors">{{.TotalErrors}}</div>
        <div class="summary-caption">Errors</div>
      </div>
      {{end}}
      <div class="summary-cell">
        <div class="summary-value events">{{len .AppRows}}</div>
        <div class="summary-caption">Event Types</div>
      </div>
    </div>
  </div>

  <div class="section">
    <div class="section-label">Activity</div>
    {{if .AppRows}}
    <table class="activity">
      {{range .AppRows}}
      <tr>
        <td class="app">{{.AppName}}</td>
        <td class="type">{{.EventType}}</td>
        <td class="count {{if .HasErrors}}err{{else}}ok{{end}}">{{.Count}}</td>
      </tr>
      {{end}}
    </table>
    {{else}}
    <p class="empty">No activity recorded for this period.</p>
    {{end}}
  </div>
</div>
</body>
</html>`
