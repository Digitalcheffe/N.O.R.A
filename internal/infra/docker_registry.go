package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// manifestAcceptHeader is the Accept header value sent to registries.  It covers
// OCI image index, Docker manifest list, and single-arch Docker manifests in
// priority order so we get the most specific manifest available.
const manifestAcceptHeader = "application/vnd.oci.image.index.v1+json," +
	"application/vnd.docker.distribution.manifest.list.v2+json," +
	"application/vnd.docker.distribution.manifest.v2+json," +
	"application/vnd.oci.image.manifest.v1+json"

// httpDoer is the subset of *http.Client used by RegistryClient, enabling
// mock injection in tests.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// RegistryClient fetches manifest digests from container registries without
// pulling image data.  It supports Docker Hub, GHCR, lscr.io, and any other
// OCI-compliant registry via an unauthenticated anonymous token flow.
type RegistryClient struct {
	httpClient       httpDoer
	warnedRegistries sync.Map // registry host → struct{}{} — logged once per host
}

// NewRegistryClient creates a RegistryClient with a default HTTP client.
func NewRegistryClient() *RegistryClient {
	return &RegistryClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// GetLatestDigest returns the Docker-Content-Digest of the latest manifest for
// image (a full image:tag string such as "ghcr.io/meeb/tubesync:latest" or
// "linuxserver/sonarr:latest").
//
// Error handling:
//   - Rate limit (429): returns an error so the caller can skip this cycle.
//   - Auth failure / network error: returns an error so the caller can skip.
//   - Unknown registry: attempts unauthenticated OCI flow; logs a one-time
//     warning and returns an error if that fails.
func (r *RegistryClient) GetLatestDigest(ctx context.Context, image string) (string, error) {
	registry, repo, tag := parseImageRef(image)

	switch registry {
	case "docker.io":
		return r.getDockerHubDigest(ctx, repo, tag)
	case "ghcr.io", "lscr.io":
		return r.getOCIDigest(ctx, registry, repo, tag)
	default:
		digest, err := r.getOCIDigest(ctx, registry, repo, tag)
		if err != nil {
			if _, alreadyLogged := r.warnedRegistries.LoadOrStore(registry, struct{}{}); !alreadyLogged {
				log.Printf("image update poller: unknown registry %q, unauthenticated attempt failed: %v", registry, err)
			}
			return "", err
		}
		return digest, nil
	}
}

// parseImageRef splits a Docker image reference into (registry, repository, tag).
//
// Examples:
//
//	"lscr.io/linuxserver/sonarr:latest" → ("lscr.io",   "linuxserver/sonarr", "latest")
//	"ghcr.io/meeb/tubesync:latest"      → ("ghcr.io",   "meeb/tubesync",      "latest")
//	"linuxserver/sonarr:latest"         → ("docker.io", "linuxserver/sonarr",  "latest")
//	"sonarr:4.0.0"                      → ("docker.io", "library/sonarr",      "4.0.0")
//	"ubuntu"                            → ("docker.io", "library/ubuntu",      "latest")
func parseImageRef(image string) (registry, repo, tag string) {
	tag = "latest"
	// Separate tag: last ":" that is not part of a port spec.
	if i := strings.LastIndex(image, ":"); i >= 0 {
		possibleTag := image[i+1:]
		// A colon inside the hostname part (e.g. "localhost:5000/myimage") has a slash
		// after it.  A tag never contains a slash.
		if !strings.Contains(possibleTag, "/") {
			tag = possibleTag
			image = image[:i]
		}
	}

	parts := strings.SplitN(image, "/", 2)
	firstPart := parts[0]

	// A registry hostname contains a dot, a colon (port), or is "localhost".
	isRegistryHost := strings.ContainsAny(firstPart, ".:") || firstPart == "localhost"

	if len(parts) == 2 && isRegistryHost {
		registry = firstPart
		repo = parts[1]
	} else {
		registry = "docker.io"
		if len(parts) == 1 {
			// Official library image, e.g. "ubuntu".
			repo = "library/" + parts[0]
		} else {
			// User-scoped image, e.g. "linuxserver/sonarr".
			repo = image
		}
	}
	return
}

// getDockerHubDigest fetches a manifest digest from Docker Hub using the
// standard token-auth flow.
func (r *RegistryClient) getDockerHubDigest(ctx context.Context, repo, tag string) (string, error) {
	token, err := r.fetchDockerHubToken(ctx, repo)
	if err != nil {
		return "", fmt.Errorf("docker hub token for %s: %w", repo, err)
	}

	url := fmt.Sprintf("https://registry-1.docker.io/v2/%s/manifests/%s", repo, tag)
	return r.fetchManifestDigest(ctx, url, token)
}

// fetchDockerHubToken fetches an anonymous pull token from auth.docker.io.
func (r *RegistryClient) fetchDockerHubToken(ctx context.Context, repo string) (string, error) {
	url := fmt.Sprintf(
		"https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull",
		repo,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch token: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %d", resp.StatusCode)
	}

	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"` // Docker Hub uses both field names
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if payload.Token != "" {
		return payload.Token, nil
	}
	if payload.AccessToken != "" {
		return payload.AccessToken, nil
	}
	return "", fmt.Errorf("no token in response")
}

// getOCIDigest fetches a manifest digest from an OCI-compliant registry,
// handling the anonymous token flow described in the OCI Distribution Spec.
func (r *RegistryClient) getOCIDigest(ctx context.Context, registry, repo, tag string) (string, error) {
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)

	// Attempt unauthenticated first.
	digest, err := r.fetchManifestDigest(ctx, url, "")
	if err == nil {
		return digest, nil
	}

	// On 401, parse WWW-Authenticate to get the token endpoint.
	token, tokenErr := r.fetchOCIAnonymousToken(ctx, registry, repo)
	if tokenErr != nil {
		return "", fmt.Errorf("oci token for %s/%s: %w", registry, repo, tokenErr)
	}

	return r.fetchManifestDigest(ctx, url, token)
}

// fetchOCIAnonymousToken obtains an anonymous pull token by probing the
// registry's /v2/ endpoint and following the WWW-Authenticate challenge.
func (r *RegistryClient) fetchOCIAnonymousToken(ctx context.Context, registry, repo string) (string, error) {
	// Hit /v2/ to provoke a 401 with a WWW-Authenticate header.
	probeURL := fmt.Sprintf("https://%s/v2/", registry)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("probe %s: %w", probeURL, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	wwwAuth := resp.Header.Get("Www-Authenticate")
	if wwwAuth == "" {
		// Registry may not require auth; return empty token so the caller retries.
		return "", fmt.Errorf("no WWW-Authenticate header from %s", registry)
	}

	tokenURL, err := buildTokenURL(wwwAuth, repo)
	if err != nil {
		return "", fmt.Errorf("parse WWW-Authenticate: %w", err)
	}

	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", err
	}
	tokenResp, err := r.httpClient.Do(tokenReq)
	if err != nil {
		return "", fmt.Errorf("fetch oci token: %w", err)
	}
	defer tokenResp.Body.Close()
	body, _ := io.ReadAll(tokenResp.Body)

	if tokenResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %d", tokenResp.StatusCode)
	}

	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode oci token: %w", err)
	}
	if payload.Token != "" {
		return payload.Token, nil
	}
	if payload.AccessToken != "" {
		return payload.AccessToken, nil
	}
	return "", fmt.Errorf("no token in oci response")
}

// buildTokenURL parses a Bearer WWW-Authenticate challenge and appends the
// pull scope for the given repository.
//
// Example input:
//
//	Bearer realm="https://ghcr.io/token",service="ghcr.io"
func buildTokenURL(wwwAuth, repo string) (string, error) {
	// Strip the "Bearer " prefix.
	header := strings.TrimPrefix(wwwAuth, "Bearer ")
	if header == wwwAuth {
		return "", fmt.Errorf("unsupported auth scheme: %q", wwwAuth)
	}

	// Parse key=value pairs.
	params := map[string]string{}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.Trim(strings.TrimSpace(kv[1]), `"`)
		params[key] = val
	}

	realm, ok := params["realm"]
	if !ok {
		return "", fmt.Errorf("no realm in WWW-Authenticate")
	}

	url := realm
	sep := "?"
	if service, ok := params["service"]; ok {
		url += sep + "service=" + service
		sep = "&"
	}
	url += sep + "scope=repository:" + repo + ":pull"
	return url, nil
}

// fetchManifestDigest performs a GET against url with optional Bearer token and
// returns the Docker-Content-Digest response header value.
func (r *RegistryClient) fetchManifestDigest(ctx context.Context, url, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", manifestAcceptHeader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("manifest request: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	switch resp.StatusCode {
	case http.StatusOK:
		digest := resp.Header.Get("Docker-Content-Digest")
		if digest == "" {
			return "", fmt.Errorf("no Docker-Content-Digest header in response from %s", url)
		}
		return digest, nil
	case http.StatusUnauthorized:
		return "", fmt.Errorf("unauthorized (401)")
	case http.StatusNotFound:
		return "", fmt.Errorf("image/tag not found (404) at %s", url)
	case http.StatusTooManyRequests:
		return "", fmt.Errorf("rate limited (429) by %s", url)
	default:
		return "", fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}
}
