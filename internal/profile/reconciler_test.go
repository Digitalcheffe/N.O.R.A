package profile_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/profile"
	"github.com/digitalcheffe/nora/internal/repo"
)

// stubRepo captures Upsert calls for assertions.
type stubRepo struct {
	upserted   []models.DigestRegistryEntry
	deactivated []string
}

func (s *stubRepo) Upsert(_ context.Context, e models.DigestRegistryEntry) error {
	s.upserted = append(s.upserted, e)
	return nil
}

func (s *stubRepo) SetActive(_ context.Context, _ string, name string, _ bool) error {
	s.deactivated = append(s.deactivated, name)
	return nil
}

func (s *stubRepo) List(_ context.Context) ([]models.DigestRegistryEntry, error) {
	return nil, nil
}

func (s *stubRepo) ListByProfile(_ context.Context, _ string) ([]models.DigestRegistryEntry, error) {
	return nil, nil
}

func (s *stubRepo) Delete(_ context.Context, _ string) error { return nil }

func (s *stubRepo) ListInactive(_ context.Context) ([]models.DigestRegistryEntry, error) {
	return nil, nil
}

func (s *stubRepo) DeleteAllInactive(_ context.Context) (int64, error) { return 0, nil }

func (s *stubRepo) SetActiveByID(_ context.Context, _ string, _ bool) error { return nil }

var _ repo.DigestRegistryRepo = (*stubRepo)(nil)

const sonarrWithAPIPolling = `
meta:
  name: Sonarr
  category: Media
  icon: sonarr
  description: TV series management
  capability: full

webhook:
  field_mappings:
    event_type: "$.eventType"
    series_title: "$.series.title"

digest:
  categories:
    - source: webhook
      label: Downloads
      match_field: event_type
      match_value: Download
    - source: webhook
      label: Health Issues
      match_field: event_type
      match_value: Health

  widgets:
    - source: api
      label: Series Tracked
      metric: total_series
    - source: api
      label: In Queue
      metric: queue_depth
    - source: webhook
      label: Downloads Today
      match_field: event_type
      match_value: Download

api_polling:
  - path: /api/v3/series
    name: total_series
    label: Total Series
    target: length
    value_type: count
    event_message: "Sonarr is tracking {value} series"
  - path: /api/v3/queue
    name: queue_depth
    label: Queue Depth
    target: $.totalRecords
    value_type: count
`

// sonarrLegacy represents an existing profile without source or widgets — backward compat.
const sonarrLegacy = `
meta:
  name: Sonarr
  category: Media
  icon: sonarr
  description: TV series management
  capability: full

webhook:
  field_mappings:
    event_type: "$.eventType"

digest:
  categories:
    - label: Downloads
      match_field: event_type
      match_value: Download
    - label: Health Issues
      match_field: event_type
      match_value: Health
`

func loadRegistry(t *testing.T, filename, yaml string) *apptemplate.Registry {
	t.Helper()
	fsys := fstest.MapFS{
		filename: &fstest.MapFile{Data: []byte(yaml)},
	}
	reg, err := apptemplate.NewRegistry(fsys)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

func TestReconcile_CategoriesAndWidgets(t *testing.T) {
	reg := loadRegistry(t, "sonarr.yaml", sonarrWithAPIPolling)
	r := &stubRepo{}
	rc := profile.NewRegistryReconciler(r, reg)

	if err := rc.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// Expect 2 categories + 3 widgets = 5 upserts.
	if len(r.upserted) != 5 {
		t.Fatalf("expected 5 upserted entries, got %d", len(r.upserted))
	}

	byName := make(map[string]models.DigestRegistryEntry, len(r.upserted))
	for _, e := range r.upserted {
		byName[e.Name] = e
	}

	// Category checks.
	downloads, ok := byName["downloads"]
	if !ok {
		t.Fatal("expected category 'downloads' to be upserted")
	}
	if downloads.EntryType != "category" {
		t.Errorf("downloads: expected entry_type=category, got %s", downloads.EntryType)
	}
	if downloads.Source != "webhook" {
		t.Errorf("downloads: expected source=webhook, got %s", downloads.Source)
	}

	// Widget checks.
	seriesWidget, ok := byName["widget_series_tracked"]
	if !ok {
		t.Fatal("expected widget 'widget_series_tracked' to be upserted")
	}
	if seriesWidget.EntryType != "widget" {
		t.Errorf("widget_series_tracked: expected entry_type=widget, got %s", seriesWidget.EntryType)
	}
	if seriesWidget.Source != "api" {
		t.Errorf("widget_series_tracked: expected source=api, got %s", seriesWidget.Source)
	}

	webhookWidget, ok := byName["widget_downloads_today"]
	if !ok {
		t.Fatal("expected widget 'widget_downloads_today' to be upserted")
	}
	if webhookWidget.Source != "webhook" {
		t.Errorf("widget_downloads_today: expected source=webhook, got %s", webhookWidget.Source)
	}
}

func TestReconcile_LegacyProfileBackwardCompat(t *testing.T) {
	reg := loadRegistry(t, "sonarr.yaml", sonarrLegacy)
	r := &stubRepo{}
	rc := profile.NewRegistryReconciler(r, reg)

	if err := rc.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// Expect 2 categories, no widgets.
	if len(r.upserted) != 2 {
		t.Fatalf("expected 2 upserted entries for legacy profile, got %d", len(r.upserted))
	}

	for _, e := range r.upserted {
		if e.EntryType != "category" {
			t.Errorf("%s: expected entry_type=category, got %s", e.Name, e.EntryType)
		}
		if e.Source != "webhook" {
			t.Errorf("%s: expected source=webhook (defaulted), got %s", e.Name, e.Source)
		}
	}
}

func TestReconcile_CategorySourceUsedWhenPresent(t *testing.T) {
	reg := loadRegistry(t, "sonarr.yaml", sonarrWithAPIPolling)
	r := &stubRepo{}
	rc := profile.NewRegistryReconciler(r, reg)

	if err := rc.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	for _, e := range r.upserted {
		if e.EntryType == "category" && e.Source == "" {
			t.Errorf("category %s: source should not be empty after reconcile", e.Name)
		}
	}
}
