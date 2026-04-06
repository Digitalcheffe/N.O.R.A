package infra

import (
	"strings"

	"github.com/digitalcheffe/nora/internal/apptemplate"
)

// MatchResult holds the outcome of a profile-matching attempt.
type MatchResult struct {
	ProfileID  string
	Confidence int // 0-100
}

// MatchContainerToProfile attempts to match a container to a profile in the
// registry using name and image heuristics. Returns nil when no match scores
// 70 or above. Rules are applied in order; the first match wins.
//
// Rule precedence (highest confidence first):
//  1. Exact container name == profile ID                                  → 95
//  2. Image base name (no tag, no registry prefix) == profile ID         → 85
//  3. Container name contains profile ID as substring                    → 80
//  4. Image base name contains profile ID as substring                   → 70
func MatchContainerToProfile(containerName, image string, registry *apptemplate.Registry) *MatchResult {
	if registry == nil {
		return nil
	}

	lName := strings.ToLower(containerName)
	lImageBase := strings.ToLower(imageBaseName(image))

	profiles := registry.List()

	// We apply rules in priority order across all profiles; collect the best.
	type candidate struct {
		profileID  string
		confidence int
	}
	var best *candidate

	for profileID := range profiles {
		lID := strings.ToLower(profileID)

		conf := 0
		switch {
		case lName == lID:
			conf = 95
		case lImageBase == lID:
			conf = 85
		case strings.Contains(lName, lID):
			conf = 80
		case strings.Contains(lImageBase, lID):
			conf = 70
		}

		if conf >= 70 && (best == nil || conf > best.confidence) {
			best = &candidate{profileID: profileID, confidence: conf}
		}
	}

	if best == nil {
		return nil
	}
	return &MatchResult{ProfileID: best.profileID, Confidence: best.confidence}
}

// imageBaseName strips the registry prefix and tag from a Docker image string,
// returning only the repository base name.
//
// Examples:
//
//	"lscr.io/linuxserver/sonarr:latest" → "sonarr"
//	"nginx:1.25"                        → "nginx"
//	"ghcr.io/home-assistant/home-assistant:stable" → "home-assistant"
func imageBaseName(image string) string {
	// Strip tag.
	if i := strings.LastIndex(image, ":"); i != -1 {
		// Ensure the colon is not part of a registry host:port
		if !strings.Contains(image[i:], "/") {
			image = image[:i]
		}
	}

	// Strip registry/organisation prefix — take only the last path segment.
	if i := strings.LastIndex(image, "/"); i != -1 {
		image = image[i+1:]
	}

	return image
}
