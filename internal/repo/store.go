package repo

import "github.com/jmoiron/sqlx"

// Store holds all repository implementations and is the single access
// point for database operations throughout the application.
type Store struct {
	Users            UserRepository
	Apps             AppRepository
	Events           EventRepository
	Checks           MonitorCheckRepository
	Rollups          RollupRepository
	Metrics          MetricsRepository
	PhysicalHosts    PhysicalHostRepository
	VirtualHosts     VirtualHostRepository
	DockerEngines    DockerEngineRepository
	ResourceReadings ResourceReadingRepository
	ResourceRollups  ResourceRollupRepository
}

// NewStore creates a Store wired to the provided database connection.
func NewStore(db *sqlx.DB) *Store {
	return &Store{
		Users:            &sqliteUserRepo{db},
		Apps:             &sqliteAppRepo{db},
		Events:           &sqliteEventRepo{db},
		Checks:           &sqliteMonitorCheckRepo{db},
		Rollups:          &sqliteRollupRepo{db},
		Metrics:          &sqliteMetricsRepo{db},
		PhysicalHosts:    &sqlitePhysicalHostRepo{db},
		VirtualHosts:     &sqliteVirtualHostRepo{db},
		DockerEngines:    &sqliteDockerEngineRepo{db},
		ResourceReadings: &sqliteResourceReadingRepo{db},
		ResourceRollups:  &sqliteResourceRollupRepo{db},
	}
}
