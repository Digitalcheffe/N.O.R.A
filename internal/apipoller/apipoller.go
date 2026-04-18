// Package apipoller contains small helpers shared by the API poller.
// Auth configuration now lives per-app in app settings (with profile
// defaults declared inside the api_polling block) rather than in static
// files in this package.
package apipoller

// ResolveAPIBaseURL returns the URL the API poller should use for a given app
// config. It prefers api_url (an explicit internal/API-only address) and falls
// back to base_url. Returns an empty string if neither is set.
func ResolveAPIBaseURL(cfg map[string]interface{}) string {
	if v, _ := cfg["api_url"].(string); v != "" {
		return v
	}
	v, _ := cfg["base_url"].(string)
	return v
}
