package jobs

import (
	"context"
	"fmt"

	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/profile"
)

// RunTemplateReload re-reads every profile YAML from disk into the in-memory
// app template registry. Use it after editing a builtin or custom profile on
// disk so the running instance picks up the change without a restart.
//
// This does NOT touch the digest_registry table — cards only appear when a
// profile entry exists AND is active in the registry, so call
// RunDigestRegistryReconcile afterwards to make new categories/widgets render.
func RunTemplateReload(_ context.Context, registry *apptemplate.Registry) error {
	if registry == nil {
		return fmt.Errorf("template reload: registry is not configured")
	}
	if err := registry.Reload(); err != nil {
		return fmt.Errorf("template reload: %w", err)
	}
	return nil
}

// RunDigestRegistryReconcile syncs the digest_registry table with the current
// set of profile digest categories and widgets. New entries are upserted,
// entries no longer present in their profile are deactivated (but retained so
// users don't lose the active flag they may have toggled).
//
// Run after RunTemplateReload — reloading templates alone will not add new
// cards until the registry is reconciled.
func RunDigestRegistryReconcile(ctx context.Context, reconciler *profile.RegistryReconciler) error {
	if reconciler == nil {
		return fmt.Errorf("digest registry reconcile: reconciler is not configured")
	}
	if err := reconciler.Reconcile(ctx); err != nil {
		return fmt.Errorf("digest registry reconcile: %w", err)
	}
	return nil
}
