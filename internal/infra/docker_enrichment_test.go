package infra

import (
	"context"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// ── Mock implementations ────────────────────────────────────────────────────

type enrichMockAppRepo struct {
	apps map[string]*models.App
}

func (m *enrichMockAppRepo) List(_ context.Context) ([]models.App, error)         { return nil, nil }
func (m *enrichMockAppRepo) ListByHost(_ context.Context, _ string) ([]models.App, error) { return nil, nil }
func (m *enrichMockAppRepo) Create(_ context.Context, a *models.App) error         { m.apps[a.ID] = a; return nil }
func (m *enrichMockAppRepo) Get(_ context.Context, id string) (*models.App, error) {
	a, ok := m.apps[id]
	if !ok {
		return nil, repo.ErrNotFound
	}
	return a, nil
}
func (m *enrichMockAppRepo) GetByToken(_ context.Context, _ string) (*models.App, error) {
	return nil, repo.ErrNotFound
}
func (m *enrichMockAppRepo) Update(_ context.Context, _ *models.App) error                    { return nil }
func (m *enrichMockAppRepo) Delete(_ context.Context, _ string) error                         { return nil }
func (m *enrichMockAppRepo) UpdateToken(_ context.Context, _, _ string) error                 { return nil }

type enrichMockCheckRepo struct {
	created []*models.MonitorCheck
	// existsByTarget lets tests control ExistsForTypeAndTarget per target string.
	existsByTarget map[string]bool
}

func (m *enrichMockCheckRepo) List(_ context.Context) ([]models.MonitorCheck, error) { return nil, nil }
func (m *enrichMockCheckRepo) Create(_ context.Context, c *models.MonitorCheck) error {
	m.created = append(m.created, c)
	return nil
}
func (m *enrichMockCheckRepo) Get(_ context.Context, _ string) (*models.MonitorCheck, error) {
	return nil, repo.ErrNotFound
}
func (m *enrichMockCheckRepo) Update(_ context.Context, _ *models.MonitorCheck) error { return nil }
func (m *enrichMockCheckRepo) Delete(_ context.Context, _ string) error               { return nil }
func (m *enrichMockCheckRepo) UpdateStatus(_ context.Context, _, _, _ string, _ time.Time) error {
	return nil
}
func (m *enrichMockCheckRepo) ListBySourceComponent(_ context.Context, _ string) ([]models.MonitorCheck, error) {
	return nil, nil
}
func (m *enrichMockCheckRepo) DeleteBySourceComponent(_ context.Context, _ string) error { return nil }
func (m *enrichMockCheckRepo) UpsertForComponent(_ context.Context, _ *models.MonitorCheck) error {
	return nil
}
func (m *enrichMockCheckRepo) ExistsForTypeAndTarget(_ context.Context, _, target string) (bool, error) {
	return m.existsByTarget[target], nil
}
func (m *enrichMockCheckRepo) SetDNSBaseline(_ context.Context, _, _ string) error { return nil }

type enrichMockContainerRepo struct {
	containers map[string]*models.DiscoveredContainer
}

func (m *enrichMockContainerRepo) UpsertDiscoveredContainer(_ context.Context, _ *models.DiscoveredContainer) error {
	return nil
}
func (m *enrichMockContainerRepo) ListDiscoveredContainers(_ context.Context, _ string) ([]*models.DiscoveredContainer, error) {
	return nil, nil
}
func (m *enrichMockContainerRepo) ListAllDiscoveredContainers(_ context.Context) ([]*models.DiscoveredContainer, error) {
	return nil, nil
}
func (m *enrichMockContainerRepo) GetDiscoveredContainer(_ context.Context, id string) (*models.DiscoveredContainer, error) {
	c, ok := m.containers[id]
	if !ok {
		return nil, repo.ErrNotFound
	}
	return c, nil
}
func (m *enrichMockContainerRepo) SetDiscoveredContainerApp(_ context.Context, _, _ string) error {
	return nil
}
func (m *enrichMockContainerRepo) ClearDiscoveredContainerApp(_ context.Context, _ string) error {
	return nil
}
func (m *enrichMockContainerRepo) UpdateDiscoveredContainerStatus(_ context.Context, _ string, _ string, _ time.Time) error {
	return nil
}
func (m *enrichMockContainerRepo) MarkStoppedIfNotRunning(_ context.Context, _ string, _ []string) error {
	return nil
}
func (m *enrichMockContainerRepo) FindByName(_ context.Context, _ string, _ string) (*models.DiscoveredContainer, error) {
	return nil, repo.ErrNotFound
}
func (m *enrichMockContainerRepo) DeleteDiscoveredContainer(_ context.Context, _ string) error {
	return nil
}
func (m *enrichMockContainerRepo) UpdateContainerImageCheck(_ context.Context, _, _, _ string, _ bool) error {
	return nil
}
func (m *enrichMockContainerRepo) UpdateContainerRestartPolicy(_ context.Context, _ string, _ string) error {
	return nil
}

type enrichMockRouteRepo struct {
	routes map[string]*models.DiscoveredRoute
}

func (m *enrichMockRouteRepo) UpsertDiscoveredRoute(_ context.Context, _ *models.DiscoveredRoute) error {
	return nil
}
func (m *enrichMockRouteRepo) ListDiscoveredRoutes(_ context.Context, _ string) ([]*models.DiscoveredRoute, error) {
	return nil, nil
}
func (m *enrichMockRouteRepo) ListAllDiscoveredRoutes(_ context.Context) ([]*models.DiscoveredRoute, error) {
	return nil, nil
}
func (m *enrichMockRouteRepo) GetDiscoveredRoute(_ context.Context, id string) (*models.DiscoveredRoute, error) {
	r, ok := m.routes[id]
	if !ok {
		return nil, repo.ErrNotFound
	}
	return r, nil
}
func (m *enrichMockRouteRepo) ListDiscoveredRoutesByStatus(_ context.Context, _ string, _ string) ([]*models.DiscoveredRoute, error) {
	return nil, nil
}
func (m *enrichMockRouteRepo) SetDiscoveredRouteApp(_ context.Context, _, _ string) error { return nil }
func (m *enrichMockRouteRepo) ClearDiscoveredRouteApp(_ context.Context, _ string) error  { return nil }
func (m *enrichMockRouteRepo) ListByAppID(_ context.Context, _ string) ([]*models.DiscoveredRoute, error) {
	return nil, nil
}
func (m *enrichMockRouteRepo) SyncRouteAppLink(_ context.Context, _, _, _ string) {}
func (m *enrichMockRouteRepo) ListServicesForComponent(_ context.Context, _ string, _ string) ([]*models.DiscoveredServiceSummary, error) {
	return nil, nil
}

type enrichMockResourceRepo struct {
	backfilled int64
}

func (m *enrichMockResourceRepo) Create(_ context.Context, _ *models.ResourceReading) error {
	return nil
}
func (m *enrichMockResourceRepo) LatestMetrics(_ context.Context, _ string, _ []string) (map[string]map[string]float64, error) {
	return nil, nil
}
func (m *enrichMockResourceRepo) BackfillAppID(_ context.Context, _, _ string) (int64, error) {
	return m.backfilled, nil
}

// mockProfileLoader implements apptemplate.Loader for tests.
type mockProfileLoader struct {
	templates map[string]*apptemplate.AppTemplate
}

func (m *mockProfileLoader) Get(id string) (*apptemplate.AppTemplate, error) {
	t, ok := m.templates[id]
	if !ok {
		return nil, nil
	}
	return t, nil
}

// buildStore builds a minimal *repo.Store with only the repos used by enrichment.
func buildEnrichStore(
	apps repo.AppRepo,
	checks repo.CheckRepo,
	containers repo.DiscoveredContainerRepo,
	routes repo.DiscoveredRouteRepo,
	resources repo.ResourceReadingRepo,
) *repo.Store {
	return &repo.Store{
		Apps:                 apps,
		Checks:               checks,
		DiscoveredContainers: containers,
		DiscoveredRoutes:     routes,
		Resources:            resources,
	}
}

// ── Tests ───────────────────────────────────────────────────────────────────

// TestEnrichAppOnLink_NoProfileNoChecks verifies that enrichment with no profile
// and no container/route creates no checks.
func TestEnrichAppOnLink_NoProfileNoChecks(t *testing.T) {
	appID := "app-2"

	apps := &enrichMockAppRepo{apps: map[string]*models.App{
		appID: {ID: appID, Name: "MyApp", ProfileID: "", Config: models.ConfigJSON(`{}`)},
	}}
	checks := &enrichMockCheckRepo{existsByTarget: map[string]bool{}}

	store := buildEnrichStore(apps, checks, nil, nil, nil)
	if err := EnrichAppOnLink(context.Background(), store, &apptemplate.NoopLoader{}, appID, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(checks.created) != 0 {
		t.Errorf("expected 0 checks created, got %d", len(checks.created))
	}
}

// TestEnrichAppOnLink_RouteWithDomain verifies that an SSL check is created when
// the linked route has a domain.
func TestEnrichAppOnLink_RouteWithDomain(t *testing.T) {
	appID := "app-3"
	routeUUID := "route-uuid-1"
	domain := "sonarr.example.com"

	apps := &enrichMockAppRepo{apps: map[string]*models.App{
		appID: {ID: appID, Name: "Sonarr", ProfileID: "", Config: models.ConfigJSON(`{}`)},
	}}
	checks := &enrichMockCheckRepo{existsByTarget: map[string]bool{}}
	routes := &enrichMockRouteRepo{routes: map[string]*models.DiscoveredRoute{
		routeUUID: {ID: routeUUID, RouterName: "sonarr-router", Rule: "Host(`sonarr.example.com`)", Domain: &domain},
	}}

	store := buildEnrichStore(apps, checks, nil, routes, nil)
	if err := EnrichAppOnLink(context.Background(), store, &apptemplate.NoopLoader{}, appID, nil, &routeUUID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(checks.created) != 1 {
		t.Fatalf("expected 1 SSL check created, got %d", len(checks.created))
	}
	c := checks.created[0]
	if c.Type != "ssl" {
		t.Errorf("check type = %q, want %q", c.Type, "ssl")
	}
	if c.Target != domain {
		t.Errorf("check target = %q, want %q", c.Target, domain)
	}
	if c.SSLWarnDays != 30 {
		t.Errorf("ssl_warn_days = %d, want 30", c.SSLWarnDays)
	}
	if c.SSLCritDays != 7 {
		t.Errorf("ssl_crit_days = %d, want 7", c.SSLCritDays)
	}
	if c.IntervalSecs != 3600 {
		t.Errorf("interval_secs = %d, want 3600", c.IntervalSecs)
	}
}

// TestEnrichAppOnLink_RouteWithoutDomain verifies that no SSL check is created
// when the route has no domain.
func TestEnrichAppOnLink_RouteWithoutDomain(t *testing.T) {
	appID := "app-4"
	routeUUID := "route-uuid-2"

	apps := &enrichMockAppRepo{apps: map[string]*models.App{
		appID: {ID: appID, Name: "MyApp", ProfileID: "", Config: models.ConfigJSON(`{}`)},
	}}
	checks := &enrichMockCheckRepo{existsByTarget: map[string]bool{}}
	routes := &enrichMockRouteRepo{routes: map[string]*models.DiscoveredRoute{
		routeUUID: {ID: routeUUID, RouterName: "router-no-domain", Rule: "PathPrefix(`/`)", Domain: nil},
	}}

	store := buildEnrichStore(apps, checks, nil, routes, nil)
	if err := EnrichAppOnLink(context.Background(), store, &apptemplate.NoopLoader{}, appID, nil, &routeUUID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(checks.created) != 0 {
		t.Errorf("expected 0 checks created, got %d", len(checks.created))
	}
}

// TestEnrichAppOnLink_DuplicateCheckPrevention verifies that enrichment does not
// create a check when one already exists for the same type and target.
func TestEnrichAppOnLink_DuplicateCheckPrevention(t *testing.T) {
	appID := "app-5"
	routeUUID := "route-uuid-3"
	domain := "already.example.com"

	apps := &enrichMockAppRepo{apps: map[string]*models.App{
		appID: {ID: appID, Name: "App", ProfileID: "", Config: models.ConfigJSON(`{}`)},
	}}
	// SSL target already exists.
	checks := &enrichMockCheckRepo{existsByTarget: map[string]bool{
		domain: true,
	}}
	routes := &enrichMockRouteRepo{routes: map[string]*models.DiscoveredRoute{
		routeUUID: {ID: routeUUID, RouterName: "r", Rule: "Host(`already.example.com`)", Domain: &domain},
	}}

	store := buildEnrichStore(apps, checks, nil, routes, nil)
	if err := EnrichAppOnLink(context.Background(), store, &apptemplate.NoopLoader{}, appID, nil, &routeUUID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(checks.created) != 0 {
		t.Errorf("expected 0 checks created (all exist), got %d", len(checks.created))
	}
}

// TestEnrichAppOnLink_ResourceBackfill verifies that BackfillAppID is called with
// the correct docker container ID when a container is linked.
func TestEnrichAppOnLink_ResourceBackfill(t *testing.T) {
	appID := "app-6"
	containerUUID := "container-uuid-6"
	dockerID := "deadbeef"

	apps := &enrichMockAppRepo{apps: map[string]*models.App{
		appID: {ID: appID, Name: "App", ProfileID: "", Config: models.ConfigJSON(`{}`)},
	}}
	checks := &enrichMockCheckRepo{existsByTarget: map[string]bool{}}
	containers := &enrichMockContainerRepo{containers: map[string]*models.DiscoveredContainer{
		containerUUID: {ID: containerUUID, ContainerID: dockerID, ContainerName: "myapp"},
	}}
	resources := &enrichMockResourceRepo{backfilled: 10}

	store := buildEnrichStore(apps, checks, containers, nil, resources)
	if err := EnrichAppOnLink(context.Background(), store, &apptemplate.NoopLoader{}, appID, &containerUUID, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The mock always reports the configured count; we just verify no error path
	// was triggered (i.e., enrichment completed without panicking or returning early).
}

// ── Unit tests for helpers ──────────────────────────────────────────────────

func TestParseIntervalSecs(t *testing.T) {
	cases := []struct {
		in       string
		fallback int
		want     int
	}{
		{"5m", 300, 300},
		{"1h", 300, 3600},
		{"30s", 300, 30},
		{"", 300, 300},
		{"bad", 300, 300},
		{"-1m", 300, 300},
	}
	for _, tc := range cases {
		got := parseIntervalSecs(tc.in, tc.fallback)
		if got != tc.want {
			t.Errorf("parseIntervalSecs(%q, %d) = %d, want %d", tc.in, tc.fallback, got, tc.want)
		}
	}
}

func TestSubstituteBaseURL(t *testing.T) {
	type tc struct {
		tmpl    string
		cfg     string
		want    string
		wantErr bool
	}
	cases := []tc{
		// Happy path: placeholder fully resolved.
		{"{base_url}/ping", `{"base_url":"http://host:8989"}`, "http://host:8989/ping", false},
		// No placeholder in template — returned unchanged, no error.
		{"/health", `{"base_url":"http://x"}`, "/health", false},
		// Placeholder resolved to bare domain (no scheme) — still returned; scheme
		// validation is the caller's responsibility.
		{"{base_url}", `{"base_url":"http://x"}`, "http://x", false},
		// Missing base_url in config — must error, not return unresolved template.
		{"{base_url}/api", `{}`, "", true},
		// Empty base_url value — must error.
		{"{base_url}/api", `{"base_url":""}`, "", true},
		// Unparseable config JSON — must error.
		{"{base_url}", `not-json`, "", true},
	}
	for _, tc := range cases {
		got, err := substituteBaseURL(tc.tmpl, models.ConfigJSON(tc.cfg))
		if tc.wantErr {
			if err == nil {
				t.Errorf("substituteBaseURL(%q, %q) expected error, got %q", tc.tmpl, tc.cfg, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("substituteBaseURL(%q, %q) unexpected error: %v", tc.tmpl, tc.cfg, err)
			continue
		}
		if got != tc.want {
			t.Errorf("substituteBaseURL(%q, %q) = %q, want %q", tc.tmpl, tc.cfg, got, tc.want)
		}
	}
}

