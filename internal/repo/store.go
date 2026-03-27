package repo

// Store bundles all repository interfaces into a single dependency.
type Store struct {
	Apps            AppRepo
	Events          EventRepo
	Checks          CheckRepo
	Rollups         RollupRepo
	Resources       ResourceReadingRepo
	ResourceRollups ResourceRollupRepo
	PhysicalHosts   PhysicalHostRepo
	VirtualHosts    VirtualHostRepo
	DockerEngines   DockerEngineRepo
	Infra           InfraRepo
}

// NewStore creates a Store backed by the given repositories.
func NewStore(apps AppRepo, events EventRepo, checks CheckRepo, rollups RollupRepo, resources ResourceReadingRepo, resourceRollups ResourceRollupRepo, physicalHosts PhysicalHostRepo, virtualHosts VirtualHostRepo, dockerEngines DockerEngineRepo, infra InfraRepo) *Store {
	return &Store{
		Apps:            apps,
		Events:          events,
		Checks:          checks,
		Rollups:         rollups,
		Resources:       resources,
		ResourceRollups: resourceRollups,
		PhysicalHosts:   physicalHosts,
		VirtualHosts:    virtualHosts,
		DockerEngines:   dockerEngines,
		Infra:           infra,
	}
}
