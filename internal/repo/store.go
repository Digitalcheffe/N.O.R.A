package repo

// Store bundles all repository interfaces into a single dependency.
type Store struct {
	Apps            AppRepo
	Events          EventRepo
	Checks          CheckRepo
	Rollups         RollupRepo
	Resources       ResourceReadingRepo
	ResourceRollups ResourceRollupRepo
	InfraComponents InfraComponentRepo
	DockerEngines   DockerEngineRepo
	Infra           InfraRepo
	Settings        SettingsRepo
	Metrics         MetricsRepo
	Users           UserRepo
}

// NewStore creates a Store backed by the given repositories.
func NewStore(
	apps AppRepo,
	events EventRepo,
	checks CheckRepo,
	rollups RollupRepo,
	resources ResourceReadingRepo,
	resourceRollups ResourceRollupRepo,
	infraComponents InfraComponentRepo,
	dockerEngines DockerEngineRepo,
	infra InfraRepo,
	settings SettingsRepo,
	metrics MetricsRepo,
	users UserRepo,
) *Store {
	return &Store{
		Apps:            apps,
		Events:          events,
		Checks:          checks,
		Rollups:         rollups,
		Resources:       resources,
		ResourceRollups: resourceRollups,
		InfraComponents: infraComponents,
		DockerEngines:   dockerEngines,
		Infra:           infra,
		Settings:        settings,
		Metrics:         metrics,
		Users:           users,
	}
}
