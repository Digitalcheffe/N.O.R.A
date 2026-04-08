package repo

import (
	"context"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/migrations"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// openDiscoveryTestDB opens an in-memory SQLite DB with all migrations applied.
// This ensures apps and infrastructure_components tables exist for FK constraints.
func openDiscoveryTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	cfg := &config.Config{DBPath: ":memory:"}
	db, err := Open(cfg, migrations.Files)
	if err != nil {
		t.Fatalf("open discovery test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// seedDockerEngine inserts a minimal infrastructure_components row of type
// docker_engine and returns its ID.
func seedDockerEngine(t *testing.T, db *sqlx.DB) string {
	t.Helper()
	id := uuid.NewString()
	_, err := db.Exec(`INSERT INTO infrastructure_components (id, name, type, collection_method) VALUES (?, 'test-docker-engine', 'docker_engine', 'docker_socket')`, id)
	if err != nil {
		t.Fatalf("seed docker engine component: %v", err)
	}
	return id
}

// seedInfraComponent inserts a minimal infrastructure_components row and returns its ID.
func seedInfraComponent(t *testing.T, db *sqlx.DB) string {
	t.Helper()
	id := uuid.NewString()
	_, err := db.Exec(`INSERT INTO infrastructure_components (id, name, type, collection_method) VALUES (?, 'test-infra', 'traefik', 'traefik_api')`, id)
	if err != nil {
		t.Fatalf("seed infra component: %v", err)
	}
	return id
}

// ── DiscoveredContainerRepo tests ─────────────────────────────────────────────

func TestDiscoveredContainerRepo_UpsertAndGet(t *testing.T) {
	db := openDiscoveryTestDB(t)
	engineID := seedDockerEngine(t, db)
	r := NewDiscoveredContainerRepo(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	c := &models.DiscoveredContainer{
		InfraComponentID: engineID,
		ContainerID:    "abc123",
		ContainerName:  "sonarr",
		Image:          "linuxserver/sonarr:latest",
		Status:         "running",
		LastSeenAt:     now,
		CreatedAt:      now,
	}

	if err := r.UpsertDiscoveredContainer(ctx, c); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if c.ID == "" {
		t.Fatal("ID must be populated after upsert")
	}

	got, err := r.GetDiscoveredContainer(ctx, c.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ContainerName != "sonarr" {
		t.Errorf("container name: want sonarr, got %s", got.ContainerName)
	}
	if got.Status != "running" {
		t.Errorf("status: want running, got %s", got.Status)
	}
}

func TestDiscoveredContainerRepo_UpsertUpdatesExisting(t *testing.T) {
	db := openDiscoveryTestDB(t)
	engineID := seedDockerEngine(t, db)
	r := NewDiscoveredContainerRepo(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	c := &models.DiscoveredContainer{
		InfraComponentID: engineID,
		ContainerID:    "abc123",
		ContainerName:  "sonarr",
		Image:          "linuxserver/sonarr:3.0",
		Status:         "running",
		LastSeenAt:     now,
		CreatedAt:      now,
	}
	if err := r.UpsertDiscoveredContainer(ctx, c); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	firstID := c.ID

	// Upsert same infra_component_id + container_name (conflict key) with updated fields.
	// The container_id (Docker hash) changes on rebuild — the name stays stable.
	c2 := &models.DiscoveredContainer{
		InfraComponentID: engineID,
		ContainerID:    "def456", // new Docker hash after container rebuild
		ContainerName:  "sonarr", // same name — this is the conflict key
		Image:          "linuxserver/sonarr:4.0",
		Status:         "stopped",
		LastSeenAt:     now.Add(time.Minute),
		CreatedAt:      now,
	}
	if err := r.UpsertDiscoveredContainer(ctx, c2); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := r.GetDiscoveredContainer(ctx, firstID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ContainerID != "def456" {
		t.Errorf("container_id: want def456, got %s", got.ContainerID)
	}
	if got.Status != "stopped" {
		t.Errorf("status: want stopped, got %s", got.Status)
	}
	if got.Image != "linuxserver/sonarr:4.0" {
		t.Errorf("image: want linuxserver/sonarr:4.0, got %s", got.Image)
	}
}

func TestDiscoveredContainerRepo_ListDiscoveredContainers(t *testing.T) {
	db := openDiscoveryTestDB(t)
	engineID := seedDockerEngine(t, db)
	engineID2 := seedDockerEngine(t, db)
	r := NewDiscoveredContainerRepo(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	for _, name := range []string{"alpha", "beta"} {
		c := &models.DiscoveredContainer{
			InfraComponentID: engineID,
			ContainerID:    name + "-id",
			ContainerName:  name,
			Image:          "img:latest",
			Status:         "running",
			LastSeenAt:     now,
			CreatedAt:      now,
		}
		if err := r.UpsertDiscoveredContainer(ctx, c); err != nil {
			t.Fatalf("upsert %s: %v", name, err)
		}
	}
	// Container belonging to a different engine — must not appear in filtered list.
	other := &models.DiscoveredContainer{
		InfraComponentID: engineID2,
		ContainerID:    "other-id",
		ContainerName:  "other",
		Image:          "img:latest",
		Status:         "running",
		LastSeenAt:     now,
		CreatedAt:      now,
	}
	if err := r.UpsertDiscoveredContainer(ctx, other); err != nil {
		t.Fatalf("upsert other: %v", err)
	}

	list, err := r.ListDiscoveredContainers(ctx, engineID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("want 2 containers for engine, got %d", len(list))
	}
}

func TestDiscoveredContainerRepo_ListAllDiscoveredContainers(t *testing.T) {
	db := openDiscoveryTestDB(t)
	engineID := seedDockerEngine(t, db)
	engineID2 := seedDockerEngine(t, db)
	r := NewDiscoveredContainerRepo(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	for i, eng := range []string{engineID, engineID2} {
		c := &models.DiscoveredContainer{
			InfraComponentID: eng,
			ContainerID:    "cid-" + string(rune('a'+i)),
			ContainerName:  "container-" + string(rune('a'+i)),
			Image:          "img:latest",
			Status:         "running",
			LastSeenAt:     now,
			CreatedAt:      now,
		}
		if err := r.UpsertDiscoveredContainer(ctx, c); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}

	all, err := r.ListAllDiscoveredContainers(ctx)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("want 2 total containers, got %d", len(all))
	}
}

func TestDiscoveredContainerRepo_SetDiscoveredContainerApp(t *testing.T) {
	db := openDiscoveryTestDB(t)
	engineID := seedDockerEngine(t, db)
	r := NewDiscoveredContainerRepo(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	c := &models.DiscoveredContainer{
		InfraComponentID: engineID,
		ContainerID:    "cid1",
		ContainerName:  "radarr",
		Image:          "img:latest",
		Status:         "running",
		LastSeenAt:     now,
		CreatedAt:      now,
	}
	if err := r.UpsertDiscoveredContainer(ctx, c); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Seed an app to satisfy the FK.
	appID := uuid.NewString()
	_, err := db.Exec(`INSERT INTO apps (id, name, token, rate_limit, created_at) VALUES (?, 'Radarr', 'tok1', 60, datetime('now'))`, appID)
	if err != nil {
		t.Fatalf("seed app: %v", err)
	}

	if err := r.SetDiscoveredContainerApp(ctx, c.ID, appID); err != nil {
		t.Fatalf("set app: %v", err)
	}

	got, err := r.GetDiscoveredContainer(ctx, c.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AppID == nil || *got.AppID != appID {
		t.Errorf("app_id: want %s, got %v", appID, got.AppID)
	}
}

func TestDiscoveredContainerRepo_SetDiscoveredContainerApp_NotFound(t *testing.T) {
	db := openDiscoveryTestDB(t)
	r := NewDiscoveredContainerRepo(db)
	ctx := context.Background()

	err := r.SetDiscoveredContainerApp(ctx, "nonexistent", "any-app")
	if err == nil {
		t.Fatal("expected error for nonexistent id, got nil")
	}
}

func TestDiscoveredContainerRepo_UpdateDiscoveredContainerStatus(t *testing.T) {
	db := openDiscoveryTestDB(t)
	engineID := seedDockerEngine(t, db)
	r := NewDiscoveredContainerRepo(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	c := &models.DiscoveredContainer{
		InfraComponentID: engineID,
		ContainerID:    "cid2",
		ContainerName:  "lidarr",
		Image:          "img:latest",
		Status:         "running",
		LastSeenAt:     now,
		CreatedAt:      now,
	}
	if err := r.UpsertDiscoveredContainer(ctx, c); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	later := now.Add(5 * time.Minute)
	if err := r.UpdateDiscoveredContainerStatus(ctx, c.ID, "stopped", later); err != nil {
		t.Fatalf("update status: %v", err)
	}

	got, err := r.GetDiscoveredContainer(ctx, c.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != "stopped" {
		t.Errorf("status: want stopped, got %s", got.Status)
	}
}

func TestDiscoveredContainerRepo_GetNotFound(t *testing.T) {
	db := openDiscoveryTestDB(t)
	r := NewDiscoveredContainerRepo(db)
	ctx := context.Background()

	_, err := r.GetDiscoveredContainer(ctx, "does-not-exist")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── DiscoveredRouteRepo tests ──────────────────────────────────────────────────

func TestDiscoveredRouteRepo_UpsertAndGet(t *testing.T) {
	db := openDiscoveryTestDB(t)
	infraID := seedInfraComponent(t, db)
	r := NewDiscoveredRouteRepo(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	domain := "sonarr.example.com"
	ro := &models.DiscoveredRoute{
		InfrastructureID: infraID,
		RouterName:       "sonarr-router",
		Rule:             "Host(`sonarr.example.com`)",
		Domain:           &domain,
		LastSeenAt:       now,
		CreatedAt:        now,
	}

	if err := r.UpsertDiscoveredRoute(ctx, ro); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if ro.ID == "" {
		t.Fatal("ID must be populated after upsert")
	}

	got, err := r.GetDiscoveredRoute(ctx, ro.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RouterName != "sonarr-router" {
		t.Errorf("router name: want sonarr-router, got %s", got.RouterName)
	}
	if got.Domain == nil || *got.Domain != "sonarr.example.com" {
		t.Errorf("domain: want sonarr.example.com, got %v", got.Domain)
	}
}

func TestDiscoveredRouteRepo_UpsertUpdatesExisting(t *testing.T) {
	db := openDiscoveryTestDB(t)
	infraID := seedInfraComponent(t, db)
	r := NewDiscoveredRouteRepo(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	d1 := "old.example.com"
	ro := &models.DiscoveredRoute{
		InfrastructureID: infraID,
		RouterName:       "my-router",
		Rule:             "Host(`old.example.com`)",
		Domain:           &d1,
		LastSeenAt:       now,
		CreatedAt:        now,
	}
	if err := r.UpsertDiscoveredRoute(ctx, ro); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	firstID := ro.ID

	d2 := "new.example.com"
	ro2 := &models.DiscoveredRoute{
		InfrastructureID: infraID,
		RouterName:       "my-router",
		Rule:             "Host(`new.example.com`)",
		Domain:           &d2,
		LastSeenAt:       now.Add(time.Minute),
		CreatedAt:        now,
	}
	if err := r.UpsertDiscoveredRoute(ctx, ro2); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := r.GetDiscoveredRoute(ctx, firstID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Domain == nil || *got.Domain != "new.example.com" {
		t.Errorf("domain: want new.example.com, got %v", got.Domain)
	}
}

func TestDiscoveredRouteRepo_ListDiscoveredRoutes(t *testing.T) {
	db := openDiscoveryTestDB(t)
	infraID := seedInfraComponent(t, db)
	infraID2 := seedInfraComponent(t, db)
	r := NewDiscoveredRouteRepo(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	for _, name := range []string{"router-a", "router-b"} {
		ro := &models.DiscoveredRoute{
			InfrastructureID: infraID,
			RouterName:       name,
			Rule:             "Host(`" + name + ".example.com`)",
			LastSeenAt:       now,
			CreatedAt:        now,
		}
		if err := r.UpsertDiscoveredRoute(ctx, ro); err != nil {
			t.Fatalf("upsert %s: %v", name, err)
		}
	}
	other := &models.DiscoveredRoute{
		InfrastructureID: infraID2,
		RouterName:       "other-router",
		Rule:             "Host(`other.example.com`)",
		LastSeenAt:       now,
		CreatedAt:        now,
	}
	if err := r.UpsertDiscoveredRoute(ctx, other); err != nil {
		t.Fatalf("upsert other: %v", err)
	}

	list, err := r.ListDiscoveredRoutes(ctx, infraID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("want 2 routes for infra, got %d", len(list))
	}
}

func TestDiscoveredRouteRepo_ListAllDiscoveredRoutes(t *testing.T) {
	db := openDiscoveryTestDB(t)
	infraID := seedInfraComponent(t, db)
	infraID2 := seedInfraComponent(t, db)
	r := NewDiscoveredRouteRepo(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	for i, iid := range []string{infraID, infraID2} {
		ro := &models.DiscoveredRoute{
			InfrastructureID: iid,
			RouterName:       "router-" + string(rune('a'+i)),
			Rule:             "Host(`x.example.com`)",
			LastSeenAt:       now,
			CreatedAt:        now,
		}
		if err := r.UpsertDiscoveredRoute(ctx, ro); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}

	all, err := r.ListAllDiscoveredRoutes(ctx)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("want 2 total routes, got %d", len(all))
	}
}

func TestDiscoveredRouteRepo_SetDiscoveredRouteApp(t *testing.T) {
	db := openDiscoveryTestDB(t)
	infraID := seedInfraComponent(t, db)
	r := NewDiscoveredRouteRepo(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	d := "sonarr.example.com"
	ro := &models.DiscoveredRoute{
		InfrastructureID: infraID,
		RouterName:       "sonarr",
		Rule:             "Host(`sonarr.example.com`)",
		Domain:           &d,
		LastSeenAt:       now,
		CreatedAt:        now,
	}
	if err := r.UpsertDiscoveredRoute(ctx, ro); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	appID := uuid.NewString()
	_, err := db.Exec(`INSERT INTO apps (id, name, token, rate_limit, created_at) VALUES (?, 'Sonarr', 'tok2', 60, datetime('now'))`, appID)
	if err != nil {
		t.Fatalf("seed app: %v", err)
	}

	if err := r.SetDiscoveredRouteApp(ctx, ro.ID, appID); err != nil {
		t.Fatalf("set app: %v", err)
	}

	got, err := r.GetDiscoveredRoute(ctx, ro.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AppID == nil || *got.AppID != appID {
		t.Errorf("app_id: want %s, got %v", appID, got.AppID)
	}
}

func TestDiscoveredRouteRepo_SetDiscoveredRouteApp_NotFound(t *testing.T) {
	db := openDiscoveryTestDB(t)
	r := NewDiscoveredRouteRepo(db)
	ctx := context.Background()

	err := r.SetDiscoveredRouteApp(ctx, "nonexistent", "any-app")
	if err == nil {
		t.Fatal("expected error for nonexistent id, got nil")
	}
}

func TestDiscoveredRouteRepo_GetNotFound(t *testing.T) {
	db := openDiscoveryTestDB(t)
	r := NewDiscoveredRouteRepo(db)
	ctx := context.Background()

	_, err := r.GetDiscoveredRoute(ctx, "does-not-exist")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
