package docker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── parseImageRef ──────────────────────────────────────────────────────────────

func TestParseImageRef(t *testing.T) {
	cases := []struct {
		input    string
		registry string
		repo     string
		tag      string
	}{
		{
			input:    "lscr.io/linuxserver/sonarr:latest",
			registry: "lscr.io",
			repo:     "linuxserver/sonarr",
			tag:      "latest",
		},
		{
			input:    "ghcr.io/meeb/tubesync:latest",
			registry: "ghcr.io",
			repo:     "meeb/tubesync",
			tag:      "latest",
		},
		{
			input:    "sonarr:4.0.0",
			registry: "docker.io",
			repo:     "library/sonarr",
			tag:      "4.0.0",
		},
		{
			input:    "linuxserver/sonarr:latest",
			registry: "docker.io",
			repo:     "linuxserver/sonarr",
			tag:      "latest",
		},
		{
			input:    "ubuntu",
			registry: "docker.io",
			repo:     "library/ubuntu",
			tag:      "latest",
		},
		{
			// Port in hostname must not be treated as tag separator.
			input:    "localhost:5000/myimage:v1",
			registry: "localhost:5000",
			repo:     "myimage",
			tag:      "v1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			reg, repo, tag := parseImageRef(tc.input)
			if reg != tc.registry {
				t.Errorf("registry: got %q, want %q", reg, tc.registry)
			}
			if repo != tc.repo {
				t.Errorf("repo: got %q, want %q", repo, tc.repo)
			}
			if tag != tc.tag {
				t.Errorf("tag: got %q, want %q", tag, tc.tag)
			}
		})
	}
}

// ── GetLatestDigest — Docker Hub ───────────────────────────────────────────────

func TestGetLatestDigest_DockerHub(t *testing.T) {
	const wantDigest = "sha256:aabbccddeeff"

	// Fake auth.docker.io token endpoint.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"token":"test-token"}`)) //nolint:errcheck
	}))
	defer tokenServer.Close()

	// Fake registry-1.docker.io manifest endpoint.
	manifestServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Docker-Content-Digest", wantDigest)
		w.WriteHeader(http.StatusOK)
	}))
	defer manifestServer.Close()

	// Build a transport that rewrites requests to our fake servers.
	transport := &rewriteTransport{
		rules: map[string]string{
			"auth.docker.io":        tokenServer.URL,
			"registry-1.docker.io": manifestServer.URL,
		},
	}

	rc := &RegistryClient{httpClient: &http.Client{Transport: transport}}

	digest, err := rc.GetLatestDigest(context.Background(), "linuxserver/sonarr:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if digest != wantDigest {
		t.Errorf("digest: got %q, want %q", digest, wantDigest)
	}
}

// ── GetLatestDigest — GHCR ────────────────────────────────────────────────────

func TestGetLatestDigest_GHCR(t *testing.T) {
	const wantDigest = "sha256:112233445566"

	// Fake GHCR server: first request returns 401, second (with token) returns digest.
	calls := 0
	ghcrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Path {
		case "/v2/":
			// Probe request — return 401 with WWW-Authenticate.
			w.Header().Set("Www-Authenticate", `Bearer realm="https://ghcr.io/token",service="ghcr.io"`)
			w.WriteHeader(http.StatusUnauthorized)
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"token":"ghcr-token"}`)) //nolint:errcheck
		default:
			// Manifest request.
			if r.Header.Get("Authorization") == "Bearer ghcr-token" {
				w.Header().Set("Docker-Content-Digest", wantDigest)
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer ghcrServer.Close()

	transport := &rewriteTransport{
		rules: map[string]string{
			"ghcr.io": ghcrServer.URL,
		},
	}

	rc := &RegistryClient{httpClient: &http.Client{Transport: transport}}

	digest, err := rc.GetLatestDigest(context.Background(), "ghcr.io/meeb/tubesync:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if digest != wantDigest {
		t.Errorf("digest: got %q, want %q", digest, wantDigest)
	}
}

// ── Rate-limit handling ────────────────────────────────────────────────────────

func TestGetLatestDigest_RateLimit(t *testing.T) {
	// Fake auth server.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"token":"tok"}`)) //nolint:errcheck
	}))
	defer tokenServer.Close()

	// Manifest server returns 429.
	manifestServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer manifestServer.Close()

	transport := &rewriteTransport{
		rules: map[string]string{
			"auth.docker.io":        tokenServer.URL,
			"registry-1.docker.io": manifestServer.URL,
		},
	}

	rc := &RegistryClient{httpClient: &http.Client{Transport: transport}}

	_, err := rc.GetLatestDigest(context.Background(), "library/nginx:latest")
	if err == nil {
		t.Fatal("expected error for 429, got nil")
	}
}

// ── rewriteTransport ──────────────────────────────────────────────────────────

// rewriteTransport rewrites outbound request hosts to test server URLs so we
// don't need real network access in tests.
type rewriteTransport struct {
	rules map[string]string // hostname → base URL of test server
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	if host == "" {
		host = req.URL.Hostname()
	}
	if base, ok := rt.rules[host]; ok {
		// Parse the test server URL to get scheme+host.
		cloned := req.Clone(req.Context())
		parsed, err := http.NewRequest(req.Method, base+req.URL.RequestURI(), req.Body)
		if err != nil {
			return nil, err
		}
		cloned.URL = parsed.URL
		return http.DefaultTransport.RoundTrip(cloned)
	}
	return http.DefaultTransport.RoundTrip(req)
}
