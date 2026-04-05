package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9_]+`)

// slugify converts a label to a stable name: lowercase, spaces to underscores,
// strip non-alphanumeric except underscores.
func slugify(label string) string {
	s := strings.ToLower(label)
	s = strings.ReplaceAll(s, " ", "_")
	s = nonAlphanumRe.ReplaceAllString(s, "")
	return s
}

// RegistryReconciler reconciles digest entries from app templates into the digest registry.
// It runs once at startup after all templates are loaded.
type RegistryReconciler struct {
	repo     repo.DigestRegistryRepo
	registry *apptemplate.Registry
}

// NewRegistryReconciler creates a RegistryReconciler.
func NewRegistryReconciler(r repo.DigestRegistryRepo, registry *apptemplate.Registry) *RegistryReconciler {
	return &RegistryReconciler{repo: r, registry: registry}
}

// Reconcile iterates over all loaded app templates, upserts their digest categories
// and widgets into the registry, and deactivates entries no longer present in the template.
func (rc *RegistryReconciler) Reconcile(ctx context.Context) error {
	templates := rc.registry.List()
	now := time.Now().UTC()

	for profileID, tmpl := range templates {
		hasEntries := len(tmpl.Digest.Categories) > 0 || len(tmpl.Digest.Widgets) > 0
		if !hasEntries {
			continue
		}

		profileSource := tmpl.SourcePath

		// Build the set of names declared in the current template to detect removals.
		currentNames := make(map[string]struct{})

		for _, cat := range tmpl.Digest.Categories {
			name := slugify(cat.Label)
			if name == "" {
				continue
			}
			currentNames[name] = struct{}{}

			source := cat.Source
			if source == "" {
				source = "webhook"
			}

			config, err := categoryConfig(cat)
			if err != nil {
				return fmt.Errorf("reconcile %s/%s: %w", profileID, name, err)
			}

			entry := models.DigestRegistryEntry{
				ID:            uuid.NewString(),
				ProfileID:     profileID,
				Source:        source,
				EntryType:     "category",
				Name:          name,
				Label:         cat.Label,
				Config:        config,
				ProfileSource: profileSource,
				Active:        true,
				CreatedAt:     now,
				UpdatedAt:     now,
			}

			if err := rc.repo.Upsert(ctx, entry); err != nil {
				return fmt.Errorf("upsert %s/%s: %w", profileID, name, err)
			}
			log.Printf("digest registry: upserted %s/%s (%s)", profileID, name, cat.Label)
		}

		for _, w := range tmpl.Digest.Widgets {
			// Prefix widget names to avoid UNIQUE(profile_id, name) collisions with categories.
			name := "widget_" + slugify(w.Label)
			if name == "widget_" {
				continue
			}
			currentNames[name] = struct{}{}

			config, err := widgetConfig(w)
			if err != nil {
				return fmt.Errorf("reconcile widget %s/%s: %w", profileID, name, err)
			}

			entry := models.DigestRegistryEntry{
				ID:            uuid.NewString(),
				ProfileID:     profileID,
				Source:        w.Source,
				EntryType:     "widget",
				Name:          name,
				Label:         w.Label,
				Config:        config,
				ProfileSource: profileSource,
				Active:        true,
				CreatedAt:     now,
				UpdatedAt:     now,
			}

			if err := rc.repo.Upsert(ctx, entry); err != nil {
				return fmt.Errorf("upsert widget %s/%s: %w", profileID, name, err)
			}
			log.Printf("digest registry: upserted widget %s/%s (%s)", profileID, name, w.Label)
		}

		// Deactivate entries present in DB but not in the current template.
		existing, err := rc.repo.ListByProfile(ctx, profileID)
		if err != nil {
			return fmt.Errorf("list registry for %s: %w", profileID, err)
		}
		for _, e := range existing {
			if _, ok := currentNames[e.Name]; !ok && e.Active {
				if err := rc.repo.SetActive(ctx, profileID, e.Name, false); err != nil {
					return fmt.Errorf("deactivate %s/%s: %w", profileID, e.Name, err)
				}
				log.Printf("digest registry: deactivated %s/%s (not in current template)", profileID, e.Name)
			}
		}
	}

	return nil
}

// categoryConfig encodes the match fields from a DigestCategory as a JSON config blob.
func categoryConfig(cat apptemplate.DigestCategory) (models.JSONText, error) {
	m := map[string]string{
		"match_field": cat.MatchField,
		"match_value": cat.MatchValue,
	}
	if cat.MatchSeverity != "" {
		m["match_severity"] = cat.MatchSeverity
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return models.JSONText(b), nil
}

// widgetConfig encodes the relevant fields from a DigestWidget as a JSON config blob.
func widgetConfig(w apptemplate.DigestWidget) (models.JSONText, error) {
	m := map[string]string{}
	if w.Metric != "" {
		m["metric"] = w.Metric
	}
	if w.MatchField != "" {
		m["match_field"] = w.MatchField
	}
	if w.MatchValue != "" {
		m["match_value"] = w.MatchValue
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return models.JSONText(b), nil
}
