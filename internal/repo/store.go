package repo

// Store bundles all repository interfaces into a single dependency.
type Store struct {
	Apps                 AppRepo
	Events               EventRepo
	Checks               CheckRepo
	Rollups              RollupRepo
	Resources            ResourceReadingRepo
	ResourceRollups      ResourceRollupRepo
	InfraComponents      InfraComponentRepo
	DockerEngines        DockerEngineRepo
	Infra                InfraRepo
	Settings             SettingsRepo
	Metrics              MetricsRepo
	Users                UserRepo
	TraefikComponents    TraefikComponentRepo
	TraefikOverview      TraefikOverviewRepo
	TraefikServices      TraefikServiceRepo
	DiscoveredContainers DiscoveredContainerRepo
	DiscoveredRoutes     DiscoveredRouteRepo
	WebPushSubscriptions WebPushSubscriptionRepo
	Snapshots            SnapshotRepo
	Rules                RuleRepo
	DigestRegistry       DigestRegistryRepo
	AppMetricSnapshots   AppMetricSnapshotRepo
	ComponentLinks       ComponentLinkRepo
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
	traefikComponents TraefikComponentRepo,
	traefikOverview TraefikOverviewRepo,
	traefikServices TraefikServiceRepo,
	discoveredContainers DiscoveredContainerRepo,
	discoveredRoutes DiscoveredRouteRepo,
	webPushSubscriptions WebPushSubscriptionRepo,
	snapshots SnapshotRepo,
	rules RuleRepo,
	digestRegistry DigestRegistryRepo,
	appMetricSnapshots AppMetricSnapshotRepo,
	componentLinks ComponentLinkRepo,
) *Store {
	return &Store{
		Apps:                 apps,
		Events:               events,
		Checks:               checks,
		Rollups:              rollups,
		Resources:            resources,
		ResourceRollups:      resourceRollups,
		InfraComponents:      infraComponents,
		DockerEngines:        dockerEngines,
		Infra:                infra,
		Settings:             settings,
		Metrics:              metrics,
		Users:                users,
		TraefikComponents:    traefikComponents,
		TraefikOverview:      traefikOverview,
		TraefikServices:      traefikServices,
		DiscoveredContainers: discoveredContainers,
		DiscoveredRoutes:     discoveredRoutes,
		WebPushSubscriptions: webPushSubscriptions,
		Snapshots:            snapshots,
		Rules:                rules,
		DigestRegistry:       digestRegistry,
		AppMetricSnapshots:   appMetricSnapshots,
		ComponentLinks:       componentLinks,
	}
}
