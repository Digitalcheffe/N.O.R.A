package jobs_test

import (
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/jobs"
	"github.com/digitalcheffe/nora/internal/models"
)

// newTestDigestJob creates a DigestJob with nil store and empty config for
// unit tests that only exercise pure functions.
func newTestDigestJob() *jobs.DigestJob {
	return jobs.NewDigestJob(nil, &config.Config{}, nil)
}

// ── ShouldSendToday ──────────────────────────────────────────────────────────

func TestShouldSendToday_Daily(t *testing.T) {
	job := newTestDigestJob()
	sched := models.DigestSchedule{Frequency: "daily"}

	days := []time.Weekday{
		time.Sunday, time.Monday, time.Tuesday, time.Wednesday,
		time.Thursday, time.Friday, time.Saturday,
	}
	for _, wd := range days {
		// Build a date that falls on the given weekday.
		base := time.Date(2026, 3, 23, 8, 0, 0, 0, time.UTC) // Monday
		offset := int(wd) - int(time.Monday)
		d := base.AddDate(0, 0, offset)
		if !job.ShouldSendToday(sched, d) {
			t.Errorf("daily: expected true on %s", d.Weekday())
		}
	}
}

func TestShouldSendToday_Weekly(t *testing.T) {
	job := newTestDigestJob()

	// Send on Monday (1).
	sched := models.DigestSchedule{Frequency: "weekly", DayOfWeek: 1}

	tests := []struct {
		date time.Time
		want bool
	}{
		{time.Date(2026, 3, 23, 8, 0, 0, 0, time.UTC), true},  // Monday
		{time.Date(2026, 3, 24, 8, 0, 0, 0, time.UTC), false}, // Tuesday
		{time.Date(2026, 3, 22, 8, 0, 0, 0, time.UTC), false}, // Sunday
		{time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC), true},  // next Monday
	}
	for _, tc := range tests {
		got := job.ShouldSendToday(sched, tc.date)
		if got != tc.want {
			t.Errorf("weekly(Mon) on %s: got %v, want %v", tc.date.Format("Mon 2006-01-02"), got, tc.want)
		}
	}
}

func TestShouldSendToday_Weekly_Sunday(t *testing.T) {
	job := newTestDigestJob()
	sched := models.DigestSchedule{Frequency: "weekly", DayOfWeek: 0} // Sunday

	sunday := time.Date(2026, 3, 22, 8, 0, 0, 0, time.UTC)
	if !job.ShouldSendToday(sched, sunday) {
		t.Errorf("weekly(Sun): expected true on Sunday")
	}
	monday := sunday.AddDate(0, 0, 1)
	if job.ShouldSendToday(sched, monday) {
		t.Errorf("weekly(Sun): expected false on Monday")
	}
}

func TestShouldSendToday_Monthly(t *testing.T) {
	job := newTestDigestJob()
	sched := models.DigestSchedule{Frequency: "monthly", DayOfMonth: 1}

	tests := []struct {
		date time.Time
		want bool
	}{
		{time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 3, 2, 8, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 2, 1, 8, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 12, 1, 8, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 12, 31, 8, 0, 0, 0, time.UTC), false},
	}
	for _, tc := range tests {
		got := job.ShouldSendToday(sched, tc.date)
		if got != tc.want {
			t.Errorf("monthly(1) on %s: got %v, want %v", tc.date.Format("2006-01-02"), got, tc.want)
		}
	}
}

func TestShouldSendToday_Monthly_Day28(t *testing.T) {
	job := newTestDigestJob()
	sched := models.DigestSchedule{Frequency: "monthly", DayOfMonth: 28}

	// Day 28 exists in all months including February.
	feb28 := time.Date(2026, 2, 28, 8, 0, 0, 0, time.UTC)
	if !job.ShouldSendToday(sched, feb28) {
		t.Errorf("monthly(28): expected true on Feb 28")
	}

	mar28 := time.Date(2026, 3, 28, 8, 0, 0, 0, time.UTC)
	if !job.ShouldSendToday(sched, mar28) {
		t.Errorf("monthly(28): expected true on Mar 28")
	}

	mar27 := time.Date(2026, 3, 27, 8, 0, 0, 0, time.UTC)
	if job.ShouldSendToday(sched, mar27) {
		t.Errorf("monthly(28): expected false on Mar 27")
	}
}

func TestShouldSendToday_UnknownFrequency(t *testing.T) {
	job := newTestDigestJob()
	sched := models.DigestSchedule{Frequency: "never"}
	if job.ShouldSendToday(sched, time.Now()) {
		t.Errorf("unknown frequency: expected false")
	}
}

// ── RenderHTML ───────────────────────────────────────────────────────────────

func TestRenderHTML_Monthly(t *testing.T) {
	job := newTestDigestJob()
	data := &jobs.DigestData{
		Title:       "Your Homelab — March 2026",
		Period:      "2026-03",
		TotalErrors: 2,
		// Use AppSections (new template path) with per-app category rows.
		AppSections: []jobs.DigestAppSection{
			{
				AppName:     "Sonarr",
				ProfileName: "sonarr",
				TotalEvents: 89,
				Categories:  []jobs.DigestCategoryRow{{Label: "Downloads", Count: 89}},
			},
			{
				AppName:     "n8n",
				ProfileName: "n8n",
				TotalEvents: 2,
				HasIssues:   true,
				Categories:  []jobs.DigestCategoryRow{{Label: "Errors", Count: 2}},
			},
		},
	}
	html, err := job.RenderHTML(data)
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	if html == "" {
		t.Fatal("RenderHTML: empty output")
	}
	// Must contain the title and key content.
	for _, want := range []string{
		"Your Homelab — March 2026",
		"Sonarr",
		"n8n",
		"#0a0c0f",
	} {
		if !contains(html, want) {
			t.Errorf("RenderHTML: output missing %q", want)
		}
	}
}

func TestRenderHTML_NoActivity(t *testing.T) {
	job := newTestDigestJob()
	data := &jobs.DigestData{
		Title:  "Your Homelab — March 2026",
		Period: "2026-03",
	}
	html, err := job.RenderHTML(data)
	if err != nil {
		t.Fatalf("RenderHTML empty: %v", err)
	}
	// With no app sections the app activity block is omitted; verify the
	// title and structural chrome still appear in the output.
	if !contains(html, "Your Homelab — March 2026") {
		t.Errorf("RenderHTML empty: expected title in output")
	}
	if !contains(html, "#0a0c0f") {
		t.Errorf("RenderHTML empty: expected template chrome in output")
	}
}

// ── EffectiveSendHour ────────────────────────────────────────────────────────

func TestEffectiveSendHour_Default(t *testing.T) {
	sched := models.DigestSchedule{Frequency: "daily"}
	if got := sched.EffectiveSendHour(); got != 8 {
		t.Errorf("expected 8, got %d", got)
	}
}

func TestEffectiveSendHour_Explicit(t *testing.T) {
	h := 14
	sched := models.DigestSchedule{Frequency: "daily", SendHour: &h}
	if got := sched.EffectiveSendHour(); got != 14 {
		t.Errorf("expected 14, got %d", got)
	}
}

func TestEffectiveSendHour_Midnight(t *testing.T) {
	h := 0
	sched := models.DigestSchedule{Frequency: "daily", SendHour: &h}
	if got := sched.EffectiveSendHour(); got != 0 {
		t.Errorf("expected 0 (midnight), got %d", got)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
