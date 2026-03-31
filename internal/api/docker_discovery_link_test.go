package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// newDiscoveryLinkRouter builds a DockerDiscoveryHandler-backed chi router
// using an in-memory DB. It returns both the router and the db so callers
// can seed data directly.
func newDiscoveryLinkRouter(t *testing.T, profiles apptemplate.Loader) (http.Handler, *sqlx.DB) {
	t.Helper()
	db := newTestDB(t)
	store := repo.NewStore(
		repo.NewAppRepo(db),
		repo.NewEventRepo(db),
		repo.NewCheckRepo(db),
		repo.NewRollupRepo(db),
		repo.NewResourceReadingRepo(db),
		repo.NewResourceRollupRepo(db),
		repo.NewInfraComponentRepo(db),
		repo.NewDockerEngineRepo(db),
		repo.NewInfraRepo(db),
		repo.NewSettingsRepo(db),
		repo.NewMetricsRepo(db),
		repo.NewUserRepo(db),
		repo.NewTraefikComponentRepo(db),
		repo.NewTraefikOverviewRepo(db),
		repo.NewTraefikServiceRepo(db),
		repo.NewDiscoveredContainerRepo(db),
		repo.NewDiscoveredRouteRepo(db),
		nil,
		nil,
		nil,
	)
	h := api.NewDockerDiscoveryHandler(store, profiles)
	r := chi.NewRouter()
	h.Routes(r)
	return r, db
}

// seedDockerEngineForLink inserts a docker_engine infrastructure_components row
// and returns its ID.
func seedDockerEngineForLink(t *testing.T, db *sqlx.DB) string {
	t.Helper()
	id := uuid.NewString()
	_, err := db.Exec(
		`INSERT INTO infrastructure_components (id, name, type, collection_method) VALUES (?, 'eng', 'docker_engine', 'docker_socket')`, id)
	if err != nil {
		t.Fatalf("seed docker engine component: %v", err)
	}
	return id
}

// seedInfraComponentForLink inserts an infrastructure_components row and returns its ID.
func seedInfraComponentForLink(t *testing.T, db *sqlx.DB) string {
	t.Helper()
	id := uuid.NewString()
	_, err := db.Exec(
		`INSERT INTO infrastructure_components (id, name, type, collection_method) VALUES (?, 'traefik', 'traefik', 'traefik_api')`, id)
	if err != nil {
		t.Fatalf("seed infra component: %v", err)
	}
	return id
}

// seedDiscoveredContainer inserts a discovered_containers row and returns its ID.
func seedDiscoveredContainer(t *testing.T, db *sqlx.DB, engineID string) string {
	t.Helper()
	containerRepo := repo.NewDiscoveredContainerRepo(db)
	now := time.Now().UTC()
	c := &models.DiscoveredContainer{
		InfraComponentID: engineID,
		ContainerID:      uuid.NewString(),
		ContainerName:  "sonarr",
		Image:          "linuxserver/sonarr:latest",
		Status:         "running",
		LastSeenAt:     now,
		CreatedAt:      now,
	}
	if err := containerRepo.UpsertDiscoveredContainer(t.Context(), c); err != nil {
		t.Fatalf("seed discovered container: %v", err)
	}
	return c.ID
}

// seedDiscoveredRoute inserts a discovered_routes row and returns its ID.
func seedDiscoveredRoute(t *testing.T, db *sqlx.DB, infraID string, domain *string) string {
	t.Helper()
	routeRepo := repo.NewDiscoveredRouteRepo(db)
	now := time.Now().UTC()
	ro := &models.DiscoveredRoute{
		InfrastructureID: infraID,
		RouterName:       "sonarr-router",
		Rule:             "Host(`sonarr.example.com`)",
		Domain:           domain,
		LastSeenAt:       now,
		CreatedAt:        now,
	}
	if err := routeRepo.UpsertDiscoveredRoute(t.Context(), ro); err != nil {
		t.Fatalf("seed discovered route: %v", err)
	}
	return ro.ID
}

// seedApp inserts an apps row and returns its ID.
func seedAppForLink(t *testing.T, db *sqlx.DB) string {
	t.Helper()
	id := uuid.NewString()
	_, err := db.Exec(
		`INSERT INTO apps (id, name, token, rate_limit, created_at) VALUES (?, 'TestApp', 'tok-link', 100, datetime('now'))`, id)
	if err != nil {
		t.Fatalf("seed app: %v", err)
	}
	return id
}

// stubbedProfiles returns a Loader whose Get always returns a non-nil template
// for "sonarr" and nil for anything else.
type stubbedProfiles struct{}

func (s *stubbedProfiles) Get(id string) (*apptemplate.AppTemplate, error) {
	if id == "sonarr" {
		return &apptemplate.AppTemplate{}, nil
	}
	return nil, nil
}

// ── LinkContainerApp ───────────────────────────────────────────────────────────

func TestLinkContainerApp_ExistingMode_OK(t *testing.T) {
	router, db := newDiscoveryLinkRouter(t, &stubbedProfiles{})
	engineID := seedDockerEngineForLink(t, db)
	containerID := seedDiscoveredContainer(t, db, engineID)
	appID := seedAppForLink(t, db)

	body, _ := json.Marshal(map[string]any{"mode": "existing", "app_id": appID})
	req := httptest.NewRequest(http.MethodPost, "/discovered-containers/"+containerID+"/link-app", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var c models.DiscoveredContainer
	if err := json.NewDecoder(rr.Body).Decode(&c); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if c.AppID == nil || *c.AppID != appID {
		t.Errorf("app_id: want %s, got %v", appID, c.AppID)
	}
}

func TestLinkContainerApp_ExistingMode_InvalidAppID(t *testing.T) {
	router, db := newDiscoveryLinkRouter(t, &stubbedProfiles{})
	engineID := seedDockerEngineForLink(t, db)
	containerID := seedDiscoveredContainer(t, db, engineID)

	body, _ := json.Marshal(map[string]any{"mode": "existing", "app_id": "does-not-exist"})
	req := httptest.NewRequest(http.MethodPost, "/discovered-containers/"+containerID+"/link-app", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestLinkContainerApp_CreateMode_OK(t *testing.T) {
	router, db := newDiscoveryLinkRouter(t, &stubbedProfiles{})
	engineID := seedDockerEngineForLink(t, db)
	containerID := seedDiscoveredContainer(t, db, engineID)

	body, _ := json.Marshal(map[string]any{
		"mode":       "create",
		"profile_id": "sonarr",
		"name":       "My Sonarr",
		"config":     map[string]any{"base_url": "http://sonarr:8989"},
	})
	req := httptest.NewRequest(http.MethodPost, "/discovered-containers/"+containerID+"/link-app", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var app models.App
	if err := json.NewDecoder(rr.Body).Decode(&app); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if app.ID == "" {
		t.Error("app.ID must not be empty")
	}
	if app.ProfileID != "sonarr" {
		t.Errorf("profile_id: want sonarr, got %s", app.ProfileID)
	}
	// Verify the container is linked.
	checkDB := repo.NewDiscoveredContainerRepo(db)
	got, _ := checkDB.GetDiscoveredContainer(t.Context(), containerID)
	if got.AppID == nil || *got.AppID != app.ID {
		t.Errorf("container app_id: want %s, got %v", app.ID, got.AppID)
	}
}

func TestLinkContainerApp_CreateMode_InvalidProfileID(t *testing.T) {
	router, db := newDiscoveryLinkRouter(t, &stubbedProfiles{})
	engineID := seedDockerEngineForLink(t, db)
	containerID := seedDiscoveredContainer(t, db, engineID)

	body, _ := json.Marshal(map[string]any{"mode": "create", "profile_id": "nonexistent", "name": "x"})
	req := httptest.NewRequest(http.MethodPost, "/discovered-containers/"+containerID+"/link-app", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestLinkContainerApp_NotFound(t *testing.T) {
	router, _ := newDiscoveryLinkRouter(t, &stubbedProfiles{})
	body, _ := json.Marshal(map[string]any{"mode": "existing", "app_id": "x"})
	req := httptest.NewRequest(http.MethodPost, "/discovered-containers/does-not-exist/link-app", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// ── UnlinkContainerApp ────────────────────────────────────────────────────────

func TestUnlinkContainerApp_OK(t *testing.T) {
	router, db := newDiscoveryLinkRouter(t, &stubbedProfiles{})
	engineID := seedDockerEngineForLink(t, db)
	containerID := seedDiscoveredContainer(t, db, engineID)
	appID := seedAppForLink(t, db)

	// Link first.
	linkBody, _ := json.Marshal(map[string]any{"mode": "existing", "app_id": appID})
	linkReq := httptest.NewRequest(http.MethodPost, "/discovered-containers/"+containerID+"/link-app", bytes.NewReader(linkBody))
	linkReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), linkReq)

	// Now unlink.
	req := httptest.NewRequest(http.MethodDelete, "/discovered-containers/"+containerID+"/link-app", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}
	// Verify app_id is null.
	checkDB := repo.NewDiscoveredContainerRepo(db)
	got, _ := checkDB.GetDiscoveredContainer(t.Context(), containerID)
	if got.AppID != nil {
		t.Errorf("app_id: want nil, got %v", got.AppID)
	}
	// Verify the app still exists.
	appRepo := repo.NewAppRepo(db)
	if _, err := appRepo.Get(t.Context(), appID); err != nil {
		t.Errorf("app should still exist after unlink: %v", err)
	}
}

func TestUnlinkContainerApp_NotFound(t *testing.T) {
	router, _ := newDiscoveryLinkRouter(t, &stubbedProfiles{})
	req := httptest.NewRequest(http.MethodDelete, "/discovered-containers/does-not-exist/link-app", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// ── LinkRouteApp ──────────────────────────────────────────────────────────────

func TestLinkRouteApp_ExistingMode_OK(t *testing.T) {
	router, db := newDiscoveryLinkRouter(t, &stubbedProfiles{})
	infraID := seedInfraComponentForLink(t, db)
	domain := "sonarr.example.com"
	routeID := seedDiscoveredRoute(t, db, infraID, &domain)
	appID := seedAppForLink(t, db)

	body, _ := json.Marshal(map[string]any{"mode": "existing", "app_id": appID})
	req := httptest.NewRequest(http.MethodPost, "/discovered-routes/"+routeID+"/link-app", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// SSL check should have been auto-created.
	checkRepo := repo.NewCheckRepo(db)
	exists, err := checkRepo.ExistsForTypeAndTarget(t.Context(), "ssl", domain)
	if err != nil {
		t.Fatalf("exists check: %v", err)
	}
	if !exists {
		t.Error("expected SSL check to be auto-created for domain")
	}
}

func TestLinkRouteApp_NoDomain_NoSSLCheck(t *testing.T) {
	router, db := newDiscoveryLinkRouter(t, &stubbedProfiles{})
	infraID := seedInfraComponentForLink(t, db)
	routeID := seedDiscoveredRoute(t, db, infraID, nil) // no domain
	appID := seedAppForLink(t, db)

	body, _ := json.Marshal(map[string]any{"mode": "existing", "app_id": appID})
	req := httptest.NewRequest(http.MethodPost, "/discovered-routes/"+routeID+"/link-app", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	// No SSL check created for domain-less routes.
	checkRepo := repo.NewCheckRepo(db)
	checks, _ := checkRepo.List(t.Context())
	for _, c := range checks {
		if c.Type == "ssl" {
			t.Error("unexpected SSL check created for route without domain")
		}
	}
}

func TestLinkRouteApp_NoDuplicateSSLCheck(t *testing.T) {
	router, db := newDiscoveryLinkRouter(t, &stubbedProfiles{})
	infraID := seedInfraComponentForLink(t, db)
	domain := "dupe.example.com"
	routeID := seedDiscoveredRoute(t, db, infraID, &domain)
	appID := seedAppForLink(t, db)

	// Pre-create an SSL check for the same domain.
	checkRepo := repo.NewCheckRepo(db)
	existing := &models.MonitorCheck{
		ID:           uuid.NewString(),
		Name:         "SSL — " + domain,
		Type:         "ssl",
		Target:       domain,
		IntervalSecs: 3600,
		SSLWarnDays:  30,
		SSLCritDays:  7,
		Enabled:      true,
		CreatedAt:    time.Now().UTC(),
	}
	if err := checkRepo.Create(t.Context(), existing); err != nil {
		t.Fatalf("pre-create check: %v", err)
	}

	// Link the route.
	body, _ := json.Marshal(map[string]any{"mode": "existing", "app_id": appID})
	req := httptest.NewRequest(http.MethodPost, "/discovered-routes/"+routeID+"/link-app", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Still only one SSL check for this domain.
	checks, _ := checkRepo.List(t.Context())
	count := 0
	for _, c := range checks {
		if c.Type == "ssl" && c.Target == domain {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 SSL check for domain, got %d", count)
	}
}

func TestLinkRouteApp_CreateMode_OK(t *testing.T) {
	router, db := newDiscoveryLinkRouter(t, &stubbedProfiles{})
	infraID := seedInfraComponentForLink(t, db)
	domain := "create.example.com"
	routeID := seedDiscoveredRoute(t, db, infraID, &domain)

	body, _ := json.Marshal(map[string]any{
		"mode":       "create",
		"profile_id": "sonarr",
		"name":       "Create Route App",
	})
	req := httptest.NewRequest(http.MethodPost, "/discovered-routes/"+routeID+"/link-app", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var app models.App
	if err := json.NewDecoder(rr.Body).Decode(&app); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if app.ID == "" {
		t.Error("app.ID must not be empty")
	}
	// SSL check auto-created.
	checkRepo := repo.NewCheckRepo(db)
	exists, _ := checkRepo.ExistsForTypeAndTarget(t.Context(), "ssl", domain)
	if !exists {
		t.Error("expected SSL check auto-created for route domain")
	}
}

func TestLinkRouteApp_InvalidAppID(t *testing.T) {
	router, db := newDiscoveryLinkRouter(t, &stubbedProfiles{})
	infraID := seedInfraComponentForLink(t, db)
	routeID := seedDiscoveredRoute(t, db, infraID, nil)

	body, _ := json.Marshal(map[string]any{"mode": "existing", "app_id": "nope"})
	req := httptest.NewRequest(http.MethodPost, "/discovered-routes/"+routeID+"/link-app", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rr.Code)
	}
}

func TestLinkRouteApp_InvalidProfileID(t *testing.T) {
	router, db := newDiscoveryLinkRouter(t, &stubbedProfiles{})
	infraID := seedInfraComponentForLink(t, db)
	routeID := seedDiscoveredRoute(t, db, infraID, nil)

	body, _ := json.Marshal(map[string]any{"mode": "create", "profile_id": "unknown", "name": "x"})
	req := httptest.NewRequest(http.MethodPost, "/discovered-routes/"+routeID+"/link-app", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rr.Code)
	}
}

// ── UnlinkRouteApp ────────────────────────────────────────────────────────────

func TestUnlinkRouteApp_OK(t *testing.T) {
	router, db := newDiscoveryLinkRouter(t, &stubbedProfiles{})
	infraID := seedInfraComponentForLink(t, db)
	routeID := seedDiscoveredRoute(t, db, infraID, nil)
	appID := seedAppForLink(t, db)

	// Link first.
	linkBody, _ := json.Marshal(map[string]any{"mode": "existing", "app_id": appID})
	linkReq := httptest.NewRequest(http.MethodPost, "/discovered-routes/"+routeID+"/link-app", bytes.NewReader(linkBody))
	linkReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), linkReq)

	req := httptest.NewRequest(http.MethodDelete, "/discovered-routes/"+routeID+"/link-app", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}
	routeRepo := repo.NewDiscoveredRouteRepo(db)
	got, _ := routeRepo.GetDiscoveredRoute(t.Context(), routeID)
	if got.AppID != nil {
		t.Errorf("app_id: want nil after unlink, got %v", got.AppID)
	}
	// App still exists.
	appRepo := repo.NewAppRepo(db)
	if _, err := appRepo.Get(t.Context(), appID); err != nil {
		t.Errorf("app should still exist after unlink: %v", err)
	}
}

func TestUnlinkRouteApp_NotFound(t *testing.T) {
	router, _ := newDiscoveryLinkRouter(t, &stubbedProfiles{})
	req := httptest.NewRequest(http.MethodDelete, "/discovered-routes/does-not-exist/link-app", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}
