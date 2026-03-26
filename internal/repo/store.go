package repo

// Store bundles all repository interfaces into a single dependency.
type Store struct {
	Apps   AppRepo
	Events EventRepo
}

// NewStore creates a Store backed by the given repositories.
func NewStore(apps AppRepo, events EventRepo) *Store {
	return &Store{Apps: apps, Events: events}
}
