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
	Settings             SettingsRepo
	Metrics              MetricsRepo
	Users                UserRepo
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
	settings SettingsRepo,
	metrics MetricsRepo,
	users UserRepo,
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
		Settings:             settings,
		Metrics:              metrics,
		Users:                users,
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
