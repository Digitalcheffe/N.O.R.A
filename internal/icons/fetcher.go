package icons

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const cdnBase = "https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons/svg"

// Fetcher downloads and caches app icons from the dashboard-icons CDN.
type Fetcher struct {
	iconsDir string
	client   *http.Client
}

// New creates a Fetcher that stores icons in iconsDir.
func New(iconsDir string) (*Fetcher, error) {
	if err := os.MkdirAll(iconsDir, 0755); err != nil {
		return nil, fmt.Errorf("create icons dir: %w", err)
	}
	return &Fetcher{
		iconsDir: iconsDir,
		client:   &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// EnsureIcon fetches the icon for profileID in the background if not already cached.
// iconSlug is the dashboard-icons CDN slug from the app template (e.g. "wireguard");
// if non-empty it is tried first on the CDN before falling back to profileID.
// The result is always stored as profileID.svg so the icon URL stays stable.
func (f *Fetcher) EnsureIcon(profileID, iconSlug string) {
	if !isValidIconID(profileID) {
		return
	}
	p, err := f.safeIconPath(profileID)
	if err != nil {
		return
	}
	if _, err := os.Stat(p); err == nil {
		return // already cached
	}
	go f.fetch(context.Background(), profileID, iconSlug)
}

// FetchAll ensures icons exist for all given profile IDs. Runs concurrently.
// slugOverrides maps profileID → CDN icon slug; pass nil if no overrides.
// Safe to call from main on startup.
func (f *Fetcher) FetchAll(ctx context.Context, profileIDs []string, slugOverrides map[string]string) {
	seen := make(map[string]bool)
	for _, id := range profileIDs {
		if !isValidIconID(id) || seen[id] {
			continue
		}
		seen[id] = true
		p, err := f.safeIconPath(id)
		if err != nil {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			continue // already cached
		}
		id := id
		slug := slugOverrides[id]
		go f.fetch(ctx, id, slug)
	}
}

// ServeIcon writes the cached SVG to w, returning true on success.
func (f *Fetcher) ServeIcon(w http.ResponseWriter, profileID string) bool {
	if !isValidIconID(profileID) {
		return false
	}
	p, err := f.safeIconPath(profileID)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
	return true
}

// isValidIconID returns true if id contains only alphanumeric characters,
// hyphens, or underscores — safe to use as a filename component.
func isValidIconID(id string) bool {
	if id == "" {
		return false
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// safeIconPath returns the absolute path for profileID's cached SVG file,
// validating that the resolved path stays within iconsDir.
func (f *Fetcher) safeIconPath(profileID string) (string, error) {
	base := filepath.Clean(f.iconsDir)
	candidate := filepath.Join(base, profileID+".svg")
	if !strings.HasPrefix(candidate, base+string(os.PathSeparator)) {
		return "", fmt.Errorf("icons: path traversal detected for id %q", profileID)
	}
	return candidate, nil
}

func (f *Fetcher) fetch(ctx context.Context, profileID, iconSlug string) {
	// Build CDN candidate list. The template's icon slug (if any) takes priority
	// so authors can point directly to any dashboard-icons name regardless of profileID.
	var candidates []string
	if iconSlug != "" && iconSlug != profileID {
		candidates = append(candidates, iconSlug)
	}
	// Try exact profileID, then hyphen variant (e.g. adguardhome → adguard-home)
	candidates = append(candidates, profileID)
	if hyph := strings.ReplaceAll(profileID, "_", "-"); hyph != profileID {
		candidates = append(candidates, hyph)
	}
	// Also try the profile name without common suffixes
	if strings.HasSuffix(profileID, "home") {
		candidates = append(candidates, profileID[:len(profileID)-4]+"-home")
	}

	dest, err := f.safeIconPath(profileID)
	if err != nil {
		log.Printf("icons: %v", err)
		return
	}

	for _, name := range candidates {
		url := fmt.Sprintf("%s/%s.svg", cdnBase, name)
		data, err := f.download(ctx, url)
		if err != nil {
			continue
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			log.Printf("icons: write %s: %v", profileID, err)
			return
		}
		log.Printf("icons: cached %s (as %s)", profileID, name)
		return
	}
	// Not found in CDN — silent, no icon available for this profile
}

func (f *Fetcher) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
