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
func (f *Fetcher) EnsureIcon(profileID string) {
	if profileID == "" {
		return
	}
	if _, err := os.Stat(f.iconPath(profileID)); err == nil {
		return // already cached
	}
	go f.fetch(context.Background(), profileID)
}

// FetchAll ensures icons exist for all given profile IDs. Runs concurrently.
// Safe to call from main on startup.
func (f *Fetcher) FetchAll(ctx context.Context, profileIDs []string) {
	seen := make(map[string]bool)
	for _, id := range profileIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := os.Stat(f.iconPath(id)); err == nil {
			continue // already cached
		}
		id := id
		go f.fetch(ctx, id)
	}
}

// ServeIcon writes the cached SVG to w, returning true on success.
func (f *Fetcher) ServeIcon(w http.ResponseWriter, profileID string) bool {
	data, err := os.ReadFile(f.iconPath(profileID))
	if err != nil {
		return false
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
	return true
}

func (f *Fetcher) iconPath(profileID string) string {
	return filepath.Join(f.iconsDir, profileID+".svg")
}

func (f *Fetcher) fetch(ctx context.Context, profileID string) {
	// Try exact name first, then hyphen variant (e.g. adguardhome → adguard-home)
	candidates := []string{profileID}
	if hyph := strings.ReplaceAll(profileID, "_", "-"); hyph != profileID {
		candidates = append(candidates, hyph)
	}
	// Also try the profile name without common suffixes
	if strings.HasSuffix(profileID, "home") {
		candidates = append(candidates, profileID[:len(profileID)-4]+"-home")
	}

	for _, name := range candidates {
		url := fmt.Sprintf("%s/%s.svg", cdnBase, name)
		data, err := f.download(ctx, url)
		if err != nil {
			continue
		}
		if err := os.WriteFile(f.iconPath(profileID), data, 0644); err != nil {
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
